//go:build windows

package tunnels

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

func isAddrInUse(err error) bool {
	return errors.Is(err, windows.WSAEADDRINUSE) || errors.Is(err, syscall.EADDRINUSE)
}

func isListenPermissionDenied(err error) bool {
	return errors.Is(err, windows.WSAEACCES) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM)
}
