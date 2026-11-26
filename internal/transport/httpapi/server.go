package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sshworkbench/internal/auth"
	"sshworkbench/internal/connections"
	"sshworkbench/internal/credentials"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/files"
	"sshworkbench/internal/hosts"
	"sshworkbench/internal/imports"
	"sshworkbench/internal/launch"
	"sshworkbench/internal/localdocs"
	"sshworkbench/internal/snippets"
	"sshworkbench/internal/terminals"
	contract "sshworkbench/internal/transport/httpapi/generated"
	"sshworkbench/internal/tunnels"
	"sshworkbench/internal/vault"
	"sshworkbench/internal/workspaces"
)

type Config struct {
	Origin        string
	DevOrigin     string
	Remote        bool
	RequireAuth   bool
	Version       string
	LocalFiles    bool
	Agent         bool
	LocalTerminal bool
}

// Services are the business modules the HTTP layer delegates to.
type Services struct {
	Auth        *auth.Service
	Hosts       *hosts.Service
	Terminals   *terminals.Service
	Files       *files.Service
	Attempts    *connections.Attempts
	Launch      *launch.Service
	Vault       *vault.Vault
	Credentials *credentials.Service
	Tunnels     *tunnels.Service
	Snippets    *snippets.Service
	Workspaces  *workspaces.Service
	Imports     *imports.Service
	Documents   *localdocs.Service
}

type Server struct {
	Services
	config  Config
	origin  *url.URL
	origins map[string]bool
	assets  fs.FS
}

type sessionKey struct{}

func New(config Config, services Services, assets fs.FS) (http.Handler, error) {
	origin, err := url.Parse(config.Origin)
	if err != nil || origin.Host == "" || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.User != nil || (origin.Scheme != "http" && origin.Scheme != "https") {
		return nil, errors.New("invalid application origin")
	}
	if config.Remote {
		config.RequireAuth = true
		if origin.Scheme != "https" {
			return nil, errors.New("remote mode requires HTTPS")
		}
	}
	server := &Server{Services: services, config: config, origin: origin, origins: map[string]bool{config.Origin: true}, assets: assets}
	if config.DevOrigin != "" {
		development, err := url.Parse(config.DevOrigin)
		if config.Remote || err != nil || development.Scheme != "http" || development.Hostname() != "127.0.0.1" || development.Path != "" || development.RawQuery != "" || development.User != nil || development.Fragment != "" {
			return nil, errors.New("dev origin must be an explicit loopback HTTP origin in local mode")
		}
		server.origins[config.DevOrigin] = true
	}
	router := http.NewServeMux()
	protected := func(pattern string, handler http.HandlerFunc) {
		router.Handle(pattern, server.protect(handler))
	}
	router.HandleFunc("GET /api/v1/auth", server.authState)
	router.HandleFunc("POST /api/v1/auth/bootstrap", server.bootstrap)
	router.HandleFunc("POST /api/v1/auth/login", server.login)
	protected("POST /api/v1/auth/logout", server.logout)
	protected("GET /api/v1/hosts", server.listHosts)
	protected("POST /api/v1/hosts", server.saveHost)
	protected("PUT /api/v1/hosts/{hostID}", server.saveHost)
	protected("DELETE /api/v1/hosts/{hostID}", server.deleteHost)
	protected("POST /api/v1/attempts", server.startAttempt)
	protected("GET /api/v1/attempts/{attemptID}", server.waitAttempt)
	protected("POST /api/v1/attempts/{attemptID}/answer", server.answerAttempt)
	protected("DELETE /api/v1/attempts/{attemptID}", server.cancelAttempt)
	protected("GET /api/v1/terminals", server.listTerminals)
	protected("DELETE /api/v1/terminals/{terminalID}", server.closeTerminal)
	protected("GET /api/v1/terminals/{terminalID}/files", server.listFiles)
	protected("POST /api/v1/terminals/{terminalID}/files", server.mutateFile)
	protected("GET /api/v1/terminals/{terminalID}/usage", server.treeUsage)
	protected("POST /api/v1/terminals/{terminalID}/upload", server.upload)
	protected("GET /api/v1/terminals/{terminalID}/download", server.download)
	protected("GET /api/v1/terminals/{terminalID}/archive", server.archive)
	protected("GET /api/v1/terminals/{terminalID}/document", server.readDocument)
	protected("PUT /api/v1/terminals/{terminalID}/document", server.saveDocument)
	protected("GET /api/v1/vault", server.vaultStatus)
	protected("POST /api/v1/vault/setup", server.setupVault)
	protected("POST /api/v1/vault/unlock", server.unlockVault)
	protected("POST /api/v1/vault/lock", server.lockVault)
	protected("POST /api/v1/vault/reset", server.resetVault)
	protected("GET /api/v1/credentials", server.listCredentials)
	protected("POST /api/v1/credentials", server.createCredential)
	protected("DELETE /api/v1/credentials/{credentialID}", server.deleteCredential)
	protected("GET /api/v1/tunnels", server.listTunnels)
	protected("POST /api/v1/tunnels", server.saveTunnel)
	protected("PUT /api/v1/tunnels/{tunnelID}", server.saveTunnel)
	protected("DELETE /api/v1/tunnels/{tunnelID}", server.deleteTunnel)
	protected("POST /api/v1/tunnels/{tunnelID}/stop", server.stopTunnel)
	protected("GET /api/v1/snippets", server.listSnippets)
	protected("POST /api/v1/snippets", server.saveSnippet)
	protected("PUT /api/v1/snippets/{snippetID}", server.saveSnippet)
	protected("DELETE /api/v1/snippets/{snippetID}", server.deleteSnippet)
	protected("GET /api/v1/workspace", server.getWorkspace)
	protected("PUT /api/v1/workspace", server.putWorkspace)
	protected("GET /api/v1/ssh-config/hosts", server.listSSHConfigHosts)
	protected("POST /api/v1/imports/ssh-config/preview", server.previewImport)
	protected("POST /api/v1/imports/ssh-config/apply", server.applyImport)
	protected("GET /api/v1/local-documents/{documentID}", server.readLocalDocument)
	protected("PUT /api/v1/local-documents/{documentID}", server.saveLocalDocument)
	protected("GET /ws/terminals/{terminalID}", server.terminalSocket)
	router.HandleFunc("/api/", func(response http.ResponseWriter, request *http.Request) {
		writeError(response, fault.New(404, "not_found", "接口不存在。"))
	})
	router.HandleFunc("/ws/", func(response http.ResponseWriter, request *http.Request) {
		writeError(response, fault.New(404, "not_found", "终端通道不存在。"))
	})
	router.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writeError(response, fault.New(405, "method_not_allowed", "此资源只支持读取。"))
			return
		}
		server.static(response, request)
	})
	return server.boundary(router), nil
}

func (server *Server) boundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		response.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self' ws: wss:; worker-src 'self' blob:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		response.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		response.Header().Set("Cache-Control", "no-store")
		if server.config.RequireAuth {
			if !strings.EqualFold(request.Host, server.origin.Host) {
				writeError(response, fault.New(403, "invalid_host_header", "请求主机与服务配置不匹配。"))
				return
			}
			origin := request.Header.Get("Origin")
			mutating := request.Method != http.MethodGet && request.Method != http.MethodHead
			websocketRequest := strings.HasPrefix(request.URL.Path, "/ws/")
			if (origin != "" && !server.origins[origin]) || ((mutating || websocketRequest) && !server.origins[origin]) {
				writeError(response, fault.New(403, "invalid_origin", "已拒绝来自其他网页的请求。"))
				return
			}
			if server.config.Remote && request.TLS == nil {
				writeError(response, fault.New(403, "https_required", "远程访问必须使用 HTTPS。"))
				return
			}
		}
		next.ServeHTTP(response, request)
	})
}

func (server *Server) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var session auth.Session
		if !server.config.RequireAuth {
			session = server.Auth.OpenSession()
		} else {
			var authenticated bool
			session, authenticated = server.session(request)
			if !authenticated {
				writeError(response, fault.New(401, "unauthenticated", "登录已过期或尚未授权。"))
				return
			}
			if request.Method != http.MethodGet && request.Method != http.MethodHead && subtle.ConstantTimeCompare([]byte(request.Header.Get("X-CSRF-Token")), []byte(session.CSRF)) != 1 {
				writeError(response, fault.New(403, "csrf_rejected", "请求授权不匹配，请重新打开页面。"))
				return
			}
		}
		ctx, cancel := context.WithCancel(request.Context())
		stop := context.AfterFunc(session.Context, func() {
			cancel()
			http.NewResponseController(response).SetReadDeadline(time.Now())
		})
		defer cancel()
		defer stop()
		request = request.WithContext(context.WithValue(ctx, sessionKey{}, session))
		next.ServeHTTP(response, request)
	})
}

func (server *Server) session(request *http.Request) (auth.Session, bool) {
	if !server.config.RequireAuth {
		return server.Auth.OpenSession(), true
	}
	cookie, err := request.Cookie(server.Auth.CookieName())
	if err != nil {
		return auth.Session{}, false
	}
	return server.Auth.Authenticate(cookie.Value)
}

func currentSession(request *http.Request) auth.Session {
	return request.Context().Value(sessionKey{}).(auth.Session)
}

func (server *Server) authState(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("X-SSHW-Service", "ssh-workbench")
	session, authenticated := server.session(request)
	state := map[string]any{
		"authenticated": authenticated,
		"authRequired":  server.config.RequireAuth,
		"remote":        server.config.Remote,
		"version":       server.config.Version,
		"capabilities":  server.capabilities(),
	}
	if authenticated {
		state["csrfToken"] = session.CSRF
		state["expiresAt"] = session.ExpiresAt
	}
	writeJSON(response, 200, state)
}

func (server *Server) bootstrap(response http.ResponseWriter, request *http.Request) {
	var input contract.BootstrapRequest
	if !decode(response, request, &input, 4096) {
		return
	}
	if !server.config.RequireAuth {
		server.authState(response, request)
		return
	}
	if session, authenticated := server.session(request); authenticated {
		if err := server.Auth.Consume(input.Token); err != nil {
			writeError(response, err)
			return
		}
		writeJSON(response, 200, map[string]any{"authenticated": true, "authRequired": true, "csrfToken": session.CSRF, "expiresAt": session.ExpiresAt, "remote": server.config.Remote, "version": server.config.Version, "capabilities": server.capabilities()})
		return
	}
	token, session, err := server.Auth.Exchange(input.Token)
	server.completeLogin(response, token, session, err)
}

func (server *Server) login(response http.ResponseWriter, request *http.Request) {
	var input contract.LoginRequest
	if !decode(response, request, &input, 4096) {
		return
	}
	address, _, _ := net.SplitHostPort(request.RemoteAddr)
	token, session, err := server.Auth.Login(address, valueOf(input.Password))
	server.completeLogin(response, token, session, err)
}

func (server *Server) completeLogin(response http.ResponseWriter, token string, session auth.Session, err error) {
	if err != nil {
		writeError(response, err)
		return
	}
	http.SetCookie(response, &http.Cookie{Name: server.Auth.CookieName(), Value: token, Path: "/", HttpOnly: true, Secure: server.config.Remote, SameSite: http.SameSiteStrictMode, Expires: session.ExpiresAt, MaxAge: 8 * 60 * 60})
	writeJSON(response, 200, map[string]any{"authenticated": true, "authRequired": server.config.RequireAuth, "csrfToken": session.CSRF, "remote": server.config.Remote, "version": server.config.Version, "capabilities": server.capabilities()})
}

func (server *Server) capabilities() map[string]bool {
	return map[string]bool{
		"localFiles":    server.config.LocalFiles,
		"agent":         server.config.Agent,
		"localTerminal": server.config.LocalTerminal,
	}
}

func (server *Server) logout(response http.ResponseWriter, request *http.Request) {
	if !server.config.RequireAuth {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	cookie, err := request.Cookie(server.Auth.CookieName())
	if err == nil {
		server.Auth.Revoke(cookie.Value)
	}
	http.SetCookie(response, &http.Cookie{Name: server.Auth.CookieName(), Path: "/", HttpOnly: true, Secure: server.config.Remote, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	response.WriteHeader(http.StatusNoContent)
}

func (server *Server) static(response http.ResponseWriter, request *http.Request) {
	filename := strings.TrimPrefix(request.URL.Path, "/")
	if filename == "" || filename == "editor" || filename == "local-editor" {
		filename = "index.html"
	}
	if !fs.ValidPath(filename) || strings.HasPrefix(filename, ".") || strings.Contains(filename, "/.") {
		http.NotFound(response, request)
		return
	}
	if _, err := fs.Stat(server.assets, filename); err != nil {
		if filename == "index.html" {
			http.Error(response, "Frontend not built. Run: cd web && npm ci && npm run build", 503)
		} else {
			http.NotFound(response, request)
		}
		return
	}
	clone := request.Clone(request.Context())
	clone.URL.Path = "/" + filename
	if filename == "index.html" {
		clone.URL.Path = "/"
	}
	if strings.HasPrefix(filename, "assets/") {
		response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	http.FileServerFS(server.assets).ServeHTTP(response, clone)
}

func decode(response http.ResponseWriter, request *http.Request, destination any, maximum int64) bool {
	if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		writeError(response, fault.New(415, "json_required", "请求须使用 JSON。"))
		return false
	}
	http.NewResponseController(response).SetReadDeadline(time.Now().Add(15 * time.Second))
	request.Body = http.MaxBytesReader(response, request.Body, maximum)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(response, fault.New(400, "invalid_json", "请求内容无效或超过大小限制。"))
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeError(response, fault.New(400, "invalid_json", "请求只能包含一个 JSON 对象。"))
		return false
	}
	return true
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.WriteHeader(status)
	json.NewEncoder(response).Encode(value)
}

func writeError(response http.ResponseWriter, err error) {
	problem := fault.As(err)
	if problem.Status >= 500 {
		slog.Warn("request failed", "code", problem.Code)
	}
	writeJSON(response, problem.Status, map[string]any{"error": problem})
}
