package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sshworkbench/internal/adapters/platform"
)

const maxPortAttempts = 20

// ExistingServerError reports that the target port is already served by an sshw instance.
type ExistingServerError struct {
	Origin string
	URL    string
	Port   int
}

func (err *ExistingServerError) Error() string {
	return fmt.Sprintf("SSH Workbench is already running on %s", err.Origin)
}

func listenWithFallback(ctx context.Context, listenAddr string, remote bool) (net.Listener, error) {
	host, portText, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return nil, err
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		return nil, fmt.Errorf("invalid listen port: %w", err)
	}
	if port == 0 {
		return (&net.ListenConfig{}).Listen(ctx, "tcp", listenAddr)
	}
	var lastErr error
	lastPort := port
	for offset := range maxPortAttempts {
		candidatePort := port + offset
		if candidatePort > 65535 {
			break
		}
		lastPort = candidatePort
		candidateAddr := net.JoinHostPort(host, strconv.Itoa(candidatePort))
		listener, listenErr := (&net.ListenConfig{}).Listen(ctx, "tcp", candidateAddr)
		if listenErr == nil {
			if offset > 0 {
				slog.Info("requested port is busy; switched to next available port", "requested", listenAddr, "listen", listener.Addr().String())
			}
			return listener, nil
		}
		if !platform.IsAddrInUse(listenErr) {
			return nil, listenErr
		}
		if !remote {
			if origin, ok := probeSSHW(ctx, host, candidatePort); ok {
				return nil, &ExistingServerError{Origin: origin, URL: origin, Port: candidatePort}
			}
		}
		lastErr = listenErr
	}
	return nil, fmt.Errorf("ports %d-%d are all in use: %w", port, lastPort, lastErr)
}

func probeHostsFor(host string) []string {
	switch host {
	case "", "0.0.0.0":
		return []string{"127.0.0.1"}
	case "::":
		return []string{"::1", "127.0.0.1"}
	case "localhost":
		return []string{"127.0.0.1", "::1"}
	default:
		return []string{host}
	}
}

func probeSSHW(ctx context.Context, host string, port int) (string, bool) {
	client := &http.Client{
		Timeout: 1200 * time.Millisecond,
		Transport: &http.Transport{
			DialContext:       (&net.Dialer{Timeout: 400 * time.Millisecond}).DialContext,
			DisableKeepAlives: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	portText := strconv.Itoa(port)
	for _, probeHost := range probeHostsFor(host) {
		origin := "http://" + net.JoinHostPort(probeHost, portText)
		if isSSHWEndpoint(ctx, client, origin) {
			return origin, true
		}
	}
	return "", false
}

func isSSHWEndpoint(ctx context.Context, client *http.Client, origin string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, origin+"/api/v1/auth", nil)
	if err != nil {
		return false
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	if response.Header.Get("X-SSHW-Service") == "ssh-workbench" {
		return true
	}
	if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "application/json") {
		return false
	}
	var payload struct {
		Authenticated *bool   `json:"authenticated"`
		AuthRequired  *bool   `json:"authRequired"`
		Remote        *bool   `json:"remote"`
		Version       *string `json:"version"`
		Capabilities  *struct {
			LocalFiles *bool `json:"localFiles"`
			Agent      *bool `json:"agent"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	if payload.Authenticated == nil || payload.AuthRequired == nil || payload.Remote == nil || payload.Version == nil || payload.Capabilities == nil || payload.Capabilities.LocalFiles == nil || payload.Capabilities.Agent == nil {
		return false
	}
	// Legacy builds lack X-SSHW-Service, so verify the HTML title as a second check.
	indexReq, err := http.NewRequestWithContext(probeCtx, http.MethodGet, origin+"/", nil)
	if err != nil {
		return false
	}
	indexResp, err := client.Do(indexReq)
	if err != nil {
		return false
	}
	indexBody, _ := io.ReadAll(io.LimitReader(indexResp.Body, 64*1024))
	indexResp.Body.Close()
	return indexResp.StatusCode == http.StatusOK && bytes.Contains(indexBody, []byte("<title>SSH Workbench</title>"))
}
