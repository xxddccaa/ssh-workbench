//go:build !windows

package platform

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"
)

// ErrAlreadyRunning means another server already owns the control channel
// for this data directory.
var ErrAlreadyRunning = errors.New("a server is already running for this data directory")

func controlPath(dataDirectory string) string {
	return filepath.Join(dataDirectory, "control.sock")
}

// ListenControl creates a Unix socket inside the private (0700) data
// directory and restricts the socket itself to the current user.
func ListenControl(dataDirectory string) (net.Listener, error) {
	filename := controlPath(dataDirectory)
	if connection, err := (&net.Dialer{Timeout: time.Second}).Dial("unix", filename); err == nil {
		connection.Close()
		return nil, ErrAlreadyRunning
	}
	if info, err := os.Lstat(filename); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("control path exists and is not a socket: " + filename)
		}
		os.Remove(filename)
	}
	listener, err := net.Listen("unix", filename)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(filename, 0600); err != nil {
		listener.Close()
		return nil, err
	}
	return listener, nil
}

func DialControl(ctx context.Context, dataDirectory string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", controlPath(dataDirectory))
}
