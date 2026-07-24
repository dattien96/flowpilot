package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// run-75035: Codex workspace-newest rollout steal + child seed pollution.
// Additive-only suite (safe-fix-contract R1/R3). Does not edit legacy tests.

func TestRun75035_CodexRefreshDoesNotLogForeignWorkspaceRollout(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	cwd := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll cwd: %v", err)
	}
	acctHome := filepath.Join(dir, "codex-home")
	base := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	writeCodexRollout(t, acctHome, "rollout-child-own", cwd, base)
	writeCodexRollout(t, acctHome, "rollout-hub-newer", cwd, base.Add(2*time.Hour))
	writeProviderAccountsConfig(t, filepath.Join(dir, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-1", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: base.Format(time.RFC3339Nano)},
	})

	ctx := context.Background()
	if err := store.UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "run-75035", ProjectID: "proj-1", WorkflowID: "wf-1",
		ProviderKey: ProviderKeyCodex, ProviderSessionID: "rollout-hub-newer",
		ProviderAccountID: "acct-1", WorkingDirectory: cwd, Status: RunStatusCompleted,
		RunKind: "chat", StartedAt: base.Format(time.RFC3339Nano), UpdatedAt: base.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("Upsert hub: %v", err)
	}
	if err := store.UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "run-75040", ProjectID: "proj-1", WorkflowID: "wf-1", ParentRunID: "run-75035",
		AgentName: "coder-agent", ProviderKey: ProviderKeyCodex, ProviderSessionID: "rollout-child-own",
		ProviderAccountID: "acct-1", WorkingDirectory: cwd, Status: RunStatusCompleted,
		RunKind: "chat", StartedAt: base.Format(time.RFC3339Nano), UpdatedAt: base.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("Upsert child: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-1"

	child := &interactiveRun{
		id:                     "run-75040",
		projectID:              "proj-1",
		parentRunID:            "run-75035",
		providerKey:            ProviderKeyCodex,
		providerSessionID:      "rollout-child-own",
		realProviderSessionID:  "rollout-child-own",
		lastCodexTurnSessionID: "rollout-child-own",
		providerAccountID:      "acct-1",
		workspaceCwd:           cwd,
		status:                 RunStatusCompleted,
		runKind:                "chat",
	}

	// adapter=nil uses discovery compatibility path; foreign hub id must be refused.
	svc.mu.Lock()
	got := svc.refreshResumeHandleLocked(child, nil)
	svc.mu.Unlock()
	if got != "" {
		t.Fatalf("refreshResumeHandleLocked returned foreign session %q; want empty", got)
	}
	if child.lastCodexTurnSessionID != "rollout-child-own" {
		t.Fatalf("lastCodexTurnSessionID = %q, want rollout-child-own", child.lastCodexTurnSessionID)
	}
	if child.realProviderSessionID != "rollout-child-own" {
		t.Fatalf("realProviderSessionID = %q, want rollout-child-own", child.realProviderSessionID)
	}
}

func TestRun75035_CodexRefreshUsesAdapterReportedSession(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	child := &interactiveRun{
		id:                     "run-child",
		parentRunID:            "run-hub",
		providerKey:            ProviderKeyCodex,
		providerSessionID:      "rollout-old",
		realProviderSessionID:  "rollout-old",
		lastCodexTurnSessionID: "rollout-old",
		workspaceCwd:           dir,
	}
	adapter := &codexLastSessionReporter{id: "rollout-from-adapter"}
	svc.mu.Lock()
	got := svc.refreshResumeHandleLocked(child, adapter)
	svc.mu.Unlock()
	if got != "rollout-from-adapter" {
		t.Fatalf("got %q, want adapter-reported rollout-from-adapter", got)
	}
	if child.lastCodexTurnSessionID != "rollout-from-adapter" {
		t.Fatalf("lastCodexTurnSessionID = %q", child.lastCodexTurnSessionID)
	}
}

func TestRun75035_CodexAdapterReporterBlocksWorkspaceSteal(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	cwd := filepath.Join(dir, "ws")
	_ = os.MkdirAll(cwd, 0o755)
	acctHome := filepath.Join(dir, "codex-home")
	base := time.Now().UTC().Add(-time.Hour)
	writeCodexRollout(t, acctHome, "workspace-newest", cwd, base.Add(time.Hour))
	writeProviderAccountsConfig(t, filepath.Join(dir, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: base.Format(time.RFC3339Nano)},
	})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id:                     "run-x",
		providerKey:            ProviderKeyCodex,
		providerSessionID:      "rollout-own",
		realProviderSessionID:  "rollout-own",
		lastCodexTurnSessionID: "rollout-own",
		providerAccountID:      "acct",
		workspaceCwd:           cwd,
	}
	// Reporting adapter with empty last id must not fall through to discovery.
	svc.mu.Lock()
	got := svc.refreshResumeHandleLocked(rs, &codexLastSessionReporter{id: ""})
	svc.mu.Unlock()
	if got != "" {
		t.Fatalf("empty adapter reporter returned %q via discovery steal", got)
	}
}

func TestRun75035_SeedChildIgnoresParentCodexSessionPollution(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	cwd := filepath.Join(dir, "workspace")
	_ = os.MkdirAll(cwd, 0o755)
	acctHome := filepath.Join(dir, "codex-home")
	base := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

	// Child-owned rollout with child-only marker.
	writeCodexRolloutLines(t, acctHome, "rollout-child-own", cwd, base, []string{
		codexMetaLine("rollout-child-own", cwd, base),
		codexUserLine("coder own prompt"),
		codexAssistantLine("CHILD_OWN_ASSISTANT_MARKER"),
	})
	// Hub rollout with freeform reply markers from the live repro.
	writeCodexRolloutLines(t, acctHome, "rollout-hub-foreign", cwd, base.Add(time.Hour), []string{
		codexMetaLine("rollout-hub-foreign", cwd, base.Add(time.Hour)),
		codexUserLine("done r hả"),
		codexAssistantLine("Đúng, đã xong rồi. Flow kết thúc và review đã được approve."),
		codexUserLine("codex-flow-done"),
		codexAssistantLine("codex-flow-done"),
	})
	writeProviderAccountsConfig(t, filepath.Join(dir, "provider-accounts.json"), []ProviderAccount{
		{ID: "acct-1", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: base.Format(time.RFC3339Nano)},
	})

	ctx := context.Background()
	if err := store.UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "run-75035", ProjectID: "proj-1", WorkflowID: "wf-1",
		ProviderKey: ProviderKeyCodex, ProviderSessionID: "rollout-hub-foreign",
		ProviderAccountID: "acct-1", WorkingDirectory: cwd, Status: RunStatusCompleted,
		RunKind: "chat",
	}); err != nil {
		t.Fatalf("hub upsert: %v", err)
	}
	if err := store.UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "run-75040", ProjectID: "proj-1", WorkflowID: "wf-1", ParentRunID: "run-75035",
		AgentName: "coder-agent", ProviderKey: ProviderKeyCodex, ProviderSessionID: "rollout-child-own",
		ProviderAccountID: "acct-1", WorkingDirectory: cwd, Status: RunStatusCompleted,
		RunKind: "chat",
	}); err != nil {
		t.Fatalf("child upsert: %v", err)
	}
	// Polluted turn log shape from the live run: own session then foreign hub session.
	for _, line := range []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: "coder own prompt"},
		{Kind: turnLogKindCodexSession, SessionID: "rollout-child-own"},
		{Kind: turnLogKindPrompt, TurnID: "turn-feedback", Prompt: "[flow-engine] Feedback received on your last submission."},
		{Kind: turnLogKindCodexSession, SessionID: "rollout-hub-foreign"},
	} {
		if err := store.AppendTurnLog(ctx, "run-75040", line); err != nil {
			t.Fatalf("AppendTurnLog: %v", err)
		}
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-1"

	child := &interactiveRun{
		id:                    "run-75040",
		projectID:             "proj-1",
		parentRunID:           "run-75035",
		providerKey:           ProviderKeyCodex,
		providerSessionID:     "rollout-child-own",
		realProviderSessionID: "rollout-child-own",
		providerAccountID:     "acct-1",
		workspaceCwd:          cwd,
		resumedFromDisk:       true,
		status:                RunStatusCompleted,
		runKind:               "chat",
		createdAt:             base.Format(time.RFC3339Nano),
	}
	svc.seedTranscriptFromDisk(child)

	var texts []string
	for _, ev := range child.events {
		if ev.Type == EventMessageCompleted && strings.TrimSpace(ev.Text) != "" {
			texts = append(texts, ev.Text)
		}
		if ev.Type == EventTurnStarted && strings.TrimSpace(ev.Prompt) != "" {
			texts = append(texts, ev.Prompt)
		}
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "CHILD_OWN_ASSISTANT_MARKER") {
		t.Fatalf("child own transcript missing; events=%v", texts)
	}
	for _, banned := range []string{"done r hả", "Đúng, đã xong rồi", "codex-flow-done"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("seed imported foreign hub marker %q into child; events=%v", banned, texts)
		}
	}
}

func TestRun75035_SeedChildIgnoresSiblingCodexSessionPollution(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	cwd := filepath.Join(dir, "ws")
	_ = os.MkdirAll(cwd, 0o755)
	acctHome := filepath.Join(dir, "codex-home")
	base := time.Now().UTC().Add(-2 * time.Hour)

	writeCodexRolloutLines(t, acctHome, "rollout-coder", cwd, base, []string{
		codexMetaLine("rollout-coder", cwd, base),
		codexAssistantLine("CODER_ONLY"),
	})
	writeCodexRolloutLines(t, acctHome, "rollout-reviewer", cwd, base.Add(time.Minute), []string{
		codexMetaLine("rollout-reviewer", cwd, base.Add(time.Minute)),
		codexAssistantLine("REVIEWER_ONLY_SIBLING"),
	})
	writeProviderAccountsConfig(t, filepath.Join(dir, "provider-accounts.json"), []ProviderAccount{
		{ID: "a", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: base.Format(time.RFC3339Nano)},
	})

	ctx := context.Background()
	_ = store.UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "hub", ProjectID: "p", WorkflowID: "w", ProviderKey: ProviderKeyCodex,
		ProviderSessionID: "rollout-hub", ProviderAccountID: "a", WorkingDirectory: cwd, Status: RunStatusCompleted,
	})
	_ = store.UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "coder", ProjectID: "p", WorkflowID: "w", ParentRunID: "hub", AgentName: "coder",
		ProviderKey: ProviderKeyCodex, ProviderSessionID: "rollout-coder", ProviderAccountID: "a",
		WorkingDirectory: cwd, Status: RunStatusCompleted,
	})
	_ = store.UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "reviewer", ProjectID: "p", WorkflowID: "w", ParentRunID: "hub", AgentName: "reviewer",
		ProviderKey: ProviderKeyCodex, ProviderSessionID: "rollout-reviewer", ProviderAccountID: "a",
		WorkingDirectory: cwd, Status: RunStatusCompleted,
	})
	_ = store.AppendTurnLog(ctx, "coder", turnLogLine{Kind: turnLogKindCodexSession, SessionID: "rollout-coder"})
	_ = store.AppendTurnLog(ctx, "coder", turnLogLine{Kind: turnLogKindCodexSession, SessionID: "rollout-reviewer"})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	child := &interactiveRun{
		id: "coder", projectID: "p", parentRunID: "hub", providerKey: ProviderKeyCodex,
		providerSessionID: "rollout-coder", realProviderSessionID: "rollout-coder",
		providerAccountID: "a", workspaceCwd: cwd, resumedFromDisk: true,
		status: RunStatusCompleted, runKind: "chat", createdAt: base.Format(time.RFC3339Nano),
	}
	svc.seedTranscriptFromDisk(child)
	joined := eventsTextJoin(child.events)
	if !strings.Contains(joined, "CODER_ONLY") {
		t.Fatalf("missing coder text: %q", joined)
	}
	if strings.Contains(joined, "REVIEWER_ONLY_SIBLING") {
		t.Fatalf("sibling reviewer text leaked into coder: %q", joined)
	}
}

func TestRun75035_SeedChildKeepsOwnMultipleCodexRollouts(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	cwd := filepath.Join(dir, "ws")
	_ = os.MkdirAll(cwd, 0o755)
	acctHome := filepath.Join(dir, "codex-home")
	base := time.Now().UTC().Add(-3 * time.Hour)

	writeCodexRolloutLines(t, acctHome, "rollout-a", cwd, base, []string{
		codexMetaLine("rollout-a", cwd, base),
		codexAssistantLine("OWN_A"),
	})
	writeCodexRolloutLines(t, acctHome, "rollout-b", cwd, base.Add(time.Minute), []string{
		codexMetaLine("rollout-b", cwd, base.Add(time.Minute)),
		codexAssistantLine("OWN_B"),
	})
	writeProviderAccountsConfig(t, filepath.Join(dir, "provider-accounts.json"), []ProviderAccount{
		{ID: "a", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: base.Format(time.RFC3339Nano)},
	})
	ctx := context.Background()
	_ = store.UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "child", ProjectID: "p", ParentRunID: "hub", ProviderKey: ProviderKeyCodex,
		ProviderSessionID: "rollout-a", ProviderAccountID: "a", WorkingDirectory: cwd, Status: RunStatusCompleted,
	})
	_ = store.AppendTurnLog(ctx, "child", turnLogLine{Kind: turnLogKindCodexSession, SessionID: "rollout-a"})
	_ = store.AppendTurnLog(ctx, "child", turnLogLine{Kind: turnLogKindCodexSession, SessionID: "rollout-b"})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	child := &interactiveRun{
		id: "child", projectID: "p", parentRunID: "hub", providerKey: ProviderKeyCodex,
		providerSessionID: "rollout-a", realProviderSessionID: "rollout-a",
		providerAccountID: "a", workspaceCwd: cwd, resumedFromDisk: true,
		status: RunStatusCompleted, runKind: "chat", createdAt: base.Format(time.RFC3339Nano),
	}
	svc.seedTranscriptFromDisk(child)
	joined := eventsTextJoin(child.events)
	if !strings.Contains(joined, "OWN_A") || !strings.Contains(joined, "OWN_B") {
		t.Fatalf("expected both own rollouts, got %q", joined)
	}
}

func TestRun75035_HubResumeStillLoadsOwnCodexSession(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	cwd := filepath.Join(dir, "ws")
	_ = os.MkdirAll(cwd, 0o755)
	acctHome := filepath.Join(dir, "codex-home")
	base := time.Now().UTC().Add(-time.Hour)
	writeCodexRolloutLines(t, acctHome, "rollout-hub", cwd, base, []string{
		codexMetaLine("rollout-hub", cwd, base),
		codexUserLine("fix bug 1+1 != 2"),
		codexAssistantLine("HUB_SYNTHESIS_OK"),
	})
	writeProviderAccountsConfig(t, filepath.Join(dir, "provider-accounts.json"), []ProviderAccount{
		{ID: "a", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: base.Format(time.RFC3339Nano)},
	})
	ctx := context.Background()
	_ = store.UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "run-75035", ProjectID: "p", ProviderKey: ProviderKeyCodex,
		ProviderSessionID: "rollout-hub", ProviderAccountID: "a", WorkingDirectory: cwd, Status: RunStatusCompleted,
	})
	_ = store.AppendTurnLog(ctx, "run-75035", turnLogLine{Kind: turnLogKindCodexSession, SessionID: "rollout-hub"})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	hub := &interactiveRun{
		id: "run-75035", projectID: "p", providerKey: ProviderKeyCodex,
		providerSessionID: "rollout-hub", realProviderSessionID: "rollout-hub",
		providerAccountID: "a", workspaceCwd: cwd, resumedFromDisk: true,
		status: RunStatusCompleted, runKind: "chat", createdAt: base.Format(time.RFC3339Nano),
	}
	svc.seedTranscriptFromDisk(hub)
	joined := eventsTextJoin(hub.events)
	if !strings.Contains(joined, "HUB_SYNTHESIS_OK") {
		t.Fatalf("hub lost own transcript: %q", joined)
	}
}

func TestRun75035_HubPreferTurnLogStillWinsWhenAssistantsPresent(t *testing.T) {
	// Guards CA-390 / run-24377 preferFlowHubTurnLogTranscript gate remains true for hubs.
	hub := &interactiveRun{id: "hub", flowEngineDriven: true, parentRunID: ""}
	entries := []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "t1", Prompt: "user"},
		{Kind: turnLogKindTranscriptTurn, TurnID: "t1", Assistant: "hub answer"},
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	if !svc.preferFlowHubTurnLogTranscript(hub, entries) {
		t.Fatal("hub with assistants should prefer turn-log transcript")
	}
	child := &interactiveRun{id: "child", parentRunID: "hub", flowEngineDriven: true}
	if svc.preferFlowHubTurnLogTranscript(child, entries) {
		t.Fatal("child must not prefer hub turn-log path")
	}
}

func TestRun75035_EnsureProviderResumeHandleChildDoesNotStealHubRollout(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	cwd := filepath.Join(dir, "ws")
	_ = os.MkdirAll(cwd, 0o755)
	acctHome := filepath.Join(dir, "codex-home")
	base := time.Now().UTC().Add(-time.Hour)
	writeCodexRollout(t, acctHome, "hub-newest", cwd, base.Add(time.Hour))
	writeProviderAccountsConfig(t, filepath.Join(dir, "provider-accounts.json"), []ProviderAccount{
		{ID: "a", ProviderKey: "codex", HomePath: acctHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: base.Format(time.RFC3339Nano)},
	})
	ctx := context.Background()
	_ = store.UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "hub", ProjectID: "p", ProviderKey: ProviderKeyCodex, ProviderSessionID: "hub-newest",
		ProviderAccountID: "a", WorkingDirectory: cwd, Status: RunStatusCompleted,
	})
	_ = store.UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "child", ProjectID: "p", ParentRunID: "hub", ProviderKey: ProviderKeyCodex,
		ProviderSessionID: "thread-1", ProviderAccountID: "a", WorkingDirectory: cwd, Status: RunStatusCompleted,
	})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	child := &interactiveRun{
		id: "child", projectID: "p", parentRunID: "hub", providerKey: ProviderKeyCodex,
		providerSessionID: "thread-1", providerAccountID: "a", workspaceCwd: cwd,
		resumedFromDisk: true,
	}
	if svc.ensureProviderResumeHandle(child, acctHome) {
		t.Fatalf("child with synthetic session must not promote hub rollout; real=%q", child.realProviderSessionID)
	}
	if child.realProviderSessionID == "hub-newest" {
		t.Fatal("child realProviderSessionID stolen hub-newest")
	}
}

func TestRun75035_ProviderParityClassification(t *testing.T) {
	// Table documents Case 2/3: write-path steal is Codex-specific; Grok has
	// LastGrokSessionID no-steal; Claude has no newest-cwd discovery in refresh.
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(dir, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)

	t.Run("codex_adapter_reported", func(t *testing.T) {
		rs := &interactiveRun{id: "c", providerKey: ProviderKeyCodex, lastCodexTurnSessionID: "old"}
		got := func() string {
			svc.mu.Lock()
			defer svc.mu.Unlock()
			return svc.refreshResumeHandleLocked(rs, &codexLastSessionReporter{id: "new-codex"})
		}()
		if got != "new-codex" {
			t.Fatalf("codex adapter path = %q", got)
		}
	})
	t.Run("grok_no_workspace_steal_without_reporter", func(t *testing.T) {
		rs := &interactiveRun{id: "g", providerKey: ProviderKeyGrok, realProviderSessionID: "own"}
		got := func() string {
			svc.mu.Lock()
			defer svc.mu.Unlock()
			return svc.refreshResumeHandleLocked(rs, nil)
		}()
		if got != "" {
			t.Fatalf("grok without reporter must not invent session, got %q", got)
		}
	})
	t.Run("claude_no_discovery_return", func(t *testing.T) {
		rs := &interactiveRun{id: "cl", providerKey: ProviderKeyClaude}
		got := func() string {
			svc.mu.Lock()
			defer svc.mu.Unlock()
			return svc.refreshResumeHandleLocked(rs, nil)
		}()
		if got != "" {
			t.Fatalf("claude refresh with nil adapter should not return discovery id, got %q", got)
		}
	})
}

// --- helpers (new-file only) ---

type codexLastSessionReporter struct {
	id string
}

func (a *codexLastSessionReporter) Key() ProviderKey { return ProviderKeyCodex }
func (a *codexLastSessionReporter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{}
}
func (a *codexLastSessionReporter) SendTurn(context.Context, TurnRequest, TurnBridge) error {
	return nil
}
func (a *codexLastSessionReporter) LastCodexSessionID() string { return a.id }

func codexMetaLine(sessionID, cwd string, ts time.Time) string {
	line := map[string]any{
		"payload": map[string]any{
			"id":        sessionID,
			"timestamp": ts.Format(time.RFC3339Nano),
			"cwd":       cwd,
		},
	}
	raw, _ := json.Marshal(line)
	return string(raw)
}

func codexUserLine(text string) string {
	line := map[string]any{
		"type": "response_item",
		"payload": map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": text},
			},
		},
	}
	raw, _ := json.Marshal(line)
	return string(raw)
}

func codexAssistantLine(text string) string {
	line := map[string]any{
		"type": "response_item",
		"payload": map[string]any{
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "output_text", "text": text},
			},
		},
	}
	raw, _ := json.Marshal(line)
	return string(raw)
}

func eventsTextJoin(events []ProviderEvent) string {
	var parts []string
	for _, ev := range events {
		if strings.TrimSpace(ev.Text) != "" {
			parts = append(parts, ev.Text)
		}
		if strings.TrimSpace(ev.Prompt) != "" {
			parts = append(parts, ev.Prompt)
		}
	}
	return strings.Join(parts, "\n")
}
