package connections

import (
	"context"
	"errors"
	"testing"
	"time"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/hosts"
)

type memoryTrust struct {
	key *KnownKey
	err error
}

func (store *memoryTrust) GetKnownKey(context.Context, string) (*KnownKey, error) {
	return store.key, store.err
}
func (store *memoryTrust) AddKnownKey(_ context.Context, key KnownKey) error {
	store.key = &key
	return nil
}

func TestTrustRequiresAcceptanceAndBlocksChangedKeys(test *testing.T) {
	store := &memoryTrust{}
	verifier := NewVerifier(store)
	key := KnownKey{Address: "host:22", Algorithm: "ssh-ed25519", Fingerprint: "SHA256:first", PublicKey: []byte{1, 2, 3}}
	if trusted, err := verifier.Check(context.Background(), key); trusted || err != nil {
		test.Fatal("unknown key treated as trusted", err)
	}
	if store.key != nil {
		test.Fatal("check recorded trust")
	}
	if err := verifier.Accept(context.Background(), key); err != nil {
		test.Fatal(err)
	}
	if trusted, err := verifier.Check(context.Background(), key); !trusted || err != nil {
		test.Fatal("accepted key not trusted", err)
	}
	key.PublicKey = []byte{4, 5, 6}
	if _, err := verifier.Check(context.Background(), key); fault.As(err).Code != "host_key_changed" {
		test.Fatal("changed key not blocked", err)
	}
	store.err = errors.New("unreadable store")
	if _, err := verifier.Check(context.Background(), key); fault.As(err).Code != "trust_store_unavailable" {
		test.Fatal("trust store did not fail closed", err)
	}
}

func waitFor(test *testing.T, registry *Attempts, owner, identifier string, version int64, state string) Attempt {
	test.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		view, err := registry.Wait(context.Background(), owner, identifier, version)
		if err != nil {
			test.Fatal(err)
		}
		if view.State == state {
			return view
		}
		version = view.Version
	}
	test.Fatalf("attempt never reached %s", state)
	return Attempt{}
}

func TestAttemptPromptsAreOwnerBoundAndSingleUse(test *testing.T) {
	registry := NewAttempts(context.Background())
	view, err := registry.Start("owner", "terminal", func(ctx context.Context, interaction Interaction) (any, error) {
		if err := interaction.ConfirmHostKey(ctx, Prompt{Hop: "target", Fingerprint: "SHA256:x"}); err != nil {
			return nil, err
		}
		answers, err := interaction.Respond(ctx, Prompt{Hop: "target", Questions: []Question{{Text: "Code: "}}})
		if err != nil {
			return nil, err
		}
		return answers[0], nil
	})
	if err != nil {
		test.Fatal(err)
	}
	if _, err := registry.Wait(context.Background(), "intruder", view.ID, 0); fault.As(err).Code != "attempt_not_found" {
		test.Fatal("other owner could observe attempt")
	}
	prompt := waitFor(test, registry, "owner", view.ID, 0, AttemptPrompt)
	if prompt.Prompt.Kind != PromptHostKey {
		test.Fatalf("unexpected prompt %+v", prompt.Prompt)
	}
	if err := registry.Answer("intruder", view.ID, Answer{PromptID: prompt.Prompt.ID, Accept: true}); err == nil {
		test.Fatal("other owner answered")
	}
	if err := registry.Answer("owner", view.ID, Answer{PromptID: "stale", Accept: true}); fault.As(err).Code != "prompt_expired" {
		test.Fatal("stale prompt accepted")
	}
	if err := registry.Answer("owner", view.ID, Answer{PromptID: prompt.Prompt.ID, Accept: true}); err != nil {
		test.Fatal(err)
	}
	if err := registry.Answer("owner", view.ID, Answer{PromptID: prompt.Prompt.ID, Accept: true}); err == nil {
		test.Fatal("prompt answered twice")
	}
	challenge := waitFor(test, registry, "owner", view.ID, prompt.Version, AttemptPrompt)
	if challenge.Prompt.Kind != PromptChallenge {
		test.Fatalf("unexpected prompt %+v", challenge.Prompt)
	}
	if err := registry.Answer("owner", view.ID, Answer{PromptID: challenge.Prompt.ID, Accept: true}); fault.As(err).Code != "invalid_answer" {
		test.Fatal("missing responses accepted")
	}
	if err := registry.Answer("owner", view.ID, Answer{PromptID: challenge.Prompt.ID, Accept: true, Responses: []string{"246810"}}); err != nil {
		test.Fatal(err)
	}
	done := waitFor(test, registry, "owner", view.ID, challenge.Version, AttemptConnected)
	if done.Result != "246810" {
		test.Fatalf("unexpected result %+v", done)
	}
}

func TestAttemptRejectionAndCancellation(test *testing.T) {
	registry := NewAttempts(context.Background())
	rejected, _ := registry.Start("owner", "terminal", func(ctx context.Context, interaction Interaction) (any, error) {
		return nil, interaction.ConfirmHostKey(ctx, Prompt{})
	})
	prompt := waitFor(test, registry, "owner", rejected.ID, 0, AttemptPrompt)
	registry.Answer("owner", rejected.ID, Answer{PromptID: prompt.Prompt.ID, Accept: false})
	failed := waitFor(test, registry, "owner", rejected.ID, prompt.Version, AttemptFailed)
	if failed.Error.Code != "host_key_rejected" {
		test.Fatal(failed.Error)
	}
	blocked := make(chan struct{})
	cancelled, _ := registry.Start("owner", "terminal", func(ctx context.Context, interaction Interaction) (any, error) {
		defer close(blocked)
		_, err := interaction.Respond(ctx, Prompt{Questions: []Question{{Text: "Password: "}}})
		return nil, err
	})
	waitFor(test, registry, "owner", cancelled.ID, 0, AttemptPrompt)
	if err := registry.Cancel("owner", cancelled.ID); err != nil {
		test.Fatal(err)
	}
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		test.Fatal("cancellation did not unblock the transport")
	}
	if view := waitFor(test, registry, "owner", cancelled.ID, 0, AttemptCancelled); view.Error == nil {
		test.Fatal("cancelled attempt has no reason")
	}
}

func TestTargetRequiresUserForInteractiveHops(test *testing.T) {
	target := Target{Host: hosts.Profile{AuthMode: hosts.AuthPassword}}
	if target.RequiresUser() {
		test.Fatal("password host marked interactive")
	}
	target.Jump = &Hop{Host: hosts.Profile{AuthMode: hosts.AuthKeyboardInteractive}}
	if !target.RequiresUser() || len(target.Hops()) != 2 {
		test.Fatal("interactive jump host not detected")
	}
}

type released struct{ done chan struct{} }

func (result released) Release() { close(result.done) }

func TestCancelAfterSuccessReleasesTheResult(test *testing.T) {
	registry := NewAttempts(context.Background())
	proceed := make(chan struct{})
	result := released{done: make(chan struct{})}
	view, _ := registry.Start("owner", "terminal", func(context.Context, Interaction) (any, error) {
		<-proceed
		return result, nil
	})
	registry.Cancel("owner", view.ID)
	close(proceed)
	select {
	case <-result.done:
	case <-time.After(5 * time.Second):
		test.Fatal("late success after cancel was not released")
	}
	if final := waitFor(test, registry, "owner", view.ID, 0, AttemptCancelled); final.Result != nil {
		test.Fatal("cancelled attempt exposed a result")
	}
}
