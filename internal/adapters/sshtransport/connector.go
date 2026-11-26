package sshtransport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/hosts"
	"sshworkbench/internal/terminals"
)

const handshakeTimeout = 20 * time.Second

type Options struct {
	// AllowAgent permits the server-side SSH agent (local mode, or remote
	// mode with an explicit owner flag).
	AllowAgent bool
	// LocalFiles permits reading private keys, ~/.ssh/known_hosts, and ProxyCommand.
	LocalFiles bool
	// StrictHostKeyConfirm forces manual confirmation on unknown host keys instead of TOFU.
	StrictHostKeyConfirm bool
}

type Connector struct {
	verifier *connections.Verifier
	options  Options
}

func New(verifier *connections.Verifier, options Options) *Connector {
	return &Connector{verifier: verifier, options: options}
}

// lease owns every client and helper connection of one authenticated
// transport, including an optional jump host. Closing it is idempotent and
// may happen concurrently with dialing (cancellation, a dying jump host), so
// resources are registered through add, which closes late arrivals at once.
type lease struct {
	mu      sync.Mutex
	client  *ssh.Client
	closers []io.Closer
	done    bool
	closed  chan struct{}
}

// add registers a resource for closing. It reports false (and closes the
// resource) when the lease is already closed.
func (transport *lease) add(closer io.Closer) bool {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.done {
		closer.Close()
		return false
	}
	transport.closers = append(transport.closers, closer)
	return true
}

func (transport *lease) current() *ssh.Client {
	transport.mu.Lock()
	defer transport.mu.Unlock()
	return transport.client
}

func (transport *lease) Alive() bool {
	select {
	case <-transport.closed:
		return false
	default:
		return true
	}
}

func (transport *lease) Close() error {
	transport.mu.Lock()
	if transport.done {
		transport.mu.Unlock()
		return nil
	}
	transport.done = true
	closers := transport.closers
	transport.closers = nil
	close(transport.closed)
	transport.mu.Unlock()
	// Newest first: the target client before the jump client it rides on.
	for index := len(closers) - 1; index >= 0; index-- {
		closers[index].Close()
	}
	return nil
}

func (transport *lease) keepAlive(client *ssh.Client) {
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-transport.closed:
				return
			case <-ticker.C:
				deadline := time.AfterFunc(10*time.Second, func() { transport.Close() })
				_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
				deadline.Stop()
				if err != nil {
					transport.Close()
					return
				}
			}
		}
	}()
	go func() {
		client.Wait()
		transport.Close()
	}()
}

func (connector *Connector) dial(ctx context.Context, target connections.Target) (*lease, error) {
	transport := &lease{closed: make(chan struct{})}
	stop := context.AfterFunc(ctx, func() { transport.Close() })
	defer stop()
	hops := target.Hops()
	for index, hop := range hops {
		role := "target"
		if index < len(hops)-1 {
			role = "jump"
		}
		previous := transport.current()
		var connection net.Conn
		var err error
		if previous == nil {
			if hop.Host.ProxyCommand != "" {
				if !connector.options.LocalFiles {
					transport.Close()
					return nil, describe(role, fault.New(403, "proxy_command_disabled", "当前模式禁止执行 ProxyCommand。"))
				}
				connection, err = dialProxyCommand(ctx, hop.Host.ProxyCommand, hop.Host.Hostname, hop.Host.Port, hop.Host.Username)
			} else {
				connection, err = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, "tcp", hop.Host.Address())
			}
		} else {
			connection, err = previous.DialContext(ctx, "tcp", hop.Host.Address())
		}
		if err != nil {
			transport.Close()
			return nil, describe(role, networkError(ctx, err, previous != nil))
		}
		if pc, ok := connection.(*proxyConn); ok {
			pc.detachDialContext()
		}
		if !transport.add(connection) {
			return nil, ctx.Err()
		}
		client, err := connector.handshake(ctx, role, connection, hop, target.Interaction, transport)
		if err != nil {
			transport.Close()
			return nil, describe(role, err)
		}
		if !transport.add(client) {
			return nil, context.Canceled
		}
		transport.mu.Lock()
		transport.client = client
		transport.mu.Unlock()
		transport.keepAlive(client)
	}
	if ctx.Err() != nil {
		transport.Close()
		return nil, ctx.Err()
	}
	return transport, nil
}

func describe(role string, err error) error {
	if role != "jump" {
		return err
	}
	var problem *fault.Error
	if errors.As(err, &problem) {
		copy := *problem
		copy.Message = "跳板机：" + problem.Message
		return &copy
	}
	return err
}

func (connector *Connector) handshake(ctx context.Context, role string, connection net.Conn, hop connections.Hop, interaction connections.Interaction, transport *lease) (*ssh.Client, error) {
	// Channels opened through a jump host do not support deadlines, so a
	// timer closes the connection instead. It pauses while a person answers.
	var timedOut atomic.Bool
	timer := time.AfterFunc(handshakeTimeout, func() {
		timedOut.Store(true)
		connection.Close()
	})
	defer timer.Stop()
	deadline := func() { timer.Reset(handshakeTimeout) }
	waitForUser := func() { timer.Stop() }
	methods, sawEncryptedKey, err := connector.authMethods(ctx, role, hop, interaction, transport, waitForUser, deadline)
	if err != nil {
		return nil, err
	}
	address := hop.Host.Address()
	var verifiedMu sync.Mutex
	var verified []byte
	algorithms := ssh.SupportedAlgorithms()
	config := &ssh.ClientConfig{
		Config:            ssh.Config{KeyExchanges: algorithms.KeyExchanges, Ciphers: algorithms.Ciphers, MACs: algorithms.MACs},
		HostKeyAlgorithms: algorithms.HostKeys,
		User:              hop.Host.Username,
		Auth:              methods,
		HostKeyCallback: func(_ string, remote net.Addr, publicKey ssh.PublicKey) error {
			presented := publicKey.Marshal()
			// x/crypto/ssh calls this again on every re-key, long after the
			// dial context is gone. Once verified, only the identical key is
			// accepted for the life of this connection — no lookup, no prompt.
			verifiedMu.Lock()
			pinned := verified
			verifiedMu.Unlock()
			if pinned != nil {
				if bytes.Equal(pinned, presented) {
					return nil
				}
				return fault.New(409, "host_key_changed", "服务器在会话中途更换了主机密钥，已断开连接。")
			}
			key := connections.KnownKey{Address: address, Algorithm: publicKey.Type(), Fingerprint: ssh.FingerprintSHA256(publicKey), PublicKey: presented}
			trusted, err := connector.verifier.Check(ctx, key)
			if err != nil {
				return err
			}
			if !trusted {
				knownHint := ""
				if connector.options.LocalFiles {
					knownHint = systemKnownHosts(hop.Host, remote, publicKey)
				}
				autoAccept := !connector.options.StrictHostKeyConfirm && knownHint != "mismatch" && (interaction != nil || knownHint == "match")
				if autoAccept {
					if err := connector.verifier.Accept(ctx, key); err != nil {
						return err
					}
				} else {
					if interaction == nil {
						return &fault.Error{Status: 409, Code: "host_key_unknown", Message: "此主机尚未被信任。后台操作不能接受新的主机密钥，请从连接对话框重新连接并核对指纹。", Details: map[string]string{"address": key.Address, "fingerprint": key.Fingerprint}}
					}
					prompt := connections.Prompt{Hop: role, Host: hop.Host.Label(), Address: key.Address, Algorithm: key.Algorithm, Fingerprint: key.Fingerprint, KnownHosts: knownHint}
					waitForUser()
					err = interaction.ConfirmHostKey(ctx, prompt)
					deadline()
					if err != nil {
						return err
					}
					if err := connector.verifier.Accept(ctx, key); err != nil {
						return err
					}
				}
			}
			verifiedMu.Lock()
			verified = presented
			verifiedMu.Unlock()
			return nil
		},
	}
	clientConnection, channels, requests, err := ssh.NewClientConn(connection, address, config)
	if !timer.Stop() && timedOut.Load() {
		if clientConnection != nil {
			clientConnection.Close()
		}
		return nil, fault.New(504, "ssh_handshake_timeout", "SSH 握手超时。")
	}
	if err != nil {
		if hop.Host.AuthMode == hosts.AuthAuto && strings.Contains(err.Error(), "unable to authenticate") {
			if sawEncryptedKey && hop.Credentials.Passphrase == "" {
				return nil, fault.New(400, "passphrase_required", "私钥已加密，请输入私钥口令。")
			}
			if hop.Credentials.Password == "" {
				return nil, fault.New(401, "password_required", "需要输入 SSH 密码完成登录。")
			}
		}
		return nil, handshakeError(ctx, err)
	}
	return ssh.NewClient(clientConnection, channels, requests), nil
}

func (connector *Connector) authMethods(ctx context.Context, role string, hop connections.Hop, interaction connections.Interaction, transport *lease, waitForUser, deadline func()) ([]ssh.AuthMethod, bool, error) {
	credentials := hop.Credentials
	if len(credentials.Password) > 4096 || len(credentials.Passphrase) > 4096 || len(credentials.PrivateKey) > 256*1024 {
		return nil, false, fault.New(400, "credentials_too_large", "凭据超过大小限制。")
	}
	switch hop.Host.AuthMode {
	case hosts.AuthAuto:
		var methods []ssh.AuthMethod
		sawEncryptedKey := false
		var signers []ssh.Signer
		seenKeys := map[string]bool{}
		addSigner := func(signer ssh.Signer) {
			if signer == nil {
				return
			}
			keyBytes := string(signer.PublicKey().Marshal())
			if seenKeys[keyBytes] {
				return
			}
			seenKeys[keyBytes] = true
			signers = append(signers, signer)
		}
		if len(credentials.PrivateKey) > 0 {
			signer, err := parsePrivateKey([]byte(credentials.PrivateKey), credentials.Passphrase)
			if err != nil {
				return nil, false, err
			}
			addSigner(signer)
		}
		configuredPaths := collectIdentityPaths(hop.Host)
		if connector.options.LocalFiles && len(configuredPaths) > 0 {
			for _, path := range configuredPaths {
				material, err := readIdentityFile(path)
				if err != nil {
					continue
				}
				signer, err := parsePrivateKey(material, credentials.Passphrase)
				if err != nil {
					if fault.As(err).Code == "passphrase_required" {
						sawEncryptedKey = true
					}
					continue
				}
				addSigner(signer)
			}
		}
		if connector.options.LocalFiles && len(configuredPaths) == 0 && len(credentials.PrivateKey) == 0 {
			if home, err := os.UserHomeDir(); err == nil {
				for _, name := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
					candidate := filepath.Join(home, ".ssh", name)
					material, err := readIdentityFile(candidate)
					if err != nil {
						continue
					}
					signer, err := parsePrivateKey(material, credentials.Passphrase)
					if err != nil {
						if fault.As(err).Code == "passphrase_required" {
							sawEncryptedKey = true
						}
						continue
					}
					addSigner(signer)
				}
			}
		}
		if !hop.Host.IdentitiesOnly && connector.options.AllowAgent {
			if connection, err := dialAgent(ctx); err == nil {
				if transport.add(connection) {
					if agentSigners, err := agent.NewClient(connection).Signers(); err == nil {
						for _, signer := range agentSigners {
							addSigner(signer)
						}
					}
				}
			}
		}
		if len(signers) > 0 {
			methods = append(methods, ssh.PublicKeys(signers...))
		}
		if credentials.Password != "" {
			password := credentials.Password
			var answered atomic.Bool
			answerPasswordPrompt := func(_, _ string, questions []string, echoes []bool) ([]string, error) {
				if len(questions) == 0 {
					return nil, nil
				}
				if len(questions) != 1 || echoes[0] || !plainPasswordPrompt(questions[0]) || !answered.CompareAndSwap(false, true) {
					return nil, errors.New("server asked more than a password")
				}
				return []string{password}, nil
			}
			methods = append(methods, ssh.Password(password), ssh.KeyboardInteractive(answerPasswordPrompt))
		} else if interaction != nil {
			challenge := func(name, instruction string, questions []string, echoes []bool) ([]string, error) {
				if len(questions) == 0 {
					return nil, nil
				}
				prompt := connections.Prompt{Hop: role, Host: hop.Host.Label(), Name: clean(name), Instruction: clean(instruction)}
				for index, question := range questions {
					prompt.Questions = append(prompt.Questions, connections.Question{Text: clean(question), Echo: index < len(echoes) && echoes[index]})
				}
				waitForUser()
				defer deadline()
				return interaction.Respond(ctx, prompt)
			}
			methods = append(methods, ssh.RetryableAuthMethod(ssh.KeyboardInteractive(challenge), 2))
		}
		return methods, sawEncryptedKey, nil
	case hosts.AuthPassword:
		password := credentials.Password
		var answered atomic.Bool
		// Many servers route passwords through PAM keyboard-interactive.
		// Answer only a plain password prompt, once per connection, so a
		// one-time-code question is never fed the account password.
		answerPasswordPrompt := func(_, _ string, questions []string, echoes []bool) ([]string, error) {
			if len(questions) == 0 {
				return nil, nil
			}
			if len(questions) != 1 || echoes[0] || !plainPasswordPrompt(questions[0]) || !answered.CompareAndSwap(false, true) {
				return nil, errors.New("server asked more than a password")
			}
			return []string{password}, nil
		}
		return []ssh.AuthMethod{ssh.Password(password), ssh.KeyboardInteractive(answerPasswordPrompt)}, false, nil
	case hosts.AuthKey:
		signer, err := connector.signer(hop)
		if err != nil {
			return nil, false, err
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, false, nil
	case hosts.AuthAgent:
		if !connector.options.AllowAgent {
			return nil, false, fault.New(403, "agent_disabled", "远程模式下默认禁止使用服务端 SSH agent。")
		}
		connection, err := dialAgent(ctx)
		if err != nil {
			return nil, false, fault.New(400, "agent_unavailable", "无法连接本机 SSH agent，请确认 agent 已启动并加载密钥。")
		}
		if !transport.add(connection) {
			return nil, false, context.Canceled
		}
		return []ssh.AuthMethod{ssh.PublicKeysCallback(agent.NewClient(connection).Signers)}, false, nil
	case hosts.AuthKeyboardInteractive:
		if interaction == nil {
			return nil, false, fault.New(401, "interaction_required", "此主机需要交互式认证，后台操作不能代替你回答，请重新连接。")
		}
		challenge := func(name, instruction string, questions []string, echoes []bool) ([]string, error) {
			if len(questions) == 0 {
				return nil, nil
			}
			prompt := connections.Prompt{Hop: role, Host: hop.Host.Label(), Name: clean(name), Instruction: clean(instruction)}
			for index, question := range questions {
				prompt.Questions = append(prompt.Questions, connections.Question{Text: clean(question), Echo: index < len(echoes) && echoes[index]})
			}
			waitForUser()
			defer deadline()
			return interaction.Respond(ctx, prompt)
		}
		return []ssh.AuthMethod{ssh.RetryableAuthMethod(ssh.KeyboardInteractive(challenge), 3)}, false, nil
	default:
		return nil, false, fault.New(400, "unsupported_auth", "不支持此认证方式。")
	}
}

func collectIdentityPaths(profile hosts.Profile) []string {
	var paths []string
	seen := map[string]bool{}
	if profile.IdentityFile != "" {
		seen[profile.IdentityFile] = true
		paths = append(paths, profile.IdentityFile)
	}
	for _, item := range profile.IdentityFiles {
		if item != "" && !seen[item] {
			seen[item] = true
			paths = append(paths, item)
		}
	}
	return paths
}

func parsePrivateKey(material []byte, passphrase string) (ssh.Signer, error) {
	var signer ssh.Signer
	var err error
	if passphrase == "" {
		signer, err = ssh.ParsePrivateKey(material)
	} else {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(material, []byte(passphrase))
	}
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		return nil, fault.New(400, "passphrase_required", "私钥已加密，请输入私钥口令。")
	}
	if err != nil {
		return nil, fault.New(400, "invalid_private_key", "无法读取私钥，请检查格式与私钥口令。")
	}
	return signer, nil
}

func (connector *Connector) signer(hop connections.Hop) (ssh.Signer, error) {
	material := []byte(hop.Credentials.PrivateKey)
	if len(material) == 0 && hop.Host.IdentityFile != "" {
		if !connector.options.LocalFiles {
			return nil, fault.New(403, "identity_file_disabled", "远程模式不读取服务端本机私钥文件，请在连接时选择私钥。")
		}
		var err error
		material, err = readIdentityFile(hop.Host.IdentityFile)
		if err != nil {
			return nil, err
		}
	}
	if len(material) == 0 {
		return nil, fault.New(400, "private_key_required", "请选择私钥文件。")
	}
	return parsePrivateKey(material, hop.Credentials.Passphrase)
}

// plainPasswordPrompt accepts "Password:" and "user@host's password:" but not
// prompts for one-time, verification or token codes.
func plainPasswordPrompt(question string) bool {
	lower := strings.ToLower(strings.TrimSpace(question))
	if !strings.HasSuffix(lower, "password:") && lower != "password" {
		return false
	}
	for _, word := range []string{"one-time", "one time", "otp", "oath", "token", "code", "verification", "new password", "pin"} {
		if strings.Contains(lower, word) {
			return false
		}
	}
	return true
}

// clean keeps server-provided prompt text printable and bounded; it is shown
// to the user as plain text, never as HTML.
func clean(value string) string {
	value = strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' {
			return character
		}
		if character < 32 || character == 127 {
			return -1
		}
		return character
	}, strings.ToValidUTF8(value, "�"))
	if len(value) > 512 {
		value = value[:512]
	}
	return strings.TrimSpace(value)
}

func networkError(ctx context.Context, err error, throughJump bool) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if throughJump {
		return fault.New(502, "jump_forward_failed", "跳板机无法连接目标主机，请检查目标地址或跳板机的转发权限。")
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return fault.New(502, "dns_failed", "无法解析主机名，请检查地址或 DNS。")
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return fault.New(502, "connection_refused", "主机拒绝连接，请检查 SSH 端口和服务是否运行。")
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return fault.New(504, "connection_timeout", "连接主机超时，请检查网络与防火墙。")
	}
	return fault.New(502, "ssh_unreachable", "无法连接 SSH 主机，请检查地址、端口和网络。")
}

func handshakeError(ctx context.Context, err error) error {
	var problem *fault.Error
	if errors.As(err, &problem) {
		return problem
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "unable to authenticate"):
		return fault.New(401, "ssh_auth_failed", "SSH 认证失败，请检查用户名、凭据与服务器允许的认证方式。")
	case strings.Contains(message, "no common algorithm"):
		return fault.New(502, "ssh_algorithm_mismatch", "服务器只提供不受支持的加密算法，未降低安全要求。")
	case errors.Is(err, io.EOF) || strings.Contains(message, "connection reset"):
		return fault.New(502, "ssh_closed", "服务器在握手期间关闭了连接，可能不是 SSH 服务或拒绝了此客户端。")
	}
	return fault.New(502, "ssh_handshake_failed", "SSH 握手失败，未降低主机验证和加密要求。")
}

func (connector *Connector) OpenTerminal(ctx context.Context, target connections.Target, rows, cols int) (terminals.Shell, error) {
	transport, err := connector.dial(ctx, target)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { transport.Close() })
	defer stop()
	session, err := transport.client.NewSession()
	if err != nil {
		transport.Close()
		return nil, fault.New(502, "ssh_channel_failed", "服务器拒绝建立终端通道。")
	}
	stdout, stdoutError := session.StdoutPipe()
	stderr, stderrError := session.StderrPipe()
	stdin, stdinError := session.StdinPipe()
	if stdoutError != nil || stderrError != nil || stdinError != nil {
		transport.Close()
		return nil, fault.New(502, "ssh_pipes_failed", "无法创建终端输入输出通道。")
	}
	if err := session.RequestPty("xterm-256color", rows, cols, ssh.TerminalModes{ssh.ECHO: 1}); err != nil {
		transport.Close()
		return nil, fault.New(502, "pty_unavailable", "服务器不支持或拒绝分配 PTY。")
	}
	if err := session.Shell(); err != nil {
		transport.Close()
		return nil, fault.New(502, "shell_unavailable", "服务器无法启动交互式 shell。")
	}
	if ctx.Err() != nil {
		transport.Close()
		return nil, ctx.Err()
	}
	return &shell{session: session, stdout: stdout, stderr: stderr, stdin: stdin, transport: transport}, nil
}

type shell struct {
	session   *ssh.Session
	stdout    io.Reader
	stderr    io.Reader
	stdin     io.Writer
	transport *lease
}

func (terminal *shell) Stdout() io.Reader                { return terminal.stdout }
func (terminal *shell) Stderr() io.Reader                { return terminal.stderr }
func (terminal *shell) Input() io.Writer                 { return terminal.stdin }
func (terminal *shell) Transport() connections.Transport { return terminal.transport }
func (terminal *shell) Resize(rows, cols int) error      { return terminal.session.WindowChange(rows, cols) }
func (terminal *shell) Close() error                     { terminal.transport.Close(); return terminal.session.Close() }
func (terminal *shell) Wait() terminals.Exit             { return exitOf(terminal.session.Wait()) }

func exitOf(err error) terminals.Exit {
	if err == nil {
		code := 0
		return terminals.Exit{Code: &code}
	}
	var exit *ssh.ExitError
	if errors.As(err, &exit) {
		code := exit.ExitStatus()
		result := terminals.Exit{Code: &code, Signal: clean(exit.Signal())}
		if exit.Signal() != "" {
			result.Code = nil
		}
		return result
	}
	return terminals.Exit{Disconnected: true}
}
