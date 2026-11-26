package sshtransport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshworkbench/internal/adapters/sqlite"
	"sshworkbench/internal/connections"
	"sshworkbench/internal/hosts"
)

type trustEverything struct{ asked chan struct{} }

func (trust trustEverything) ConfirmHostKey(ctx context.Context, _ connections.Prompt) error {
	if trust.asked != nil {
		select {
		case trust.asked <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}
func (trustEverything) Respond(context.Context, connections.Prompt) ([]string, error) {
	return nil, errors.New("unexpected")
}

// rekeyingServer re-keys every 4 KiB and streams payload bytes on any
// session channel, so a client must survive many key exchanges.
func rekeyingServer(test *testing.T, payload int, signers ...ssh.Signer) string {
	test.Helper()
	config := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	config.RekeyThreshold = 4096
	for _, signer := range signers {
		config.AddHostKey(signer)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() { listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				_, channels, requests, err := ssh.NewServerConn(connection, config)
				if err != nil {
					connection.Close()
					return
				}
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					channel, channelRequests, err := incoming.Accept()
					if err != nil {
						continue
					}
					go func() {
						for request := range channelRequests {
							request.Reply(true, nil)
						}
					}()
					go func() {
						defer channel.Close()
						chunk := make([]byte, 1024)
						for written := 0; written < payload; written += len(chunk) {
							if _, err := channel.Write(chunk); err != nil {
								return
							}
						}
					}()
				}
			}()
		}
	}()
	return listener.Addr().String()
}

func newSigner(test *testing.T) ssh.Signer {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		test.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		test.Fatal(err)
	}
	return signer
}

func testConnector(test *testing.T) *Connector {
	store, err := sqlite.Open(filepath.Join(test.TempDir(), "private"))
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() { store.Close() })
	return New(connections.NewVerifier(store), Options{StrictHostKeyConfirm: true})
}

func targetFor(address string, interaction connections.Interaction) connections.Target {
	host, portText, _ := net.SplitHostPort(address)
	port, _ := strconv.Atoi(portText)
	return connections.Target{Host: hosts.Profile{Hostname: host, Port: port, Username: "user", AuthMode: hosts.AuthPassword}, Credentials: connections.Credentials{Password: "secret"}, Interaction: interaction}
}

func TestConnectionsSurviveRekeysAfterTheDialContextEnds(test *testing.T) {
	const payload = 256 * 1024
	address := rekeyingServer(test, payload, newSigner(test))
	connector := testConnector(test)
	ctx, cancel := context.WithCancel(context.Background())
	transport, err := connector.dial(ctx, targetFor(address, trustEverything{}))
	if err != nil {
		test.Fatal(err)
	}
	defer transport.Close()
	// Attempts, terminals and file operations all cancel the dial context
	// once the connection is up.
	cancel()
	session, err := transport.client.NewSession()
	if err != nil {
		test.Fatal(err)
	}
	output, _ := session.StdoutPipe()
	session.Shell()
	done := make(chan int64, 1)
	go func() {
		count, _ := io.Copy(io.Discard, output)
		done <- count
	}()
	select {
	case count := <-done:
		if count != payload {
			test.Fatalf("connection dropped during re-key: received %d of %d bytes", count, payload)
		}
	case <-time.After(15 * time.Second):
		test.Fatal("stream stalled during re-key")
	}
}

func TestCancellingDuringHostKeyPromptReleasesEverything(test *testing.T) {
	address := rekeyingServer(test, 0, newSigner(test))
	connector := testConnector(test)
	asked := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		transport, err := connector.dial(ctx, targetFor(address, trustEverything{asked: asked}))
		if transport != nil {
			transport.Close()
		}
		result <- err
	}()
	select {
	case <-asked:
	case <-time.After(10 * time.Second):
		test.Fatal("host key prompt not raised")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			test.Fatal("cancelled dial succeeded")
		}
	case <-time.After(10 * time.Second):
		test.Fatal("cancelled dial did not return")
	}
}

func TestAutoTrustNewHostAndProxyCommand(test *testing.T) {
	address := rekeyingServer(test, 1024, newSigner(test))
	store, err := sqlite.Open(filepath.Join(test.TempDir(), "private"))
	if err != nil {
		test.Fatal(err)
	}
	defer store.Close()
	connector := New(connections.NewVerifier(store), Options{LocalFiles: true, StrictHostKeyConfirm: false})
	host, portText, _ := net.SplitHostPort(address)
	port, _ := strconv.Atoi(portText)
	// Connect via ProxyCommand (nc/sh TCP forwarder using Go's own test server isn't needed if nc isn't present;
	// instead test expandProxyCommand + TOFU auto-trust on direct dial).
	if expanded := expandProxyCommand("cloudflared access ssh --hostname %h --port %p --user %r %%", "host.example", 2222, "alice"); expanded != "cloudflared access ssh --hostname host.example --port 2222 --user alice %" {
		test.Fatalf("unexpected ProxyCommand expansion: %q", expanded)
	}
	target := connections.Target{
		Host:        hosts.Profile{Hostname: host, Port: port, Username: "user", AuthMode: hosts.AuthAuto},
		Credentials: connections.Credentials{Password: "secret"},
		Interaction: trustEverything{asked: make(chan struct{}, 1)},
	}
	transport, err := connector.dial(context.Background(), target)
	if err != nil {
		test.Fatalf("TOFU auto-trust dial failed: %v", err)
	}
	transport.Close()
}

func TestPlainPasswordPromptsOnly(test *testing.T) {
	for _, prompt := range []string{"Password:", "Password: ", "deploy@web's password:", "password"} {
		if !plainPasswordPrompt(prompt) {
			test.Errorf("rejected %q", prompt)
		}
	}
	for _, prompt := range []string{"One-time password (OATH) for `deploy':", "Verification code:", "Enter PIN for token:", "New password:", "OTP password:", "Passcode:"} {
		if plainPasswordPrompt(prompt) {
			test.Errorf("answered %q with the account password", prompt)
		}
	}
}
