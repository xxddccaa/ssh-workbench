//go:build windows

package platform

import (
	"errors"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// IsAddrInUse reports whether err indicates a TCP bind address conflict.
func IsAddrInUse(err error) bool {
	return errors.Is(err, windows.WSAEADDRINUSE) || errors.Is(err, syscall.EADDRINUSE)
}

// DetachCommand configures cmd to run detached from the parent console window.
func DetachCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
		HideWindow:    true,
	}
}
