package sshserver_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"

	"sshworkbench/tests/fixtures/sshserver"
)

const prompt = "\x1b]7;file://127.0.0.1/workspace\x07\x1b[32mtest\x1b[0m:/workspace $ "

func startServer(t *testing.T) *sshserver.Server {
	t.Helper()
	server, err := sshserver.Start("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	return server
}

func dial(server *sshserver.Server, user string, methods ...ssh.AuthMethod) (*ssh.Client, error) {
	return ssh.Dial("tcp", server.Address(), &ssh.ClientConfig{
		User:            user,
		Auth:            methods,
		HostKeyCallback: ssh.FixedHostKey(server.PublicKey()),
		Timeout:         5 * time.Second,
	})
}

func mustDial(t *testing.T, server *sshserver.Server, user string, methods ...ssh.AuthMethod) *ssh.Client {
	t.Helper()
	client, err := dial(server, user, methods...)
	if err != nil {
		t.Fatalf("dial as %s: %v", user, err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func mustFailDial(t *testing.T, server *sshserver.Server, user string, methods ...ssh.AuthMethod) {
	t.Helper()
	client, err := dial(server, user, methods...)
	if err == nil {
		client.Close()
		t.Fatalf("dial as %s unexpectedly succeeded", user)
	}
}

type shell struct {
	session *ssh.Session
	input   io.WriteCloser
	output  io.Reader
	pending []byte
}

// openShell starts an interactive PTY shell and consumes the banner up to the
// first prompt. A watchdog closes the client so a stuck read fails the test
// instead of hanging it.
func openShell(t *testing.T, client *ssh.Client) *shell {
	t.Helper()
	watchdog := time.AfterFunc(30*time.Second, func() { client.Close() })
	t.Cleanup(func() { watchdog.Stop() })
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	input, err := session.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := session.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.RequestPty("xterm-256color", 28, 100, ssh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	terminal := &shell{session: session, input: input, output: output}
	banner := terminal.readUntil(t, prompt)
	if !strings.Contains(string(banner), "SSH Workbench 协议验收环境") {
		t.Fatalf("unexpected banner %q", banner)
	}
	return terminal
}

// readUntil returns everything before marker and consumes the marker.
func (terminal *shell) readUntil(t *testing.T, marker string) []byte {
	t.Helper()
	buffer := terminal.pending
	chunk := make([]byte, 64*1024)
	searched := 0
	for {
		if index := bytes.Index(buffer[searched:], []byte(marker)); index >= 0 {
			end := searched + index
			terminal.pending = append([]byte(nil), buffer[end+len(marker):]...)
			return buffer[:end]
		}
		searched = max(0, len(buffer)-len(marker)+1)
		count, err := terminal.output.Read(chunk)
		buffer = append(buffer, chunk[:count]...)
		if err != nil && count == 0 {
			t.Fatalf("reading shell output before %q: %v (have %d bytes)", marker, err, len(buffer))
		}
	}
}

func (terminal *shell) send(t *testing.T, command string) {
	t.Helper()
	if _, err := io.WriteString(terminal.input, command+"\r"); err != nil {
		t.Fatal(err)
	}
}

// run sends command and returns its output without the echoed command line.
func (terminal *shell) run(t *testing.T, command string) string {
	t.Helper()
	terminal.send(t, command)
	output := string(terminal.readUntil(t, prompt))
	echoed := command + "\r\n"
	if !strings.HasPrefix(output, echoed) {
		t.Fatalf("output of %q does not start with the echoed command: %q", command, output)
	}
	return strings.TrimPrefix(output, echoed)
}

func TestPasswordShell(t *testing.T) {
	server := startServer(t)
	terminal := openShell(t, mustDial(t, server, sshserver.Username, ssh.Password(sshserver.Password)))
	if output := terminal.run(t, "echo 中文测试"); output != "中文测试\r\n" {
		t.Fatalf("echo output %q", output)
	}
	if output := terminal.run(t, "pwd"); output != "/workspace\r\n" {
		t.Fatalf("pwd output %q", output)
	}
}

func TestPasswordRejections(t *testing.T) {
	server := startServer(t)
	mustFailDial(t, server, sshserver.Username, ssh.Password("wrong"))
	mustFailDial(t, server, sshserver.MFAUsername, ssh.Password(sshserver.MFAPassword))
	mustFailDial(t, server, sshserver.KeyUsername, ssh.Password(sshserver.Password))
}

func TestKeyboardInteractive(t *testing.T) {
	server := startServer(t)
	answer := func(code string, rounds *int) ssh.AuthMethod {
		return ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
			*rounds++
			if len(questions) == 0 {
				return nil, nil
			}
			if strings.Join(questions, "|") != "Password: |Verification code: " || len(echos) != 2 || echos[0] || echos[1] {
				t.Errorf("unexpected challenge %q echo %v", questions, echos)
			}
			return []string{sshserver.MFAPassword, code}, nil
		})
	}

	rounds := 0
	client := mustDial(t, server, sshserver.MFAUsername, answer(sshserver.MFACode, &rounds))
	if rounds != 1 {
		t.Fatalf("expected one challenge round, got %d", rounds)
	}
	terminal := openShell(t, client)
	if output := terminal.run(t, "pwd"); output != "/workspace\r\n" {
		t.Fatalf("pwd output %q", output)
	}

	rounds = 0
	mustFailDial(t, server, sshserver.MFAUsername, answer("000000", &rounds))
	if rounds != 1 {
		t.Fatalf("expected one challenge round for the wrong code, got %d", rounds)
	}

	rounds = 0
	mustFailDial(t, server, sshserver.Username, answer(sshserver.MFACode, &rounds))
	if rounds != 0 {
		t.Fatalf("password user must not be challenged, got %d rounds", rounds)
	}
}

func TestPublicKey(t *testing.T) {
	server := startServer(t)
	keyPEM := server.ClientKeyPEM()
	if !bytes.HasPrefix(keyPEM, []byte("-----BEGIN OPENSSH PRIVATE KEY-----")) {
		t.Fatalf("unexpected key format %q", keyPEM[:min(len(keyPEM), 40)])
	}
	signer, err := ssh.ParsePrivateKey(keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	client := mustDial(t, server, sshserver.KeyUsername, ssh.PublicKeys(signer))
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	session.Close()

	const passphrase = "本地测试口令"
	encrypted, err := server.EncryptedClientKeyPEM(passphrase)
	if err != nil {
		t.Fatal(err)
	}
	var missing *ssh.PassphraseMissingError
	if _, err := ssh.ParsePrivateKey(encrypted); !errors.As(err, &missing) {
		t.Fatalf("encrypted key parsed without passphrase: %v", err)
	}
	if _, err := ssh.ParsePrivateKeyWithPassphrase(encrypted, []byte("wrong")); err == nil {
		t.Fatal("encrypted key parsed with the wrong passphrase")
	}
	decrypted, err := ssh.ParsePrivateKeyWithPassphrase(encrypted, []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	mustDial(t, server, sshserver.KeyUsername, ssh.PublicKeys(decrypted))
	if _, err := server.EncryptedClientKeyPEM(""); err == nil {
		t.Fatal("empty passphrase accepted")
	}

	mustFailDial(t, server, sshserver.Username, ssh.PublicKeys(signer))
	_, otherKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherSigner, err := ssh.NewSignerFromKey(otherKey)
	if err != nil {
		t.Fatal(err)
	}
	mustFailDial(t, server, sshserver.KeyUsername, ssh.PublicKeys(otherSigner))
}

func TestDirectTCPIP(t *testing.T) {
	server := startServer(t)
	echo, err := sshserver.StartEcho("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(echo.Close)
	client := mustDial(t, server, sshserver.Username, ssh.Password(sshserver.Password))
	_, port, err := net.SplitHostPort(echo.Address())
	if err != nil {
		t.Fatal(err)
	}

	payload := bytes.Repeat([]byte("隧道往返：中文字节不丢失。\n"), 16*1024)
	for _, host := range []string{"127.0.0.1", "localhost"} {
		t.Run(host, func(t *testing.T) {
			tunnel, err := client.Dial("tcp", net.JoinHostPort(host, port))
			if err != nil {
				t.Fatal(err)
			}
			defer tunnel.Close()
			written := make(chan error, 1)
			go func() {
				_, err := tunnel.Write(payload)
				if err == nil {
					err = tunnel.(interface{ CloseWrite() error }).CloseWrite()
				}
				written <- err
			}()
			received, err := io.ReadAll(tunnel)
			if err != nil {
				t.Fatal(err)
			}
			if err := <-written; err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(received, payload) {
				t.Fatalf("echoed %d bytes, want %d", len(received), len(payload))
			}
		})
	}

	// Rejection happens before any dial; these destinations are never contacted.
	for _, destination := range []string{"192.0.2.1:80", "fixture.invalid:22", "[::1]:22"} {
		_, err := client.Dial("tcp", destination)
		var openError *ssh.OpenChannelError
		if !errors.As(err, &openError) || openError.Reason != ssh.Prohibited {
			t.Fatalf("forward to %s: expected prohibited rejection, got %v", destination, err)
		}
	}

	// The connection remains usable after rejected and completed forwards.
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	session.Close()
}

func TestEchoLifecycle(t *testing.T) {
	if _, err := sshserver.StartEcho("0.0.0.0:0"); err == nil {
		t.Fatal("echo accepted a non-loopback address")
	}
	echo, err := sshserver.StartEcho("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(echo.Close)
	connection, err := net.DialTimeout("tcp", echo.Address(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	connection.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.WriteString(connection, "回声"); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, len("回声"))
	if _, err := io.ReadFull(connection, reply); err != nil || string(reply) != "回声" {
		t.Fatalf("echo reply %q: %v", reply, err)
	}
	echo.Close()
	if count, err := connection.Read(reply); err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("connection still open after Close (read %d bytes): %v", count, err)
	}
}

func TestExitStatus(t *testing.T) {
	server := startServer(t)
	client := mustDial(t, server, sshserver.Username, ssh.Password(sshserver.Password))

	terminal := openShell(t, client)
	terminal.send(t, "exit 7")
	err := terminal.session.Wait()
	var exitError *ssh.ExitError
	if !errors.As(err, &exitError) || exitError.ExitStatus() != 7 {
		t.Fatalf("expected exit status 7, got %v", err)
	}

	terminal = openShell(t, client)
	if output := terminal.run(t, "exit 256"); !strings.Contains(output, "0 到 255") {
		t.Fatalf("out-of-range exit output %q", output)
	}
	terminal.send(t, "exit")
	if err := terminal.session.Wait(); err != nil {
		t.Fatalf("plain exit: %v", err)
	}
}

func TestEmitMebibytes(t *testing.T) {
	server := startServer(t)
	terminal := openShell(t, mustDial(t, server, sshserver.Username, ssh.Password(sshserver.Password)))
	output := terminal.run(t, "emit-mib 1")
	if len(output) != 1<<20 {
		t.Fatalf("emit-mib 1 produced %d bytes, want exactly 1 MiB", len(output))
	}
	if !utf8.ValidString(output) {
		t.Fatal("emit-mib output is not valid UTF-8")
	}
	lines := strings.SplitAfter(output, "\r\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for index, line := range lines {
		if !strings.HasSuffix(line, "\r\n") || !strings.Contains(line, "中文") {
			t.Fatalf("line %d malformed: %q", index+1, line)
		}
	}
	if !strings.HasPrefix(lines[0], "00000001 ") {
		t.Fatalf("first line %q", lines[0])
	}

	if output := terminal.run(t, "emit-mib 0"); !strings.Contains(output, "1 到 1024") {
		t.Fatalf("emit-mib 0 output %q", output)
	}
	if output := terminal.run(t, "pwd"); output != "/workspace\r\n" {
		t.Fatalf("pwd after emit-mib %q", output)
	}
}
