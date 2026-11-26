//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"sshworkbench/internal/adapters/platform"
	"sshworkbench/internal/ident"
)

const daemonReadyFileEnv = "SSHW_READY_FILE"

func initDaemonChild() (func(daemonMessage), bool) {
	isChild := os.Getenv(daemonChildEnv) == "1"
	readyPath := os.Getenv(daemonReadyFileEnv)
	_ = os.Unsetenv(daemonChildEnv)
	_ = os.Unsetenv(daemonReadyFileEnv)
	if !isChild {
		return nil, false
	}
	var once sync.Once
	return func(message daemonMessage) {
		once.Do(func() {
			if readyPath == "" {
				return
			}
			data, err := json.Marshal(message)
			if err != nil {
				return
			}
			tmpPath := readyPath + ".tmp"
			if err := os.WriteFile(tmpPath, data, 0600); err != nil {
				return
			}
			_ = os.Rename(tmpPath, readyPath)
		})
	}, true
}

func spawnDaemon(dataDir string, arguments []string) (daemonMessage, int, string, error) {
	executable, err := os.Executable()
	if err != nil {
		return daemonMessage{}, 0, "", err
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

	readyPath := filepath.Join(dataDir, "ready-"+ident.New()+".json")
	defer os.Remove(readyPath)
	defer os.Remove(readyPath + ".tmp")

	cmd := exec.Command(executable, arguments...)
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(filteredEnv(os.Environ(), daemonChildEnv, daemonReadyFileEnv), daemonChildEnv+"=1", daemonReadyFileEnv+"="+readyPath)
	platform.DetachCommand(cmd)

	if err := cmd.Start(); err != nil {
		return daemonMessage{}, 0, "", err
	}
	pid := cmd.Process.Pid

	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(readyPath); err == nil && len(strings.TrimSpace(string(raw))) > 0 {
			var message daemonMessage
			if err := json.Unmarshal(raw, &message); err != nil {
				return daemonMessage{}, pid, logPath, fmt.Errorf("解析后台服务状态失败：%w", err)
			}
			if message.Error == "" && !message.AlreadyRunning && message.Existing == nil {
				_ = cmd.Process.Release()
			}
			return message, pid, logPath, nil
		}
		select {
		case waitErr := <-exited:
			if raw, err := os.ReadFile(readyPath); err == nil && len(strings.TrimSpace(string(raw))) > 0 {
				var message daemonMessage
				if jsonErr := json.Unmarshal(raw, &message); jsonErr == nil {
					return message, pid, logPath, nil
				}
			}
			if waitErr != nil {
				return daemonMessage{}, pid, logPath, fmt.Errorf("后台服务启动失败（%v），请查看日志：%s", waitErr, logPath)
			}
			return daemonMessage{}, pid, logPath, fmt.Errorf("后台服务启动失败，请查看日志：%s", logPath)
		case <-time.After(25 * time.Millisecond):
		}
	}
	_ = cmd.Process.Release()
	return daemonMessage{}, pid, logPath, fmt.Errorf("等待后台服务就绪超时，请查看日志：%s", logPath)
}
