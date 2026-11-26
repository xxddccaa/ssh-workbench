package vault

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"sshworkbench/internal/fault"
)

type memorySettings map[string]string

func (settings memorySettings) GetSetting(_ context.Context, name string) (string, error) {
	return settings[name], nil
}
func (settings memorySettings) PutSetting(_ context.Context, name, value string) error {
	settings[name] = value
	return nil
}
func (settings memorySettings) DeleteSetting(_ context.Context, name string) error {
	delete(settings, name)
	return nil
}

type memoryKeyring struct {
	secrets map[string]string
	broken  bool
}

func (keyring *memoryKeyring) Get(service, account string) (string, error) {
	if keyring.broken {
		return "", errors.New("unavailable")
	}
	secret, ok := keyring.secrets[service+"/"+account]
	if !ok {
		return "", errors.New("missing")
	}
	return secret, nil
}
func (keyring *memoryKeyring) Set(service, account, secret string) error {
	if keyring.broken {
		return errors.New("unavailable")
	}
	keyring.secrets[service+"/"+account] = secret
	return nil
}
func (keyring *memoryKeyring) Delete(service, account string) error {
	delete(keyring.secrets, service+"/"+account)
	return nil
}

func code(err error) string {
	var problem *fault.Error
	if errors.As(err, &problem) {
		return problem.Code
	}
	return ""
}

func TestPasswordVaultRoundTripLockAndRateLimit(test *testing.T) {
	ctx := context.Background()
	settings := memorySettings{}
	vault := New(settings, &memoryKeyring{secrets: map[string]string{}})
	if code(vault.Setup(ctx, ModePassword, "short")) != "weak_master_password" {
		test.Fatal("weak password accepted")
	}
	if err := vault.Setup(ctx, ModePassword, "correct horse battery"); err != nil {
		test.Fatal(err)
	}
	if code(vault.Setup(ctx, ModePassword, "correct horse battery")) != "vault_exists" {
		test.Fatal("vault initialized twice")
	}
	sealed, err := vault.Seal(ctx, []byte("ssh-secret"), []byte("credential:a"))
	if err != nil {
		test.Fatal(err)
	}
	if bytes.Contains([]byte(settings[headerSetting]), []byte("ssh-secret")) || bytes.Contains(sealed, []byte("ssh-secret")) {
		test.Fatal("plaintext leaked")
	}
	if _, err := vault.Open(ctx, sealed, []byte("credential:b")); err == nil {
		test.Fatal("additional data not bound")
	}
	vault.Lock()
	if _, err := vault.Open(ctx, sealed, []byte("credential:a")); code(err) != "vault_locked" {
		test.Fatalf("locked vault opened: %v", err)
	}
	tampered := memorySettings{headerSetting: strings.Replace(settings[headerSetting], `"memory":65536`, `"memory":4194304`, 1)}
	if _, err := New(tampered, &memoryKeyring{}).Status(ctx); code(err) != "vault_corrupted" {
		test.Fatalf("oversized KDF parameters accepted: %v", err)
	}
	reopened := New(settings, &memoryKeyring{secrets: map[string]string{}})
	for attempt := 0; attempt < failureLimit; attempt++ {
		if code(reopened.Unlock(ctx, "wrong password!!")) != "invalid_master_password" {
			test.Fatal("wrong password accepted")
		}
	}
	if code(reopened.Unlock(ctx, "correct horse battery")) != "vault_rate_limited" {
		test.Fatal("unlock not rate limited")
	}
	reopened.lockedUntil = reopened.lockedUntil.AddDate(-1, 0, 0)
	if err := reopened.Unlock(ctx, "correct horse battery"); err != nil {
		test.Fatal(err)
	}
	plaintext, err := reopened.Open(ctx, sealed, []byte("credential:a"))
	if err != nil || string(plaintext) != "ssh-secret" {
		test.Fatalf("round trip failed: %q %v", plaintext, err)
	}
	if err := reopened.Reset(ctx); err != nil {
		test.Fatal(err)
	}
	if status, _ := reopened.Status(ctx); status.Initialized {
		test.Fatal("reset left vault initialized")
	}
}

func TestKeyringVaultUnlocksFromSystemStoreAndFailsClosed(test *testing.T) {
	ctx := context.Background()
	settings := memorySettings{}
	keyring := &memoryKeyring{secrets: map[string]string{}}
	if err := New(settings, &memoryKeyring{broken: true}).Setup(ctx, ModeKeyring, ""); code(err) != "keyring_unavailable" {
		test.Fatalf("unavailable keyring not reported: %v", err)
	}
	vault := New(settings, keyring)
	if err := vault.Setup(ctx, ModeKeyring, ""); err != nil {
		test.Fatal(err)
	}
	sealed, err := vault.Seal(ctx, []byte("key material"), []byte("x"))
	if err != nil {
		test.Fatal(err)
	}
	restarted := New(settings, keyring)
	status, err := restarted.Status(ctx)
	if err != nil || !status.Unlocked || status.Mode != ModeKeyring {
		test.Fatalf("keyring vault did not unlock: %+v %v", status, err)
	}
	if plaintext, err := restarted.Open(ctx, sealed, []byte("x")); err != nil || string(plaintext) != "key material" {
		test.Fatal("keyring round trip failed")
	}
	keyring.broken = true
	unavailable := New(settings, keyring)
	if _, err := unavailable.Open(ctx, sealed, []byte("x")); code(err) != "vault_locked" {
		test.Fatalf("missing keyring did not fail closed: %v", err)
	}
	keyring.broken = false
	for name := range keyring.secrets {
		keyring.secrets[name] = "00"
	}
	if _, err := New(settings, keyring).Open(ctx, sealed, []byte("x")); code(err) != "vault_locked" {
		test.Fatal("wrong keyring key accepted")
	}
	settings[headerSetting] = "{broken"
	if _, err := New(settings, keyring).Status(ctx); code(err) != "vault_corrupted" {
		test.Fatal("corrupted header not reported")
	}
}
