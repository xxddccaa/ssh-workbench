package sshtransport

import (
	"context"
	"net"

	"github.com/Microsoft/go-winio"
)

func dialAgent(ctx context.Context) (net.Conn, error) {
	return winio.DialPipeContext(ctx, `\\.\pipe\openssh-ssh-agent`)
}
