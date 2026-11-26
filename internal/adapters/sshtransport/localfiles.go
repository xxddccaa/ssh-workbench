package sshtransport

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/hosts"
)

const identityLimit = 256 * 1024

// sshDirectory resolves the current OS user's ~/.ssh through symlinks, so
// containment checks compare real paths rather than strings.
func sshDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(filepath.Join(home, ".ssh"))
}

func within(directory, candidate string) bool {
	relative, err := filepath.Rel(directory, candidate)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// readIdentityFile reads a configured private key from ~/.ssh or an absolute/~/ path,
// following symlinks (e.g. OrbStack, Colima, or dotfiles repositories).
func readIdentityFile(configured string) ([]byte, error) {
	if err := hosts.ValidateIdentityFile(configured); err != nil {
		return nil, err
	}
	unavailable := fault.New(400, "identity_file_unavailable", "无法读取配置的本机私钥文件，请确认路径存在且当前用户可读。")
	normalized := strings.ReplaceAll(configured, "\\", "/")
	var requested string
	if normalized == "~" || strings.HasPrefix(normalized, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, unavailable
		}
		requested = filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(normalized, "~/")))
	} else {
		requested = filepath.FromSlash(normalized)
	}
	resolved, err := filepath.EvalSymlinks(requested)
	if err != nil {
		return nil, unavailable
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, unavailable
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > identityLimit {
		return nil, unavailable
	}
	material, err := io.ReadAll(io.LimitReader(file, identityLimit+1))
	if err != nil || len(material) > identityLimit {
		return nil, unavailable
	}
	return material, nil
}

// systemKnownHosts compares a presented key with the user's OpenSSH
// known_hosts. The result is only a hint shown next to the fingerprint; trust
// is still recorded exclusively in the application's own store after the
// user confirms.
func systemKnownHosts(profile hosts.Profile, remote net.Addr, key ssh.PublicKey) string {
	directory, err := sshDirectory()
	if err != nil {
		return ""
	}
	filename := filepath.Join(directory, "known_hosts")
	if info, err := os.Stat(filename); err != nil || !info.Mode().IsRegular() || info.Size() > 8*1024*1024 {
		return ""
	}
	callback, err := knownhosts.New(filename)
	if err != nil {
		return ""
	}
	if remote == nil {
		remote = &net.TCPAddr{IP: net.IPv4zero}
	}
	err = callback(profile.Address(), remote, key)
	if err == nil {
		return "match"
	}
	var keyError *knownhosts.KeyError
	if errors.As(err, &keyError) {
		if len(keyError.Want) == 0 {
			return "absent"
		}
		return "mismatch"
	}
	var revoked *knownhosts.RevokedError
	if errors.As(err, &revoked) {
		return "mismatch"
	}
	return ""
}
