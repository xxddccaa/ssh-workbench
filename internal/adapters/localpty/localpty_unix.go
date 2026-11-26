//go:build !windows

package localpty

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/terminals"
)

// Supported reports whether this OS supports built-in local PTY sessions.
func Supported() bool {
	return true
}

// DefaultShell returns the preferred interactive shell on the local host.
func DefaultShell() string {
	if candidate := strings.TrimSpace(os.Getenv("SHELL")); candidate != "" && isExecutableFile(candidate) {
		return candidate
	}
	for _, candidate := range []string{"/bin/zsh", "/bin/bash", "/bin/sh"} {
		if isExecutableFile(candidate) {
			return candidate
		}
	}
	return "/bin/sh"
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

func shellArgs(shellPath string) []string {
	switch filepath.Base(shellPath) {
	case "zsh", "bash", "fish", "sh", "ksh", "ash":
		return []string{"-l"}
	default:
		return nil
	}
}

func localEnv() []string {
	raw := os.Environ()
	env := make([]string, 0, len(raw)+2)
	for _, entry := range raw {
		if strings.HasPrefix(entry, "TERM=") || strings.HasPrefix(entry, "COLORTERM=") {
			continue
		}
		env = append(env, entry)
	}
	return append(env, "TERM=xterm-256color", "COLORTERM=truecolor")
}

// Open spawns an interactive local shell attached to a new pseudo-terminal.
func Open(ctx context.Context, rows, cols int) (terminals.Shell, error) {
	return OpenCommand(ctx, DefaultShell(), shellArgs(DefaultShell()), rows, cols)
}

// OpenCommand spawns the given command inside a new pseudo-terminal.
func OpenCommand(ctx context.Context, name string, args []string, rows, cols int) (terminals.Shell, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.Command(name, args...)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if info, statErr := os.Stat(home); statErr == nil && info.IsDir() {
			cmd.Dir = home
		}
	}
	cmd.Env = localEnv()
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return nil, fault.New(500, "local_pty_failed", "无法启动本地终端："+err.Error())
	}
	shell := &unixShell{cmd: cmd, ptmx: ptmx, done: make(chan struct{})}
	if err := ctx.Err(); err != nil {
		_ = shell.Close()
		return nil, err
	}
	return shell, nil
}

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }

type unixShell struct {
	cmd       *exec.Cmd
	ptmx      *os.File
	done      chan struct{}
	closePTY  sync.Once
	closeOnce sync.Once
}

func (shell *unixShell) closeMaster() error {
	var err error
	shell.closePTY.Do(func() {
		err = shell.ptmx.Close()
	})
	return err
}

func (shell *unixShell) Stdout() io.Reader                { return shell.ptmx }
func (shell *unixShell) Stderr() io.Reader                { return eofReader{} }
func (shell *unixShell) Input() io.Writer                 { return shell.ptmx }
func (shell *unixShell) Transport() connections.Transport { return nil }

func (shell *unixShell) Resize(rows, cols int) error {
	return pty.Setsize(shell.ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (shell *unixShell) Wait() terminals.Exit {
	err := shell.cmd.Wait()
	state := shell.cmd.ProcessState
	close(shell.done)
	// Allow buffered PTY output to drain before closing master FD if a child kept the slave open.
	time.AfterFunc(150*time.Millisecond, func() { _ = shell.closeMaster() })
	return exitOf(err, state)
}

func (shell *unixShell) Close() error {
	var err error
	shell.closeOnce.Do(func() {
		select {
		case <-shell.done:
		default:
			if shell.cmd.Process != nil {
				pid := shell.cmd.Process.Pid
				if syscall.Kill(-pid, syscall.SIGHUP) != nil {
					_ = shell.cmd.Process.Signal(syscall.SIGHUP)
				}
			}
		}
		err = shell.closeMaster()
		timer := time.NewTimer(300 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-shell.done:
		case <-timer.C:
			if shell.cmd.Process != nil {
				pid := shell.cmd.Process.Pid
				if syscall.Kill(-pid, syscall.SIGKILL) != nil {
					_ = shell.cmd.Process.Kill()
				}
			}
		}
	})
	return err
}

func exitOf(err error, state *os.ProcessState) terminals.Exit {
	if err == nil {
		code := 0
		return terminals.Exit{Code: &code}
	}
	if state != nil {
		if ws, ok := state.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				return terminals.Exit{Signal: ws.Signal().String()}
			}
			code := ws.ExitStatus()
			return terminals.Exit{Code: &code}
		}
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		return terminals.Exit{Code: &code}
	}
	return terminals.Exit{Disconnected: true}
}
