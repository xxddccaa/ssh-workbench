package connections

import (
	"context"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/ident"
)

const (
	AttemptConnecting = "connecting"
	AttemptPrompt     = "prompt"
	AttemptConnected  = "connected"
	AttemptFailed     = "failed"
	AttemptCancelled  = "cancelled"

	PromptHostKey   = "host-key"
	PromptChallenge = "keyboard-interactive"

	attemptLifetime   = 5 * time.Minute
	promptLifetime    = 3 * time.Minute
	finishedRetention = 2 * time.Minute
	attemptLimit      = 32
	questionLimit     = 8
	answerLimit       = 4096
)

type Question struct {
	Text string `json:"text"`
	Echo bool   `json:"echo"`
}

// Prompt is everything the user needs to decide; it never contains secrets.
type Prompt struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Hop         string     `json:"hop"`
	Host        string     `json:"host"`
	Address     string     `json:"address,omitempty"`
	Algorithm   string     `json:"algorithm,omitempty"`
	Fingerprint string     `json:"fingerprint,omitempty"`
	KnownHosts  string     `json:"knownHosts,omitempty"`
	Name        string     `json:"name,omitempty"`
	Instruction string     `json:"instruction,omitempty"`
	Questions   []Question `json:"questions,omitempty"`
}

type Answer struct {
	PromptID  string
	Accept    bool
	Responses []string
}

// Interaction lets a transport ask the person behind an attempt. Both calls
// block until an answer, cancellation or timeout.
type Interaction interface {
	ConfirmHostKey(context.Context, Prompt) error
	Respond(context.Context, Prompt) ([]string, error)
}

type Attempt struct {
	ID      string       `json:"id"`
	Purpose string       `json:"purpose"`
	State   string       `json:"state"`
	Version int64        `json:"version"`
	Prompt  *Prompt      `json:"prompt,omitempty"`
	Error   *fault.Error `json:"error,omitempty"`
	Result  any          `json:"result,omitempty"`
}

type attempt struct {
	view    Attempt
	owner   string
	changed chan struct{}
	answers chan Answer
	cancel  context.CancelFunc
}

type Attempts struct {
	mu       sync.Mutex
	ctx      context.Context
	attempts map[string]*attempt
}

func NewAttempts(ctx context.Context) *Attempts {
	return &Attempts{ctx: ctx, attempts: make(map[string]*attempt)}
}

// Start runs work in the background on behalf of owner. The returned view is
// polled with Wait; prompts raised by work are answered with Answer.
func (registry *Attempts) Start(owner, purpose string, work func(context.Context, Interaction) (any, error)) (Attempt, error) {
	ctx, cancel := context.WithTimeout(registry.ctx, attemptLifetime)
	item := &attempt{view: Attempt{ID: ident.New(), Purpose: purpose, State: AttemptConnecting, Version: 1}, owner: owner, changed: make(chan struct{}), answers: make(chan Answer, 1), cancel: cancel}
	registry.mu.Lock()
	active := 0
	for _, existing := range registry.attempts {
		if existing.view.State == AttemptConnecting || existing.view.State == AttemptPrompt {
			active++
		}
	}
	if active >= attemptLimit || registry.ctx.Err() != nil {
		registry.mu.Unlock()
		cancel()
		return Attempt{}, fault.New(429, "too_many_attempts", "进行中的连接过多，请等待或取消已有连接。")
	}
	registry.attempts[item.view.ID] = item
	view := item.view
	registry.mu.Unlock()
	go func() {
		defer cancel()
		result, err := work(ctx, &interaction{registry: registry, item: item})
		registry.finish(item, ctx, result, err)
	}()
	return view, nil
}

// Releaser is implemented by results that own a resource; it is called when
// a cancellation arrives after the work already succeeded, so nothing the
// user abandoned stays open.
type Releaser interface {
	Release()
}

func (registry *Attempts) finish(item *attempt, ctx context.Context, result any, err error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	item.view.Prompt = nil
	switch {
	case item.view.State == AttemptCancelled || (err != nil && errors.Is(ctx.Err(), context.Canceled)):
		if releaser, ok := result.(Releaser); ok && err == nil {
			go releaser.Release()
		}
		item.view.State = AttemptCancelled
		item.view.Error = fault.New(499, "connection_cancelled", "连接已取消。")
	case err == nil:
		item.view.State = AttemptConnected
		item.view.Result = result
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		item.view.State = AttemptFailed
		item.view.Error = fault.New(408, "connection_timeout", "连接或确认超时，请重新连接。")
	default:
		item.view.State = AttemptFailed
		item.view.Error = fault.As(err)
	}
	registry.signalLocked(item)
	identifier := item.view.ID
	time.AfterFunc(finishedRetention, func() {
		registry.mu.Lock()
		delete(registry.attempts, identifier)
		registry.mu.Unlock()
	})
}

func (registry *Attempts) signalLocked(item *attempt) {
	item.view.Version++
	close(item.changed)
	item.changed = make(chan struct{})
}

func (registry *Attempts) lookupLocked(owner, identifier string) (*attempt, error) {
	item := registry.attempts[identifier]
	if item == nil || item.owner != owner {
		return nil, fault.New(404, "attempt_not_found", "连接请求不存在或已过期。")
	}
	return item, nil
}

// Wait returns as soon as the attempt is newer than after, or after a bounded
// long-poll interval with the unchanged view.
func (registry *Attempts) Wait(ctx context.Context, owner, identifier string, after int64) (Attempt, error) {
	timer := time.NewTimer(25 * time.Second)
	defer timer.Stop()
	for {
		registry.mu.Lock()
		item, err := registry.lookupLocked(owner, identifier)
		if err != nil {
			registry.mu.Unlock()
			return Attempt{}, err
		}
		view := item.view
		changed := item.changed
		registry.mu.Unlock()
		if view.Version > after {
			return view, nil
		}
		select {
		case <-changed:
		case <-timer.C:
			return view, nil
		case <-ctx.Done():
			return Attempt{}, ctx.Err()
		}
	}
}

func (registry *Attempts) Answer(owner, identifier string, answer Answer) error {
	if len(answer.Responses) > questionLimit {
		return fault.New(400, "invalid_answer", "回答数量无效。")
	}
	for _, response := range answer.Responses {
		if len(response) > answerLimit || !utf8.ValidString(response) {
			return fault.New(400, "invalid_answer", "回答内容无效或过长。")
		}
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	item, err := registry.lookupLocked(owner, identifier)
	if err != nil {
		return err
	}
	if item.view.State != AttemptPrompt || item.view.Prompt == nil || item.view.Prompt.ID != answer.PromptID {
		return fault.New(410, "prompt_expired", "这个确认已失效，请查看最新的连接状态。")
	}
	if item.view.Prompt.Kind == PromptChallenge && answer.Accept && len(answer.Responses) != len(item.view.Prompt.Questions) {
		return fault.New(400, "invalid_answer", "请回答服务器提出的全部问题。")
	}
	select {
	case item.answers <- answer:
	default:
		return fault.New(409, "answer_pending", "已提交回答，正在处理。")
	}
	item.view.State = AttemptConnecting
	item.view.Prompt = nil
	registry.signalLocked(item)
	return nil
}

func (registry *Attempts) Cancel(owner, identifier string) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	item, err := registry.lookupLocked(owner, identifier)
	if err != nil {
		return err
	}
	if item.view.State == AttemptConnecting || item.view.State == AttemptPrompt {
		item.view.State = AttemptCancelled
		item.view.Prompt = nil
		registry.signalLocked(item)
	}
	item.cancel()
	return nil
}

func (registry *Attempts) CloseOwner(owner string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for _, item := range registry.attempts {
		if item.owner == owner {
			item.cancel()
		}
	}
}

type interaction struct {
	registry *Attempts
	item     *attempt
}

func (session *interaction) ask(ctx context.Context, prompt Prompt) (Answer, error) {
	registry := session.registry
	prompt.ID = ident.New()
	registry.mu.Lock()
	if session.item.view.State != AttemptConnecting {
		registry.mu.Unlock()
		return Answer{}, context.Canceled
	}
	select {
	case <-session.item.answers:
	default:
	}
	session.item.view.State = AttemptPrompt
	session.item.view.Prompt = &prompt
	registry.signalLocked(session.item)
	registry.mu.Unlock()
	timer := time.NewTimer(promptLifetime)
	defer timer.Stop()
	for {
		select {
		case answer := <-session.item.answers:
			if answer.PromptID == prompt.ID {
				return answer, nil
			}
		case <-timer.C:
			return Answer{}, fault.New(408, "prompt_timeout", "等待确认超时，连接已停止。")
		case <-ctx.Done():
			return Answer{}, ctx.Err()
		}
	}
}

func (session *interaction) ConfirmHostKey(ctx context.Context, prompt Prompt) error {
	prompt.Kind = PromptHostKey
	answer, err := session.ask(ctx, prompt)
	if err != nil {
		return err
	}
	if !answer.Accept {
		return fault.New(403, "host_key_rejected", "已拒绝信任此主机密钥，连接已停止。")
	}
	return nil
}

func (session *interaction) Respond(ctx context.Context, prompt Prompt) ([]string, error) {
	prompt.Kind = PromptChallenge
	if len(prompt.Questions) > questionLimit {
		return nil, fault.New(502, "challenge_too_large", "服务器的认证问题数量超出限制。")
	}
	answer, err := session.ask(ctx, prompt)
	if err != nil {
		return nil, err
	}
	if !answer.Accept {
		return nil, fault.New(401, "challenge_declined", "已取消交互式认证。")
	}
	return answer.Responses, nil
}
