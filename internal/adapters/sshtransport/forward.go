package sshtransport

import (
	"context"
	"net"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/tunnels"
)

type forwarder struct {
	transport *lease
}

func (connector *Connector) OpenForwarder(ctx context.Context, target connections.Target) (tunnels.Forwarder, error) {
	transport, err := connector.dial(ctx, target)
	if err != nil {
		return nil, err
	}
	return &forwarder{transport: transport}, nil
}

func (tunnel *forwarder) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	connection, err := tunnel.transport.client.DialContext(ctx, network, address)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fault.New(502, "tunnel_target_refused", "SSH 主机拒绝或无法连接隧道目标。")
	}
	return connection, nil
}

func (tunnel *forwarder) Done() <-chan struct{} { return tunnel.transport.closed }
func (tunnel *forwarder) Close() error          { return tunnel.transport.Close() }
