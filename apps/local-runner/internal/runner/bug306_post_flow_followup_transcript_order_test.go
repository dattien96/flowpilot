package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// BUG-306: after a flow finishes, the FIRST follow-up user turn is sent to the
// provider wrapped in composeAgentContextBlock's "[FlowPilot system note —
// sub-agents …]" prefix (BUG-122), so the provider session file persists the
// wrapped text as that user turn. On restart, overlayRawTurnPrompts classified
// the wrapped slot as a pure system prompt and skipped it, and its front-to-back
// positional match also could not account for the suppressed hub turn-1 (which
// has no provider frame, BUG-300). The two together mis-aligned the overlay:
// prompt-1's text landed on the last follow-up's slot (rendered at the BOTTOM)
// and the follow-ups were prepended to the TOP. Confirmed live on run-18371
// (CP-51 A10): the restored transcript showed follow-up prompts first and the
// original "fix bug 1+1 != 2" last.
//
// cross-provider-parity: Case 1/2. overlayRawTurnPrompts is the shared overlay
// for Claude and Codex, and Grok's overlayRawGrokTurnPrompts delegates straight
// to it — one representative provider (Claude) exercises the exact fix path for
// all three. isSystemPrompt is deliberately left unchanged so BUG-300 / run1264
// (which keep an agent-context-WRAPPED join note internal) stay green.
//
// additive-tests-only: this file only adds a new test; no existing test file is
// modified.
func TestPostFlowFollowUpTranscriptKeepsPromptOrderOnRestart(t *testing.T) {
	root := t.TempDir()
	acctHome := filepath.Join(root, "claude-home")
	sessionID := "claude-hub-session-306"
	claudeDir := filepath.Join(acctHome, ".claude", "projects", "project")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// A round-0 synthesis reinvoke that ALSO carries the agent-context prefix —
	// this is the ambiguous shape BUG-300/run1264 keep internal (must stay hidden).
	joinedNote := composeAgentContextBlock([]string{"[flow-engine] An agent has already been spawned to work on this request."}) +
		"\n\n[flow-engine joined result note]\nFlow round 0 — 1 result joined.\n\"reviewer\" (claude): APPROVE\n---\n" +
		"[flow-engine] Agent results ready. You must call submit_review_outcome."

	// The first post-flow follow-up: a GENUINE user prompt wrapped in the exact
	// same agent-context prefix (this is what the runtime persists to the .jsonl).
	rawFollowUp1 := "hello bug này fix gì vậy"
	wrappedFollowUp1 := composeAgentContextBlock([]string{"Flow completed."}) + "\n\n" + rawFollowUp1
	rawFollowUp2 := "Ý là code ok rồi hả"

	// The session file's user frames: synthesis reinvoke, wrapped follow-up, bare
	// follow-up. The original "fix bug 1+1 != 2" prompt has NO frame (hub turn-1
	// spawned children without ever calling the provider).
	writeLinesToPath083(t, filepath.Join(claudeDir, sessionID+".jsonl"), []string{
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":` + jsonQuote083(joinedNote) + `}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Round 0 requested changes."}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":` + jsonQuote083(wrappedFollowUp1) + `}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"There was no real bug to fix."}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":` + jsonQuote083(rawFollowUp2) + `}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Correct, calc.go is already right."}]}}`,
	})

	// Turn log records every user prompt (raw) in order, plus the internal join note.
	store := newRun2334TurnLogStore(t, root, "run-306", []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: "fix bug 1+1 != 2"},
		{Kind: turnLogKindPrompt, TurnID: "turn-2", Prompt: joinedNote},
		{Kind: turnLogKindPrompt, TurnID: "turn-3", Prompt: rawFollowUp1},
		{Kind: turnLogKindPrompt, TurnID: "turn-4", Prompt: rawFollowUp2},
	})
	writeProviderAccountsConfig083(t, root, []ProviderAccount{{
		ID: "acct-claude", ProviderKey: string(ProviderKeyClaude), HomePath: acctHome,
		SlotIndex: 1, AuthStatus: "connected", IsActive: true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := newRun2334ResumedChat("run-306", ProviderKeyClaude, "acct-claude", sessionID)
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	// Collect user-facing prompt bubbles in transcript order.
	var promptOrder []string
	for _, e := range rs.events {
		if e.Type != EventTurnStarted || e.Prompt == "" {
			continue
		}
		// No wrapped/orchestration prompt may surface as a user bubble.
		if isSystemPrompt(e.Prompt) {
			t.Fatalf("internal/orchestration prompt replayed as a user-facing bubble: %q", e.Prompt)
		}
		promptOrder = append(promptOrder, e.Prompt)
	}

	want := []string{"fix bug 1+1 != 2", rawFollowUp1, rawFollowUp2}
	if len(promptOrder) != len(want) {
		t.Fatalf("prompt bubble count = %d %q, want %d %q", len(promptOrder), promptOrder, len(want), want)
	}
	for i := range want {
		if promptOrder[i] != want[i] {
			t.Fatalf("prompt order[%d] = %q, want %q (full order %q)", i, promptOrder[i], want[i], promptOrder)
		}
	}
	// The original flow-start prompt must be FIRST, never last (the run-18371 symptom).
	if promptOrder[0] != "fix bug 1+1 != 2" {
		t.Fatalf("original flow-start prompt is not first: %q", promptOrder)
	}
}

// TestIsOverlayableUserTurn locks down the discriminator at the heart of BUG-306:
// a genuine user turn — bare OR carrying the agent-context note prefix — is
// overlay-able, while every pure orchestration prompt is not. The critical row is
// the agent-context-WRAPPED join note, which must stay internal (BUG-300 /
// run1264): a naive "wrapper ⇒ user turn" rule would resurface it as a bubble.
func TestIsOverlayableUserTurn(t *testing.T) {
	wrappedUser := composeAgentContextBlock([]string{"Flow completed."}) + "\n\n" + "hello bug này fix gì vậy"
	wrappedJoinNote := composeAgentContextBlock([]string{"[flow-engine] An agent has already been spawned."}) +
		"\n\n[flow-engine joined result note]\nFlow round 0 — 2 results joined.\nsynthesis"
	cases := []struct {
		name   string
		prompt string
		want   bool
	}{
		{"bare user prompt", "what does this bug fix?", true},
		{"pure join note", "[flow-engine joined result note]\nFlow round 0 — 2 results joined.", false},
		{"sub-agent spawn prompt", "[FlowPilot sub-agent — reviewer] Review the coder result.", false},
		{"agent-context-wrapped user prompt", wrappedUser, true},
		{"agent-context-wrapped join note (BUG-300 trap)", wrappedJoinNote, false},
		{"malformed wrapper without closing marker", "[FlowPilot system note — sub-agents started and then truncated", false},
	}
	for _, c := range cases {
		if got := isOverlayableUserTurn(c.prompt); got != c.want {
			t.Errorf("%s: isOverlayableUserTurn = %v, want %v", c.name, got, c.want)
		}
	}

	// stripAgentContextBlock must recover the exact raw user text after the note.
	if rest, wrapped := stripAgentContextBlock(wrappedUser); !wrapped || rest != "hello bug này fix gì vậy" {
		t.Fatalf("stripAgentContextBlock(wrappedUser) = (%q,%v), want (%q,true)", rest, wrapped, "hello bug này fix gì vậy")
	}
	if _, wrapped := stripAgentContextBlock("plain user prompt"); wrapped {
		t.Fatal("stripAgentContextBlock(plain) reported wrapped=true")
	}
}

// TestPostFlowFollowUpTranscriptKeepsPromptOrderOnRestartGrok is the Grok
// counterpart of the Claude ordering test. Grok reconstructs through its own
// seedGrokTranscriptFromDisk, whose overlayRawGrokTurnPrompts delegates to the
// shared overlayRawTurnPrompts — this proves the BUG-306 fix reaches every
// provider, not just the Claude/Codex path (cross-provider-parity).
func TestPostFlowFollowUpTranscriptKeepsPromptOrderOnRestartGrok(t *testing.T) {
	root := t.TempDir()
	grokHome := filepath.Join(root, ".grok")
	cwd := `D:\working\gate-sandbox`
	sessionID := "sess-306-grok"

	joinedNote := composeAgentContextBlock([]string{"[flow-engine] An agent has already been spawned to work on this request."}) +
		"\n\n[flow-engine joined result note]\nFlow round 0 — 1 result joined.\n\"reviewer\" (grok): APPROVE\n---\n" +
		"[flow-engine] Agent results ready. You must call submit_review_outcome."
	rawFollowUp1 := "hello bug này fix gì vậy"
	wrappedFollowUp1 := composeAgentContextBlock([]string{"Flow completed."}) + "\n\n" + rawFollowUp1
	rawFollowUp2 := "Ý là code ok rồi hả"

	wrapUser := func(text string) string {
		return fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+text+"\n</user_query>")
	}
	writeGrokChatHistoryFixture(t, grokHome, cwd, sessionID, []string{
		wrapUser(joinedNote),
		`{"type":"assistant","content":"Round 0 requested changes."}`,
		wrapUser(wrappedFollowUp1),
		`{"type":"assistant","content":"There was no real bug to fix."}`,
		wrapUser(rawFollowUp2),
		`{"type":"assistant","content":"Correct, calc.go is already right."}`,
	})
	writeGrokAuthFileFixture(t, grokHome)
	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-grok", ProviderKey: "grok", HomePath: grokHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})

	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	entries := []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: "fix bug 1+1 != 2"},
		{Kind: turnLogKindPrompt, TurnID: "turn-2", Prompt: joinedNote},
		{Kind: turnLogKindPrompt, TurnID: "turn-3", Prompt: rawFollowUp1},
		{Kind: turnLogKindPrompt, TurnID: "turn-4", Prompt: rawFollowUp2},
		{Kind: turnLogKindGrokSession, TurnID: "turn-4", SessionID: sessionID},
	}
	for _, e := range entries {
		if err := store.AppendTurnLog(context.Background(), "run-306-grok", e); err != nil {
			t.Fatalf("AppendTurnLog: %v", err)
		}
	}

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{Key: ProviderKeyGrok, Status: ProviderStatusAvailable, Capabilities: ProviderCapabilities{Streaming: true}})
	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-grok"
	rs := &interactiveRun{
		id:                "run-306-grok",
		providerKey:       ProviderKeyGrok,
		providerAccountID: "acct-grok",
		workspaceCwd:      cwd,
		runKind:           "workflow",
		flowEngineDriven:  true,
		status:            RunStatusCompleted,
		resumedFromDisk:   true,
		subs:              map[int64]chan ProviderEvent{},
		idempotency:       map[string]string{},
		createdAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.seedGrokTranscriptFromDisk(rs)

	var promptOrder []string
	for _, e := range rs.events {
		if e.Type != EventTurnStarted || e.Prompt == "" {
			continue
		}
		if isSystemPrompt(e.Prompt) {
			t.Fatalf("internal/orchestration prompt replayed as a user-facing bubble: %q", e.Prompt)
		}
		promptOrder = append(promptOrder, e.Prompt)
	}
	want := []string{"fix bug 1+1 != 2", rawFollowUp1, rawFollowUp2}
	if len(promptOrder) != len(want) {
		t.Fatalf("prompt bubble count = %d %q, want %d %q", len(promptOrder), promptOrder, len(want), want)
	}
	for i := range want {
		if promptOrder[i] != want[i] {
			t.Fatalf("Grok prompt order[%d] = %q, want %q (full order %q)", i, promptOrder[i], want[i], promptOrder)
		}
	}
}
