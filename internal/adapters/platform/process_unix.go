//go:build !windows

package platform

import (
	"errors"
	"os/exec"
	"syscall"
)

// IsAddrInUse reports whether err indicates a TCP/Unix bind address conflict.
func IsAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}

// DetachCommand configures cmd to run in its own session detached from the controlling terminal.
func DetachCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
