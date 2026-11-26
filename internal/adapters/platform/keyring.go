package platform

import "github.com/zalando/go-keyring"

// SystemKeyring stores secrets in the macOS Keychain, Windows Credential
// Manager or a Secret Service provider on Linux. On macOS the secret is passed
// to /usr/bin/security over stdin, not as a process argument.
type SystemKeyring struct{}

func (SystemKeyring) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (SystemKeyring) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}

func (SystemKeyring) Delete(service, account string) error {
	return keyring.Delete(service, account)
}
