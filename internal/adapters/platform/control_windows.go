package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"path/filepath"
	"strings"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

var ErrAlreadyRunning = errors.New("a server is already running for this data directory")

var errForeignPipe = errors.New("the control pipe is owned by another user; refusing to use it")

func controlPipe(dataDirectory string) string {
	absolute, err := filepath.Abs(dataDirectory)
	if err != nil {
		absolute = dataDirectory
	}
	digest := sha256.Sum256([]byte(strings.ToLower(absolute)))
	return `\\.\pipe\ssh-workbench-` + hex.EncodeToString(digest[:12])
}

// verifyOwner checks that the process serving a pipe runs as the current
// user. Pipe names are predictable, so another local account could create
// the name first; such a pipe is rejected rather than trusted.
func verifyOwner(connection net.Conn) error {
	handle, ok := connection.(interface{ Fd() uintptr })
	if !ok {
		return errForeignPipe
	}
	var processID uint32
	if err := windows.GetNamedPipeServerProcessId(windows.Handle(handle.Fd()), &processID); err != nil {
		return errForeignPipe
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, processID)
	if err != nil {
		return errForeignPipe
	}
	defer windows.CloseHandle(process)
	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return errForeignPipe
	}
	defer token.Close()
	server, err := token.GetTokenUser()
	if err != nil {
		return errForeignPipe
	}
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || !server.User.Sid.Equals(current.User.Sid) {
		return errForeignPipe
	}
	return nil
}

// ListenControl creates a named pipe whose DACL grants access only to the
// current user's SID; other local accounts cannot connect.
func ListenControl(dataDirectory string) (net.Listener, error) {
	name := controlPipe(dataDirectory)
	if connection, err := winio.DialPipe(name, nil); err == nil {
		defer connection.Close()
		if err := verifyOwner(connection); err != nil {
			return nil, err
		}
		return nil, ErrAlreadyRunning
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	descriptor := "D:P(A;;GA;;;" + user.User.Sid.String() + ")"
	return winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: descriptor, MessageMode: false})
}

func DialControl(ctx context.Context, dataDirectory string) (net.Conn, error) {
	connection, err := winio.DialPipeContext(ctx, controlPipe(dataDirectory))
	if err != nil {
		return nil, err
	}
	if err := verifyOwner(connection); err != nil {
		connection.Close()
		return nil, err
	}
	return connection, nil
}
