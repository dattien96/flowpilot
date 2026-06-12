package runner

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// flakyAdapter fails with a recoverable error for the first failUntil attempts, then
// completes the turn.
type flakyAdapter struct {
	mu        sync.Mutex
	attempts  int
	failUntil int
	failErr   error
}

func (a *flakyAdapter) Key() ProviderKey                   { return ProviderKeyCodex }
func (a *flakyAdapter) Capabilities() ProviderCapabilities { return ProviderCapabilities{} }
func (a *flakyAdapter) SendTurn(_ context.Context, _ TurnRequest, b TurnBridge) error {
	a.mu.Lock()
	a.attempts++
	n := a.attempts
	a.mu.Unlock()
	if n <= a.failUntil {
		return a.failErr
	}
	b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
	return nil
}

func registryWithAdapter(a ProviderRuntimeAdapter) *ProviderRegistry {
	r := newProviderRegistry()
	r.register(ProviderRegistration{
		Key: ProviderKeyCodex, DisplayName: "Codex", Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return a },
	})
	return r
}

// send-with-retry re-sends a turn after a recoverable error and ultimately completes.
func TestSendTurnWithRetryRecovers(t *testing.T) {
	a := &flakyAdapter{failUntil: 2, failErr: errors.New("codex app-server stream closed mid-turn")}
	svc := NewInteractiveServiceWithRegistry(registryWithAdapter(a))
	bridge := &captureBridge{}
	err := svc.sendTurnWithRetry(context.Background(), a, TurnRequest{RunID: "r1", Prompt: "hi"}, bridge)
	if err != nil {
		t.Fatalf("expected recovery, got %v", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.attempts != 3 {
		t.Fatalf("attempts = %d, want 3 (2 failures + 1 success)", a.attempts)
	}
}

// a non-recoverable error (interrupt / expiry) is NOT retried.
func TestSendTurnWithRetryDoesNotRetryTerminal(t *testing.T) {
	a := &flakyAdapter{failUntil: 5, failErr: context.Canceled}
	svc := NewInteractiveServiceWithRegistry(registryWithAdapter(a))
	err := svc.sendTurnWithRetry(context.Background(), a, TurnRequest{RunID: "r1"}, &captureBridge{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled passed through, got %v", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (terminal error not retried)", a.attempts)
	}

	exp := &flakyAdapter{failUntil: 5, failErr: errApprovalExpired}
	svc2 := NewInteractiveServiceWithRegistry(registryWithAdapter(exp))
	if err := svc2.sendTurnWithRetry(context.Background(), exp, TurnRequest{}, &captureBridge{}); !errors.Is(err, errApprovalExpired) {
		t.Fatalf("approval expiry should pass through, got %v", err)
	}
	if exp.attempts != 1 {
		t.Fatalf("expiry attempts = %d, want 1", exp.attempts)
	}
}

// retry gives up after maxTurnAttempts and returns the last error.
func TestSendTurnWithRetryGivesUp(t *testing.T) {
	a := &flakyAdapter{failUntil: 99, failErr: errors.New("persistent failure")}
	svc := NewInteractiveServiceWithRegistry(registryWithAdapter(a))
	err := svc.sendTurnWithRetry(context.Background(), a, TurnRequest{}, &captureBridge{})
	if err == nil || err.Error() != "persistent failure" {
		t.Fatalf("expected persistent failure after retries, got %v", err)
	}
	if a.attempts != svc.maxTurnAttempts {
		t.Fatalf("attempts = %d, want maxTurnAttempts %d", a.attempts, svc.maxTurnAttempts)
	}
}
