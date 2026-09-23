package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// BUG-384: every SendTurn retry attempt re-issues session/prompt and Grok
// persists each as a <user_query> frame. On replay the surplus frames must not
// surface as phantom user turns carrying the full composed prompt.
//
// Wire shape (cp46 run-1): 2 typed prompts, 3 attempts each → 6 user frames.
// The turn log knows the 2 genuine prompts; retries re-send byte-identical
// composed text.
func TestBug384_GrokRetryFramesDoNotPolluteReplay(t *testing.T) {
	root := t.TempDir()
	grokHome := filepath.Join(root, ".grok")
	cwd := "/workspace/retry-bed"
	sessionID := "session-run-384"
	firstPrompt := "Reply with exactly: ok"
	secondPrompt := "Now say done"
	composed := func(p string) string {
		return p + "\n\n---\nWhen you need to ask the user a question, call the FlowPilot MCP tool `ask_user`…\n\n---\nWhen you need to spawn a sub-agent…"
	}
	// 3 retry attempts per logical turn: identical user_query frames, no
	// assistant reply between attempts of the failed first turn.
	writeGrokChatHistoryFixture(t, grokHome, cwd, sessionID, []string{
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+composed(firstPrompt)+"\n</user_query>"),
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+composed(firstPrompt)+"\n</user_query>"),
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+composed(firstPrompt)+"\n</user_query>"),
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+composed(secondPrompt)+"\n</user_query>"),
		`{"type":"assistant","content":"done"}`,
	})
	writeGrokAuthFileFixture(t, grokHome)
	writeProviderAccountsConfig083(t, root, []ProviderAccount{{
		ID: "acct-grok", ProviderKey: "grok", HomePath: grokHome, SlotIndex: 1,
		AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}})

	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	for _, line := range []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-3", Prompt: firstPrompt},
		{Kind: turnLogKindPrompt, TurnID: "turn-8", Prompt: secondPrompt},
		{Kind: turnLogKindGrokSession, SessionID: sessionID},
	} {
		if err := store.AppendTurnLog(context.Background(), "run-384", line); err != nil {
			t.Fatalf("AppendTurnLog: %v", err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-grok"
	rs := &interactiveRun{
		id: "run-384", providerKey: ProviderKeyGrok, providerAccountID: "acct-grok",
		workspaceCwd: cwd, runKind: "chat", status: RunStatusCompleted,
		createdAt: "2026-07-18T16:49:01Z", resumedFromDisk: true,
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	var prompts []string
	for _, event := range events {
		if event.Type == EventTurnStarted && event.Prompt != "" {
			prompts = append(prompts, event.Prompt)
			if containsFoldOpencode(event.Prompt, "FlowPilot MCP tool") {
				t.Fatalf("internal composed prompt leaked as user turn: %q", event.Prompt)
			}
		}
	}
	if len(prompts) != 2 {
		t.Fatalf("phantom retry turns leaked: got %d user turns %v, want 2", len(prompts), prompts)
	}
	if prompts[0] != firstPrompt || prompts[1] != secondPrompt {
		t.Fatalf("raw prompts not overlaid: %v", prompts)
	}
}

// BUG-384 second half: a turn that FAILED on the provider must not replay as
// turn_completed — the synthetic tail closing a non-terminal transcript must
// preserve the failed state.
func TestBug384_FailedTurnReplaysAsFailedNotCompleted(t *testing.T) {
	root := t.TempDir()
	grokHome := filepath.Join(root, ".grok")
	cwd := "/workspace/retry-bed"
	sessionID := "session-run-384f"
	writeGrokChatHistoryFixture(t, grokHome, cwd, sessionID, []string{
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\nReply once\n</user_query>"),
	})
	writeGrokAuthFileFixture(t, grokHome)
	writeProviderAccountsConfig083(t, root, []ProviderAccount{{
		ID: "acct-grok", ProviderKey: "grok", HomePath: grokHome, SlotIndex: 1,
		AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}})

	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	for _, line := range []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-3", Prompt: "Reply once"},
		{Kind: turnLogKindGrokSession, SessionID: sessionID},
	} {
		if err := store.AppendTurnLog(context.Background(), "run-384f", line); err != nil {
			t.Fatalf("AppendTurnLog: %v", err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-grok"
	rs := &interactiveRun{
		id: "run-384f", providerKey: ProviderKeyGrok, providerAccountID: "acct-grok",
		workspaceCwd: cwd, runKind: "chat", status: RunStatusFailed,
		createdAt: "2026-07-18T16:49:01Z", resumedFromDisk: true,
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	if len(events) == 0 {
		t.Fatal("no replayed events")
	}
	last := events[len(events)-1]
	if last.Type == EventTurnCompleted {
		t.Fatalf("failed turn replayed as turn_completed — masks the failure; events=%v", summarizeEventTypes(events))
	}
	if last.Type != EventTurnFailed {
		t.Fatalf("last event = %s, want turn_failed for a failed run", last.Type)
	}
}
