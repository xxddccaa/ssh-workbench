// Package launch turns a user's connect request into a connection attempt:
// it resolves host profiles, the optional jump host and saved credentials,
// then opens a terminal or starts a tunnel while the attempt relays prompts.
package launch

import (
	"context"
	"os/user"
	"strings"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/hosts"
	"sshworkbench/internal/terminals"
	"sshworkbench/internal/tunnels"
)

type QuickConnect struct {
	Name         string
	Hostname     string
	Port         int
	Username     string
	AuthMode     string
	IdentityFile string
	ProxyCommand string
}

type Request struct {
	HostID          string
	QuickConnect    *QuickConnect
	Rows            int
	Cols            int
	Credentials     connections.Credentials
	JumpCredentials connections.Credentials
	UseSaved        bool
	RememberLabel   string
}

type Result struct {
	Terminal *terminals.Info `json:"terminal,omitempty"`
	Tunnel   *tunnels.View   `json:"tunnel,omitempty"`
	Warning  string          `json:"warning,omitempty"`
	release  func()
}

// Release closes what the result opened; the attempt registry calls it when
// the user cancelled after the connection had already succeeded.
func (result Result) Release() {
	if result.release != nil {
		result.release()
	}
}

type Vault interface {
	Resolve(context.Context, string) (connections.Credentials, error)
	Remember(context.Context, string, connections.Credentials) (string, error)
}

type SSHConfigResolver interface {
	ResolveHost(alias string) (hosts.Profile, *hosts.Profile, error)
}

type Service struct {
	hosts     *hosts.Service
	vault     Vault
	attempts  *connections.Attempts
	terminals *terminals.Service
	tunnels   *tunnels.Service
	sshConfig SSHConfigResolver
}

func New(hostService *hosts.Service, vault Vault, attempts *connections.Attempts, terminalService *terminals.Service, tunnelService *tunnels.Service, sshConfig SSHConfigResolver) *Service {
	return &Service{hosts: hostService, vault: vault, attempts: attempts, terminals: terminalService, tunnels: tunnelService, sshConfig: sshConfig}
}

func currentUsername() string {
	current, err := user.Current()
	if err != nil {
		return ""
	}
	name := current.Username
	if index := strings.LastIndexAny(name, `\`); index >= 0 {
		name = name[index+1:]
	}
	return name
}

func (service *Service) target(ctx context.Context, owner string, request Request) (connections.Target, hosts.Profile, bool, error) {
	if request.QuickConnect != nil {
		quick := request.QuickConnect
		username := strings.TrimSpace(quick.Username)
		if username == "" {
			username = currentUsername()
		}
		if username == "" {
			username = "root"
		}
		port := quick.Port
		if port == 0 {
			port = 22
		}
		authMode := quick.AuthMode
		if authMode == "" {
			authMode = hosts.AuthAuto
		}
		name := strings.TrimSpace(quick.Name)
		if name == "" {
			name = username + "@" + quick.Hostname
		}
		profile := hosts.Normalize(hosts.Profile{
			ID:           "quick:" + username + "@" + quick.Hostname,
			Name:         name,
			Hostname:     quick.Hostname,
			Port:         port,
			Username:     username,
			AuthMode:     authMode,
			Color:        "teal",
			IdentityFile: quick.IdentityFile,
			ProxyCommand: quick.ProxyCommand,
		})
		if err := hosts.Validate(profile); err != nil {
			return connections.Target{}, hosts.Profile{}, false, err
		}
		return connections.Target{Owner: owner, Host: profile, Credentials: request.Credentials}, profile, false, nil
	}
	if strings.HasPrefix(request.HostID, "sshcfg:") {
		if service.sshConfig == nil {
			return connections.Target{}, hosts.Profile{}, false, fault.New(404, "ssh_config_disabled", "未启用本机 ~/.ssh/config 读取。")
		}
		alias := strings.TrimPrefix(request.HostID, "sshcfg:")
		profile, jump, err := service.sshConfig.ResolveHost(alias)
		if err != nil {
			return connections.Target{}, hosts.Profile{}, false, err
		}
		target := connections.Target{Owner: owner, Host: profile, Credentials: request.Credentials}
		if jump != nil {
			target.Jump = &connections.Hop{Host: *jump, Credentials: request.JumpCredentials}
		}
		return target, profile, false, nil
	}
	profile, err := service.hosts.Get(ctx, request.HostID)
	if err != nil {
		return connections.Target{}, hosts.Profile{}, false, err
	}
	credentials, err := service.credentialsFor(ctx, profile, request.Credentials, request.UseSaved)
	if err != nil {
		return connections.Target{}, hosts.Profile{}, false, err
	}
	target := connections.Target{Owner: owner, Host: profile, Credentials: credentials}
	if profile.JumpHostID != "" {
		jump, err := service.hosts.Get(ctx, profile.JumpHostID)
		if err != nil {
			return connections.Target{}, hosts.Profile{}, false, fault.New(409, "jump_host_missing", "此主机配置的跳板机已不存在，请编辑主机。")
		}
		if jump.JumpHostID != "" {
			return connections.Target{}, hosts.Profile{}, false, fault.New(409, "nested_jump_host", "当前只支持单级跳板。")
		}
		jumpCredentials, err := service.credentialsFor(ctx, jump, request.JumpCredentials, request.UseSaved)
		if err != nil {
			return connections.Target{}, hosts.Profile{}, false, err
		}
		target.Jump = &connections.Hop{Host: jump, Credentials: jumpCredentials}
	}
	return target, profile, true, nil
}

func (service *Service) credentialsFor(ctx context.Context, profile hosts.Profile, provided connections.Credentials, useSaved bool) (connections.Credentials, error) {
	if !useSaved || profile.CredentialID == "" || !provided.Empty() {
		return provided, nil
	}
	return service.vault.Resolve(ctx, profile.CredentialID)
}

// Terminal starts an attempt that ends with a running terminal session.
func (service *Service) Terminal(ctx context.Context, owner string, request Request) (connections.Attempt, error) {
	if err := terminals.ValidateSize(request.Rows, request.Cols); err != nil {
		return connections.Attempt{}, err
	}
	target, profile, persisted, err := service.target(ctx, owner, request)
	if err != nil {
		return connections.Attempt{}, err
	}
	return service.attempts.Start(owner, "terminal", func(ctx context.Context, interaction connections.Interaction) (any, error) {
		target.Interaction = interaction
		session, err := service.terminals.Create(ctx, target, request.Rows, request.Cols)
		if err != nil {
			return nil, err
		}
		info := session.Info()
		if ctx.Err() != nil {
			service.terminals.Remove(owner, info.ID)
			return nil, ctx.Err()
		}
		if persisted {
			service.hosts.MarkUsed(context.WithoutCancel(ctx), profile.ID)
		}
		result := Result{Terminal: &info, release: func() { service.terminals.Remove(owner, info.ID) }}
		if persisted {
			result.Warning = service.remember(context.WithoutCancel(ctx), profile, request)
		}
		return result, nil
	})
}

// remember saves credentials only after they authenticated successfully. A
// failure is reported as a warning because the session itself is fine.
func (service *Service) remember(ctx context.Context, profile hosts.Profile, request Request) string {
	if request.RememberLabel == "" || request.Credentials.Empty() {
		return ""
	}
	identifier, err := service.vault.Remember(ctx, request.RememberLabel, request.Credentials)
	if err != nil {
		return "已连接，但凭据未保存：" + fault.As(err).Message
	}
	profile.CredentialID = identifier
	if _, err := service.hosts.Save(ctx, profile.ID, profile); err != nil {
		return "凭据已加密保存，但未能关联到主机：" + fault.As(err).Message
	}
	return ""
}

// Tunnel binds the loopback port first, so a port conflict is reported
// before any SSH work, then starts an attempt that brings the tunnel up.
func (service *Service) Tunnel(ctx context.Context, owner, tunnelID string, request Request) (connections.Attempt, error) {
	spec, err := service.tunnels.Get(ctx, tunnelID)
	if err != nil {
		return connections.Attempt{}, err
	}
	request.HostID = spec.HostID
	target, profile, persisted, err := service.target(ctx, owner, request)
	if err != nil {
		return connections.Attempt{}, err
	}
	listener, err := service.tunnels.Listen(spec)
	if err != nil {
		return connections.Attempt{}, err
	}
	attempt, err := service.attempts.Start(owner, "tunnel", func(ctx context.Context, interaction connections.Interaction) (any, error) {
		target.Interaction = interaction
		status, err := service.tunnels.Start(ctx, owner, spec, listener, target)
		if err != nil {
			return nil, err
		}
		if persisted {
			service.hosts.MarkUsed(context.WithoutCancel(ctx), profile.ID)
		}
		if ctx.Err() != nil {
			service.tunnels.Stop(spec.ID)
			return nil, ctx.Err()
		}
		view := tunnels.View{Spec: spec, Status: status}
		warning := ""
		if persisted {
			warning = service.remember(context.WithoutCancel(ctx), profile, request)
		}
		return Result{Tunnel: &view, Warning: warning, release: func() { service.tunnels.Stop(spec.ID) }}, nil
	})
	if err != nil {
		listener.Close()
	}
	return attempt, err
}
