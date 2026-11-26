// Package imports creates host profiles from a declared subset of OpenSSH
// client configuration. It reads a snapshot, never modifies the original
// file, never executes ProxyCommand/LocalCommand/Match exec and records trust
// only through the normal confirmation flow on first connect.
package imports

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/hosts"
	"sshworkbench/internal/sshconfig"
)

type Source struct {
	Local   bool
	Content []byte
}

type Candidate struct {
	sshconfig.Candidate
	Existing bool `json:"existing"`
}

type Preview struct {
	Candidates  []Candidate            `json:"candidates"`
	Diagnostics []sshconfig.Diagnostic `json:"diagnostics"`
}

type Skip struct {
	Alias  string `json:"alias"`
	Reason string `json:"reason"`
}

type Result struct {
	Created []hosts.Profile `json:"created"`
	Skipped []Skip          `json:"skipped"`
}

type Service struct {
	hosts      *hosts.Service
	allowAgent bool
}

func New(hostService *hosts.Service, allowAgent bool) *Service {
	return &Service{hosts: hostService, allowAgent: allowAgent}
}

func (service *Service) parse(source Source) (sshconfig.Result, string, error) {
	if !source.Local {
		result, err := sshconfig.ParseText(source.Content)
		if err != nil {
			return sshconfig.Result{}, "", fault.New(400, "invalid_ssh_config", "无法解析配置内容。")
		}
		return result, "", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return sshconfig.Result{}, "", fault.New(500, "home_unavailable", "无法确定当前用户的主目录。")
	}
	result, err := sshconfig.ParseLocal(home)
	if err != nil {
		return sshconfig.Result{}, "", fault.New(400, "invalid_ssh_config", "无法解析 ~/.ssh/config。")
	}
	return result, home, nil
}

type SSHConfigHost struct {
	ID             string                 `json:"id"`
	Alias          string                 `json:"alias"`
	Hostname       string                 `json:"hostname"`
	Port           int                    `json:"port"`
	User           string                 `json:"user"`
	IdentityFiles  []string               `json:"identityFiles,omitempty"`
	IdentitiesOnly bool                   `json:"identitiesOnly"`
	UseAgent       bool                   `json:"useAgent"`
	ProxyCommand   string                 `json:"proxyCommand,omitempty"`
	Jump           *sshconfig.Jump        `json:"jump,omitempty"`
	Importable     bool                   `json:"importable"`
	Diagnostics    []sshconfig.Diagnostic `json:"diagnostics"`
}

type SSHConfigHosts struct {
	Hosts       []SSHConfigHost        `json:"hosts"`
	Diagnostics []sshconfig.Diagnostic `json:"diagnostics"`
}

// ListHosts parses the local ~/.ssh/config and returns ready-to-connect host
// entries without requiring an explicit import step.
func (service *Service) ListHosts() (SSHConfigHosts, error) {
	result, _, err := service.parse(Source{Local: true})
	if err != nil {
		return SSHConfigHosts{}, err
	}
	userFallback := localUsername(true)
	list := SSHConfigHosts{
		Hosts:       make([]SSHConfigHost, 0, len(result.Candidates)),
		Diagnostics: result.Diagnostics,
	}
	if list.Diagnostics == nil {
		list.Diagnostics = []sshconfig.Diagnostic{}
	}
	for _, candidate := range result.Candidates {
		if candidate.Diagnostics == nil {
			candidate.Diagnostics = []sshconfig.Diagnostic{}
		}
		username := candidate.User
		if username == "" {
			username = userFallback
		}
		list.Hosts = append(list.Hosts, SSHConfigHost{
			ID:             "sshcfg:" + candidate.Alias,
			Alias:          candidate.Alias,
			Hostname:       candidate.Hostname,
			Port:           candidate.Port,
			User:           username,
			IdentityFiles:  candidate.IdentityFiles,
			IdentitiesOnly: candidate.IdentitiesOnly,
			UseAgent:       candidate.UseAgent,
			ProxyCommand:   candidate.ProxyCommand,
			Jump:           candidate.Jump,
			Importable:     candidate.Importable,
			Diagnostics:    candidate.Diagnostics,
		})
	}
	return list, nil
}

// ResolveHost builds an ephemeral target profile (and optional jump profile)
// directly from ~/.ssh/config for an "sshcfg:<alias>" ID.
func (service *Service) ResolveHost(alias string) (hosts.Profile, *hosts.Profile, error) {
	result, _, err := service.parse(Source{Local: true})
	if err != nil {
		return hosts.Profile{}, nil, err
	}
	byAlias := make(map[string]sshconfig.Candidate, len(result.Candidates))
	for _, candidate := range result.Candidates {
		byAlias[candidate.Alias] = candidate
	}
	candidate, ok := byAlias[alias]
	if !ok {
		return hosts.Profile{}, nil, fault.New(404, "ssh_config_host_not_found", "~/.ssh/config 中未找到主机："+alias)
	}
	userFallback := localUsername(true)
	profile := candidateToProfile("sshcfg:"+candidate.Alias, candidate, userFallback)
	var jumpProfile *hosts.Profile
	if candidate.Jump != nil {
		if candidate.Jump.Alias != "" {
			if jumpCandidate, exists := byAlias[candidate.Jump.Alias]; exists {
				jp := candidateToProfile("sshcfg:"+jumpCandidate.Alias, jumpCandidate, userFallback)
				if candidate.Jump.User != "" {
					jp.Username = candidate.Jump.User
				}
				if candidate.Jump.Port != 0 {
					jp.Port = candidate.Jump.Port
				}
				jumpProfile = &jp
			}
		}
		if jumpProfile == nil {
			jumpUser := candidate.Jump.User
			if jumpUser == "" {
				jumpUser = profile.Username
			}
			jumpPort := candidate.Jump.Port
			if jumpPort == 0 {
				jumpPort = 22
			}
			jp := hosts.Profile{
				ID:       "sshcfg-jump:" + candidate.Jump.Hostname,
				Name:     candidate.Jump.Hostname,
				Hostname: candidate.Jump.Hostname,
				Port:     jumpPort,
				Username: jumpUser,
				AuthMode: hosts.AuthAuto,
				Color:    "amber",
			}
			jumpProfile = &jp
		}
	}
	return profile, jumpProfile, nil
}

func candidateToProfile(id string, candidate sshconfig.Candidate, userFallback string) hosts.Profile {
	username := candidate.User
	if username == "" {
		username = userFallback
	}
	identity := ""
	if len(candidate.IdentityFiles) > 0 {
		identity = candidate.IdentityFiles[0]
	}
	return hosts.Profile{
		ID:             id,
		Name:           candidate.Alias,
		Hostname:       candidate.Hostname,
		Port:           candidate.Port,
		Username:       username,
		AuthMode:       hosts.AuthAuto,
		Group:          "SSH Config",
		Color:          "teal",
		ProxyCommand:   candidate.ProxyCommand,
		IdentityFile:   identity,
		IdentityFiles:  candidate.IdentityFiles,
		IdentitiesOnly: candidate.IdentitiesOnly,
	}
}

func (service *Service) Preview(ctx context.Context, source Source) (Preview, error) {
	result, _, err := service.parse(source)
	if err != nil {
		return Preview{}, err
	}
	existing, err := service.hosts.List(ctx)
	if err != nil {
		return Preview{}, err
	}
	preview := Preview{Candidates: make([]Candidate, 0, len(result.Candidates)), Diagnostics: result.Diagnostics}
	if preview.Diagnostics == nil {
		preview.Diagnostics = []sshconfig.Diagnostic{}
	}
	for _, candidate := range result.Candidates {
		if candidate.Diagnostics == nil {
			candidate.Diagnostics = []sshconfig.Diagnostic{}
		}
		preview.Candidates = append(preview.Candidates, Candidate{Candidate: candidate, Existing: duplicate(existing, candidate.Alias, candidate.Hostname, candidate.Port, candidate.User) != ""})
	}
	return preview, nil
}

func duplicate(profiles []hosts.Profile, name, hostname string, port int, username string) string {
	for _, profile := range profiles {
		if profile.Name == name || (strings.EqualFold(profile.Hostname, hostname) && profile.Port == port && (username == "" || profile.Username == username)) {
			return profile.ID
		}
	}
	return ""
}

// Apply re-parses the source on the server, so the browser can only choose
// aliases, never inject host fields. Jump aliases are imported first.
func (service *Service) Apply(ctx context.Context, source Source, aliases []string, group string) (Result, error) {
	if len(aliases) == 0 || len(aliases) > 500 {
		return Result{}, fault.New(400, "invalid_import_selection", "请选择 1–500 个主机。")
	}
	group = strings.TrimSpace(group)
	if group == "" {
		group = "SSH 配置导入"
	}
	if utf8.RuneCountInString(group) > 80 {
		return Result{}, fault.New(400, "invalid_group", "分组名称不正确。")
	}
	parsed, home, err := service.parse(source)
	if err != nil {
		return Result{}, err
	}
	candidates := make(map[string]sshconfig.Candidate, len(parsed.Candidates))
	for _, candidate := range parsed.Candidates {
		candidates[candidate.Alias] = candidate
	}
	run := &application{service: service, ctx: ctx, home: home, group: group, candidates: candidates, created: map[string]string{}, result: Result{Created: []hosts.Profile{}, Skipped: []Skip{}}}
	run.localUser = localUsername(source.Local)
	for _, alias := range aliases {
		run.importAlias(alias, false)
	}
	return run.result, nil
}

func localUsername(local bool) string {
	if !local {
		return ""
	}
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

type application struct {
	service    *Service
	ctx        context.Context
	home       string
	group      string
	localUser  string
	candidates map[string]sshconfig.Candidate
	created    map[string]string
	result     Result
}

func (run *application) skip(alias, reason string) string {
	run.result.Skipped = append(run.result.Skipped, Skip{Alias: alias, Reason: reason})
	return ""
}

// importAlias returns the profile ID for alias, creating it if needed.
func (run *application) importAlias(alias string, asJump bool) string {
	if identifier, done := run.created[alias]; done {
		return identifier
	}
	run.created[alias] = ""
	candidate, exists := run.candidates[alias]
	if !exists {
		return run.skip(alias, "配置中没有这个主机。")
	}
	if !candidate.Importable {
		return run.skip(alias, "包含不支持的设置，请查看诊断。")
	}
	existing, err := run.service.hosts.List(run.ctx)
	if err != nil {
		return run.skip(alias, "无法读取已有主机。")
	}
	username := candidate.User
	if username == "" {
		username = run.localUser
	}
	if identifier := duplicate(existing, candidate.Alias, candidate.Hostname, candidate.Port, username); identifier != "" {
		if asJump {
			run.created[alias] = identifier
			return identifier
		}
		return run.skip(alias, "已存在同名或同地址的主机。")
	}
	if username == "" {
		return run.skip(alias, "缺少 User，无法确定登录用户名。")
	}
	profile := hosts.Profile{Name: truncate(candidate.Alias, 80), Hostname: candidate.Hostname, Port: candidate.Port, Username: username, Group: run.group, Color: "teal", ProxyCommand: candidate.ProxyCommand}
	profile.AuthMode, profile.IdentityFile = run.authentication(candidate)
	if candidate.Jump != nil {
		if asJump {
			return run.skip(alias, "它本身还需要跳板；当前只支持单级跳板。")
		}
		jumpID := run.jump(candidate)
		if jumpID == "" {
			return run.skip(alias, "跳板机未能导入。")
		}
		profile.JumpHostID = jumpID
	}
	saved, err := run.service.hosts.Save(run.ctx, "", profile)
	if err != nil {
		return run.skip(alias, fault.As(err).Message)
	}
	run.created[alias] = saved.ID
	run.result.Created = append(run.result.Created, saved)
	return saved.ID
}

func (run *application) jump(candidate sshconfig.Candidate) string {
	jump := candidate.Jump
	if jump.Alias != "" {
		return run.importAlias(jump.Alias, true)
	}
	username := jump.User
	if username == "" {
		username = candidate.User
	}
	if username == "" {
		username = run.localUser
	}
	existing, err := run.service.hosts.List(run.ctx)
	if err != nil || username == "" {
		return ""
	}
	if identifier := duplicate(existing, "", jump.Hostname, jump.Port, username); identifier != "" {
		return identifier
	}
	key := "jump:" + username + "@" + jump.Hostname
	if identifier := run.created[key]; identifier != "" {
		return identifier
	}
	profile := hosts.Profile{Name: truncate("跳板 "+jump.Hostname, 80), Hostname: jump.Hostname, Port: jump.Port, Username: username, Group: run.group, Color: "amber", AuthMode: hosts.AuthPassword}
	if run.service.allowAgent {
		profile.AuthMode = hosts.AuthAgent
	}
	saved, err := run.service.hosts.Save(run.ctx, "", profile)
	if err != nil {
		return ""
	}
	run.created[key] = saved.ID
	run.result.Created = append(run.result.Created, saved)
	return saved.ID
}

// authentication picks the closest supported method. In local mode, AuthAuto
// tries configured keys, SSH agent, default keys and password/interactive fallback.
func (run *application) authentication(candidate sshconfig.Candidate) (string, string) {
	if run.home != "" {
		var keyRef string
		for _, filename := range candidate.IdentityFiles {
			relative, err := filepath.Rel(run.home, filename)
			if err == nil && relative != "." && !strings.HasPrefix(relative, "..") && !filepath.IsAbs(relative) {
				candidateRef := "~/" + filepath.ToSlash(relative)
				if hosts.ValidateIdentityFile(candidateRef) == nil {
					keyRef = candidateRef
					break
				}
			}
			if hosts.ValidateIdentityFile(filename) == nil {
				keyRef = filename
				break
			}
		}
		return hosts.AuthAuto, keyRef
	}
	if candidate.UseAgent && run.service.allowAgent {
		return hosts.AuthAgent, ""
	}
	if len(candidate.IdentityFiles) > 0 {
		return hosts.AuthKey, ""
	}
	return hosts.AuthPassword, ""
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
