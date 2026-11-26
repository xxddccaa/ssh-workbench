//go:build !windows

package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestControlSocketIsPrivateAndSingleInstance(test *testing.T) {
	directory, err := os.MkdirTemp("", "sshwb")
	if err != nil {
		test.Fatal(err)
	}
	defer os.RemoveAll(directory)
	listener, err := ListenControl(directory)
	if err != nil {
		test.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(directory, "control.sock"))
	if err != nil || info.Mode().Perm() != 0600 {
		test.Fatalf("socket permissions: %v %v", info.Mode(), err)
	}
	if _, err := ListenControl(directory); !errors.Is(err, ErrAlreadyRunning) {
		test.Fatalf("second instance not detected: %v", err)
	}
	connection, err := DialControl(context.Background(), directory)
	if err != nil {
		test.Fatal(err)
	}
	connection.Close()
	listener.Close()
	replacement, err := ListenControl(directory)
	if err != nil {
		test.Fatal("stale socket not replaced", err)
	}
	replacement.Close()
}
