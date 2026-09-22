package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// BUG-361: OpenCode quota/billing signals must fail the turn with a stable
// usage-limit event instead of hanging (prompt never returns) or
// blank-completing (unknown stopReason → Completed). New file; no
// pre-existing test is modified.

func TestBug361ClassifierQuotaTokens(t *testing.T) {
	mustQuota := []string{
		// Pre-existing tokens (locked, not added here).
		"usage limit reached", "extra usage unavailable", "out of credits",
		"out_of_credits", "quota reset", "rate limit",
		"personal-team-blocked", "spending-limit", "spending_limit",
		// BUG-361 ACP shapes.
		"request failed: rate_limited", "Rate-Limit exceeded, retry later",
		"RATE_LIMITED", "usage_limit exceeded", "USAGE-LIMIT",
		"quota exceeded for model", "QUOTA_EXCEEDED",
		"insufficient credit balance", "insufficient_credit",
		"payment required", "payment_required",
		// Live ACP probe 2026-09-07, opencode 1.18.29 gpt-5.4-nano:
		// JSON-RPC -32603 (jsonRpcErrorMessage keeps only `message`).
		"Internal error: No payment method. Add a payment method here: https://opencode.ai/workspace/wrk_01M09SAZWWH08C6K4N7NDH028T/billing",
		"no payment method",
		// Full RPC error strings.
		`session/prompt: -32000: quota_exceeded: model over quota`,
	}
	for _, msg := range mustQuota {
		if !isProviderUsageLimitError(errors.New(msg)) {
			t.Errorf("isProviderUsageLimitError(%q) = false, want true", msg)
		}
	}
	mustHealthy := []string{
		"network timeout", "exit status 1", "context canceled",
		"opencode session/prompt returned no result",
		"session/load returned no sessionId", "",
		"Internal error: boom",
	}
	for _, msg := range mustHealthy {
		var err error
		if msg != "" {
			err = errors.New(msg)
		}
		if isProviderUsageLimitError(err) {
			t.Errorf("isProviderUsageLimitError(%q) = true, want false", msg)
		}
	}
}

func TestBug361OpencodeQuotaStopReason(t *testing.T) {
	mustFail := []string{
		"rate_limited", "rate-limited", "quota_exceeded", "usage_limit",
		"billing", "payment_required", "insufficient_credits",
		"  QUOTA_EXCEEDED  ",
	}
	for _, reason := range mustFail {
		if got := opencodeStopReasonToEvent(reason); got != EventTurnFailed {
			t.Errorf("opencodeStopReasonToEvent(%q) = %v, want turn_failed", reason, got)
		}
		if !opencodeIsQuotaStopReason(reason) {
			t.Errorf("opencodeIsQuotaStopReason(%q) = false, want true", reason)
		}
	}
	mustComplete := []string{"end_turn", "stop", "completed", "success", "", "max_tokens", "refusal", "cancelled", "weird_future_reason"}
	for _, reason := range mustComplete {
		if got := opencodeStopReasonToEvent(reason); got != EventTurnCompleted {
			t.Errorf("opencodeStopReasonToEvent(%q) = %v, want turn_completed (default unchanged)", reason, got)
		}
		if opencodeIsQuotaStopReason(reason) {
			t.Errorf("opencodeIsQuotaStopReason(%q) = true, want false", reason)
		}
	}
	// Previously mapped failures stay failed.
	for _, reason := range []string{"error", "failed", "aborted"} {
		if got := opencodeStopReasonToEvent(reason); got != EventTurnFailed {
			t.Errorf("opencodeStopReasonToEvent(%q) = %v, want turn_failed", reason, got)
		}
	}
}

func bug361ServeQuotaRPCError(fg *fakeOpencode, message string) func(fg *fakeOpencode, m map[string]any) {
	return func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": "ses_quota"})
		case "session/prompt":
			fg.send(map[string]any{"jsonrpc": "2.0", "id": m["id"],
				"error": map[string]any{"code": -32000, "message": message}})
		}
	}
}

func bug361TurnFailedEvents(events []ProviderEvent) []ProviderEvent {
	var out []ProviderEvent
	for _, ev := range events {
		if ev.Type == EventTurnFailed {
			out = append(out, ev)
		}
	}
	return out
}

func TestBug361OpencodeSendTurnQuotaRPCErrorFailsFast(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	fg.serve(bug361ServeQuotaRPCError(fg, "session/prompt failed: rate_limited: model over quota"))
	a := newOpencodeAdapter(d, t.TempDir())
	a.sessionStore = &recordingSessionStore{}
	bridge := &fakeOpencodeBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.SendTurn(ctx, TurnRequest{RunID: "run-361", Prompt: "hi", Cwd: t.TempDir()}, bridge); err != nil {
		t.Fatalf("quota RPC error must be consumed into a terminal event, got err: %v", err)
	}
	failed := bug361TurnFailedEvents(bridge.events)
	if len(failed) != 1 {
		t.Fatalf("want exactly 1 turn_failed, got %+v", bridge.events)
	}
	if !strings.Contains(strings.ToLower(failed[0].Error), "usage limit") {
		t.Fatalf("turn_failed error = %q, want stable usage-limit copy", failed[0].Error)
	}
	if failed[0].Recoverable {
		t.Fatal("quota turn_failed must not be recoverable (no retry loop)")
	}
	for _, ev := range bridge.events {
		if ev.Type == EventTurnCompleted {
			t.Fatalf("quota turn must not complete: %+v", bridge.events)
		}
	}
}

func TestBug361OpencodeSendTurnNoPaymentMethodFailsFast(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	fg.serve(bug361ServeQuotaRPCError(fg, "Internal error: No payment method. Add a payment method here: https://opencode.ai/workspace/wrk_01M09SAZWWH08C6K4N7NDH028T/billing"))
	a := newOpencodeAdapter(d, t.TempDir())
	a.sessionStore = &recordingSessionStore{}
	bridge := &fakeOpencodeBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := a.SendTurn(ctx, TurnRequest{RunID: "run-361-pay", Prompt: "hi", Cwd: t.TempDir()}, bridge); err != nil {
		t.Fatalf("no-payment-method RPC must be consumed into a terminal event, got err: %v", err)
	}
	failed := bug361TurnFailedEvents(bridge.events)
	if len(failed) != 1 {
		t.Fatalf("want exactly 1 turn_failed, got %+v", bridge.events)
	}
	if !strings.Contains(strings.ToLower(failed[0].Error), "usage limit") {
		t.Fatalf("turn_failed error = %q, want stable usage-limit copy", failed[0].Error)
	}
	if failed[0].Recoverable {
		t.Fatal("billing turn_failed must not be recoverable")
	}
}

func TestBug361OpencodeSendTurnNonQuotaRPCErrorUnchanged(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	fg.serve(bug361ServeQuotaRPCError(fg, "boom"))
	a := newOpencodeAdapter(d, t.TempDir())
	a.sessionStore = &recordingSessionStore{}
	bridge := &fakeOpencodeBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := a.SendTurn(ctx, TurnRequest{RunID: "run-361", Prompt: "hi", Cwd: t.TempDir()}, bridge)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("non-quota RPC error must still return err, got %v (events=%+v)", err, bridge.events)
	}
	if len(bug361TurnFailedEvents(bridge.events)) != 0 {
		t.Fatalf("non-quota error must not emit turn_failed itself: %+v", bridge.events)
	}
}

func TestBug361OpencodeSendTurnQuotaStopReasonFails(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": "ses_quota_sr"})
		case "session/prompt":
			go func() {
				fg.notify("session/update", map[string]any{"sessionId": "ses_quota_sr", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "partial"}}})
				time.Sleep(10 * time.Millisecond)
				fg.reply(m["id"], map[string]any{"stopReason": "rate_limited"})
			}()
		}
	})
	a := newOpencodeAdapter(d, t.TempDir())
	a.sessionStore = &recordingSessionStore{}
	bridge := &fakeOpencodeBridge{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.SendTurn(ctx, TurnRequest{RunID: "run-361", Prompt: "hi", Cwd: t.TempDir()}, bridge); err != nil {
		t.Fatalf("quota stopReason must be consumed into a terminal event, got err: %v", err)
	}
	failed := bug361TurnFailedEvents(bridge.events)
	if len(failed) != 1 {
		t.Fatalf("want exactly 1 turn_failed, got %+v", bridge.events)
	}
	if !strings.Contains(strings.ToLower(failed[0].Error), "usage limit") {
		t.Fatalf("turn_failed error = %q, want stable usage-limit copy", failed[0].Error)
	}
	if failed[0].Recoverable {
		t.Fatal("quota stopReason turn_failed must not be recoverable (no retry loop)")
	}
	for _, ev := range bridge.events {
		if ev.Type == EventTurnCompleted {
			t.Fatalf("quota stopReason must not complete: %+v", bridge.events)
		}
	}
}
