// Package credentials keeps optional, explicitly saved SSH secrets sealed by
// the vault. Browsers only ever see labels and identifiers.
package credentials

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/ident"
)

type Record struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
}

type Sealed struct {
	Record
	Secret []byte
}

type Repository interface {
	ListCredentials(context.Context) ([]Sealed, error)
	GetCredential(context.Context, string) (Sealed, error)
	PutCredential(context.Context, Sealed) error
	DeleteCredential(context.Context, string) error
	DeleteAllCredentials(context.Context) error
}

type Vault interface {
	Seal(context.Context, []byte, []byte) ([]byte, error)
	Open(context.Context, []byte, []byte) ([]byte, error)
	Reset(context.Context) error
}

type Unlinker interface {
	Unlink(context.Context, string) error
}

type Service struct {
	vault      Vault
	repository Repository
	hosts      Unlinker
}

func New(vault Vault, repository Repository, hosts Unlinker) *Service {
	return &Service{vault: vault, repository: repository, hosts: hosts}
}

func additional(identifier string) []byte {
	return []byte("sshwb-credential:" + identifier)
}

func (service *Service) List(ctx context.Context) ([]Record, error) {
	stored, err := service.repository.ListCredentials(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(stored))
	for _, item := range stored {
		records = append(records, item.Record)
	}
	return records, nil
}

func (service *Service) Create(ctx context.Context, label string, secret connections.Credentials) (Record, error) {
	label = strings.TrimSpace(label)
	if label == "" || utf8.RuneCountInString(label) > 80 || !utf8.ValidString(label) || strings.ContainsAny(label, "\x00\r\n") {
		return Record{}, fault.New(400, "invalid_credential_label", "请填写 1–80 个字符的凭据名称。")
	}
	if secret.Password == "" && secret.PrivateKey == "" {
		return Record{}, fault.New(400, "empty_credential", "请填写密码或选择私钥。")
	}
	if len(secret.Password) > 4096 || len(secret.Passphrase) > 4096 || len(secret.PrivateKey) > 256*1024 {
		return Record{}, fault.New(400, "credentials_too_large", "凭据超过大小限制。")
	}
	if secret.PrivateKey != "" && !strings.Contains(secret.PrivateKey, "PRIVATE KEY") {
		return Record{}, fault.New(400, "invalid_private_key", "所选文件不是受支持的私钥。")
	}
	stored, err := service.repository.ListCredentials(ctx)
	if err != nil {
		return Record{}, err
	}
	if len(stored) >= 200 {
		return Record{}, fault.New(409, "credential_limit", "最多保存 200 条凭据。")
	}
	record := Record{ID: ident.New(), Label: label, Kind: "password", CreatedAt: time.Now().UTC()}
	if secret.PrivateKey != "" {
		record.Kind = "key"
	}
	plaintext, err := json.Marshal(secret)
	if err != nil {
		return Record{}, err
	}
	defer clear(plaintext)
	sealed, err := service.vault.Seal(ctx, plaintext, additional(record.ID))
	if err != nil {
		return Record{}, err
	}
	if err := service.repository.PutCredential(ctx, Sealed{Record: record, Secret: sealed}); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Remember saves credentials that just authenticated successfully.
func (service *Service) Remember(ctx context.Context, label string, secret connections.Credentials) (string, error) {
	record, err := service.Create(ctx, label, secret)
	return record.ID, err
}

func (service *Service) Resolve(ctx context.Context, identifier string) (connections.Credentials, error) {
	stored, err := service.repository.GetCredential(ctx, identifier)
	if err != nil {
		return connections.Credentials{}, err
	}
	plaintext, err := service.vault.Open(ctx, stored.Secret, additional(identifier))
	if err != nil {
		return connections.Credentials{}, err
	}
	defer clear(plaintext)
	var secret connections.Credentials
	if err := json.Unmarshal(plaintext, &secret); err != nil {
		return connections.Credentials{}, fault.New(422, "credential_unreadable", "保存的凭据无法解密，可能已损坏。")
	}
	return secret, nil
}

func (service *Service) Delete(ctx context.Context, identifier string) error {
	if err := service.repository.DeleteCredential(ctx, identifier); err != nil {
		return err
	}
	return service.hosts.Unlink(ctx, identifier)
}

// Reset destroys the vault key and every sealed credential. It is the only
// way forward when a master password is lost.
func (service *Service) Reset(ctx context.Context) error {
	stored, err := service.repository.ListCredentials(ctx)
	if err != nil {
		return err
	}
	if err := service.vault.Reset(ctx); err != nil {
		return err
	}
	if err := service.repository.DeleteAllCredentials(ctx); err != nil {
		return err
	}
	for _, item := range stored {
		if err := service.hosts.Unlink(ctx, item.ID); err != nil {
			return err
		}
	}
	return nil
}
