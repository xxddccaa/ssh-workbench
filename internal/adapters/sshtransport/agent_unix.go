//go:build !windows

package sshtransport

import (
	"context"
	"errors"
	"net"
	"os"
)

func dialAgent(ctx context.Context) (net.Conn, error) {
	address := os.Getenv("SSH_AUTH_SOCK")
	if address == "" {
		return nil, errors.New("SSH_AUTH_SOCK is not set")
	}
	return (&net.Dialer{}).DialContext(ctx, "unix", address)
}
