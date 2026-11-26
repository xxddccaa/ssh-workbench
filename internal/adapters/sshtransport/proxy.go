package sshtransport

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

func expandProxyCommand(command, hostname string, port int, username string) string {
	portText := strconv.Itoa(port)
	var out strings.Builder
	for i := 0; i < len(command); i++ {
		if command[i] != '%' || i+1 >= len(command) {
			out.WriteByte(command[i])
			continue
		}
		i++
		switch command[i] {
		case '%':
			out.WriteByte('%')
		case 'h':
			out.WriteString(shellQuote(hostname))
		case 'p':
			out.WriteString(portText)
		case 'r':
			out.WriteString(shellQuote(username))
		default:
			out.WriteByte('%')
			out.WriteByte(command[i])
		}
	}
	return out.String()
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') &&
			r != '.' && r != '_' && r != '-' && r != ':' && r != '@' {
			return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
		}
	}
	return value
}

func dialProxyCommand(ctx context.Context, command, hostname string, port int, username string) (net.Conn, error) {
	expanded := expandProxyCommand(command, hostname, port, username)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", expanded)
	} else {
		cmd = exec.Command("sh", "-c", expanded)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("proxy command start: %w", err)
	}
	conn := &proxyConn{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		addr:   proxyAddr(hostname + ":" + strconv.Itoa(port)),
	}
	stop := context.AfterFunc(ctx, func() {
		// Only close during dial if the dial context itself is cancelled or times out before handshake finishes.
		// After dial returns, the caller stops this hook and lease owns conn.Close().
		conn.Close()
	})
	conn.stopDialHook = stop
	return conn, nil
}

type proxyAddr string

func (a proxyAddr) Network() string { return "proxycommand" }
func (a proxyAddr) String() string  { return string(a) }

type proxyConn struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       io.ReadCloser
	addr         net.Addr
	stopDialHook func() bool
	closeOnce    sync.Once
}

func (c *proxyConn) detachDialContext() {
	if c.stopDialHook != nil {
		c.stopDialHook()
	}
}

func (c *proxyConn) Read(p []byte) (int, error)  { return c.stdout.Read(p) }
func (c *proxyConn) Write(p []byte) (int, error) { return c.stdin.Write(p) }
func (c *proxyConn) Close() error {
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		_ = c.stdout.Close()
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		_ = c.cmd.Wait()
	})
	return nil
}
func (c *proxyConn) LocalAddr() net.Addr              { return proxyAddr("local") }
func (c *proxyConn) RemoteAddr() net.Addr             { return c.addr }
func (c *proxyConn) SetDeadline(time.Time) error      { return nil }
func (c *proxyConn) SetReadDeadline(time.Time) error  { return nil }
func (c *proxyConn) SetWriteDeadline(time.Time) error { return nil }
