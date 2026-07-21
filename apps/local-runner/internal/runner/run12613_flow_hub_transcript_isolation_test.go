package runner

// run-12613 / run-20332: a flow hub must keep loading its own provider history.
// Durable turn-log rows supplement a missing provider frame; they never replace
// provider history, or legacy multi-turn hubs reopen with an incomplete chat.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun12613FlowHubLoadsProviderHistoryBeforeTurnLogFallback(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const (
		parentID  = "run-12613"
		coderID   = "run-12618"
		sessionID = "019f7fe8-shared-with-coder"
		cwd       = "/repo/gate-sandbox"
	)
	grokHome := filepath.Join(root, "grok-home")
	histDir := filepath.Join(grokHome, "sessions", percentEncodeGrokCwd(cwd), sessionID)
	if err := os.MkdirAll(histDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	providerHistory := `{"type":"assistant","content":"Parent hub: round one requests changes."}
{"type":"assistant","content":"Parent hub: round two is approved."}
`
	if err := os.WriteFile(filepath.Join(histDir, "chat_history.jsonl"), []byte(providerHistory), 0o644); err != nil {
		t.Fatalf("write history: %v", err)
	}

	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentID, ProviderKey: ProviderKeyGrok, ProviderSessionID: sessionID,
		ProviderAccountID: "acct-g", WorkingDirectory: cwd, RunKind: "workflow",
		Status: RunStatusRunning, LoopState: AgentLoopState{Status: "done", Cap: 3},
		LastMessage: "Parent hub: round two is approved.",
		StartedAt:   "2026-07-20T12:00:00Z", UpdatedAt: "2026-07-20T12:30:00Z",
	})
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: coderID, ParentRunID: parentID, AgentName: "coder", Label: "grok-coder",
		ProviderKey: ProviderKeyGrok, ProviderSessionID: sessionID,
		ProviderAccountID: "acct-g", WorkingDirectory: cwd, RunKind: "chat",
		Status: RunStatusCompleted, LastMessage: "child-only prose must not replace hub history",
		StartedAt: "2026-07-20T12:00:01Z", UpdatedAt: "2026-07-20T12:10:00Z",
	})
	_ = store.AppendTurnLog(context.Background(), parentID, turnLogLine{
		Kind: turnLogKindPrompt, TurnID: "t1", Prompt: "fix bug 1+1 !=2",
	})
	_ = store.AppendTurnLog(context.Background(), parentID, turnLogLine{
		Kind: turnLogKindGrokSession, SessionID: sessionID,
	})
	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-g", ProviderKey: "grok", HomePath: grokHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true},
	})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-g"
	rs := &interactiveRun{
		id: parentID, providerKey: ProviderKeyGrok, providerSessionID: sessionID,
		realProviderSessionID: sessionID, providerAccountID: "acct-g",
		workspaceCwd: cwd, runKind: "workflow", flowEngineDriven: true,
		status: RunStatusRunning, resumedFromDisk: true,
		lastMessage: "Parent hub: round two is approved.",
		createdAt:   "2026-07-20T12:00:00Z",
		subs:        map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[parentID] = rs
	svc.mu.Unlock()

	svc.seedGrokTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	var assistants []string
	for _, ev := range events {
		if ev.Type == EventMessageCompleted {
			assistants = append(assistants, ev.Text)
		}
	}
	if len(assistants) != 2 {
		t.Fatalf("want both provider-history messages, got %v", assistants)
	}
	if assistants[0] != "Parent hub: round one requests changes." || assistants[1] != "Parent hub: round two is approved." {
		t.Fatalf("provider history was not preserved in order: %v", assistants)
	}
}

func TestRun12613ResumeAgentCardUsesFlowNodeLabel(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const parentID = "run-12613-labels"
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentID, ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-p",
		RunKind: "workflow", Status: RunStatusCompleted,
	})
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-child", ParentRunID: parentID, AgentName: "coder", Label: "grok-coder",
		ProviderKey: ProviderKeyGrok, ProviderSessionID: "thread-c", RunKind: "chat",
		Status: RunStatusCompleted, StartedAt: "2026-07-20T12:00:01Z", LastMessage: "implemented",
	})
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	for _, a := range svc.resumedParentAgentAnnotations(parentID) {
		if a.Type == EventAgentSpawnedByUser {
			if a.AgentName != "grok-coder" {
				t.Fatalf("AgentName = %q, want grok-coder", a.AgentName)
			}
			return
		}
	}
	t.Fatal("missing spawn annotation")
}

func percentEncodeGrokCwd(cwd string) string {
	path := grokChatHistoryPath("/tmp/grok", cwd, "sess-x")
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, p := range parts {
		if p == "sessions" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return cwd
}
