// Package tunnels runs local TCP forwards (ssh -L). A tunnel is an object of
// its own: it owns an SSH transport and a loopback listener on the machine
// running this server, and it outlives any browser tab until it is stopped,
// its owner logs out, or the application exits.
package tunnels

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/ident"
)

const (
	StateStopped = "stopped"
	StateRunning = "running"
	StateFailed  = "failed"

	connectionLimit = 64
	tunnelLimit     = 32
)

type Spec struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	HostID     string    `json:"hostId"`
	ListenHost string    `json:"listenHost"`
	ListenPort int       `json:"listenPort"`
	TargetHost string    `json:"targetHost"`
	TargetPort int       `json:"targetPort"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Status struct {
	State             string       `json:"state"`
	Listen            string       `json:"listen,omitempty"`
	ActiveConnections int64        `json:"activeConnections"`
	TotalConnections  int64        `json:"totalConnections"`
	BytesSent         int64        `json:"bytesSent"`
	BytesReceived     int64        `json:"bytesReceived"`
	StartedAt         *time.Time   `json:"startedAt,omitempty"`
	Error             *fault.Error `json:"error,omitempty"`
}

type View struct {
	Spec
	Status Status `json:"status"`
}

type Forwarder interface {
	DialContext(context.Context, string, string) (net.Conn, error)
	Done() <-chan struct{}
	Close() error
}

type Connector interface {
	OpenForwarder(context.Context, connections.Target) (Forwarder, error)
}

type Repository interface {
	ListTunnels(context.Context) ([]Spec, error)
	GetTunnel(context.Context, string) (Spec, error)
	PutTunnel(context.Context, Spec) error
	DeleteTunnel(context.Context, string) error
}

type runtime struct {
	owner     string
	listener  net.Listener
	forwarder Forwarder
	cancel    context.CancelFunc
	started   time.Time
	active    atomic.Int64
	total     atomic.Int64
	sent      atomic.Int64
	received  atomic.Int64
	stopOnce  sync.Once
	done      chan struct{}
	mu        sync.Mutex
	failure   *fault.Error
	stopped   bool
}

type Service struct {
	mu         sync.Mutex
	ctx        context.Context
	repository Repository
	connector  Connector
	runtimes   map[string]*runtime
	failures   map[string]*fault.Error
	closed     bool
}

func New(ctx context.Context, repository Repository, connector Connector) *Service {
	return &Service{ctx: ctx, repository: repository, connector: connector, runtimes: make(map[string]*runtime), failures: make(map[string]*fault.Error)}
}

func (service *Service) HostReferences(ctx context.Context, hostID string) (int, error) {
	specs, err := service.repository.ListTunnels(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, spec := range specs {
		if spec.HostID == hostID {
			count++
		}
	}
	return count, nil
}

func (service *Service) List(ctx context.Context) ([]View, error) {
	specs, err := service.repository.ListTunnels(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(specs))
	for _, spec := range specs {
		views = append(views, View{Spec: spec, Status: service.status(spec.ID)})
	}
	return views, nil
}

func (service *Service) Get(ctx context.Context, identifier string) (Spec, error) {
	return service.repository.GetTunnel(ctx, identifier)
}

func (service *Service) status(identifier string) Status {
	service.mu.Lock()
	active := service.runtimes[identifier]
	failure := service.failures[identifier]
	service.mu.Unlock()
	if active == nil {
		if failure != nil {
			return Status{State: StateFailed, Error: failure}
		}
		return Status{State: StateStopped}
	}
	started := active.started
	return Status{State: StateRunning, Listen: active.listener.Addr().String(), ActiveConnections: active.active.Load(), TotalConnections: active.total.Load(), BytesSent: active.sent.Load(), BytesReceived: active.received.Load(), StartedAt: &started}
}

func Validate(spec Spec) error {
	if spec.Name == "" || utf8.RuneCountInString(spec.Name) > 80 || !utf8.ValidString(spec.Name) || strings.ContainsAny(spec.Name, "\x00\r\n") {
		return fault.New(400, "invalid_tunnel_name", "请填写 1–80 个字符的隧道名称。")
	}
	if spec.HostID == "" {
		return fault.New(400, "invalid_tunnel_host", "请选择 SSH 主机。")
	}
	if spec.ListenHost != "127.0.0.1" && spec.ListenHost != "::1" {
		return fault.New(400, "public_listen_refused", "本地转发只能监听回环地址 127.0.0.1 或 ::1，不会对外公开。")
	}
	if spec.ListenPort < 1 || spec.ListenPort > 65535 || spec.TargetPort < 1 || spec.TargetPort > 65535 {
		return fault.New(400, "invalid_tunnel_port", "端口范围为 1–65535。")
	}
	target := strings.Trim(spec.TargetHost, "[]")
	if target == "" || len(target) > 253 || strings.ContainsAny(target, " /\\@\t\r\n\x00") || (strings.Contains(target, ":") && net.ParseIP(target) == nil) {
		return fault.New(400, "invalid_tunnel_target", "目标地址须为主机名或 IP，从 SSH 主机一侧解析。")
	}
	return nil
}

func (service *Service) Save(ctx context.Context, identifier string, spec Spec) (Spec, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.TargetHost = strings.Trim(strings.TrimSpace(spec.TargetHost), "[]")
	if spec.ListenHost == "" {
		spec.ListenHost = "127.0.0.1"
	}
	if err := Validate(spec); err != nil {
		return Spec{}, err
	}
	now := time.Now().UTC()
	spec.ID, spec.CreatedAt, spec.UpdatedAt = identifier, now, now
	if identifier == "" {
		existing, err := service.repository.ListTunnels(ctx)
		if err != nil {
			return Spec{}, err
		}
		if len(existing) >= tunnelLimit {
			return Spec{}, fault.New(409, "tunnel_limit", "最多保存 32 条隧道。")
		}
		spec.ID = ident.New()
	} else {
		previous, err := service.repository.GetTunnel(ctx, identifier)
		if err != nil {
			return Spec{}, err
		}
		if service.running(identifier) {
			return Spec{}, fault.New(409, "tunnel_running", "请先停止隧道再修改。")
		}
		spec.CreatedAt = previous.CreatedAt
	}
	if err := service.repository.PutTunnel(ctx, spec); err != nil {
		return Spec{}, err
	}
	return spec, nil
}

func (service *Service) running(identifier string) bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.runtimes[identifier] != nil
}

func (service *Service) Delete(ctx context.Context, identifier string) error {
	if service.running(identifier) {
		return fault.New(409, "tunnel_running", "请先停止隧道再删除。")
	}
	service.mu.Lock()
	delete(service.failures, identifier)
	service.mu.Unlock()
	return service.repository.DeleteTunnel(ctx, identifier)
}

// Listen binds the loopback port before any SSH work, so a port conflict is
// reported immediately and never leaves a half-built tunnel behind.
func (service *Service) Listen(spec Spec) (net.Listener, error) {
	if err := Validate(spec); err != nil {
		return nil, err
	}
	service.mu.Lock()
	busy := service.runtimes[spec.ID] != nil
	service.mu.Unlock()
	if busy {
		return nil, fault.New(409, "tunnel_running", "隧道已在运行。")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(spec.ListenHost, strconv.Itoa(spec.ListenPort)))
	if err != nil {
		switch {
		case errors.Is(err, syscall.EADDRINUSE):
			return nil, fault.New(409, "port_in_use", "本机端口 "+strconv.Itoa(spec.ListenPort)+" 已被占用，请换一个端口或停止占用它的程序。")
		case errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM):
			return nil, fault.New(403, "port_permission_denied", "当前用户无权监听此端口（例如 1024 以下端口）。")
		default:
			return nil, fault.New(502, "listen_failed", "无法监听本机端口。")
		}
	}
	return listener, nil
}

// Start opens the SSH transport for an already bound listener and begins
// forwarding. The listener is closed on every failure path.
func (service *Service) Start(ctx context.Context, owner string, spec Spec, listener net.Listener, target connections.Target) (Status, error) {
	forwarder, err := service.connector.OpenForwarder(ctx, target)
	if err != nil {
		listener.Close()
		return Status{}, err
	}
	runtimeContext, cancel := context.WithCancel(service.ctx)
	active := &runtime{owner: owner, listener: listener, forwarder: forwarder, cancel: cancel, started: time.Now().UTC(), done: make(chan struct{})}
	service.mu.Lock()
	if service.closed || service.runtimes[spec.ID] != nil || ctx.Err() != nil {
		service.mu.Unlock()
		cancel()
		listener.Close()
		forwarder.Close()
		return Status{}, fault.New(409, "tunnel_running", "隧道已在运行或服务正在退出。")
	}
	service.runtimes[spec.ID] = active
	delete(service.failures, spec.ID)
	service.mu.Unlock()
	address := net.JoinHostPort(spec.TargetHost, strconv.Itoa(spec.TargetPort))
	go service.accept(runtimeContext, spec.ID, active, address)
	go func() {
		select {
		case <-forwarder.Done():
			service.stop(spec.ID, active, fault.New(502, "tunnel_transport_closed", "SSH 连接已断开，隧道已停止。"))
		case <-runtimeContext.Done():
		}
	}()
	return service.status(spec.ID), nil
}

func (service *Service) accept(ctx context.Context, identifier string, active *runtime, address string) {
	for {
		connection, err := active.listener.Accept()
		if err != nil {
			if ctx.Err() == nil {
				service.stop(identifier, active, fault.New(502, "listener_closed", "本机监听意外关闭，隧道已停止。"))
			}
			return
		}
		if active.active.Load() >= connectionLimit {
			connection.Close()
			continue
		}
		active.active.Add(1)
		active.total.Add(1)
		go func() {
			defer active.active.Add(-1)
			service.forward(ctx, active, connection, address)
		}()
	}
}

func (service *Service) forward(ctx context.Context, active *runtime, local net.Conn, address string) {
	defer local.Close()
	dialContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	remote, err := active.forwarder.DialContext(dialContext, "tcp", address)
	cancel()
	if err != nil {
		active.mu.Lock()
		active.failure = fault.New(502, "tunnel_target_refused", "SSH 主机无法连接隧道目标 "+address+"。")
		active.mu.Unlock()
		return
	}
	defer remote.Close()
	stop := context.AfterFunc(ctx, func() {
		local.Close()
		remote.Close()
	})
	defer stop()
	var wait sync.WaitGroup
	wait.Add(2)
	pipe := func(destination, source net.Conn, counter *atomic.Int64) {
		defer wait.Done()
		count, _ := io.Copy(destination, source)
		counter.Add(count)
		if closer, ok := destination.(interface{ CloseWrite() error }); ok {
			closer.CloseWrite()
		} else {
			destination.Close()
		}
	}
	go pipe(remote, local, &active.sent)
	go pipe(local, remote, &active.received)
	wait.Wait()
}

func (service *Service) stop(identifier string, active *runtime, failure *fault.Error) {
	active.stopOnce.Do(func() {
		service.mu.Lock()
		if service.runtimes[identifier] == active {
			delete(service.runtimes, identifier)
			if failure != nil {
				service.failures[identifier] = failure
			}
		}
		service.mu.Unlock()
		active.cancel()
		active.listener.Close()
		active.forwarder.Close()
		close(active.done)
	})
}

func (service *Service) Stop(identifier string) error {
	service.mu.Lock()
	active := service.runtimes[identifier]
	service.mu.Unlock()
	if active == nil {
		return fault.New(404, "tunnel_not_running", "隧道没有在运行。")
	}
	service.stop(identifier, active, nil)
	return nil
}

func (service *Service) CloseOwner(owner string) {
	service.mu.Lock()
	var owned []string
	for identifier, active := range service.runtimes {
		if active.owner == owner {
			owned = append(owned, identifier)
		}
	}
	service.mu.Unlock()
	for _, identifier := range owned {
		service.Stop(identifier)
	}
}

func (service *Service) Close() {
	service.mu.Lock()
	service.closed = true
	identifiers := make([]string, 0, len(service.runtimes))
	for identifier := range service.runtimes {
		identifiers = append(identifiers, identifier)
	}
	service.mu.Unlock()
	for _, identifier := range identifiers {
		service.Stop(identifier)
	}
}
