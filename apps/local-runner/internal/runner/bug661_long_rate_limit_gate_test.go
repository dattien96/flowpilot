package runner

import (
	"context"
	"errors"
	"testing"
	"time"
)

// BUG-661: a rate_limited provider failure whose RetryAfter exceeds the
// Task-445 in-turn retry bound (or survives every send attempt) is
// terminal-for-now — it must enter the post-failure quota routing gate and
// mark the account blocked, never silently terminal-fail the run.
//
// Two delivery paths must both reach the gate:
//   - typed error: SendTurn returns *providerLimitError → finishTurn's
//     post-failure switch (F-1).
//   - adapter-emitted: the adapter emits provider_limit_reached + turn_failed
//     through the bridge and returns nil (devin/opencode non-recoverable
//     path — the live run-616108/run-644825 shape) — finishTurn's error
//     branch never sees it, so the settled terminal must trigger the gate
//     at the bridge seam (F-5).

func bug661Service(t *testing.T) *InteractiveService {
	t.Helper()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cx-1", "codex", 1, false),
	})
	s := task448Service(t, ProviderKeyCodex)
	task449WriteSettings(t, QuotaRoutingSettings{Mode: QuotaRotationManual})
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cx-0": 0, "cx-1": 90}, time.Now().UTC().Format(time.RFC3339))
	return s
}

func bug661Questions(t *testing.T, s *InteractiveService, runID string) []*questionRecord {
	t.Helper()
	waitFor(t, func() bool { return len(task449Questions(s, runID)) == 1 }, "quota route card")
	return task449Questions(s, runID)
}

// Live-repro path: adapter emits the limit + terminal failure via the bridge
// and returns nil — finishTurn's error branch is never entered.
func TestBug661_AdapterEmittedRateLimitEntersQuotaGate(t *testing.T) {
	s := bug661Service(t)
	_, child := task449FlowChild(t, s, "cx-0")
	bridge := &turnBridge{svc: s, rs: child, ctx: context.Background(), turnID: "turn-1"}

	bridge.Emit(ProviderEvent{Type: EventProviderLimitReached, ProviderLimit: &ProviderLimit{
		Kind:              ProviderLimitRateLimited,
		ProviderKey:       ProviderKeyCodex,
		AccountID:         "cx-0",
		RetryAfterSeconds: 2220,
	}})
	bridge.Emit(ProviderEvent{Type: EventTurnFailed, Error: "usage limit reached", Recoverable: false})

	qs := bug661Questions(t, s, child.id)
	if questionRecordKind(qs[0]) != quotaRouteQuestionKind {
		t.Fatalf("expected quota route card, got %q", questionRecordKind(qs[0]))
	}
}

// Typed-error path: an exhausted/long rate limit returned from SendTurn must
// reach the gate through finishTurn's post-failure switch.
func TestBug661_FinishTurnRateLimitEntersQuotaGate(t *testing.T) {
	s := bug661Service(t)
	_, child := task449FlowChild(t, s, "cx-0")
	child.events = append(child.events, ProviderEvent{Type: EventTurnStarted, ProviderTurnID: "turn-1"})

	limit := &ProviderLimit{
		Kind:              ProviderLimitRateLimited,
		ProviderKey:       ProviderKeyCodex,
		AccountID:         "cx-0",
		RetryAfterSeconds: 2220,
	}
	_, _ = s.finishTurn(child, "turn-1", &providerLimitError{limit: limit, err: errors.New("rate limited")})

	qs := bug661Questions(t, s, child.id)
	if questionRecordKind(qs[0]) != quotaRouteQuestionKind {
		t.Fatalf("expected quota route card, got %q", questionRecordKind(qs[0]))
	}
}

// Task-445 invariant: a bounded Retry-After rate limit is still in-turn
// retryable — the gate belongs to terminal failures only.
func TestBug661_TransientRateLimitStillInTurnRetried(t *testing.T) {
	limit := &ProviderLimit{
		Kind:              ProviderLimitRateLimited,
		RetryAfterSeconds: 5,
	}
	if !providerLimitRecoverable(*limit) {
		t.Fatal("bounded Retry-After rate limit must stay in-turn recoverable")
	}
	if !isRecoverableSendError(&providerLimitError{limit: limit, err: errors.New("rate limited")}) {
		t.Fatal("send loop must keep retrying a bounded rate limit")
	}
}

// Near-miss: even a SHORT Retry-After reaches the gate once it is delivered
// as a terminal failure — exhaustion of in-turn retries is itself a quota
// signal (V-3).
func TestBug661_ExhaustedShortRateLimitEntersGate(t *testing.T) {
	s := bug661Service(t)
	_, child := task449FlowChild(t, s, "cx-0")
	child.events = append(child.events, ProviderEvent{Type: EventTurnStarted, ProviderTurnID: "turn-1"})

	limit := &ProviderLimit{
		Kind:              ProviderLimitRateLimited,
		ProviderKey:       ProviderKeyCodex,
		AccountID:         "cx-0",
		RetryAfterSeconds: 5,
	}
	_, _ = s.finishTurn(child, "turn-1", &providerLimitError{limit: limit, err: errors.New("rate limited")})

	qs := bug661Questions(t, s, child.id)
	if questionRecordKind(qs[0]) != quotaRouteQuestionKind {
		t.Fatalf("exhausted transient rate limit must enter the gate, got %q", questionRecordKind(qs[0]))
	}
}

// F-3: the just-failed account enters the durable block ledger so rotation
// cannot re-pick it during the reset window (both delivery paths).
func TestBug661_RateLimitedAccountBlocked(t *testing.T) {
	s := bug661Service(t)
	_, child := task449FlowChild(t, s, "cx-0")
	bridge := &turnBridge{svc: s, rs: child, ctx: context.Background(), turnID: "turn-1"}

	bridge.Emit(ProviderEvent{Type: EventProviderLimitReached, ProviderLimit: &ProviderLimit{
		Kind:              ProviderLimitRateLimited,
		ProviderKey:       ProviderKeyCodex,
		AccountID:         "cx-0",
		RetryAfterSeconds: 2220,
	}})
	bridge.Emit(ProviderEvent{Type: EventTurnFailed, Error: "usage limit reached", Recoverable: false})
	bug661Questions(t, s, child.id)

	s.mu.Lock()
	state := s.loadQuotaRuntimeState()
	s.mu.Unlock()
	for _, b := range state.Blocked {
		if b.AccountID == "cx-0" && b.Reason == string(ProviderLimitRateLimited) {
			return
		}
	}
	t.Fatalf("rate-limited account not recorded blocked: %+v", state.Blocked)
}

// Degraded input: a turn_failed with NO preceding limit must not enter the
// gate (no phantom parks on ordinary failures).
func TestBug661_UnrelatedFailureDoesNotGate(t *testing.T) {
	s := bug661Service(t)
	_, child := task449FlowChild(t, s, "cx-0")
	bridge := &turnBridge{svc: s, rs: child, ctx: context.Background(), turnID: "turn-1"}

	bridge.Emit(ProviderEvent{Type: EventTurnFailed, Error: "boom", Recoverable: false})

	// Gate is async; give it a beat then assert nothing parked.
	time.Sleep(200 * time.Millisecond)
	if qs := task449Questions(s, child.id); len(qs) != 0 {
		t.Fatalf("ordinary failure must not park a quota card, got %d", len(qs))
	}
}

// Ordering: a limit from a PREVIOUS turn must not fire the gate on the next
// turn's unrelated failure — correlation is per-turn.
func TestBug661_StaleLimitDoesNotGateNextTurn(t *testing.T) {
	s := bug661Service(t)
	_, child := task449FlowChild(t, s, "cx-0")
	bridge := &turnBridge{svc: s, rs: child, ctx: context.Background(), turnID: "turn-1"}

	bridge.Emit(ProviderEvent{Type: EventProviderLimitReached, ProviderLimit: &ProviderLimit{
		Kind:              ProviderLimitRateLimited,
		ProviderKey:       ProviderKeyCodex,
		AccountID:         "cx-0",
		RetryAfterSeconds: 2220,
	}})
	// Turn 1 does not fail (transient resolved mid-stream); turn 2 fails plain.
	bridge2 := &turnBridge{svc: s, rs: child, ctx: context.Background(), turnID: "turn-2"}
	bridge2.Emit(ProviderEvent{Type: EventTurnFailed, Error: "boom", Recoverable: false})

	time.Sleep(200 * time.Millisecond)
	if qs := task449Questions(s, child.id); len(qs) != 0 {
		t.Fatalf("stale limit must not gate an unrelated turn failure, got %d cards", len(qs))
	}
}
