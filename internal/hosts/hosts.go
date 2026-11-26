package hosts

import (
	"context"
	"net"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/ident"
)

const (
	AuthAuto                = "auto"
	AuthPassword            = "password"
	AuthKey                 = "key"
	AuthAgent               = "agent"
	AuthKeyboardInteractive = "keyboard-interactive"
)

type Profile struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Hostname       string     `json:"hostname"`
	Port           int        `json:"port"`
	Username       string     `json:"username"`
	AuthMode       string     `json:"authMode"`
	Group          string     `json:"group"`
	Color          string     `json:"color"`
	Favorite       bool       `json:"favorite"`
	Production     bool       `json:"production"`
	JumpHostID     string     `json:"jumpHostId,omitempty"`
	ProxyCommand   string     `json:"proxyCommand,omitempty"`
	IdentityFile   string     `json:"identityFile,omitempty"`
	IdentityFiles  []string   `json:"-"`
	IdentitiesOnly bool       `json:"-"`
	CredentialID   string     `json:"credentialId,omitempty"`
	LastUsedAt     *time.Time `json:"lastUsedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

func (profile Profile) Address() string {
	return net.JoinHostPort(profile.Hostname, strconv.Itoa(profile.Port))
}

func (profile Profile) Label() string {
	return profile.Username + "@" + profile.Address()
}

type Repository interface {
	ListHosts(context.Context) ([]Profile, error)
	GetHost(context.Context, string) (Profile, error)
	PutHost(context.Context, Profile) error
	DeleteHost(context.Context, string) error
}

// References lets other modules veto deleting a host they depend on, such as
// a saved tunnel.
type References interface {
	HostReferences(context.Context, string) (int, error)
}

type Service struct {
	repository Repository
	references []References
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) AddReferences(references References) {
	service.references = append(service.references, references)
}

func (service *Service) List(ctx context.Context) ([]Profile, error) {
	return service.repository.ListHosts(ctx)
}

func (service *Service) Get(ctx context.Context, hostID string) (Profile, error) {
	return service.repository.GetHost(ctx, hostID)
}

func Normalize(profile Profile) Profile {
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Hostname = strings.Trim(strings.TrimSpace(profile.Hostname), "[]")
	profile.Username = strings.TrimSpace(profile.Username)
	profile.Group = strings.TrimSpace(profile.Group)
	profile.ProxyCommand = strings.TrimSpace(profile.ProxyCommand)
	profile.IdentityFile = strings.TrimSpace(profile.IdentityFile)
	if profile.Port == 0 {
		profile.Port = 22
	}
	if profile.AuthMode == "" {
		profile.AuthMode = AuthAuto
	}
	if profile.Color == "" {
		profile.Color = "teal"
	}
	if profile.AuthMode != AuthKey && profile.AuthMode != AuthAuto {
		profile.IdentityFile = ""
	}
	return profile
}

func (service *Service) Save(ctx context.Context, hostID string, profile Profile) (Profile, error) {
	profile = Normalize(profile)
	if err := Validate(profile); err != nil {
		return Profile{}, err
	}
	now := time.Now().UTC()
	profile.ID = hostID
	profile.CreatedAt = now
	profile.UpdatedAt = now
	profile.LastUsedAt = nil
	if hostID == "" {
		profile.ID = ident.New()
	} else {
		existing, err := service.Get(ctx, hostID)
		if err != nil {
			return Profile{}, err
		}
		profile.CreatedAt = existing.CreatedAt
		profile.LastUsedAt = existing.LastUsedAt
	}
	if err := service.validateJump(ctx, profile); err != nil {
		return Profile{}, err
	}
	if err := service.repository.PutHost(ctx, profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (service *Service) validateJump(ctx context.Context, profile Profile) error {
	if profile.JumpHostID == "" {
		return nil
	}
	if profile.JumpHostID == profile.ID {
		return fault.New(400, "invalid_jump_host", "跳板机不能是主机自身。")
	}
	jump, err := service.Get(ctx, profile.JumpHostID)
	if err != nil {
		return fault.New(400, "invalid_jump_host", "所选跳板机不存在。")
	}
	if jump.JumpHostID != "" {
		return fault.New(400, "nested_jump_host", "当前只支持单级跳板：所选跳板机本身不能再经过其他跳板。")
	}
	dependents, err := service.jumpDependents(ctx, profile.ID)
	if err != nil {
		return err
	}
	if dependents > 0 {
		return fault.New(400, "nested_jump_host", "此主机已被其他主机用作跳板，不能再为它设置跳板。")
	}
	return nil
}

func (service *Service) jumpDependents(ctx context.Context, hostID string) (int, error) {
	if hostID == "" {
		return 0, nil
	}
	profiles, err := service.repository.ListHosts(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, candidate := range profiles {
		if candidate.JumpHostID == hostID {
			count++
		}
	}
	return count, nil
}

// MarkUsed records a successful connection for the "recent" list. Failures
// are not surfaced because they must not break an established session.
func (service *Service) MarkUsed(ctx context.Context, hostID string) {
	profile, err := service.Get(ctx, hostID)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	profile.LastUsedAt = &now
	service.repository.PutHost(ctx, profile)
}

// Unlink removes a deleted credential from every host that referenced it.
func (service *Service) Unlink(ctx context.Context, credentialID string) error {
	profiles, err := service.repository.ListHosts(ctx)
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.CredentialID == credentialID {
			profile.CredentialID = ""
			if err := service.repository.PutHost(ctx, profile); err != nil {
				return err
			}
		}
	}
	return nil
}

func (service *Service) Delete(ctx context.Context, hostID string) error {
	dependents, err := service.jumpDependents(ctx, hostID)
	if err != nil {
		return err
	}
	if dependents > 0 {
		return fault.New(409, "host_in_use", "此主机仍被其他主机用作跳板，请先修改那些主机。")
	}
	for _, references := range service.references {
		count, err := references.HostReferences(ctx, hostID)
		if err != nil {
			return err
		}
		if count > 0 {
			return fault.New(409, "host_in_use", "此主机仍被已保存的隧道使用，请先删除或修改隧道。")
		}
	}
	return service.repository.DeleteHost(ctx, hostID)
}

func Validate(profile Profile) error {
	if !validText(profile.Name, 80) || !validText(profile.Username, 128) || !validText(profile.Hostname, 253) {
		return fault.New(400, "invalid_host", "请填写有效的名称、主机地址和用户名。")
	}
	if strings.ContainsAny(profile.Hostname, " /\\@\t\r\n") || profile.Port < 1 || profile.Port > 65535 {
		return fault.New(400, "invalid_address", "主机地址不能包含协议或路径，端口范围为 1–65535。")
	}
	if strings.Contains(profile.Hostname, ":") && net.ParseIP(profile.Hostname) == nil {
		return fault.New(400, "invalid_address", "IPv6 地址格式不正确，请单独填写端口。")
	}
	if utf8.RuneCountInString(profile.Group) > 80 || !utf8.ValidString(profile.Group) || strings.ContainsAny(profile.Group, "\x00\r\n") {
		return fault.New(400, "invalid_group", "分组名称不正确。")
	}
	switch profile.AuthMode {
	case AuthAuto, AuthPassword, AuthKey, AuthAgent, AuthKeyboardInteractive:
	default:
		return fault.New(400, "invalid_auth_mode", "认证方式须为自动、密码、私钥、SSH agent 或交互式认证。")
	}
	if len(profile.ProxyCommand) > 2048 || !utf8.ValidString(profile.ProxyCommand) || strings.ContainsAny(profile.ProxyCommand, "\x00\r\n") {
		return fault.New(400, "invalid_proxy_command", "ProxyCommand 格式无效。")
	}
	if profile.Color != "teal" && profile.Color != "blue" && profile.Color != "amber" && profile.Color != "rose" {
		return fault.New(400, "invalid_color", "主机标记颜色不正确。")
	}
	if profile.IdentityFile != "" {
		if err := ValidateIdentityFile(profile.IdentityFile); err != nil {
			return err
		}
	}
	if len(profile.JumpHostID) > 64 || len(profile.CredentialID) > 64 {
		return fault.New(400, "invalid_reference", "关联的跳板或凭据无效。")
	}
	return nil
}

// ValidateIdentityFile checks the shape of a configured key path (~ / absolute path).
func ValidateIdentityFile(filename string) error {
	if len(filename) > 1024 || !utf8.ValidString(filename) || strings.ContainsAny(filename, "\x00\r\n") {
		return fault.New(400, "invalid_identity_file", "私钥路径无效。")
	}
	normalized := strings.ReplaceAll(filename, "\\", "/")
	if strings.HasSuffix(normalized, "/") || strings.Contains(normalized, "/../") || normalized == ".." || strings.HasSuffix(normalized, "/..") {
		return fault.New(400, "invalid_identity_file", "私钥路径无效。")
	}
	if !strings.HasPrefix(normalized, "~/") && !path.IsAbs(normalized) && !(len(normalized) >= 3 && normalized[1] == ':' && normalized[2] == '/') {
		return fault.New(400, "invalid_identity_file", "私钥路径须为 ~/... 或绝对路径，例如 ~/.ssh/id_ed25519。")
	}
	return nil
}

func validText(value string, maximum int) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= maximum && !strings.ContainsAny(value, "\x00\r\n")
}
