package sshserver

import (
	"net"
	"sync"
)

// Echo is a loopback-only TCP service that sends every byte back on the
// connection it arrived on. It is the forwarding target for tunnel tests.
type Echo struct {
	listener    net.Listener
	mu          sync.Mutex
	connections map[net.Conn]bool
	closed      bool
	active      sync.WaitGroup
}

func StartEcho(address string) (*Echo, error) {
	address, err := loopbackAddress(address)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	echo := &Echo{listener: listener, connections: make(map[net.Conn]bool)}
	echo.active.Add(1)
	go echo.serve()
	return echo, nil
}

func (echo *Echo) Address() string { return echo.listener.Addr().String() }

func (echo *Echo) serve() {
	defer echo.active.Done()
	for {
		connection, err := echo.listener.Accept()
		if err != nil {
			return
		}
		echo.mu.Lock()
		if echo.closed {
			echo.mu.Unlock()
			connection.Close()
			return
		}
		echo.connections[connection] = true
		echo.active.Add(1)
		echo.mu.Unlock()
		go echo.reflect(connection)
	}
}

// reflect echoes until the peer half-closes or fails, then closes the
// connection so the peer observes EOF after the last echoed byte.
func (echo *Echo) reflect(connection net.Conn) {
	defer echo.active.Done()
	defer func() {
		connection.Close()
		echo.mu.Lock()
		delete(echo.connections, connection)
		echo.mu.Unlock()
	}()
	buffer := make([]byte, 32*1024)
	for {
		count, err := connection.Read(buffer)
		if count > 0 {
			if _, writeError := connection.Write(buffer[:count]); writeError != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// Close stops accepting, closes every open connection and waits for the
// per-connection goroutines to finish.
func (echo *Echo) Close() {
	echo.mu.Lock()
	echo.closed = true
	echo.listener.Close()
	for connection := range echo.connections {
		connection.Close()
	}
	echo.mu.Unlock()
	echo.active.Wait()
}
