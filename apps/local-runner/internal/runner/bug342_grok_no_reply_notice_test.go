package runner

import (
	"context"
	"strings"
	"testing"
)

// BUG-342 (live run-464841, grok-4.5, scan posture): after the read-only
// posture auto-denies a Grok tool, Grok cancels the WHOLE prompt —
// session/prompt_complete has agentResult:null, stopReason=cancelled,
// cancellationCategory=PermissionRejected, and no answer text exists anywhere.
// The Grok terminal handler only special-cased refusal/error, so cancelled
// fell through to a blank EventTurnCompleted — a blank bubble (the exact
// BUG-341 shape opencode fixed via CA-713). Unlike opencode there is no
// session/load replay to recover from: the fix is ONLY the notice layer.
//
// The deny→notice path keys on: (a) a FlowPilot deny marked this turn,
// (b) stopReason=cancelled, (c) empty final text. A user-initiated cancel
// (no deny) or a turn that streamed text keeps today's behavior.

func bug342LastNotice(b *fakeGrokBridge) (delta string, final string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ev := range b.events {
		if ev.Type == EventMessageDelta && strings.HasPrefix(ev.Text, "[no reply text]") {
			delta = ev.Text
		}
	}
	for _, ev := range b.events {
		if ev.Type == EventTurnCompleted {
			final = ev.FinalMessage
		}
	}
	return delta, final
}

func bug342HasFailedEvent(b *fakeGrokBridge) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ev := range b.events {
		if ev.Type == EventTurnFailed {
			return true
		}
	}
	return false
}

// bug342Adapter builds a bare grokAdapter with the deny-tracking map wired —
// emitTerminal and the mark/query helpers need no dispatcher, so no fake grok
// server is required.
func bug342Adapter() *grokAdapter {
	return &grokAdapter{permissionDenied: map[string]string{}}
}

func TestGrokNoReplyNoticeWhenCancelledAfterDeniedPermission(t *testing.T) {
	a := bug342Adapter()
	bridge := &fakeGrokBridge{}
	sessionID := "bug342-scan-deny"
	a.markGrokPermissionDenied(sessionID, "run_terminal_command")

	err := a.emitTerminal(context.Background(), TurnRequest{RunID: "run-464841"}, bridge, sessionID,
		map[string]any{"stopReason": "cancelled", "cancellationCategory": "PermissionRejected"}, "")
	if err != nil {
		t.Fatalf("emitTerminal: %v", err)
	}
	delta, final := bug342LastNotice(bridge)
	if !strings.HasPrefix(delta, "[no reply text]") {
		t.Fatalf("expected no-reply notice delta, got %q", delta)
	}
	if !strings.Contains(delta, "run_terminal_command") {
		t.Fatalf("notice should name the blocked tool, got %q", delta)
	}
	if final != delta {
		t.Fatalf("FinalMessage = %q, want notice %q", final, delta)
	}
	if bug342HasFailedEvent(bridge) {
		t.Fatal("denied-cancelled turn must be EventTurnCompleted with notice, not EventTurnFailed")
	}
}

func TestGrokNoReplyNoticeCancelledWithoutDenyKeepsBlankTurn(t *testing.T) {
	a := bug342Adapter()
	bridge := &fakeGrokBridge{}
	sessionID := "bug342-user-cancel"

	// No markGrokPermissionDenied — a genuine user stop / provider cancel.
	err := a.emitTerminal(context.Background(), TurnRequest{}, bridge, sessionID,
		map[string]any{"stopReason": "cancelled"}, "")
	if err != nil {
		t.Fatalf("emitTerminal: %v", err)
	}
	delta, final := bug342LastNotice(bridge)
	if delta != "" {
		t.Fatalf("cancelled WITHOUT deny must not emit a notice, got %q", delta)
	}
	if final != "" {
		t.Fatalf("cancelled without deny keeps today's blank FinalMessage, got %q", final)
	}
}

func TestGrokNoReplyNoticeTextWinsWhenStreamed(t *testing.T) {
	a := bug342Adapter()
	bridge := &fakeGrokBridge{}
	sessionID := "bug342-text-wins"
	a.markGrokPermissionDenied(sessionID, "write")

	// lastText non-empty (assistant streamed something after all) — text wins.
	err := a.emitTerminal(context.Background(), TurnRequest{}, bridge, sessionID,
		map[string]any{"stopReason": "cancelled"}, "partial answer here")
	if err != nil {
		t.Fatalf("emitTerminal: %v", err)
	}
	delta, final := bug342LastNotice(bridge)
	if delta != "" {
		t.Fatalf("deny + streamed text must not emit a notice, got %q", delta)
	}
	if final != "partial answer here" {
		t.Fatalf("FinalMessage = %q, want the streamed text", final)
	}

	// Same via the result payload carrying text.
	bridge2 := &fakeGrokBridge{}
	err = a.emitTerminal(context.Background(), TurnRequest{}, bridge2, sessionID,
		map[string]any{"stopReason": "cancelled", "text": "answer in result"}, "")
	if err != nil {
		t.Fatalf("emitTerminal: %v", err)
	}
	delta2, final2 := bug342LastNotice(bridge2)
	if delta2 != "" || final2 != "answer in result" {
		t.Fatalf("deny + result text must win: delta=%q final=%q", delta2, final2)
	}
}

func TestGrokNoReplyNoticeRefusalAndErrorUnchanged(t *testing.T) {
	a := bug342Adapter()
	sessionID := "bug342-refusal"
	a.markGrokPermissionDenied(sessionID, "write")

	for _, reason := range []string{"refusal", "error"} {
		bridge := &fakeGrokBridge{}
		if err := a.emitTerminal(context.Background(), TurnRequest{}, bridge, sessionID,
			map[string]any{"stopReason": reason}, ""); err != nil {
			t.Fatalf("emitTerminal(%s): %v", reason, err)
		}
		if !bug342HasFailedEvent(bridge) {
			t.Fatalf("stopReason=%s must stay EventTurnFailed", reason)
		}
		if delta, _ := bug342LastNotice(bridge); delta != "" {
			t.Fatalf("stopReason=%s must not emit a notice, got %q", reason, delta)
		}
	}
}

func TestGrokPermissionDeniedMarkAndQuery(t *testing.T) {
	a := bug342Adapter()
	sessionID := "bug342-marks"
	if denied, _ := a.grokPermissionDeniedFor(sessionID); denied {
		t.Fatal("no deny marked yet — must report not denied")
	}
	a.markGrokPermissionDenied(sessionID, "rm foo")
	denied, tool := a.grokPermissionDeniedFor(sessionID)
	if !denied || tool != "rm foo" {
		t.Fatalf("after mark: denied=%v tool=%q, want (true, \"rm foo\")", denied, tool)
	}
	// A different session is not denied.
	if d2, _ := a.grokPermissionDeniedFor("bug342-other"); d2 {
		t.Fatal("other session must not be denied")
	}
}
