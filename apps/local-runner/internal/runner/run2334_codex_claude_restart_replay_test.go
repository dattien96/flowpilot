package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRun2334ClaudeRestartAnchorsSidecarsByDurableTurnID(t *testing.T) {
	root := t.TempDir()
	acctHome := filepath.Join(root, "claude-home")
	sessionID := "claude-run-2334"
	claudeDir := filepath.Join(acctHome, ".claude", "projects", "project")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeLinesToPath083(t, filepath.Join(claudeDir, sessionID+".jsonl"), []string{
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"first provider prompt"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"first answer"}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"second provider prompt"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"second answer"}]}}`,
	})

	store := newRun2334TurnLogStore(t, root, "run-2334-claude", []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: "first raw prompt"},
		{Kind: turnLogKindPrompt, TurnID: "turn-2", Prompt: "second raw prompt"},
	})
	writeProviderAccountsConfig083(t, root, []ProviderAccount{{
		ID: "acct-claude", ProviderKey: string(ProviderKeyClaude), HomePath: acctHome,
		SlotIndex: 1, AuthStatus: "connected", IsActive: true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := newRun2334ResumedChat("run-2334-claude", ProviderKeyClaude, "acct-claude", sessionID)
	seedRun2334Sidecars(rs)
	seedRun2334Service(t, svc, rs)

	svc.seedTranscriptFromDisk(rs)
	assertRun2334TurnAnchors(t, rs)
}

func TestRun2334CodexRestartAnchorsSidecarsByDurableTurnID(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex-home")
	sessionID := "codex-run-2334"
	writeCodexRolloutLines(t, codexHome, sessionID, "/repo", time.Now().UTC(), []string{
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"first provider prompt"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"first answer"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"second provider prompt"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"second answer"}]}}`,
	})

	store := newRun2334TurnLogStore(t, root, "run-2334-codex", []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: "first raw prompt"},
		{Kind: turnLogKindPrompt, TurnID: "turn-2", Prompt: "second raw prompt"},
		{Kind: turnLogKindCodexSession, SessionID: sessionID},
	})
	writeProviderAccountsConfig083(t, root, []ProviderAccount{{
		ID: "acct-codex", ProviderKey: string(ProviderKeyCodex), HomePath: codexHome,
		SlotIndex: 1, AuthStatus: "connected", IsActive: true,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := newRun2334ResumedChat("run-2334-codex", ProviderKeyCodex, "acct-codex", sessionID)
	seedRun2334Sidecars(rs)
	seedRun2334Service(t, svc, rs)

	svc.seedTranscriptFromDisk(rs)
	assertRun2334TurnAnchors(t, rs)
}

func newRun2334TurnLogStore(t *testing.T, root, runID string, lines []turnLogLine) *localFileSessionStore {
	t.Helper()
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	for _, line := range lines {
		if err := store.AppendTurnLog(context.Background(), runID, line); err != nil {
			t.Fatalf("AppendTurnLog(%s): %v", line.Kind, err)
		}
	}
	return store
}

func newRun2334ResumedChat(runID string, provider ProviderKey, accountID, sessionID string) *interactiveRun {
	return &interactiveRun{
		id: runID, providerKey: provider, providerAccountID: accountID,
		providerSessionID: sessionID, realProviderSessionID: sessionID,
		workspaceCwd: "/repo", runKind: "chat", status: RunStatusCompleted,
		createdAt: "2026-07-18T16:49:01Z", resumedFromDisk: true,
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
}

func seedRun2334Sidecars(rs *interactiveRun) {
	rs.events = []ProviderEvent{
		{Type: EventPermissionRequired, ProviderTurnID: "turn-1", ApprovalID: "approval-1"},
		{Type: EventUserQuestionRequired, ProviderTurnID: "turn-2", QuestionID: "question-2"},
	}
	rs.sidecarPrefixCount = int64(len(rs.events))
}

func seedRun2334Service(t *testing.T, svc *InteractiveService, rs *interactiveRun) {
	t.Helper()
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()
}

func assertRun2334TurnAnchors(t *testing.T, rs *interactiveRun) {
	t.Helper()
	var prompts []string
	indices := map[string]int{}
	for i, event := range rs.events {
		if event.Type == EventTurnStarted && event.Prompt != "" {
			prompts = append(prompts, event.Prompt)
			indices[event.ProviderTurnID] = i
		}
		switch event.Type {
		case EventPermissionRequired:
			indices["approval"] = i
		case EventUserQuestionRequired:
			indices["question"] = i
		}
	}
	if len(prompts) != 2 || prompts[0] != "first raw prompt" || prompts[1] != "second raw prompt" {
		t.Fatalf("replayed prompts = %q, want raw prompts in turn-log order", prompts)
	}
	if indices["approval"] <= indices["turn-1"] || indices["approval"] >= indices["turn-2"] {
		t.Fatalf("approval index = %d, want after turn-1 and before turn-2; indices=%v", indices["approval"], indices)
	}
	if indices["question"] <= indices["turn-2"] {
		t.Fatalf("question index = %d, want after turn-2; indices=%v", indices["question"], indices)
	}
}
