package runner

// run-24377 (CP-51 A10 residual / live Grok Review Loop):
// Hub and coder can share the same Grok provider_session_id. After restart,
// seedGrokTranscriptFromDisk used to load that shared chat_history.jsonl into
// the hub, replaying child turns + scrambling user-facing prompt order (follow-up
// mid-timeline) and dumping agent cards at the bottom.
//
// Fix: flow hubs with durable transcript_turn assistants rebuild prose from the
// per-run turn log only; agent cards still come from appendResumedParentAnnotations.
//
// cross-provider-parity: Case 1 — preferFlowHubTurnLogTranscript / seedFlowHub
// take no providerKey. Grok is the live reporter (shared session); Claude path
// uses the same prefer gate in seedTranscriptFromDisk.
//
// additive-tests-only: new file only.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun24377FlowHubPrefersTurnLogWhenSharedGrokSessionPolluted(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	const (
		parentID  = "run-24377"
		coderID   = "run-24382"
		sessionID = "019f8526-shared-hub-coder"
		cwd       = "/Users/tiendat/Desktop/BE/gate-sandbox"
	)
	grokHome := filepath.Join(root, "grok-home")
	histDir := filepath.Join(grokHome, "sessions", percentEncodeGrokCwd(cwd), sessionID)
	if err := os.MkdirAll(histDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Polluted shared session: child coder turns + join notes + real follow-up.
	// Loading this whole file for the hub is the bug.
	polluted := strings.Join([]string{
		`{"type":"user","content":[{"type":"text","text":"<user_query>\nYou are the implementation agent for a scoped FlowPilot node.\n[FlowPilot sub-agent — agent: coder]\nfix bug\n</user_query>"}]}`,
		`{"type":"assistant","content":"CHILD ONLY — should never appear on hub resume."}`,
		`{"type":"user","content":[{"type":"text","text":"<user_query>\n[flow-engine joined result note]\nFlow round 0 — 2 results joined.\n</user_query>"}]}`,
		`{"type":"assistant","content":"POLLUTED synth text from shared file."}`,
		`{"type":"user","content":[{"type":"text","text":"<user_query>\n[FlowPilot system note — sub-agents started in this session via the UI (not by you): - Flow completed.]\n\ndone rồi hả, trả lời ok or not.\n</user_query>"}]}`,
		`{"type":"assistant","content":"POLLUTED follow-up answer from shared file."}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(histDir, "chat_history.jsonl"), []byte(polluted), 0o644); err != nil {
		t.Fatalf("write history: %v", err)
	}

	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: parentID, ProviderKey: ProviderKeyGrok, ProviderSessionID: sessionID,
		ProviderAccountID: "acct-g", WorkingDirectory: cwd, RunKind: "chat",
		Status: RunStatusCompleted, LoopState: AgentLoopState{Status: "done", Cap: 3, Mode: "explicit"},
		StartedAt: "2026-07-21T14:48:00Z", UpdatedAt: "2026-07-21T14:59:00Z",
	})
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: coderID, ParentRunID: parentID, AgentName: "coder", Label: "coder",
		ProviderKey: ProviderKeyGrok, ProviderSessionID: sessionID, // same id as hub
		ProviderAccountID: "acct-g", WorkingDirectory: cwd, RunKind: "chat",
		Status: RunStatusCompleted, LastMessage: "coder final", TurnCount: 4,
		StartedAt: "2026-07-21T14:48:09Z", UpdatedAt: "2026-07-21T14:56:01Z",
	})
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-r0a", ParentRunID: parentID, AgentName: "reviewer", Label: "reviewer_correctness",
		ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-r0a", RunKind: "chat",
		Status: RunStatusCompleted, LastMessage: "request changes", TurnCount: 1,
		StartedAt: "2026-07-21T14:56:02Z", UpdatedAt: "2026-07-21T14:56:51Z",
	})
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-r0b", ParentRunID: parentID, AgentName: "reviewer", Label: "reviewer_security",
		ProviderKey: ProviderKeyGrok, ProviderSessionID: "sess-r0b", RunKind: "chat",
		Status: RunStatusCompleted, LastMessage: "request changes", TurnCount: 1,
		StartedAt: "2026-07-21T14:56:02Z", UpdatedAt: "2026-07-21T14:56:52Z",
	})

	// Authoritative hub turn log (matches live run-24377 shape).
	hubLog := []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: "fix bug 1 + 1 != 2"},
		{Kind: turnLogKindPrompt, TurnID: "turn-2", Prompt: "[flow-engine joined result note]\nFlow round 0 — 2 results joined."},
		{Kind: turnLogKindGrokSession, SessionID: sessionID},
		{Kind: turnLogKindTranscriptTurn, TurnID: "turn-2", Assistant: "Round 0: both reviewers request changes."},
		{Kind: turnLogKindPrompt, TurnID: "turn-3", Prompt: "[flow-engine joined result note]\nFlow round 1 — 2 results joined."},
		{Kind: turnLogKindTranscriptTurn, TurnID: "turn-3", Assistant: "Round 1: still request changes."},
		{Kind: turnLogKindPrompt, TurnID: "turn-4", Prompt: "[flow-engine joined result note]\nFlow round 2 — 2 results joined."},
		{Kind: turnLogKindTranscriptTurn, TurnID: "turn-4", Assistant: "Round 2: both approve. Flow done."},
		{Kind: turnLogKindPrompt, TurnID: "turn-5", Prompt: "done rồi hả, trả lời ok or not."},
		{Kind: turnLogKindTranscriptTurn, TurnID: "turn-5", Assistant: "ok — flow done; correctness + security both approved."},
	}
	for _, row := range hubLog {
		if err := store.AppendTurnLog(context.Background(), parentID, row); err != nil {
			t.Fatalf("AppendTurnLog: %v", err)
		}
	}
	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-g", ProviderKey: "grok", HomePath: grokHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true},
	})

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-g"
	rs := &interactiveRun{
		id: parentID, providerKey: ProviderKeyGrok, providerSessionID: sessionID,
		realProviderSessionID: sessionID, providerAccountID: "acct-g",
		workspaceCwd: cwd, runKind: "chat", flowEngineDriven: true,
		status: RunStatusCompleted, resumedFromDisk: true,
		createdAt: "2026-07-21T14:48:00Z", updatedAt: "2026-07-21T14:59:00Z",
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[parentID] = rs
	svc.mu.Unlock()

	// Full resume entry (same as history-open): seed + agent annotations.
	svc.seedGrokTranscriptFromDisk(rs)
	svc.appendResumedParentAnnotations(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	// 1) No child-only pollution from the shared session file.
	for _, ev := range events {
		if ev.Type == EventMessageCompleted {
			if strings.Contains(ev.Text, "CHILD ONLY") || strings.Contains(ev.Text, "POLLUTED") {
				t.Fatalf("hub resume loaded polluted shared-session text: %q", ev.Text)
			}
		}
		if ev.Type == EventTurnStarted && strings.Contains(ev.Prompt, "implementation agent") {
			t.Fatalf("hub resume surfaced child spawn prompt: %q", ev.Prompt)
		}
	}

	// 2) User-facing prompts: original first, follow-up last (not mid-timeline).
	var prompts []string
	for _, ev := range events {
		if ev.Type == EventTurnStarted && strings.TrimSpace(ev.Prompt) != "" {
			if isSystemPrompt(ev.Prompt) {
				t.Fatalf("system prompt surfaced as bubble: %q", ev.Prompt)
			}
			prompts = append(prompts, ev.Prompt)
		}
	}
	wantPrompts := []string{"fix bug 1 + 1 != 2", "done rồi hả, trả lời ok or not."}
	if len(prompts) != len(wantPrompts) {
		t.Fatalf("prompts = %q, want %q", prompts, wantPrompts)
	}
	for i := range wantPrompts {
		if prompts[i] != wantPrompts[i] {
			t.Fatalf("prompts[%d] = %q, want %q (full %q)", i, prompts[i], wantPrompts[i], prompts)
		}
	}
	// Follow-up must not appear before the last synthesis message.
	followIdx, lastSynthIdx := -1, -1
	for i, ev := range events {
		if ev.Type == EventTurnStarted && ev.Prompt == "done rồi hả, trả lời ok or not." {
			followIdx = i
		}
		if ev.Type == EventMessageCompleted && strings.Contains(ev.Text, "Round 2") {
			lastSynthIdx = i
		}
	}
	if followIdx < 0 || lastSynthIdx < 0 {
		t.Fatalf("missing follow-up or last synth: follow=%d synth=%d types=%v",
			followIdx, lastSynthIdx, summarizeEventTypes(events))
	}
	if followIdx < lastSynthIdx {
		t.Fatalf("follow-up prompt at %d before last synthesis at %d: %v",
			followIdx, lastSynthIdx, summarizeEventTypes(events))
	}

	// 3) Original user prompt must be the first timeline frame — not agent cards
	// stacked above it (run-24377 "moved everything to the top" after the
	// bottom-dump fix).
	if len(events) == 0 || events[0].Type != EventTurnStarted || events[0].Prompt != "fix bug 1 + 1 != 2" {
		t.Fatalf("events[0] must be original user prompt, got %v", summarizeEventTypes(events))
	}
	firstAgent, lastMsg := -1, -1
	for i, ev := range events {
		switch ev.Type {
		case EventAgentSpawnedByUser:
			if firstAgent < 0 {
				firstAgent = i
			}
		case EventMessageCompleted:
			lastMsg = i
		}
	}
	if firstAgent < 0 {
		t.Fatalf("no agent cards restored: %v", summarizeEventTypes(events))
	}
	if lastMsg < 0 {
		t.Fatalf("no hub messages: %v", summarizeEventTypes(events))
	}
	if firstAgent == 0 {
		t.Fatalf("agent cards dumped at top before original prompt: %v", summarizeEventTypes(events))
	}
	// At least one agent card should appear before the final synthesis / follow-up tail.
	if firstAgent > lastMsg {
		t.Fatalf("all agent cards after last hub message (bottom dump): %v", summarizeEventTypes(events))
	}
}

func TestBuildFlowHubTranscriptEventsFromTurnLogSkipsSystemPromptsKeepsSynthesis(t *testing.T) {
	entries := []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "t1", Prompt: "fix bug"},
		{Kind: turnLogKindPrompt, TurnID: "t2", Prompt: "[flow-engine joined result note]\nFlow round 0"},
		{Kind: turnLogKindTranscriptTurn, TurnID: "t2", Assistant: "synth0"},
		{Kind: turnLogKindPrompt, TurnID: "t3", Prompt: "follow up please"},
		{Kind: turnLogKindTranscriptTurn, TurnID: "t3", Assistant: "all good"},
	}
	got := buildFlowHubTranscriptEventsFromTurnLog(entries)
	var types []string
	for _, ev := range got {
		types = append(types, string(ev.Type)+":"+firstNonEmptyResumeValue(ev.Prompt, ev.Text))
	}
	want := []string{
		"turn_started:fix bug",
		"message_completed:synth0",
		"turn_started:follow up please",
		"message_completed:all good",
	}
	if len(types) != len(want) {
		t.Fatalf("events = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("events[%d] = %q, want %q", i, types[i], want[i])
		}
	}
}

func TestPreferFlowHubTurnLogTranscriptGates(t *testing.T) {
	svc := &InteractiveService{}
	entriesWithAssist := []turnLogLine{
		{Kind: turnLogKindTranscriptTurn, TurnID: "t1", Assistant: "hello"},
	}
	hub := &interactiveRun{id: "h", flowEngineDriven: true}
	if !svc.preferFlowHubTurnLogTranscript(hub, entriesWithAssist) {
		t.Fatal("flow hub with assistants must prefer turn log")
	}
	child := &interactiveRun{id: "c", parentRunID: "h", flowEngineDriven: true}
	if svc.preferFlowHubTurnLogTranscript(child, entriesWithAssist) {
		t.Fatal("child must not use hub turn-log prefer path")
	}
	plain := &interactiveRun{id: "p", flowEngineDriven: false}
	if svc.preferFlowHubTurnLogTranscript(plain, entriesWithAssist) {
		t.Fatal("non-flow chat must keep provider history path")
	}
	if svc.preferFlowHubTurnLogTranscript(hub, nil) {
		t.Fatal("empty turn log must fall through to provider path")
	}
}
