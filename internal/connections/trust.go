package connections

import (
	"bytes"
	"context"
	"time"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/hosts"
)

type Credentials struct {
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

func (credentials Credentials) Empty() bool {
	return credentials.Password == "" && credentials.PrivateKey == "" && credentials.Passphrase == ""
}

// Hop is one authenticated SSH endpoint. The final host and an optional single
// jump host each carry their own profile and credentials and are verified
// independently.
type Hop struct {
	Host        hosts.Profile
	Credentials Credentials
}

// Transport is an established, authenticated connection owned by a terminal.
// Other capabilities may reuse it only when re-authenticating would require
// the user again (keyboard-interactive); the owner closes it.
type Transport interface {
	Alive() bool
}

const LocalHostID = "local:shell"

type Target struct {
	Owner       string
	Host        hosts.Profile
	Credentials Credentials
	Jump        *Hop
	// Local indicates a local OS shell session rather than an SSH target.
	Local bool
	// Interaction is set only while a user is present in a connection
	// attempt. Background re-dials leave it nil, so they can never accept a
	// new host key or answer a challenge on the user's behalf.
	Interaction Interaction
	Shared      Transport
}

func (target Target) Hops() []Hop {
	hops := make([]Hop, 0, 2)
	if target.Jump != nil {
		hops = append(hops, *target.Jump)
	}
	return append(hops, Hop{Host: target.Host, Credentials: target.Credentials})
}

// RequiresUser reports whether authenticating again needs a person, so a new
// transport cannot be dialed in the background.
func (target Target) RequiresUser() bool {
	for _, hop := range target.Hops() {
		if hop.Host.AuthMode == hosts.AuthKeyboardInteractive {
			return true
		}
	}
	return false
}

type KnownKey struct {
	Address     string    `json:"address"`
	Algorithm   string    `json:"algorithm"`
	Fingerprint string    `json:"fingerprint"`
	PublicKey   []byte    `json:"-"`
	TrustedAt   time.Time `json:"trustedAt"`
}

type TrustRepository interface {
	GetKnownKey(context.Context, string) (*KnownKey, error)
	AddKnownKey(context.Context, KnownKey) error
}

type Verifier struct {
	repository TrustRepository
}

func NewVerifier(repository TrustRepository) *Verifier {
	return &Verifier{repository: repository}
}

// Check returns true for a previously trusted identical key, false for an
// unknown address, and an error for changed keys or an unreadable store. It
// never records trust; Accept does that after explicit confirmation.
func (verifier *Verifier) Check(ctx context.Context, key KnownKey) (bool, error) {
	known, err := verifier.repository.GetKnownKey(ctx, key.Address)
	if err != nil {
		return false, fault.New(503, "trust_store_unavailable", "无法读取主机信任记录，已停止连接。")
	}
	if known == nil {
		return false, nil
	}
	if bytes.Equal(known.PublicKey, key.PublicKey) {
		return true, nil
	}
	return false, &fault.Error{Status: 409, Code: "host_key_changed", Message: "主机密钥与已保存记录不同，已阻止连接。请先通过可信渠道核对服务器身份。", Details: map[string]string{"address": key.Address, "fingerprint": key.Fingerprint, "previousFingerprint": known.Fingerprint}}
}

func (verifier *Verifier) Accept(ctx context.Context, key KnownKey) error {
	key.TrustedAt = time.Now().UTC()
	if err := verifier.repository.AddKnownKey(ctx, key); err != nil {
		return fault.As(err)
	}
	return nil
}
