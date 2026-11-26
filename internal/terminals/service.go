package terminals

import (
	"context"
	"io"
	"sync"
	"time"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/ident"
)

type Shell interface {
	Stdout() io.Reader
	Stderr() io.Reader
	Input() io.Writer
	Resize(rows, cols int) error
	Wait() Exit
	Transport() connections.Transport
	Close() error
}

// Exit is how a remote shell ended: an exit status, a signal, or a lost
// connection without either.
type Exit struct {
	Code         *int
	Signal       string
	Disconnected bool
}

type Connector interface {
	OpenTerminal(context.Context, connections.Target, int, int) (Shell, error)
}

type Info struct {
	ID         string    `json:"id"`
	HostID     string    `json:"hostId"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	ExitCode   *int      `json:"exitCode,omitempty"`
	ExitSignal string    `json:"exitSignal,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Session struct {
	mu        sync.Mutex
	info      Info
	target    connections.Target
	shell     Shell
	stream    *Stream
	ctx       context.Context
	cancel    context.CancelFunc
	attached  bool
	timer     *time.Timer
	expiresAt time.Time
	closeOnce sync.Once
	onExpire  func()
}

type Service struct {
	mu        sync.Mutex
	connector Connector
	ctx       context.Context
	cancel    context.CancelFunc
	sessions  map[string]*Session
	pending   int
	closed    bool
}

func New(ctx context.Context, connector Connector) *Service {
	root, cancel := context.WithCancel(ctx)
	return &Service{connector: connector, ctx: root, cancel: cancel, sessions: make(map[string]*Session)}
}

func (service *Service) Create(ctx context.Context, target connections.Target, rows, cols int) (*Session, error) {
	if err := ValidateSize(rows, cols); err != nil {
		return nil, err
	}
	service.mu.Lock()
	if service.closed || len(service.sessions)+service.pending >= 8 {
		service.mu.Unlock()
		return nil, fault.New(429, "terminal_limit", "最多同时打开 8 个终端。")
	}
	service.pending++
	service.mu.Unlock()
	defer func() {
		service.mu.Lock()
		service.pending--
		service.mu.Unlock()
	}()
	dialContext, cancelDial := context.WithCancel(ctx)
	stopShutdown := context.AfterFunc(service.ctx, cancelDial)
	defer stopShutdown()
	defer cancelDial()
	shell, err := service.connector.OpenTerminal(dialContext, target, rows, cols)
	if err != nil {
		return nil, err
	}
	target.Interaction = nil
	sessionContext, cancel := context.WithCancel(service.ctx)
	session := &Session{info: Info{ID: ident.New(), HostID: target.Host.ID, Name: target.Host.Name, Status: "running", CreatedAt: time.Now().UTC()}, target: target, shell: shell, stream: NewStream(BufferLimit), ctx: sessionContext, cancel: cancel}
	session.onExpire = func() { service.expire(session.info.ID) }
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		session.Close()
		return nil, context.Canceled
	}
	service.sessions[session.info.ID] = session
	service.mu.Unlock()
	session.mu.Lock()
	session.expiresAt = time.Now().Add(time.Minute)
	session.timer = time.AfterFunc(time.Until(session.expiresAt), session.onExpire)
	session.mu.Unlock()
	session.start()
	return session, nil
}

func (service *Service) Get(owner, sessionID string) (*Session, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	session := service.sessions[sessionID]
	if session == nil || session.target.Owner != owner {
		return nil, fault.New(404, "terminal_not_found", "终端不存在或已结束。")
	}
	return session, nil
}

// Target returns what other capabilities need to reach the same host. It
// never carries an interaction: background work cannot ask the user.
func (service *Service) Target(owner, sessionID string) (connections.Target, error) {
	session, err := service.Get(owner, sessionID)
	if err != nil {
		return connections.Target{}, err
	}
	target := session.target
	target.Interaction = nil
	target.Shared = session.shell.Transport()
	return target, nil
}

func (service *Service) List(owner string) []Info {
	service.mu.Lock()
	defer service.mu.Unlock()
	result := make([]Info, 0)
	for _, session := range service.sessions {
		if session.target.Owner == owner {
			result = append(result, session.Info())
		}
	}
	return result
}

func (service *Service) Remove(owner, sessionID string) error {
	service.mu.Lock()
	session := service.sessions[sessionID]
	if session == nil || session.target.Owner != owner {
		service.mu.Unlock()
		return fault.New(404, "terminal_not_found", "终端不存在或已结束。")
	}
	delete(service.sessions, sessionID)
	service.mu.Unlock()
	session.Close()
	return nil
}

func (service *Service) CloseOwner(owner string) {
	for _, session := range service.List(owner) {
		service.Remove(owner, session.ID)
	}
}

func (service *Service) expire(sessionID string) {
	service.mu.Lock()
	session := service.sessions[sessionID]
	if session == nil {
		service.mu.Unlock()
		return
	}
	session.mu.Lock()
	if session.attached || time.Now().Before(session.expiresAt) {
		session.mu.Unlock()
		service.mu.Unlock()
		return
	}
	session.info.Status = "closed"
	delete(service.sessions, sessionID)
	session.mu.Unlock()
	service.mu.Unlock()
	session.Close()
}

func (service *Service) Close() {
	service.mu.Lock()
	service.closed = true
	active := service.sessions
	service.sessions = make(map[string]*Session)
	service.mu.Unlock()
	service.cancel()
	for _, session := range active {
		session.Close()
	}
}

func (session *Session) start() {
	var readers sync.WaitGroup
	readers.Add(2)
	for _, source := range []io.Reader{session.shell.Stdout(), session.shell.Stderr()} {
		go func() {
			defer readers.Done()
			buffer := make([]byte, FrameLimit)
			for {
				count, err := source.Read(buffer)
				if count > 0 {
					if appendError := session.stream.Append(session.ctx, buffer[:count]); appendError != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		}()
	}
	go func() {
		exit := session.shell.Wait()
		session.mu.Lock()
		if session.info.Status != "closed" {
			session.info.Status = "exited"
			session.info.ExitCode = exit.Code
			session.info.ExitSignal = exit.Signal
			if exit.Disconnected {
				session.info.Status = "disconnected"
			}
		}
		session.mu.Unlock()
		readers.Wait()
		session.stream.Close()
	}()
}

func (session *Session) Info() Info {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.info
}

func (session *Session) Stream() *Stream {
	return session.stream
}

func (session *Session) Attach(offset uint64) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.attached || session.info.Status == "closed" {
		return fault.New(409, "terminal_attached", "终端已在另一个窗口打开，或已关闭。")
	}
	if err := session.stream.Ack(offset); err != nil {
		return fault.New(409, "resume_unavailable", "原终端画面已不可恢复，请关闭此会话并重新连接。")
	}
	session.attached = true
	if session.timer != nil {
		session.timer.Stop()
	}
	return nil
}

func (session *Session) Detach() {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.attached = false
	if session.info.Status != "closed" {
		session.expiresAt = time.Now().Add(time.Minute)
		session.timer = time.AfterFunc(time.Until(session.expiresAt), session.onExpire)
	}
}

func (session *Session) Write(data []byte) error {
	if len(data) > FrameLimit {
		return fault.New(400, "input_too_large", "单次输入超过限制。")
	}
	deadline := time.AfterFunc(5*time.Second, func() { session.Close() })
	defer deadline.Stop()
	_, err := session.shell.Input().Write(data)
	return err
}

func (session *Session) Resize(rows, cols int) error {
	if err := ValidateSize(rows, cols); err != nil {
		return err
	}
	deadline := time.AfterFunc(5*time.Second, func() { session.Close() })
	defer deadline.Stop()
	return session.shell.Resize(rows, cols)
}

func (session *Session) Close() {
	session.closeOnce.Do(func() {
		session.mu.Lock()
		session.info.Status = "closed"
		if session.timer != nil {
			session.timer.Stop()
		}
		session.mu.Unlock()
		session.cancel()
		session.stream.Close()
		session.shell.Close()
	})
}

func ValidateSize(rows, cols int) error {
	if rows < 1 || rows > 500 || cols < 2 || cols > 1000 {
		return fault.New(400, "invalid_terminal_size", "终端尺寸无效。")
	}
	return nil
}
