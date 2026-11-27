package httpapi_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"

	"sshworkbench/internal/adapters/localpty"
	"sshworkbench/internal/adapters/sqlite"
	"sshworkbench/internal/auth"
	"sshworkbench/internal/bootstrap"
	"sshworkbench/internal/connections"
	"sshworkbench/internal/files"
	"sshworkbench/internal/hosts"
	"sshworkbench/internal/terminals"
	"sshworkbench/internal/transport/httpapi"
	"sshworkbench/tests/fixtures/sshserver"
)

type harness struct {
	test      *testing.T
	server    *httptest.Server
	client    *http.Client
	auth      *auth.Service
	terminals *terminals.Service
	services  httpapi.Services
	csrf      string
	owner     string
}

type memoryKeyring struct {
	mu      sync.Mutex
	secrets map[string]string
}

func (keyring *memoryKeyring) Get(service, account string) (string, error) {
	keyring.mu.Lock()
	defer keyring.mu.Unlock()
	secret, ok := keyring.secrets[service+"/"+account]
	if !ok {
		return "", errors.New("not found")
	}
	return secret, nil
}
func (keyring *memoryKeyring) Set(service, account, secret string) error {
	keyring.mu.Lock()
	defer keyring.mu.Unlock()
	keyring.secrets[service+"/"+account] = secret
	return nil
}
func (keyring *memoryKeyring) Delete(service, account string) error {
	keyring.mu.Lock()
	defer keyring.mu.Unlock()
	delete(keyring.secrets, service+"/"+account)
	return nil
}

func newHarnessWithMode(test *testing.T, requireAuth bool, strictHostKey bool) *harness {
	test.Helper()
	// Never read the developer's real ~/.ssh (known_hosts, config, keys).
	home := test.TempDir()
	test.Setenv("HOME", home)
	test.Setenv("USERPROFILE", home)
	store, err := sqlite.Open(filepath.Join(test.TempDir(), "private"))
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() { store.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	services, closeServices := bootstrap.Compose(ctx, store, bootstrap.Options{
		RequireAuth:          requireAuth,
		StrictHostKeyConfirm: strictHostKey,
		Keyring:              &memoryKeyring{secrets: map[string]string{}},
	})
	test.Cleanup(cancel)
	test.Cleanup(closeServices)
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	handler, err := httpapi.New(httpapi.Config{Origin: origin, RequireAuth: requireAuth, Version: "test", LocalFiles: true, Agent: true, LocalTerminal: localpty.Supported()}, services, fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Test</title>")}})
	if err != nil {
		test.Fatal(err)
	}
	server.Config.Handler = handler
	server.Start()
	test.Cleanup(server.Close)
	client := server.Client()
	client.Jar, _ = cookiejar.New(nil)
	return &harness{test: test, server: server, client: client, auth: services.Auth, terminals: services.Terminals, services: services}
}

func newHarness(test *testing.T) *harness {
	test.Helper()
	return newHarnessWithMode(test, true, true)
}

func (fixture *harness) call(method, route string, value any, expected int) ([]byte, http.Header) {
	fixture.test.Helper()
	var data []byte
	contentType := "application/json"
	if raw, ok := value.([]byte); ok {
		data = raw
		contentType = "application/octet-stream"
	} else if value != nil {
		data, _ = json.Marshal(value)
	}
	request, err := http.NewRequest(method, fixture.server.URL+route, bytes.NewReader(data))
	if err != nil {
		fixture.test.Fatal(err)
	}
	request.Header.Set("Origin", fixture.server.URL)
	request.Header.Set("Content-Type", contentType)
	if fixture.csrf != "" {
		request.Header.Set("X-CSRF-Token", fixture.csrf)
	}
	response, err := fixture.client.Do(request)
	if err != nil {
		fixture.test.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		fixture.test.Fatal(err)
	}
	if response.StatusCode != expected {
		fixture.test.Fatalf("%s %s: wanted %d, got %d: %s", method, route, expected, response.StatusCode, body)
	}
	return body, response.Header
}

func (fixture *harness) authorize() {
	body, headers := fixture.call("POST", "/api/v1/auth/bootstrap", map[string]string{"token": fixture.auth.BootstrapToken()}, 200)
	var state struct {
		CSRF string `json:"csrfToken"`
	}
	if err := json.Unmarshal(body, &state); err != nil {
		fixture.test.Fatal(err)
	}
	fixture.csrf = state.CSRF
	response := &http.Response{Header: headers}
	for _, cookie := range response.Cookies() {
		if cookie.Name == fixture.auth.CookieName() {
			session, valid := fixture.auth.Authenticate(cookie.Value)
			if valid {
				fixture.owner = session.ID
			}
		}
	}
	if !strings.Contains(headers.Get("Set-Cookie"), "HttpOnly") || !strings.Contains(headers.Get("Set-Cookie"), "SameSite=Strict") {
		fixture.test.Fatal("unsafe session cookie")
	}
}

func TestHTTPBoundaryAndStaticIndex(test *testing.T) {
	fixture := newHarness(test)
	body, headers := fixture.call("GET", "/", nil, 200)
	if !bytes.Contains(body, []byte("<title>Test")) || headers.Get("Content-Security-Policy") == "" {
		test.Fatal("static assets not served safely")
	}
	fixture.call("GET", "/api/v1/hosts", nil, 401)
	fixture.authorize()
	fixture.call("GET", "/api/v1/hosts", nil, 200)
	for _, alteration := range []string{"origin", "missing-origin", "host", "csrf"} {
		request, _ := http.NewRequest("POST", fixture.server.URL+"/api/v1/hosts", strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", fixture.server.URL)
		request.Header.Set("X-CSRF-Token", fixture.csrf)
		switch alteration {
		case "origin":
			request.Header.Set("Origin", "https://evil.example")
		case "missing-origin":
			request.Header.Del("Origin")
		case "host":
			request.Host = "evil.example"
		case "csrf":
			request.Header.Del("X-CSRF-Token")
		}
		response, err := fixture.client.Do(request)
		if err != nil {
			test.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 403 {
			test.Fatalf("%s boundary failed: %d", alteration, response.StatusCode)
		}
	}
	second, err := fixture.auth.IssueBootstrap()
	if err != nil {
		test.Fatal(err)
	}
	_, joined := fixture.call("POST", "/api/v1/auth/bootstrap", map[string]string{"token": second}, 200)
	if joined.Get("Set-Cookie") != "" {
		test.Fatal("a second authorized window replaced the existing session cookie")
	}
	fixture.call("POST", "/api/v1/hosts", hostRequest("同一会话", "127.0.0.1:22", "u", "password", nil), 201)
	fixture.call("POST", "/api/v1/auth/bootstrap", map[string]string{"token": second}, 401)
	fixture.call("POST", "/api/v1/auth/logout", nil, 204)
	fixture.call("GET", "/api/v1/hosts", nil, 401)
}

func TestSSHUnicodeSFTPConflictAndLogout(test *testing.T) {
	fixture := newHarness(test)
	remote, err := sshserver.Start("")
	if err != nil {
		test.Fatal(err)
	}
	defer remote.Close()
	fixture.authorize()
	_, portValue, _ := net.SplitHostPort(remote.Address())
	port, _ := strconv.Atoi(portValue)
	profileBytes, _ := fixture.call("POST", "/api/v1/hosts", map[string]any{"name": "中文主机", "hostname": "127.0.0.1", "port": port, "username": sshserver.Username, "authMode": "password", "color": "teal", "group": "", "favorite": false}, 201)
	var profile hosts.Profile
	json.Unmarshal(profileBytes, &profile)
	request := map[string]any{"purpose": "terminal", "hostId": profile.ID, "rows": 28, "cols": 100, "credentials": connections.Credentials{Password: sshserver.Password}}
	terminal := fixture.open(request, "target")
	again := fixture.open(request)
	fixture.call("DELETE", "/api/v1/terminals/"+again.ID, nil, 204)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var cursor uint64
	socket := fixture.connectSocket(ctx, terminal.ID, cursor)
	readUntil(test, ctx, socket, &cursor, "/workspace $")
	if err := socket.Write(ctx, websocket.MessageBinary, append([]byte{2}, []byte("echo 中文传输😀\r")...)); err != nil {
		test.Fatal(err)
	}
	readUntil(test, ctx, socket, &cursor, "\r\n中文传输😀\r\n")
	socket.CloseNow()
	socket = fixture.connectSocket(ctx, terminal.ID, cursor)
	defer socket.CloseNow()
	if err := socket.Write(ctx, websocket.MessageBinary, append([]byte{2}, []byte("emit\r")...)); err != nil {
		test.Fatal(err)
	}
	readUntil(test, ctx, socket, &cursor, "分包中文😀：终端字节流测试")
	base := "/api/v1/terminals/" + terminal.ID
	listing, _ := fixture.call("GET", base+"/files?path=.", nil, 200)
	if !bytes.Contains(listing, []byte("欢迎.txt")) || !bytes.Contains(listing, []byte("/workspace")) {
		test.Fatalf("bad SFTP listing: %s", listing)
	}
	filename := "/workspace/上传 中文.bin"
	content := []byte("你好😀\x00binary\xff\n")
	uploadRoute := base + "/upload?" + url.Values{"path": {"/workspace"}, "name": {"上传 中文.bin"}}.Encode()
	fixture.call("POST", uploadRoute, content, 201)
	fixture.call("POST", uploadRoute, []byte("must not overwrite"), 409)
	downloaded, downloadHeaders := fixture.call("GET", base+"/download?path="+url.QueryEscape(filename), nil, 200)
	if !bytes.Equal(downloaded, content) || !strings.Contains(downloadHeaders.Get("Content-Disposition"), "filename*=utf-8''") {
		test.Fatalf("download changed bytes or filename: %q %v", downloaded, downloadHeaders)
	}
	fixture.call("POST", base+"/upload?path=/workspace&name=../escape", []byte("no"), 400)
	documentRoute := base + "/document?path=" + url.QueryEscape("/workspace/欢迎.txt")
	original, _ := fixture.call("GET", documentRoute, nil, 200)
	var document files.Document
	json.Unmarshal(original, &document)
	if !document.BOM || document.LineEnding != "crlf" {
		test.Fatalf("lost text format: %+v", document)
	}
	document.Content = "修改后的中文\n第二行\n"
	savedBytes, _ := fixture.call("PUT", base+"/document", document, 200)
	var saved files.Document
	json.Unmarshal(savedBytes, &saved)
	if saved.Content != "修改后的中文\r\n第二行\r\n" || !saved.BOM {
		test.Fatalf("save altered format: %+v", saved)
	}
	if err := remote.WriteFile(document.Path, "外部修改\n"); err != nil {
		test.Fatal(err)
	}
	conflict, _ := fixture.call("PUT", base+"/document", saved, 409)
	if !bytes.Contains(conflict, []byte("document_conflict")) {
		test.Fatalf("conflict not detected: %s", conflict)
	}
	if leftovers, _ := fixture.call("GET", base+"/files?path=/workspace", nil, 200); bytes.Contains(leftovers, []byte(".sshwb-")) {
		test.Fatalf("save left a staging directory behind: %s", leftovers)
	}
	fixture.call("POST", "/api/v1/auth/logout", nil, 204)
	if len(fixture.terminals.List(fixture.owner)) != 0 {
		test.Fatal("logout left terminal resources active")
	}
	closeContext, cancelClose := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelClose()
	for {
		if _, _, err := socket.Read(closeContext); err != nil {
			if closeContext.Err() != nil {
				test.Fatal("logout did not close the WebSocket")
			}
			break
		}
	}
	fixture.call("GET", base+"/files?path=/workspace", nil, 401)
}

func (fixture *harness) connectSocket(ctx context.Context, terminalID string, offset uint64) *websocket.Conn {
	fixture.test.Helper()
	address := "ws" + strings.TrimPrefix(fixture.server.URL, "http") + "/ws/terminals/" + terminalID + "?offset=" + strconv.FormatUint(offset, 10)
	var lastError error
	for attempt := 0; attempt < 25; attempt++ {
		connection, response, err := websocket.Dial(ctx, address, &websocket.DialOptions{HTTPClient: fixture.client, HTTPHeader: http.Header{"Origin": {fixture.server.URL}}})
		if err == nil {
			return connection
		}
		if response != nil {
			response.Body.Close()
		}
		lastError = err
		time.Sleep(10 * time.Millisecond)
	}
	fixture.test.Fatalf("could not attach: %v", lastError)
	return nil
}

func TestDirectOpenAndSSHConfigAutoConnect(test *testing.T) {
	fixture := newHarnessWithMode(test, false, false)
	remote, err := sshserver.Start("")
	if err != nil {
		test.Fatal(err)
	}
	defer remote.Close()
	// Direct-open mode requires no bootstrap token, works with arbitrary Host/Origin, and authenticates /auth directly.
	authBody, _ := fixture.call("GET", "/api/v1/auth", nil, 200)
	if !bytes.Contains(authBody, []byte(`"authenticated":true`)) || !bytes.Contains(authBody, []byte(`"authRequired":false`)) {
		test.Fatalf("unexpected direct-open auth state: %s", authBody)
	}
	request, _ := http.NewRequest("GET", fixture.server.URL+"/api/v1/hosts", nil)
	request.Host = "localhost:8622"
	request.Header.Set("Origin", "http://localhost:8622")
	resp, err := fixture.client.Do(request)
	if err != nil {
		test.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		test.Fatalf("direct-open localhost Host rejected: %d", resp.StatusCode)
	}

	// Write ~/.ssh/config + default ~/.ssh/id_ed25519 inside the isolated test HOME.
	home := os.Getenv("HOME")
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		test.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), remote.ClientKeyPEM(), 0600); err != nil {
		test.Fatal(err)
	}
	_, portValue, _ := net.SplitHostPort(remote.Address())
	port, _ := strconv.Atoi(portValue)
	cfg := fmt.Sprintf("Host my-mac-box\n  HostName 127.0.0.1\n  Port %d\n  User %s\n", port, sshserver.KeyUsername)
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(cfg), 0600); err != nil {
		test.Fatal(err)
	}

	listed, _ := fixture.call("GET", "/api/v1/ssh-config/hosts", nil, 200)
	if !bytes.Contains(listed, []byte(`"id":"sshcfg:my-mac-box"`)) {
		test.Fatalf("expected sshcfg:my-mac-box in list: %s", listed)
	}

	// Connecting via sshcfg:my-mac-box auto-selects default ~/.ssh/id_ed25519 and auto-trusts TOFU host key with zero prompts.
	terminal := fixture.open(map[string]any{"purpose": "terminal", "hostId": "sshcfg:my-mac-box", "rows": 24, "cols": 80, "credentials": map[string]string{}})
	fixture.call("DELETE", "/api/v1/terminals/"+terminal.ID, nil, 204)

	// Quick connect without saving a host profile also works directly.
	quickTerm := fixture.open(map[string]any{"purpose": "terminal", "quickConnect": map[string]any{"hostname": "127.0.0.1", "port": port, "username": sshserver.Username, "authMode": "auto"}, "rows": 24, "cols": 80, "credentials": map[string]string{"password": sshserver.Password}})
	fixture.call("DELETE", "/api/v1/terminals/"+quickTerm.ID, nil, 204)

	if localpty.Supported() {
		localTerm := fixture.open(map[string]any{"purpose": "terminal", "hostId": connections.LocalHostID, "rows": 24, "cols": 80, "credentials": map[string]string{}})
		fixture.call("GET", "/api/v1/terminals/"+localTerm.ID+"/files?path=.", nil, 400)
		wsCtx, wsCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer wsCancel()
		var cursor uint64
		ws := fixture.connectSocket(wsCtx, localTerm.ID, cursor)
		if err := ws.Write(wsCtx, websocket.MessageBinary, append([]byte{2}, []byte("echo local-ws-ok\r")...)); err != nil {
			test.Fatal(err)
		}
		readUntil(test, wsCtx, ws, &cursor, "local-ws-ok")
		ws.CloseNow()
		fixture.call("DELETE", "/api/v1/terminals/"+localTerm.ID, nil, 204)
	}
}

func readUntil(test *testing.T, ctx context.Context, socket *websocket.Conn, cursor *uint64, needle string) {
	test.Helper()
	var content []byte
	for !bytes.Contains(content, []byte(needle)) {
		messageType, data, err := socket.Read(ctx)
		if err != nil {
			test.Fatalf("waiting for %q: %v, received %q", needle, err, content)
		}
		if messageType != websocket.MessageBinary {
			continue
		}
		if len(data) < 10 || data[0] != 1 {
			test.Fatal("invalid output frame")
		}
		if offset := binary.BigEndian.Uint64(data[1:9]); offset != *cursor {
			test.Fatalf("output gap: %d != %d", offset, *cursor)
		}
		content = append(content, data[9:]...)
		*cursor += uint64(len(data) - 9)
		if err := socket.Write(ctx, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"ack","offset":"%d"}`, *cursor))); err != nil {
			test.Fatal(err)
		}
	}
}
