// Package resource measures the resource budgets from
// docs/04-delivery-validation.md against the loopback SSH fixture. It is
// opt-in (SSHWB_BENCH=1, `make bench`) because it moves hundreds of MiB.
package resource

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshworkbench/internal/adapters/sqlite"
	"sshworkbench/internal/bootstrap"
	"sshworkbench/internal/connections"
	"sshworkbench/internal/hosts"
	"sshworkbench/internal/launch"
	"sshworkbench/internal/transport/httpapi"
	"sshworkbench/tests/fixtures/sshserver"
)

const owner = "bench-owner"

type nullKeyring struct{}

func (nullKeyring) Get(string, string) (string, error) { return "", errors.New("unused") }
func (nullKeyring) Set(string, string, string) error   { return errors.New("unused") }
func (nullKeyring) Delete(string, string) error        { return nil }

// startFixture runs the SSH fixture in its own process so its in-memory
// filesystem does not count against this process's memory measurements.
func startFixture(test *testing.T, environment []string) string {
	test.Helper()
	binary := filepath.Join(test.TempDir(), "fixture-sshd")
	build := exec.Command("go", "build", "-o", binary, "sshworkbench/tests/fixtures/sshd")
	build.Env = environment
	if output, err := build.CombinedOutput(); err != nil {
		test.Fatalf("build fixture: %v %s", err, output)
	}
	command := exec.Command(binary, "--listen", "127.0.0.1:0")
	stdout, _ := command.StdoutPipe()
	if err := command.Start(); err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() {
		command.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { command.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			command.Process.Kill()
		}
	})
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if address, found := strings.CutPrefix(scanner.Text(), "Fixture SSH: "); found {
			go io.Copy(io.Discard, stdout)
			return address
		}
	}
	test.Fatal("fixture did not start")
	return ""
}

func trustFixture(test *testing.T, store *sqlite.Store, address string) {
	test.Helper()
	var captured ssh.PublicKey
	config := &ssh.ClientConfig{User: "probe", Auth: []ssh.AuthMethod{ssh.Password("x")}, Timeout: 5 * time.Second, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		captured = key
		return errors.New("captured")
	}}
	ssh.Dial("tcp", address, config)
	if captured == nil {
		test.Fatal("could not read fixture host key")
	}
	if err := store.AddKnownKey(context.Background(), connections.KnownKey{Address: address, Algorithm: captured.Type(), Fingerprint: ssh.FingerprintSHA256(captured), PublicKey: captured.Marshal(), TrustedAt: time.Now()}); err != nil {
		test.Fatal(err)
	}
}

func open(test *testing.T, services httpapi.Services, hostID string) string {
	test.Helper()
	ctx := context.Background()
	attempt, err := services.Launch.Terminal(ctx, owner, launch.Request{HostID: hostID, Rows: 24, Cols: 80, Credentials: connections.Credentials{Password: sshserver.Password}})
	if err != nil {
		test.Fatal(err)
	}
	for attempt.State == connections.AttemptConnecting {
		attempt, err = services.Attempts.Wait(ctx, owner, attempt.ID, attempt.Version)
		if err != nil {
			test.Fatal(err)
		}
	}
	result, ok := attempt.Result.(launch.Result)
	if attempt.State != connections.AttemptConnected || !ok || result.Terminal == nil {
		test.Fatalf("terminal not opened: %+v", attempt)
	}
	return result.Terminal.ID
}

// peakHeap samples HeapInuse until stop is closed.
func peakHeap(stop <-chan struct{}) func() uint64 {
	var peak atomic.Uint64
	var done sync.WaitGroup
	done.Add(1)
	go func() {
		defer done.Done()
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		var stats runtime.MemStats
		for {
			runtime.ReadMemStats(&stats)
			if stats.HeapInuse > peak.Load() {
				peak.Store(stats.HeapInuse)
			}
			select {
			case <-stop:
				return
			case <-ticker.C:
			}
		}
	}()
	return func() uint64 { done.Wait(); return peak.Load() }
}

func openDescriptors() int {
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		return -1
	}
	return len(entries)
}

func settledGoroutines(limit int) int {
	deadline := time.Now().Add(10 * time.Second)
	for {
		runtime.GC()
		count := runtime.NumGoroutine()
		if count <= limit || time.Now().After(deadline) {
			return count
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type pattern struct{ remaining int64 }

func (source *pattern) Read(buffer []byte) (int, error) {
	if source.remaining <= 0 {
		return 0, io.EOF
	}
	count := min(int64(len(buffer)), source.remaining)
	for index := range buffer[:count] {
		buffer[index] = byte('a' + index%26)
	}
	source.remaining -= count
	return int(count), nil
}

func TestResourceBudgets(test *testing.T) {
	if os.Getenv("SSHWB_BENCH") == "" {
		test.Skip("set SSHWB_BENCH=1 (make bench) to run resource budgets")
	}
	// Build with the real module cache, then isolate HOME for the app.
	address := startFixture(test, os.Environ())
	home := test.TempDir()
	test.Setenv("HOME", home)
	test.Setenv("USERPROFILE", home)
	store, err := sqlite.Open(filepath.Join(test.TempDir(), "private"))
	if err != nil {
		test.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	services, closeServices := bootstrap.Compose(ctx, store, bootstrap.Options{Keyring: nullKeyring{}})
	defer closeServices()
	trustFixture(test, store, address)
	hostname, portText, _ := net.SplitHostPort(address)
	port, _ := strconv.Atoi(portText)
	profile, err := services.Hosts.Save(ctx, "", hosts.Profile{Name: "bench", Hostname: hostname, Port: port, Username: sshserver.Username, AuthMode: hosts.AuthPassword})
	if err != nil {
		test.Fatal(err)
	}

	test.Run("100 terminal open/close cycles return to baseline", func(test *testing.T) {
		warm := open(test, services, profile.ID)
		services.Terminals.Remove(owner, warm)
		baseline := settledGoroutines(0)
		descriptors := openDescriptors()
		started := time.Now()
		for cycle := 0; cycle < 100; cycle++ {
			identifier := open(test, services, profile.ID)
			if err := services.Terminals.Remove(owner, identifier); err != nil {
				test.Fatal(err)
			}
		}
		after := settledGoroutines(baseline + 4)
		afterDescriptors := openDescriptors()
		test.Logf("100 cycles in %s; goroutines %d → %d; file descriptors %d → %d", time.Since(started).Round(time.Millisecond), baseline, after, descriptors, afterDescriptors)
		if after > baseline+4 {
			test.Fatalf("goroutines leaked: %d → %d", baseline, after)
		}
		if descriptors >= 0 && afterDescriptors > descriptors+4 {
			test.Fatalf("file descriptors leaked: %d → %d", descriptors, afterDescriptors)
		}
	})

	test.Run("100 MiB continuous output stays within bounded memory", func(test *testing.T) {
		identifier := open(test, services, profile.ID)
		defer services.Terminals.Remove(owner, identifier)
		session, err := services.Terminals.Get(owner, identifier)
		if err != nil {
			test.Fatal(err)
		}
		runtime.GC()
		stop := make(chan struct{})
		peak := peakHeap(stop)
		if err := session.Write([]byte("emit-mib 100\r")); err != nil {
			test.Fatal(err)
		}
		var offset uint64
		started := time.Now()
		readContext, cancelRead := context.WithTimeout(ctx, 5*time.Minute)
		defer cancelRead()
		for offset < 100*1024*1024 {
			chunk, err := session.Stream().Next(readContext, offset)
			if err != nil {
				test.Fatal(err)
			}
			offset = chunk.Offset + uint64(len(chunk.Data))
			if err := session.Stream().Ack(offset); err != nil {
				test.Fatal(err)
			}
		}
		elapsed := time.Since(started)
		close(stop)
		heap := peak()
		test.Logf("streamed %d MiB in %s (%.1f MiB/s); peak heap in use %.1f MiB", offset>>20, elapsed.Round(time.Millisecond), float64(offset)/(1<<20)/elapsed.Seconds(), float64(heap)/(1<<20))
		if heap > 96<<20 {
			test.Fatalf("heap grew with output: %.1f MiB", float64(heap)/(1<<20))
		}
	})

	test.Run("large upload streams without buffering", func(test *testing.T) {
		size := int64(1024)
		if value, err := strconv.ParseInt(os.Getenv("SSHWB_BENCH_UPLOAD_MIB"), 10, 64); err == nil && value > 0 && value <= 1024 {
			size = value
		}
		identifier := open(test, services, profile.ID)
		defer services.Terminals.Remove(owner, identifier)
		// A browser keeps its terminal attached; without that the server
		// reclaims an idle terminal after 60 seconds.
		session, err := services.Terminals.Get(owner, identifier)
		if err != nil || session.Attach(0) != nil {
			test.Fatal("could not attach terminal", err)
		}
		defer session.Detach()
		runtime.GC()
		stop := make(chan struct{})
		peak := peakHeap(stop)
		started := time.Now()
		name := fmt.Sprintf("bench-%d.bin", size)
		err = services.Files.Upload(ctx, owner, identifier, sshserver.DiscardDirectory, name, &pattern{remaining: size << 20})
		elapsed := time.Since(started)
		close(stop)
		heap := peak()
		if err != nil {
			test.Fatal(err)
		}
		test.Logf("uploaded %d MiB in %s (%.1f MiB/s); peak heap in use %.1f MiB", size, elapsed.Round(time.Millisecond), float64(size)/elapsed.Seconds(), float64(heap)/(1<<20))
		if heap > 128<<20 {
			test.Fatalf("upload buffered in memory: %.1f MiB", float64(heap)/(1<<20))
		}
	})
}
