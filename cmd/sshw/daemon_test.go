package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func buildTestBinary(test *testing.T) string {
	test.Helper()
	binDir := test.TempDir()
	binary := filepath.Join(binDir, "sshw")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", binary, ".")
	if output, err := cmd.CombinedOutput(); err != nil {
		test.Fatalf("build test binary: %v\n%s", err, output)
	}
	return binary
}

func shortTempDir(test *testing.T) string {
	test.Helper()
	// Keep Unix socket path under macOS sun_path (104 bytes).
	dir, err := os.MkdirTemp("", "sshw-cli-*")
	if err != nil {
		test.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		_ = os.RemoveAll(dir)
		test.Fatal(err)
	}
	test.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func freePort(test *testing.T) int {
	test.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatal(err)
	}
	defer listener.Close()
	_, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, err := strconv.Atoi(portText)
	if err != nil {
		test.Fatal(err)
	}
	return port
}

func TestDaemonLifecycleReuseAndPortFallback(test *testing.T) {
	binary := buildTestBinary(test)
	home := shortTempDir(test)
	dataDir := filepath.Join(shortTempDir(test), "d")

	runCLI := func(args ...string) (string, error) {
		cmd := exec.Command(binary, args...)
		cmd.Env = append(filteredEnv(os.Environ(), "HOME", "USERPROFILE", "SSH_AUTH_SOCK", daemonChildEnv), "HOME="+home, "USERPROFILE="+home, "SSH_AUTH_SOCK=")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	blocker := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.NotFound(response, nil)
	}))
	defer blocker.Close()
	_, blockerPortText, _ := net.SplitHostPort(blocker.Listener.Addr().String())
	blockerPort, _ := strconv.Atoi(blockerPortText)

	// Starting on a port occupied by a non-sshw app should advance and daemonize.
	startOut, err := runCLI("--listen", fmt.Sprintf("127.0.0.1:%d", blockerPort), "--data-dir", dataDir, "--no-browser")
	if err != nil {
		test.Fatalf("start daemon failed: %v\n%s", err, startOut)
	}
	test.Cleanup(func() { _, _ = runCLI("stop", "--data-dir", dataDir) })
	if !strings.Contains(startOut, "已在后台运行") || !strings.Contains(startOut, "已自动切换") {
		test.Fatalf("unexpected start output:\n%s", startOut)
	}

	var activeOrigin string
	for line := range strings.SplitSeq(startOut, "\n") {
		if after, ok := strings.CutPrefix(line, "Open: "); ok {
			activeOrigin = strings.TrimSpace(after)
			break
		}
	}
	if activeOrigin == "" || activeOrigin == "http://127.0.0.1:"+blockerPortText {
		test.Fatalf("expected switched origin, got %q from:\n%s", activeOrigin, startOut)
	}

	// Second invocation with the same data-dir should reuse the running daemon.
	secondOut, err := runCLI("--data-dir", dataDir, "--no-browser")
	if err != nil {
		test.Fatalf("second run failed: %v\n%s", err, secondOut)
	}
	if !strings.Contains(secondOut, "已在后台运行") || !strings.Contains(secondOut, activeOrigin) {
		test.Fatalf("second run did not reuse running server:\n%s", secondOut)
	}

	// Status command should report the active daemon.
	statusOut, err := runCLI("status", "--data-dir", dataDir)
	if err != nil {
		test.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, activeOrigin) {
		test.Fatalf("status missing origin %q:\n%s", activeOrigin, statusOut)
	}

	// A different data-dir targeting the sshw port should detect sshw and reuse without switching ports.
	otherDir := filepath.Join(shortTempDir(test), "d2")
	activeHostPort := strings.TrimPrefix(activeOrigin, "http://")
	reuseOut, err := runCLI("--listen", activeHostPort, "--data-dir", otherDir, "--no-browser")
	if err != nil {
		test.Fatalf("reuse by port probe failed: %v\n%s", err, reuseOut)
	}
	if !strings.Contains(reuseOut, "直接复用") || !strings.Contains(reuseOut, activeOrigin) {
		test.Fatalf("expected direct reuse without port switch:\n%s", reuseOut)
	}

	// Stop command should shut down the background server cleanly.
	stopOut, err := runCLI("stop", "--data-dir", dataDir)
	if err != nil {
		test.Fatalf("stop failed: %v\n%s", err, stopOut)
	}
	if !strings.Contains(stopOut, "已停止") {
		test.Fatalf("unexpected stop output:\n%s", stopOut)
	}

	if _, err := runCLI("status", "--data-dir", dataDir); err == nil {
		test.Fatal("expected status to fail after stop")
	}

	// Clean free port should bind without port fallback.
	port := freePort(test)
	freshDir := filepath.Join(shortTempDir(test), "d3")
	freshOut, err := runCLI("--listen", fmt.Sprintf("127.0.0.1:%d", port), "--data-dir", freshDir, "--no-browser")
	if err != nil {
		test.Fatalf("clean start failed: %v\n%s", err, freshOut)
	}
	test.Cleanup(func() { _, _ = runCLI("stop", "--data-dir", freshDir) })
	expected := fmt.Sprintf("http://127.0.0.1:%d", port)
	if !strings.Contains(freshOut, expected) || strings.Contains(freshOut, "已自动切换") {
		test.Fatalf("expected clean start on %s without fallback:\n%s", expected, freshOut)
	}
}
