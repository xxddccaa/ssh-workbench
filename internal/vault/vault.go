// Package vault encrypts saved SSH credentials with a random data key. The
// data key never touches the database in plaintext: it is either kept in the
// operating system's credential store or wrapped by a key derived from a
// master password with Argon2id. Without one of those the stored credentials
// are unreadable; there is no silent plaintext fallback.
package vault

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/ident"
)

const (
	ModeKeyring  = "keyring"
	ModePassword = "password"

	keyringService = "ssh-workbench"
	headerSetting  = "vault_header"
	checkPlaintext = "ssh-workbench vault check v1"
	keyLength      = chacha20poly1305.KeySize
	minimumLength  = 12
	failureLimit   = 5
	lockoutPeriod  = time.Minute
)

type Settings interface {
	GetSetting(context.Context, string) (string, error)
	PutSetting(context.Context, string, string) error
	DeleteSetting(context.Context, string) error
}

type Keyring interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

type Status struct {
	Initialized bool   `json:"initialized"`
	Mode        string `json:"mode,omitempty"`
	Unlocked    bool   `json:"unlocked"`
}

type header struct {
	Version    int    `json:"version"`
	Mode       string `json:"mode"`
	Account    string `json:"account,omitempty"`
	Salt       []byte `json:"salt,omitempty"`
	Time       uint32 `json:"time,omitempty"`
	Memory     uint32 `json:"memory,omitempty"`
	Threads    uint8  `json:"threads,omitempty"`
	WrappedKey []byte `json:"wrappedKey,omitempty"`
	Check      []byte `json:"check"`
}

type Vault struct {
	mu          sync.Mutex
	settings    Settings
	keyring     Keyring
	key         []byte
	failures    int
	lockedUntil time.Time
}

func New(settings Settings, keyring Keyring) *Vault {
	return &Vault{settings: settings, keyring: keyring}
}

var (
	errLocked      = fault.New(423, "vault_locked", "凭据库已锁定，请先解锁。")
	errUninitiated = fault.New(409, "vault_not_initialized", "尚未启用加密凭据库。")
)

func (vault *Vault) loadHeader(ctx context.Context) (*header, error) {
	value, err := vault.settings.GetSetting(ctx, headerSetting)
	if err != nil {
		return nil, fault.New(503, "vault_unavailable", "无法读取凭据库配置。")
	}
	if value == "" {
		return nil, nil
	}
	var stored header
	err = json.Unmarshal([]byte(value), &stored)
	// Bound the key-derivation cost so a tampered header cannot make
	// unlocking allocate gigabytes or spin for minutes.
	unreasonable := stored.Mode == ModePassword && (stored.Time < 1 || stored.Time > 10 || stored.Memory < 8*1024 || stored.Memory > 1024*1024 || stored.Threads < 1 || stored.Threads > 16 || len(stored.Salt) < 16)
	if err != nil || stored.Version != 1 || (stored.Mode != ModeKeyring && stored.Mode != ModePassword) || unreasonable {
		return nil, fault.New(503, "vault_corrupted", "凭据库配置无法识别，已停止使用保存的凭据。")
	}
	return &stored, nil
}

// Status reports the vault state. A keyring-protected vault unlocks itself
// here when the operating system store is reachable.
func (vault *Vault) Status(ctx context.Context) (Status, error) {
	stored, err := vault.loadHeader(ctx)
	if err != nil || stored == nil {
		return Status{}, err
	}
	vault.mu.Lock()
	defer vault.mu.Unlock()
	if vault.key == nil && stored.Mode == ModeKeyring {
		vault.unlockKeyringLocked(stored)
	}
	return Status{Initialized: true, Mode: stored.Mode, Unlocked: vault.key != nil}, nil
}

func (vault *Vault) Setup(ctx context.Context, mode, password string) error {
	vault.mu.Lock()
	defer vault.mu.Unlock()
	existing, err := vault.loadHeader(ctx)
	if err != nil {
		return err
	}
	if existing != nil {
		return fault.New(409, "vault_exists", "凭据库已经启用。")
	}
	key := make([]byte, keyLength)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	stored := header{Version: 1, Mode: mode}
	switch mode {
	case ModeKeyring:
		stored.Account = "vault-" + ident.New()
		if err := vault.keyring.Set(keyringService, stored.Account, hex.EncodeToString(key)); err != nil {
			return fault.New(503, "keyring_unavailable", "系统凭据存储不可用（例如无图形会话的 Linux）。请改用主密码模式。")
		}
		readBack, err := vault.keyring.Get(keyringService, stored.Account)
		if err != nil || subtle.ConstantTimeCompare([]byte(readBack), []byte(hex.EncodeToString(key))) != 1 {
			vault.keyring.Delete(keyringService, stored.Account)
			return fault.New(503, "keyring_unavailable", "系统凭据存储未能保存密钥。请改用主密码模式。")
		}
	case ModePassword:
		if err := validatePassword(password); err != nil {
			return err
		}
		stored.Salt = make([]byte, 16)
		if _, err := rand.Read(stored.Salt); err != nil {
			return err
		}
		stored.Time, stored.Memory, stored.Threads = 3, 64*1024, 4
		wrapped, err := seal(derive(password, &stored), key, []byte("vault-key"))
		if err != nil {
			return err
		}
		stored.WrappedKey = wrapped
	default:
		return fault.New(400, "invalid_vault_mode", "凭据库保护方式须为系统凭据存储或主密码。")
	}
	stored.Check, err = seal(key, []byte(checkPlaintext), []byte("check"))
	if err != nil {
		return err
	}
	document, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	if err := vault.settings.PutSetting(ctx, headerSetting, string(document)); err != nil {
		if mode == ModeKeyring {
			vault.keyring.Delete(keyringService, stored.Account)
		}
		return fault.New(503, "vault_unavailable", "无法保存凭据库配置。")
	}
	vault.key = key
	return nil
}

func validatePassword(password string) error {
	if utf8.RuneCountInString(password) < minimumLength || len(password) > 1024 || !utf8.ValidString(password) {
		return fault.New(400, "weak_master_password", "主密码至少 12 个字符。它不能被找回，请妥善保存。")
	}
	return nil
}

func derive(password string, stored *header) []byte {
	return argon2.IDKey([]byte(password), stored.Salt, stored.Time, stored.Memory, stored.Threads, keyLength)
}

func (vault *Vault) unlockKeyringLocked(stored *header) bool {
	encoded, err := vault.keyring.Get(keyringService, stored.Account)
	if err != nil {
		return false
	}
	key, err := hex.DecodeString(encoded)
	if err != nil || len(key) != keyLength || !verify(key, stored) {
		return false
	}
	vault.key = key
	return true
}

func verify(key []byte, stored *header) bool {
	plaintext, err := open(key, stored.Check, []byte("check"))
	return err == nil && subtle.ConstantTimeCompare(plaintext, []byte(checkPlaintext)) == 1
}

// Unlock opens a password-protected vault, or re-reads the keyring key.
// Repeated wrong passwords lock unlocking out for a minute.
func (vault *Vault) Unlock(ctx context.Context, password string) error {
	stored, err := vault.loadHeader(ctx)
	if err != nil {
		return err
	}
	if stored == nil {
		return errUninitiated
	}
	vault.mu.Lock()
	defer vault.mu.Unlock()
	if vault.key != nil {
		return nil
	}
	if stored.Mode == ModeKeyring {
		if !vault.unlockKeyringLocked(stored) {
			return fault.New(503, "keyring_unavailable", "无法从系统凭据存储读取凭据库密钥。")
		}
		return nil
	}
	if time.Now().Before(vault.lockedUntil) {
		return fault.New(429, "vault_rate_limited", "解锁失败次数过多，请一分钟后重试。")
	}
	if len(password) > 1024 {
		return fault.New(400, "invalid_master_password", "主密码不正确。")
	}
	key, err := open(derive(password, stored), stored.WrappedKey, []byte("vault-key"))
	if err != nil || !verify(key, stored) {
		vault.failures++
		if vault.failures >= failureLimit {
			vault.failures = 0
			vault.lockedUntil = time.Now().Add(lockoutPeriod)
		}
		return fault.New(401, "invalid_master_password", "主密码不正确。")
	}
	vault.failures = 0
	vault.key = key
	return nil
}

func (vault *Vault) Lock() {
	vault.mu.Lock()
	defer vault.mu.Unlock()
	clear(vault.key)
	vault.key = nil
}

func (vault *Vault) currentKey(ctx context.Context) ([]byte, error) {
	stored, err := vault.loadHeader(ctx)
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, errUninitiated
	}
	vault.mu.Lock()
	defer vault.mu.Unlock()
	if vault.key == nil && stored.Mode == ModeKeyring {
		vault.unlockKeyringLocked(stored)
	}
	if vault.key == nil {
		return nil, errLocked
	}
	return append([]byte(nil), vault.key...), nil
}

func (vault *Vault) Seal(ctx context.Context, plaintext, additional []byte) ([]byte, error) {
	key, err := vault.currentKey(ctx)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	return seal(key, plaintext, additional)
}

func (vault *Vault) Open(ctx context.Context, sealed, additional []byte) ([]byte, error) {
	key, err := vault.currentKey(ctx)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	plaintext, err := open(key, sealed, additional)
	if err != nil {
		return nil, fault.New(422, "credential_unreadable", "保存的凭据无法解密，可能已损坏。")
	}
	return plaintext, nil
}

// Reset forgets the vault key. Callers must delete the sealed records, which
// become permanently unreadable.
func (vault *Vault) Reset(ctx context.Context) error {
	vault.mu.Lock()
	defer vault.mu.Unlock()
	stored, err := vault.loadHeader(ctx)
	var problem *fault.Error
	if err != nil && (!errors.As(err, &problem) || problem.Code != "vault_corrupted") {
		return err
	}
	if stored != nil && stored.Mode == ModeKeyring {
		vault.keyring.Delete(keyringService, stored.Account)
	}
	clear(vault.key)
	vault.key = nil
	vault.failures = 0
	return vault.settings.DeleteSetting(ctx, headerSetting)
}

func seal(key, plaintext, additional []byte) ([]byte, error) {
	cipher, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, cipher.NonceSize(), cipher.NonceSize()+len(plaintext)+cipher.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return cipher.Seal(nonce, nonce, plaintext, additional), nil
}

func open(key, sealed, additional []byte) ([]byte, error) {
	cipher, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < cipher.NonceSize()+cipher.Overhead() {
		return nil, errors.New("sealed value too short")
	}
	return cipher.Open(nil, sealed[:cipher.NonceSize()], sealed[cipher.NonceSize():], additional)
}
