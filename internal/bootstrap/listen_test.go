package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestListenWithFallbackAdvancesPastNonSSHWPort(test *testing.T) {
	blocker := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.NotFound(response, nil)
	}))
	defer blocker.Close()

	_, portText, _ := net.SplitHostPort(blocker.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)

	listener, err := listenWithFallback(context.Background(), fmt.Sprintf("127.0.0.1:%d", port), false)
	if err != nil {
		test.Fatalf("listenWithFallback failed: %v", err)
	}
	defer listener.Close()

	_, actualPortText, _ := net.SplitHostPort(listener.Addr().String())
	actualPort, _ := strconv.Atoi(actualPortText)
	if actualPort <= port {
		test.Fatalf("expected fallback port > %d, got %d", port, actualPort)
	}
}

func TestListenWithFallbackReusesSSHWHeaderPortWithoutSwitching(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/v1/auth" {
			response.Header().Set("X-SSHW-Service", "ssh-workbench")
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"authenticated":true}`))
			return
		}
		http.NotFound(response, request)
	}))
	defer server.Close()

	_, portText, _ := net.SplitHostPort(server.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)

	_, err := listenWithFallback(context.Background(), fmt.Sprintf("127.0.0.1:%d", port), false)
	var existing *ExistingServerError
	if !errors.As(err, &existing) {
		test.Fatalf("expected ExistingServerError, got %v", err)
	}
	if existing.Port != port || existing.Origin != "http://127.0.0.1:"+portText {
		test.Fatalf("unexpected existing server info: %+v", existing)
	}
}

func TestListenWithFallbackRecognizesLegacySSHWWithoutHeader(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/auth":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"authenticated":true,"authRequired":false,"remote":false,"version":"0.3.1","capabilities":{"localFiles":true,"agent":true}}`))
		case "/":
			response.Header().Set("Content-Type", "text/html")
			_, _ = response.Write([]byte(`<!doctype html><title>SSH Workbench</title>`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	_, portText, _ := net.SplitHostPort(server.Listener.Addr().String())
	port, _ := strconv.Atoi(portText)

	_, err := listenWithFallback(context.Background(), fmt.Sprintf("127.0.0.1:%d", port), false)
	var existing *ExistingServerError
	if !errors.As(err, &existing) {
		test.Fatalf("expected ExistingServerError for legacy sshw, got %v", err)
	}
	if existing.Port != port {
		test.Fatalf("expected port %d, got %d", port, existing.Port)
	}
}

func TestListenWithFallbackSupportsEphemeralPortZero(test *testing.T) {
	listener, err := listenWithFallback(context.Background(), "127.0.0.1:0", false)
	if err != nil {
		test.Fatalf("listen port 0: %v", err)
	}
	defer listener.Close()
	_, portText, _ := net.SplitHostPort(listener.Addr().String())
	if portText == "" || portText == "0" {
		test.Fatalf("expected assigned port, got %q", portText)
	}
}

func TestProbeHostsForMappings(test *testing.T) {
	if hosts := probeHostsFor("0.0.0.0"); len(hosts) != 1 || hosts[0] != "127.0.0.1" {
		test.Fatalf("unexpected probe hosts for 0.0.0.0: %v", hosts)
	}
	if hosts := probeHostsFor("::"); len(hosts) != 2 || hosts[0] != "::1" || hosts[1] != "127.0.0.1" {
		test.Fatalf("unexpected probe hosts for ::: %v", hosts)
	}
}
