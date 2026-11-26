//go:build windows

package localpty

import (
	"context"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/terminals"
)

// Supported reports whether this OS supports built-in local PTY sessions.
func Supported() bool {
	return false
}

// DefaultShell returns the preferred interactive shell on the local host.
func DefaultShell() string {
	return ""
}

// Open returns an unsupported error on Windows until ConPTY support is added.
func Open(context.Context, int, int) (terminals.Shell, error) {
	return nil, fault.New(501, "local_terminal_unsupported", "当前操作系统暂不支持内置本地终端。")
}
