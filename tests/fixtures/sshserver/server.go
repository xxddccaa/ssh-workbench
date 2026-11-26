package sshserver

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const Username = "workbench-test"
const Password = "local-fixture-only"

// MFAUsername can only authenticate through keyboard-interactive with one
// challenge round that asks for MFAPassword and MFACode.
const MFAUsername = "workbench-mfa"
const MFAPassword = "local-mfa-only"
const MFACode = "246810"

// KeyUsername can only authenticate with the per-server client key returned by
// ClientKeyPEM or EncryptedClientKeyPEM.
const KeyUsername = "workbench-key"

const clientKeyComment = "workbench-key@ssh-workbench-fixture"

// maximumEmitMebibytes bounds the emit-mib shell command.
const maximumEmitMebibytes = 1024

// emitLineText is 39 CJK characters (117 bytes); with an eight-digit sequence
// number, a space and CRLF each emit-mib line is 128 bytes, which divides one
// MiB exactly.
const emitLineText = "资源验收：中文批量输出，检查内存占用与通道背压是否稳定，数据均不应丢失或乱码。"
const emitLineLength = len("00000000 ") + len(emitLineText) + len("\r\n")

type Server struct {
	listener        net.Listener
	signer          ssh.Signer
	clientKey       ed25519.PrivateKey
	clientPublicKey ssh.PublicKey
	clientKeyPEM    []byte
	handlers        sftp.Handlers
	mu              sync.Mutex
	connections     map[net.Conn]bool
	closed          bool
}

func Start(address string) (*Server, error) {
	address, err := loopbackAddress(address)
	if err != nil {
		return nil, err
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, err
	}
	clientPublicKey, clientKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	clientSSHPublicKey, err := ssh.NewPublicKey(clientPublicKey)
	if err != nil {
		return nil, err
	}
	clientKeyBlock, err := ssh.MarshalPrivateKey(clientKey, clientKeyComment)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	server := &Server{
		listener:        listener,
		signer:          signer,
		clientKey:       clientKey,
		clientPublicKey: clientSSHPublicKey,
		clientKeyPEM:    pem.EncodeToMemory(clientKeyBlock),
		handlers:        withDiscard(sftp.InMemHandler()),
		connections:     make(map[net.Conn]bool),
	}
	go server.serve()
	if err := server.seed(); err != nil {
		server.Close()
		return nil, err
	}
	return server, nil
}

// loopbackAddress defaults an empty address to an ephemeral 127.0.0.1 port and
// refuses anything that is not an explicit 127.0.0.1 listen address.
func loopbackAddress(address string) (string, error) {
	if address == "" {
		address = "127.0.0.1:0"
	}
	hostname, _, err := net.SplitHostPort(address)
	if err != nil || hostname != "127.0.0.1" {
		return "", fmt.Errorf("fixture only supports loopback")
	}
	return address, nil
}

func (server *Server) Address() string          { return server.listener.Addr().String() }
func (server *Server) PublicKey() ssh.PublicKey { return server.signer.PublicKey() }

// ClientKeyPEM returns the unencrypted OpenSSH private key accepted for
// KeyUsername. The key is generated per server and never written to disk here.
func (server *Server) ClientKeyPEM() []byte { return bytes.Clone(server.clientKeyPEM) }

// EncryptedClientKeyPEM returns the KeyUsername private key as an OpenSSH PEM
// protected by passphrase.
func (server *Server) EncryptedClientKeyPEM(passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, errors.New("fixture key passphrase must not be empty")
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(server.clientKey, clientKeyComment, []byte(passphrase))
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(block), nil
}

func (server *Server) configuration() *ssh.ServerConfig {
	configuration := &ssh.ServerConfig{
		PasswordCallback: func(metadata ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if metadata.User() != Username || subtle.ConstantTimeCompare(password, []byte(Password)) != 1 {
				return nil, fmt.Errorf("invalid fixture credentials")
			}
			return nil, nil
		},
		PublicKeyCallback: func(metadata ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if metadata.User() != KeyUsername || subtle.ConstantTimeCompare(key.Marshal(), server.clientPublicKey.Marshal()) != 1 {
				return nil, fmt.Errorf("invalid fixture credentials")
			}
			return nil, nil
		},
		KeyboardInteractiveCallback: func(metadata ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			if metadata.User() != MFAUsername {
				return nil, fmt.Errorf("invalid fixture credentials")
			}
			answers, err := challenge("", "", []string{"Password: ", "Verification code: "}, []bool{false, false})
			if err != nil {
				return nil, err
			}
			if len(answers) != 2 {
				return nil, fmt.Errorf("invalid fixture credentials")
			}
			passwordMatches := subtle.ConstantTimeCompare([]byte(answers[0]), []byte(MFAPassword))
			codeMatches := subtle.ConstantTimeCompare([]byte(answers[1]), []byte(MFACode))
			if passwordMatches&codeMatches != 1 {
				return nil, fmt.Errorf("invalid fixture credentials")
			}
			return nil, nil
		},
	}
	configuration.AddHostKey(server.signer)
	return configuration
}

func (server *Server) serve() {
	configuration := server.configuration()
	for {
		connection, err := server.listener.Accept()
		if err != nil {
			return
		}
		server.mu.Lock()
		if server.closed {
			server.mu.Unlock()
			connection.Close()
			return
		}
		server.connections[connection] = true
		server.mu.Unlock()
		go func() {
			defer func() {
				connection.Close()
				server.mu.Lock()
				delete(server.connections, connection)
				server.mu.Unlock()
			}()
			connection.SetDeadline(time.Now().Add(15 * time.Second))
			client, channels, requests, err := ssh.NewServerConn(connection, configuration)
			if err != nil {
				return
			}
			defer client.Close()
			connection.SetDeadline(time.Time{})
			go ssh.DiscardRequests(requests)
			for incoming := range channels {
				switch incoming.ChannelType() {
				case "session":
					channel, channelRequests, err := incoming.Accept()
					if err != nil {
						continue
					}
					go server.session(channel, channelRequests)
				case "direct-tcpip":
					go forward(incoming)
				default:
					incoming.Reject(ssh.UnknownChannelType, "session and direct-tcpip only")
				}
			}
		}()
	}
}

// directTCPIPRequest is the RFC 4254 section 7.2 channel open payload.
type directTCPIPRequest struct {
	DestinationHost string
	DestinationPort uint32
	OriginHost      string
	OriginPort      uint32
}

// forward serves a direct-tcpip channel. Only loopback destinations are
// allowed; "localhost" is mapped to 127.0.0.1 so no name resolution happens.
func forward(incoming ssh.NewChannel) {
	var request directTCPIPRequest
	if err := ssh.Unmarshal(incoming.ExtraData(), &request); err != nil {
		incoming.Reject(ssh.ConnectionFailed, "malformed direct-tcpip request")
		return
	}
	if request.DestinationHost != "127.0.0.1" && request.DestinationHost != "localhost" {
		incoming.Reject(ssh.Prohibited, "fixture only forwards to loopback")
		return
	}
	if request.DestinationPort == 0 || request.DestinationPort > 65535 {
		incoming.Reject(ssh.ConnectionFailed, "invalid destination port")
		return
	}
	dialer := net.Dialer{Timeout: 3 * time.Second}
	target, err := dialer.Dial("tcp4", net.JoinHostPort("127.0.0.1", strconv.FormatUint(uint64(request.DestinationPort), 10)))
	if err != nil {
		incoming.Reject(ssh.ConnectionFailed, "loopback destination unavailable")
		return
	}
	channel, requests, err := incoming.Accept()
	if err != nil {
		target.Close()
		return
	}
	relay(channel, requests, target)
}

// relay copies bytes both ways, propagating half-closes. It returns once both
// directions have finished; a client channel close or a dropped SSH connection
// closes the target, which unblocks any pending copy.
func relay(channel ssh.Channel, requests <-chan *ssh.Request, target net.Conn) {
	defer target.Close()
	defer channel.Close()
	go func() {
		ssh.DiscardRequests(requests)
		target.Close()
	}()
	var copies sync.WaitGroup
	copies.Add(2)
	go func() {
		defer copies.Done()
		io.Copy(target, channel)
		if halfCloser, ok := target.(interface{ CloseWrite() error }); ok {
			halfCloser.CloseWrite()
		} else {
			target.Close()
		}
	}()
	go func() {
		defer copies.Done()
		io.Copy(channel, target)
		channel.CloseWrite()
	}()
	copies.Wait()
}

func (server *Server) session(channel ssh.Channel, requests <-chan *ssh.Request) {
	started := false
	for request := range requests {
		switch request.Type {
		case "pty-req", "window-change":
			request.Reply(true, nil)
		case "subsystem":
			var payload struct{ Name string }
			if ssh.Unmarshal(request.Payload, &payload) != nil || payload.Name != "sftp" || started {
				request.Reply(false, nil)
				continue
			}
			started = true
			request.Reply(true, nil)
			go func() {
				defer channel.Close()
				handler := sftp.NewRequestServer(channel, server.handlers, sftp.WithStartDirectory("/workspace"))
				defer handler.Close()
				handler.Serve()
			}()
		case "shell":
			if started {
				request.Reply(false, nil)
				continue
			}
			started = true
			request.Reply(true, nil)
			go serveShell(channel)
		default:
			request.Reply(false, nil)
		}
	}
	channel.Close()
}

func serveShell(channel ssh.Channel) {
	defer channel.Close()
	io.WriteString(channel, "\x1b[38;5;115mSSH Workbench 协议验收环境\x1b[0m\r\n")
	io.WriteString(channel, "内存文件系统 · 不执行本机命令\r\n试试：echo 中文测试、pwd、emit、exit\r\n\r\n")
	prompt := func() {
		io.WriteString(channel, "\x1b]7;file://127.0.0.1/workspace\x07\x1b[32mtest\x1b[0m:/workspace $ ")
	}
	prompt()
	reader := bufio.NewReader(channel)
	var line []byte
	for {
		character, err := reader.ReadByte()
		if err != nil {
			return
		}
		switch character {
		case 3:
			line = nil
			io.WriteString(channel, "^C\r\n")
			prompt()
		case 127, 8:
			if len(line) > 0 {
				line = line[:len(line)-1]
				io.WriteString(channel, "\b \b")
			}
		case '\r', '\n':
			io.WriteString(channel, "\r\n")
			command := strings.TrimSpace(string(line))
			line = nil
			switch {
			case command == "exit":
				sendExitStatus(channel, 0)
				return
			case strings.HasPrefix(command, "exit "):
				status, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(command, "exit ")), 10, 8)
				if err == nil {
					sendExitStatus(channel, uint32(status))
					return
				}
				io.WriteString(channel, "exit：退出码必须是 0 到 255 之间的整数。\r\n")
			case strings.HasPrefix(command, "echo "):
				io.WriteString(channel, strings.TrimPrefix(command, "echo ")+"\r\n")
			case command == "pwd":
				io.WriteString(channel, "/workspace\r\n")
			case command == "emit":
				for _, octet := range []byte("分包中文😀：终端字节流测试\r\n") {
					channel.Write([]byte{octet})
				}
			case command == "emit-large":
				for count := 0; count < 4096; count++ {
					if _, err := io.WriteString(channel, "背压验证：中文输出不丢失。\r\n"); err != nil {
						return
					}
				}
			case strings.HasPrefix(command, "emit-mib "):
				mebibytes, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(command, "emit-mib ")))
				if err != nil || mebibytes < 1 || mebibytes > maximumEmitMebibytes {
					io.WriteString(channel, "emit-mib：大小必须是 1 到 1024 之间的整数（MiB）。\r\n")
				} else if emitMebibytes(channel, mebibytes) != nil {
					return
				}
			case command != "":
				io.WriteString(channel, "此内存验收环境不执行任意系统命令。\r\n")
			}
			prompt()
		default:
			if len(line) < 64*1024 {
				line = append(line, character)
				channel.Write([]byte{character})
			}
		}
	}
}

func sendExitStatus(channel ssh.Channel, status uint32) {
	channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
}

// emitMebibytes writes exactly mebibytes MiB of numbered CJK text lines.
// Channel writes block on the client's window, so output follows backpressure.
func emitMebibytes(channel ssh.Channel, mebibytes int) error {
	const linesPerWrite = 256
	lineCount := (mebibytes << 20) / emitLineLength
	buffer := make([]byte, 0, linesPerWrite*emitLineLength)
	for index := 1; index <= lineCount; index++ {
		buffer = fmt.Appendf(buffer, "%08d %s\r\n", index, emitLineText)
		if len(buffer) >= linesPerWrite*emitLineLength || index == lineCount {
			if _, err := channel.Write(buffer); err != nil {
				return err
			}
			buffer = buffer[:0]
		}
	}
	return nil
}

func (server *Server) fileClient() (*sftp.Client, func(), error) {
	client, err := ssh.Dial("tcp", server.Address(), &ssh.ClientConfig{User: Username, Auth: []ssh.AuthMethod{ssh.Password(Password)}, HostKeyCallback: ssh.FixedHostKey(server.PublicKey()), Timeout: 5 * time.Second})
	if err != nil {
		return nil, nil, err
	}
	filesystem, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, nil, err
	}
	return filesystem, func() { filesystem.Close(); client.Close() }, nil
}

func (server *Server) seed() error {
	client, closeClient, err := server.fileClient()
	if err != nil {
		return err
	}
	defer closeClient()
	for _, directory := range []string{"/workspace", "/workspace/config", "/workspace/logs", DiscardDirectory} {
		if err := client.Mkdir(directory); err != nil {
			return err
		}
	}
	for filename, content := range map[string]string{"/workspace/欢迎.txt": "\ufeff你好，世界！\r\n这里是 SFTP 文本编辑测试。\r\n", "/workspace/README.md": "# SSH Workbench\n\n欢迎使用中文终端与 SFTP 工作区。\n", "/workspace/config/app.yaml": "name: ssh-workbench\nmode: development\n", "/workspace/.profile": "export LANG=C.UTF-8\n"} {
		file, err := client.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
		if err != nil {
			return err
		}
		_, writeError := io.WriteString(file, content)
		file.Close()
		if writeError != nil {
			return writeError
		}
	}
	return nil
}

func (server *Server) WriteFile(filename, content string) error {
	client, release, err := server.fileClient()
	if err != nil {
		return err
	}
	defer release()
	file, err := client.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.WriteString(file, content)
	return err
}

func (server *Server) Close() {
	server.mu.Lock()
	defer server.mu.Unlock()
	server.closed = true
	server.listener.Close()
	for connection := range server.connections {
		connection.Close()
	}
}

func (server *Server) RunUntil(ctx context.Context) { <-ctx.Done(); server.Close() }
