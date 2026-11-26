//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"sshworkbench/internal/adapters/platform"
)

const daemonReadyPipeEnv = "SSHW_READY_PIPE"

func initDaemonChild() (func(daemonMessage), bool) {
	isChild := os.Getenv(daemonChildEnv) == "1"
	fdText := os.Getenv(daemonReadyPipeEnv)
	// Unset before anything (including localpty shells) can inherit daemon markers.
	_ = os.Unsetenv(daemonChildEnv)
	_ = os.Unsetenv(daemonReadyPipeEnv)
	if !isChild {
		return nil, false
	}
	fd, err := strconv.Atoi(fdText)
	if err != nil || fd < 3 {
		return func(daemonMessage) {}, true
	}
	pipe := os.NewFile(uintptr(fd), "sshw-ready")
	var once sync.Once
	return func(message daemonMessage) {
		once.Do(func() {
			if pipe == nil {
				return
			}
			_ = json.NewEncoder(pipe).Encode(message)
			_ = pipe.Close()
		})
	}, true
}

func spawnDaemon(dataDir string, arguments []string) (daemonMessage, int, string, error) {
	executable, err := os.Executable()
	if err != nil {
		return daemonMessage{}, 0, "", err
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	logFile, logPath, err := openLogFile(dataDir)
	if err != nil {
		return daemonMessage{}, 0, "", err
	}
	defer logFile.Close()

	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return daemonMessage{}, 0, "", err
	}
	defer devNull.Close()

	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		return daemonMessage{}, 0, "", err
	}
	defer readPipe.Close()

	cmd := exec.Command(executable, arguments...)
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.ExtraFiles = []*os.File{writePipe}
	cmd.Env = append(filteredEnv(os.Environ(), daemonChildEnv, daemonReadyPipeEnv), daemonChildEnv+"=1", daemonReadyPipeEnv+"=3")
	platform.DetachCommand(cmd)

	if err := cmd.Start(); err != nil {
		_ = writePipe.Close()
		return daemonMessage{}, 0, "", err
	}
	pid := cmd.Process.Pid
	_ = writePipe.Close()

	type readResult struct {
		data []byte
		err  error
	}
	results := make(chan readResult, 1)
	go func() {
		data, readErr := io.ReadAll(io.LimitReader(readPipe, 64*1024))
		results <- readResult{data: data, err: readErr}
	}()

	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case result := <-results:
		if result.err != nil {
			_ = cmd.Wait()
			return daemonMessage{}, pid, logPath, result.err
		}
		trimmed := strings.TrimSpace(string(result.data))
		if trimmed == "" {
			_ = cmd.Wait()
			return daemonMessage{}, pid, logPath, fmt.Errorf("后台服务启动失败，详情请查看日志：%s", logPath)
		}
		var message daemonMessage
		if err := json.Unmarshal([]byte(trimmed), &message); err != nil {
			_ = cmd.Wait()
			return daemonMessage{}, pid, logPath, fmt.Errorf("解析后台服务状态失败：%w", err)
		}
		if message.Error != "" || message.AlreadyRunning || message.Existing != nil {
			_ = cmd.Wait()
		} else {
			_ = cmd.Process.Release()
		}
		return message, pid, logPath, nil
	case <-timer.C:
		_ = cmd.Process.Release()
		return daemonMessage{}, pid, logPath, fmt.Errorf("等待后台服务就绪超时，请查看日志：%s", logPath)
	}
}
