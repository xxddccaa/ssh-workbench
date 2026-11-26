//go:build !windows

package localpty

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestOpenCommandLifecycleAndResize(test *testing.T) {
	if !Supported() {
		test.Fatal("expected localpty to be supported on unix")
	}
	shell, err := OpenCommand(context.Background(), "/bin/sh", []string{"-c", "stty size; echo local-pty-ok; exit 0"}, 28, 100)
	if err != nil {
		test.Fatalf("open local pty: %v", err)
	}
	defer shell.Close()

	if err := shell.Resize(40, 120); err != nil {
		test.Fatalf("resize pty: %v", err)
	}
	var output bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(&output, shell.Stdout())
	}()

	exit := shell.Wait()
	if exit.Code == nil || *exit.Code != 0 {
		test.Fatalf("unexpected exit: %+v", exit)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		test.Fatal("timed out waiting for pty stdout")
	}
	if !strings.Contains(output.String(), "local-pty-ok") {
		test.Fatalf("missing expected output: %q", output.String())
	}
}

func TestOpenCommandCloseTerminatesRunningShell(test *testing.T) {
	shell, err := OpenCommand(context.Background(), "/bin/sh", []string{"-c", "cat"}, 24, 80)
	if err != nil {
		test.Fatalf("open local pty: %v", err)
	}
	if _, err := io.WriteString(shell.Input(), "hello-pty\n"); err != nil {
		test.Fatalf("write input: %v", err)
	}
	buf := make([]byte, 256)
	deadline := time.Now().Add(2 * time.Second)
	var collected string
	for time.Now().Before(deadline) {
		n, readErr := shell.Stdout().Read(buf)
		if n > 0 {
			collected += string(buf[:n])
			if strings.Contains(collected, "hello-pty") {
				break
			}
		}
		if readErr != nil {
			break
		}
	}
	if !strings.Contains(collected, "hello-pty") {
		test.Fatalf("expected echoed input, got %q", collected)
	}
	waited := make(chan struct{})
	go func() {
		shell.Wait()
		close(waited)
	}()
	if err := shell.Close(); err != nil {
		test.Fatalf("close shell: %v", err)
	}
	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		test.Fatal("shell did not exit after Close")
	}
}
