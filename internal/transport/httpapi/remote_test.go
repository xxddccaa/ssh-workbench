package httpapi_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"sshworkbench/internal/adapters/sqlite"
	"sshworkbench/internal/auth"
	"sshworkbench/internal/bootstrap"
	"sshworkbench/internal/transport/httpapi"
)

func TestRemoteTLSRequiresLoginAndDisablesLocalCapabilities(test *testing.T) {
	home := test.TempDir()
	test.Setenv("HOME", home)
	test.Setenv("USERPROFILE", home)
	store, err := sqlite.Open(filepath.Join(test.TempDir(), "private"))
	if err != nil {
		test.Fatal(err)
	}
	defer store.Close()
	password := "owner-test-password"
	hash, err := auth.HashPassword(password)
	if err != nil {
		test.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	services, closeServices := bootstrap.Compose(ctx, store, bootstrap.Options{Remote: true, PasswordHash: hash, Keyring: &memoryKeyring{secrets: map[string]string{}}})
	defer closeServices()
	server := httptest.NewUnstartedServer(nil)
	origin := "https://" + server.Listener.Addr().String()
	handler, err := httpapi.New(httpapi.Config{Origin: origin, Remote: true, Version: "test"}, services, fstest.MapFS{})
	if err != nil {
		test.Fatal(err)
	}
	server.Config.Handler = handler
	server.StartTLS()
	defer server.Close()
	client := server.Client()
	client.Jar, _ = cookiejar.New(nil)
	fixture := &harness{test: test, server: server, client: client, auth: services.Auth, terminals: services.Terminals, services: services}
	fixture.call("GET", "/api/v1/hosts", nil, 401)
	fixture.call("POST", "/api/v1/auth/bootstrap", map[string]string{"token": "wrong"}, 401)
	fixture.call("POST", "/api/v1/auth/login", map[string]string{"password": "wrong"}, 401)
	body, headers := fixture.call("POST", "/api/v1/auth/login", map[string]string{"password": password}, 200)
	cookie := headers.Get("Set-Cookie")
	for _, required := range []string{"__Host-sshwb_session=", "Secure", "HttpOnly", "SameSite=Strict", "Path=/"} {
		if !strings.Contains(cookie, required) {
			test.Errorf("cookie lacks %s: %s", required, cookie)
		}
	}
	if !bytes.Contains(body, []byte(`"localFiles":false`)) || !bytes.Contains(body, []byte(`"agent":false`)) || !bytes.Contains(body, []byte(`"localTerminal":false`)) {
		test.Fatalf("remote capabilities not restricted: %s", body)
	}
	fixture.csrf = string(bytes.Split(bytes.Split(body, []byte(`"csrfToken":"`))[1], []byte(`"`))[0])
	fixture.call(http.MethodGet, "/api/v1/hosts", nil, 200)
	fixture.call("POST", "/api/v1/hosts", hostRequest("key", "127.0.0.1:22", "u", "key", map[string]any{"identityFile": "~/.ssh/id_ed25519"}), 403)
	fixture.call("POST", "/api/v1/imports/ssh-config/preview", map[string]string{"source": "local"}, 403)
	fixture.call("GET", "/api/v1/local-documents/anything", nil, 403)
	fixture.call("POST", "/api/v1/attempts", map[string]any{"purpose": "terminal", "hostId": "local:shell", "rows": 24, "cols": 80, "credentials": map[string]string{}}, 403)
}
