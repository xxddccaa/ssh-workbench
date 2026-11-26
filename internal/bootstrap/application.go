package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"sshworkbench/internal/adapters/localpty"
	"sshworkbench/internal/adapters/platform"
	"sshworkbench/internal/adapters/sqlite"
	"sshworkbench/internal/adapters/sshtransport"
	"sshworkbench/internal/auth"
	"sshworkbench/internal/connections"
	"sshworkbench/internal/control"
	"sshworkbench/internal/credentials"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/files"
	"sshworkbench/internal/hosts"
	"sshworkbench/internal/imports"
	"sshworkbench/internal/launch"
	"sshworkbench/internal/localdocs"
	"sshworkbench/internal/snippets"
	"sshworkbench/internal/terminals"
	"sshworkbench/internal/transport/httpapi"
	"sshworkbench/internal/tunnels"
	"sshworkbench/internal/vault"
	"sshworkbench/internal/workspaces"
	"sshworkbench/web"
)

type Config struct {
	Listen          string
	DataDir         string
	Auth            bool
	Remote          bool
	Origin          string
	DevOrigin       string
	TLSCert         string
	TLSKey          string
	AllowAgent      bool
	DisableLocalSSH bool
	Version         string
	// Keyring overrides the OS credential store (tests).
	Keyring vault.Keyring
}

type Ready struct {
	Origin          string
	Listen          string
	RequestedListen string
	URL             string
	Version         string
	PID             int
	Remote          bool
	AuthRequired    bool
}

// ErrAlreadyRunning is returned when another local server owns the data
// directory; the CLI then asks that server for a new authorization instead.
var ErrAlreadyRunning = platform.ErrAlreadyRunning

func (config Config) Validate() error {
	if _, _, err := net.SplitHostPort(config.Listen); err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if config.Remote {
		origin, err := url.Parse(config.Origin)
		if err != nil || origin.Scheme != "https" || origin.Host == "" || config.TLSCert == "" || config.TLSKey == "" {
			return errors.New("remote mode requires --origin https://your-host:port, --tls-cert and --tls-key")
		}
		if config.DevOrigin != "" {
			return errors.New("development proxy is not allowed in remote mode")
		}
	}
	if config.DataDir == "" {
		return errors.New("a private data directory is required")
	}
	return nil
}

func Run(ctx context.Context, config Config, onReady func(Ready)) error {
	if err := config.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	store, err := sqlite.Open(config.DataDir)
	if err != nil {
		return err
	}
	defer store.Close()
	controlListener, err := platform.ListenControl(config.DataDir)
	if errors.Is(err, platform.ErrAlreadyRunning) {
		return ErrAlreadyRunning
	}
	if err != nil {
		slog.Warn("local control channel unavailable; `sshw open` and `edit` will not work", "reason", err.Error())
	} else {
		defer controlListener.Close()
	}
	passwordHash, err := store.GetSetting(ctx, "owner_password")
	if err != nil {
		return err
	}
	if config.Remote && passwordHash == "" {
		return errors.New("set the owner password locally first: sshw password --data-dir <directory>")
	}
	var tlsConfiguration *tls.Config
	if config.Remote {
		certificate, err := tls.LoadX509KeyPair(config.TLSCert, config.TLSKey)
		if err != nil {
			return fmt.Errorf("load TLS certificate: %w", err)
		}
		tlsConfiguration = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}}
	}
	listener, err := listenWithFallback(ctx, config.Listen, config.Remote)
	if err != nil {
		return err
	}
	defer listener.Close()
	actualListen := listener.Addr().String()
	listenAddr := actualListen
	browserHost, port, _ := net.SplitHostPort(listenAddr)
	if browserHost == "" || browserHost == "0.0.0.0" || browserHost == "::" {
		listenAddr = net.JoinHostPort("127.0.0.1", port)
	}
	origin := "http://" + listenAddr
	if config.Remote {
		origin = config.Origin
	}
	requireAuth := config.Auth || config.Remote
	services, closeServices := Compose(ctx, store, Options{
		RequireAuth:     requireAuth,
		Remote:          config.Remote,
		AllowAgent:      config.AllowAgent,
		DisableLocalSSH: config.DisableLocalSSH,
		PasswordHash:    passwordHash,
		Keyring:         config.Keyring,
	})
	defer closeServices()
	authentication := services.Auth
	allowAgent := !config.DisableLocalSSH && (!config.Remote || config.AllowAgent)
	localFiles := !config.DisableLocalSSH && !config.Remote
	localTerminal := localFiles && localpty.Supported()
	handler, err := httpapi.New(httpapi.Config{
		Origin:        origin,
		DevOrigin:     config.DevOrigin,
		Remote:        config.Remote,
		RequireAuth:   requireAuth,
		Version:       config.Version,
		LocalFiles:    localFiles,
		Agent:         allowAgent,
		LocalTerminal: localTerminal,
	}, services, web.Assets())
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 32 * 1024, TLSConfig: tlsConfiguration}
	failures := make(chan error, 1)
	go func() {
		if config.Remote {
			failures <- server.ServeTLS(listener, "", "")
		} else {
			failures <- server.Serve(listener)
		}
	}()
	browserOrigin := origin
	if config.DevOrigin != "" {
		browserOrigin = config.DevOrigin
	}
	browserURL := browserOrigin
	if requireAuth && !config.Remote {
		browserURL += "/#bootstrap=" + authentication.BootstrapToken()
	}
	if controlListener != nil {
		go control.Serve(ctx, controlListener, &controlHandler{
			origin:      browserOrigin,
			listen:      actualListen,
			version:     config.Version,
			remote:      config.Remote,
			requireAuth: requireAuth,
			auth:        authentication,
			documents:   services.Documents,
			stop:        cancel,
		})
	}
	if onReady != nil {
		onReady(Ready{
			Origin:          origin,
			Listen:          actualListen,
			RequestedListen: config.Listen,
			URL:             browserURL,
			Version:         config.Version,
			PID:             os.Getpid(),
			Remote:          config.Remote,
			AuthRequired:    requireAuth,
		})
	}
	select {
	case err := <-failures:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	cancel()
	closeServices()
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownContext); err != nil {
		slog.Warn("graceful shutdown timed out")
		server.Close()
	}
	return nil
}

type Options struct {
	RequireAuth          bool
	Remote               bool
	AllowAgent           bool
	DisableLocalSSH      bool
	StrictHostKeyConfirm bool
	PasswordHash         string
	Keyring              vault.Keyring
}

type terminalRouter struct {
	ssh          terminals.Connector
	localEnabled bool
}

func (router terminalRouter) OpenTerminal(ctx context.Context, target connections.Target, rows, cols int) (terminals.Shell, error) {
	if target.Local {
		if !router.localEnabled {
			return nil, fault.New(403, "local_terminal_disabled", "当前模式或系统不支持本地终端。")
		}
		return localpty.Open(ctx, rows, cols)
	}
	return router.ssh.OpenTerminal(ctx, target, rows, cols)
}

// Compose builds every business service and wires their lifecycles. The
// returned function stops logins, tunnels and terminals in that order; it is
// safe to call more than once.
func Compose(ctx context.Context, store *sqlite.Store, options Options) (httpapi.Services, func()) {
	keyring := options.Keyring
	if keyring == nil {
		keyring = platform.SystemKeyring{}
	}
	allowAgent := !options.DisableLocalSSH && (!options.Remote || options.AllowAgent)
	localFiles := !options.DisableLocalSSH && !options.Remote
	localTerminal := localFiles && localpty.Supported()
	hostService := hosts.New(store)
	verifier := connections.NewVerifier(store)
	connector := sshtransport.New(verifier, sshtransport.Options{AllowAgent: allowAgent, LocalFiles: localFiles, StrictHostKeyConfirm: options.Remote || options.StrictHostKeyConfirm})
	attempts := connections.NewAttempts(ctx)
	terminalService := terminals.New(ctx, terminalRouter{ssh: connector, localEnabled: localTerminal})
	tunnelService := tunnels.New(ctx, store, connector)
	hostService.AddReferences(tunnelService)
	secrets := vault.New(store, keyring)
	credentialService := credentials.New(secrets, store, hostService)
	importService := imports.New(hostService, allowAgent)
	var sshConfigResolver launch.SSHConfigResolver
	if localFiles {
		sshConfigResolver = importService
	}
	services := httpapi.Services{
		Hosts:       hostService,
		Terminals:   terminalService,
		Files:       files.New(connector, terminalService),
		Attempts:    attempts,
		Launch:      launch.New(hostService, credentialService, attempts, terminalService, tunnelService, sshConfigResolver, localTerminal),
		Vault:       secrets,
		Credentials: credentialService,
		Tunnels:     tunnelService,
		Snippets:    snippets.New(store),
		Workspaces:  workspaces.New(store),
		Imports:     importService,
	}
	if localFiles {
		services.Documents = localdocs.New()
	}
	// Revoking a login ends everything it started: pending prompts,
	// terminals, tunnels and the unlocked vault key.
	requireAuth := options.RequireAuth || options.Remote
	services.Auth = auth.NewWithMode(options.Remote, requireAuth, options.PasswordHash, func(owner string) {
		attempts.CloseOwner(owner)
		terminalService.CloseOwner(owner)
		tunnelService.CloseOwner(owner)
		secrets.Lock()
	})
	return services, func() {
		services.Auth.Close()
		tunnelService.Close()
		terminalService.Close()
		secrets.Lock()
	}
}

type controlHandler struct {
	origin      string
	listen      string
	version     string
	remote      bool
	requireAuth bool
	auth        *auth.Service
	documents   *localdocs.Service
	stop        func()
}

func (handler *controlHandler) Open() (string, error) {
	if handler.remote {
		return "", errors.New("远程模式下请使用密码登录。")
	}
	if !handler.requireAuth {
		return handler.origin, nil
	}
	token, err := handler.auth.IssueBootstrap()
	if err != nil {
		return "", err
	}
	return handler.origin + "/#bootstrap=" + token, nil
}

func (handler *controlHandler) Status() control.Response {
	return control.Response{
		Origin:  handler.origin,
		Listen:  handler.listen,
		Version: handler.version,
		PID:     os.Getpid(),
		Remote:  handler.remote,
	}
}

func (handler *controlHandler) Stop() {
	if handler.stop != nil {
		handler.stop()
	}
}

func (handler *controlHandler) Edit(filename string) (string, error) {
	if handler.documents == nil {
		return "", errors.New("local file editing is disabled")
	}
	registration, err := handler.documents.Register(filename)
	if err != nil {
		return "", err
	}
	if !handler.requireAuth {
		return handler.origin + "/local-editor?document=" + url.QueryEscape(registration.ID), nil
	}
	token, err := handler.auth.IssueBootstrap()
	if err != nil {
		return "", err
	}
	return handler.origin + "/local-editor?document=" + url.QueryEscape(registration.ID) + "#bootstrap=" + token, nil
}
