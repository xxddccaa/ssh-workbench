package auth

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/ident"
)

type Session struct {
	ID        string
	CSRF      string
	ExpiresAt time.Time
	Context   context.Context
	cancel    context.CancelFunc
	timer     *time.Timer
}

type attempt struct {
	start time.Time
	count int
}

type Service struct {
	mu               sync.Mutex
	remote           bool
	requireAuth      bool
	passwordHash     string
	initialBootstrap string
	bootstraps       map[[32]byte]time.Time
	cookieName       string
	sessions         map[[32]byte]Session
	openSession      Session
	hasOpenSession   bool
	attempts         map[string]attempt
	passwordWorkers  chan struct{}
	onRevoke         func(string)
	closed           bool
}

func New(remote bool, passwordHash string, onRevoke func(string)) *Service {
	return NewWithMode(remote, true, passwordHash, onRevoke)
}

func NewWithMode(remote, requireAuth bool, passwordHash string, onRevoke func(string)) *Service {
	service := &Service{
		remote:          remote,
		requireAuth:     requireAuth,
		passwordHash:    passwordHash,
		cookieName:      "sshwb_session",
		bootstraps:      make(map[[32]byte]time.Time),
		sessions:        make(map[[32]byte]Session),
		attempts:        make(map[string]attempt),
		passwordWorkers: make(chan struct{}, 2),
		onRevoke:        onRevoke,
	}
	if remote {
		service.cookieName = "__Host-sshwb_session"
	}
	if !remote && requireAuth {
		service.initialBootstrap, _ = service.IssueBootstrap()
	}
	return service
}

// OpenSession returns a shared persistent session when authentication is not required.
func (service *Service) OpenSession() Session {
	service.mu.Lock()
	defer service.mu.Unlock()
	if !service.hasOpenSession {
		sessionContext, cancel := context.WithCancel(context.Background())
		service.openSession = Session{
			ID:        "open-workspace",
			CSRF:      ident.New(),
			ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
			Context:   sessionContext,
			cancel:    cancel,
		}
		service.hasOpenSession = true
	}
	return service.openSession
}

// BootstrapToken is the one-time token printed at startup.
func (service *Service) BootstrapToken() string {
	return service.initialBootstrap
}

// IssueBootstrap creates another short-lived, single-use local authorization.
// It is reachable only through the OS-user-only control channel.
func (service *Service) IssueBootstrap() (string, error) {
	if service.remote {
		return "", fault.New(403, "local_bootstrap_disabled", "远程模式不使用本机授权链接。")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	now := time.Now()
	for digest, expires := range service.bootstraps {
		if now.After(expires) {
			delete(service.bootstraps, digest)
		}
	}
	if service.closed || len(service.bootstraps) >= 8 {
		return "", fault.New(429, "too_many_bootstraps", "未使用的授权链接过多，请两分钟后再试。")
	}
	token := ident.New()
	service.bootstraps[sha256.Sum256([]byte(token))] = now.Add(2 * time.Minute)
	return token, nil
}

func (service *Service) CookieName() string {
	return service.cookieName
}

func (service *Service) Exchange(token string) (string, Session, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if err := service.consumeLocked(token); err != nil {
		return "", Session{}, err
	}
	return service.createLocked()
}

// Consume spends a bootstrap token without creating a session. It is used
// when the browser already holds a valid session, so a second window opened
// by `sshworkbench open`/`edit` joins it instead of replacing the cookie that
// existing tabs (and their terminals) depend on.
func (service *Service) Consume(token string) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.consumeLocked(token)
}

func (service *Service) consumeLocked(token string) error {
	digest := sha256.Sum256([]byte(token))
	expires, exists := service.bootstraps[digest]
	if service.remote || token == "" || !exists || time.Now().After(expires) {
		return fault.New(401, "bootstrap_invalid", "本机授权链接已过期或已使用。请在终端运行 sshworkbench open 获取新链接。")
	}
	delete(service.bootstraps, digest)
	return nil
}

func (service *Service) Login(address, password string) (string, Session, error) {
	if !service.remote {
		return "", Session{}, fault.New(403, "local_bootstrap_required", "本机模式请使用启动时提供的授权链接。")
	}
	service.mu.Lock()
	for client, history := range service.attempts {
		if time.Since(history.start) > time.Minute {
			delete(service.attempts, client)
		}
	}
	history, known := service.attempts[address]
	if !known && len(service.attempts) >= 1024 {
		// Evict the oldest record rather than refusing everyone: rotating
		// addresses must not lock the owner out. Throughput stays bounded by
		// the two password-hashing workers below.
		var oldest string
		for client, candidate := range service.attempts {
			if oldest == "" || candidate.start.Before(service.attempts[oldest].start) {
				oldest = client
			}
		}
		delete(service.attempts, oldest)
	}
	if history.start.IsZero() {
		history.start = time.Now()
	}
	if history.count >= 6 {
		service.mu.Unlock()
		return "", Session{}, fault.New(429, "login_rate_limited", "登录尝试过多，请一分钟后重试。")
	}
	history.count++
	service.attempts[address] = history
	service.mu.Unlock()
	select {
	case service.passwordWorkers <- struct{}{}:
		defer func() { <-service.passwordWorkers }()
	default:
		return "", Session{}, fault.New(429, "login_busy", "登录服务繁忙，请稍后重试。")
	}
	if !VerifyPassword(service.passwordHash, password) {
		return "", Session{}, fault.New(401, "invalid_password", "密码不正确。")
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.createLocked()
}

func (service *Service) createLocked() (string, Session, error) {
	if service.closed || len(service.sessions) >= 32 {
		return "", Session{}, fault.New(429, "too_many_logins", "登录会话过多或服务正在退出。")
	}
	token := ident.New()
	digest := sha256.Sum256([]byte(token))
	sessionContext, cancel := context.WithCancel(context.Background())
	session := Session{ID: ident.New(), CSRF: ident.New(), ExpiresAt: time.Now().Add(8 * time.Hour), Context: sessionContext, cancel: cancel}
	session.timer = time.AfterFunc(time.Until(session.ExpiresAt), func() { service.revokeDigest(digest) })
	service.sessions[digest] = session
	return token, session, nil
}

func (service *Service) Authenticate(token string) (Session, bool) {
	digest := sha256.Sum256([]byte(token))
	service.mu.Lock()
	session, exists := service.sessions[digest]
	service.mu.Unlock()
	return session, exists && time.Now().Before(session.ExpiresAt)
}

func (service *Service) Revoke(token string) {
	service.revokeDigest(sha256.Sum256([]byte(token)))
}

func (service *Service) revokeDigest(digest [32]byte) {
	service.mu.Lock()
	session, exists := service.sessions[digest]
	if exists {
		delete(service.sessions, digest)
		session.timer.Stop()
		session.cancel()
	}
	service.mu.Unlock()
	if exists && service.onRevoke != nil {
		service.onRevoke(session.ID)
	}
}

func (service *Service) Close() {
	service.mu.Lock()
	service.closed = true
	digests := make([][32]byte, 0, len(service.sessions))
	for digest := range service.sessions {
		digests = append(digests, digest)
	}
	hasOpen := service.hasOpenSession
	openSession := service.openSession
	service.hasOpenSession = false
	service.mu.Unlock()
	for _, digest := range digests {
		service.revokeDigest(digest)
	}
	if hasOpen {
		openSession.cancel()
		if service.onRevoke != nil {
			service.onRevoke(openSession.ID)
		}
	}
}
