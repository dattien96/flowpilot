package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestRun1264SettleFinalizesWhenFlowDoneDespiteStalePendingGate(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("run-1264", "turn-3384")
	if err := store.CreatePrepared(ctx, rec, testEnvelope("run-1264", "turn-3384")); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	got, rev, err := store.Get(ctx, "run-1264", "turn-3384")
	if err != nil {
		t.Fatalf("Get prepared: %v", err)
	}
	rev, err = store.CASAdvance(ctx, "run-1264", "turn-3384", rev, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatalf("send_claimed: %v", err)
	}
	rev, err = store.CASAdvance(ctx, "run-1264", "turn-3384", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err != nil {
		t.Fatalf("send_started: %v", err)
	}
	payload := []byte(`{"type":"turn_completed"}`)
	proof := TerminalEvidence{
		ProviderKey:          "grok",
		EvidenceKind:         string(EventTurnCompleted),
		Outcome:              "completed",
		PayloadCanonicalJSON: payload,
		PayloadSHA256:        HashBytes(payload),
		ObservedAt:           nowRFC3339Nano(),
	}
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "run-1264", "turn-3384", rev, proof, got.IntentOwnerRunID, got.OuterIntentKey, got.OuterIntentGen); err != nil {
		t.Fatalf("CommitTerminalAndSettleIntent: %v", err)
	}

	svc, _ := newTestServer(t)
	svc.dispatchStore = store
	svc.mu.Lock()
	svc.runs["run-1264"] = &interactiveRun{
		id:                    "run-1264",
		status:                RunStatusRunning,
		pendingFlowGateSettle: true,
		idempotency:           map[string]string{},
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop("run-1264", AgentLoopState{Status: "done", Round: 1, Cap: 3, Mode: "explicit"})

	d := svc.newSettleDriver()
	if d == nil {
		t.Fatal("newSettleDriver returned nil")
	}
	if err := d.DriveSettle(ctx, "run-1264", "turn-3384"); err != nil {
		t.Fatalf("DriveSettle: %v", err)
	}
	final, _, err := store.Get(ctx, "run-1264", "turn-3384")
	if err != nil {
		t.Fatalf("Get final: %v", err)
	}
	if final.SettlePhase != SettleFinalized {
		t.Fatalf("settle phase = %s, want %s", final.SettlePhase, SettleFinalized)
	}
	items, err := store.ListAttention(ctx)
	if err != nil {
		t.Fatalf("ListAttention: %v", err)
	}
	for _, item := range items {
		if item.RunID == "run-1264" && item.TurnID == "turn-3384" && item.Kind == "settle_pending" {
			t.Fatalf("settle_pending attention still shown after done flow finalized: %+v", item)
		}
	}
}

func TestRun1264RestoreKeepsInternalJoinedNoteOutOfUserFacingState(t *testing.T) {
	root := t.TempDir()
	grokHome := filepath.Join(root, ".grok")
	cwd := `D:\working\gate-sandbox`
	internalPrompt := "[FlowPilot system note — sub-agents started in this session via the UI]\n\n[flow-engine joined result note]\ninternal synthesis"
	writeGrokChatHistoryFixture(t, grokHome, cwd, "sess-internal", []string{
		fmt.Sprintf(`{"type":"user","content":[{"type":"text","text":%q}]}`, "<user_query>\n"+internalPrompt+"\n</user_query>"),
		`{"type":"assistant","content":"Review outcome submitted: approved"}`,
	})
	writeGrokAuthFileFixture(t, grokHome)
	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-grok", ProviderKey: "grok", HomePath: grokHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})

	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	if err := store.AppendTurnLog(context.Background(), "run-1264", turnLogLine{Kind: turnLogKindPrompt, TurnID: "turn-1266", Prompt: "fix bug 1+1 != 2"}); err != nil {
		t.Fatalf("AppendTurnLog original prompt: %v", err)
	}
	if err := store.AppendTurnLog(context.Background(), "run-1264", turnLogLine{Kind: turnLogKindPrompt, TurnID: "turn-3384", Prompt: internalPrompt}); err != nil {
		t.Fatalf("AppendTurnLog internal prompt: %v", err)
	}
	if err := store.AppendTurnLog(context.Background(), "run-1264", turnLogLine{Kind: turnLogKindGrokSession, TurnID: "turn-3384", SessionID: "sess-internal"}); err != nil {
		t.Fatalf("AppendTurnLog grok session: %v", err)
	}

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{Key: ProviderKeyGrok, Status: ProviderStatusAvailable, Capabilities: ProviderCapabilities{Streaming: true}})
	svc := NewInteractiveServiceWithStore(reg, newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-grok"
	rs := &interactiveRun{
		id:                "run-1264",
		providerKey:       ProviderKeyGrok,
		providerAccountID: "acct-grok",
		workspaceCwd:      cwd,
		runKind:           "chat",
		status:            RunStatusCompleted,
		lastPrompt:        "fix bug 1+1 != 2",
		resumedFromDisk:   true,
		subs:              map[int64]chan ProviderEvent{},
		idempotency:       map[string]string{},
		createdAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.seedGrokTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	lastPrompt := rs.lastPrompt
	svc.mu.Unlock()
	if lastPrompt != "fix bug 1+1 != 2" {
		t.Fatalf("lastPrompt = %q, want original user prompt", lastPrompt)
	}
	var sawOriginal bool
	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt == "fix bug 1+1 != 2" {
			sawOriginal = true
		}
		if e.Type == EventTurnStarted && isSystemPrompt(e.Prompt) {
			t.Fatalf("internal flow-engine prompt replayed as user-facing prompt: %q", e.Prompt)
		}
	}
	if !sawOriginal {
		t.Fatal("original user prompt was not restored into the replayed transcript")
	}
}

func TestRun1264CodexRestoreAlsoKeepsInternalJoinedNoteOutOfUserFacingState(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex-home")
	writeProviderAccountsConfig083(t, root, []ProviderAccount{
		{ID: "acct-codex", ProviderKey: "codex", HomePath: codexHome, SlotIndex: 1, AuthStatus: "connected", IsActive: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	})
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	internalPrompt := "[FlowPilot system note — sub-agents started in this session via the UI]\n\n[flow-engine joined result note]\ninternal synthesis"
	if err := store.AppendTurnLog(context.Background(), "run-1264-codex", turnLogLine{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: "fix bug 1+1 != 2"}); err != nil {
		t.Fatalf("AppendTurnLog original prompt: %v", err)
	}
	if err := store.AppendTurnLog(context.Background(), "run-1264-codex", turnLogLine{Kind: turnLogKindPrompt, TurnID: "turn-2", Prompt: internalPrompt}); err != nil {
		t.Fatalf("AppendTurnLog internal prompt: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	svc.activeAccountID = "acct-codex"
	rs := &interactiveRun{
		id:                "run-1264-codex",
		providerKey:       ProviderKeyCodex,
		providerSessionID: "thread-codex",
		providerAccountID: "acct-codex",
		workspaceCwd:      "/repo",
		runKind:           "workflow",
		status:            RunStatusCompleted,
		lastPrompt:        "fix bug 1+1 != 2",
		resumedFromDisk:   true,
		subs:              map[int64]chan ProviderEvent{},
		idempotency:       map[string]string{},
		createdAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.seedTranscriptFromDisk(rs)

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()
	var sawOriginal bool
	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt == "fix bug 1+1 != 2" {
			sawOriginal = true
		}
		if e.Type == EventTurnStarted && isSystemPrompt(e.Prompt) {
			t.Fatalf("codex internal flow-engine prompt replayed as user-facing prompt: %q", e.Prompt)
		}
	}
	if !sawOriginal {
		t.Fatal("codex original user prompt was not restored into the replayed transcript")
	}
}

func TestRun1264StartTurnDoesNotOverwriteTitleWithJoinedResultNote(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	stepID := "chat-" + parent.RunID
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: stepID, Prompt: "fix bug 1+1 != 2"}, "", ""); apiErr != nil {
		t.Fatalf("first startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "first turn completes", time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[parent.RunID]
		return !rs.turnInFlight && !rs.pendingFlowGateSettle && rs.postTurnGateCancel == nil
	})
	joinedNote := "[FlowPilot system note — sub-agents started in this session via the UI]\n\n[flow-engine joined result note]\ninternal synthesis"
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: stepID, Prompt: joinedNote}, "", ""); apiErr != nil {
		t.Fatalf("joined-note startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "joined-note turn completes", time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})

	svc.mu.Lock()
	got := svc.runs[parent.RunID].lastPrompt
	svc.mu.Unlock()
	if got != "fix bug 1+1 != 2" {
		t.Fatalf("lastPrompt = %q, want original user prompt", got)
	}
}

func TestRun1264RestoreOrdersDurableAgentLifecycleAndAvoidsDuplicateSpawn(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const (
		parentRunID = "run-1264-parent"
		childRunID  = "run-1264-coder"
	)
	for _, session := range []ProviderSessionState{
		{RunID: parentRunID, ProviderKey: ProviderKeyCodex, ProviderSessionID: "thread-parent", RunKind: "workflow", Status: RunStatusCompleted},
		{
			RunID: childRunID, ParentRunID: parentRunID, AgentName: "coder", Role: "coder", ProviderKey: ProviderKeyCodex,
			ProviderSessionID: "thread-coder", RunKind: "chat", Status: RunStatusCompleted,
			StartedAt: "2026-07-18T14:00:01Z", UpdatedAt: "2026-07-18T14:00:04Z", LastMessage: "implemented",
		},
	} {
		if err := store.UpsertProviderSession(context.Background(), session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyCodex, providerSessionID: "thread-parent", createdAt: "2026-07-18T14:00:00Z",
		events: []ProviderEvent{
			{Type: EventTurnStarted, Prompt: "fix bug 1+1 != 2", OccurredAt: "2026-07-18T14:00:00Z"},
			// Simulates a durable spawn event written by a current runner before restart.
			{Type: EventAgentSpawnedByUser, AgentName: "coder", ChildRunID: childRunID, OccurredAt: "2026-07-18T14:00:01Z"},
			{Type: EventMessageCompleted, Text: "Both reviewers approved", OccurredAt: "2026-07-18T14:00:03Z"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.appendResumedParentAnnotations(rs)

	if got, want := len(rs.events), 4; got != want {
		t.Fatalf("restored events = %d, want %d: %+v", got, want, rs.events)
	}
	if rs.events[1].Type != EventAgentSpawnedByUser || rs.events[1].ChildRunID != childRunID {
		t.Fatalf("spawn event = %+v, want one durable coder spawn before synthesis", rs.events[1])
	}
	// Non-flow path time-sorts: synthesis (14:00:03) lands before the result
	// (14:00:04). The invariant is one spawn + one result for the child, not
	// a global result dump after an unrelated batch of spawns.
	if rs.events[2].Text != "Both reviewers approved" {
		t.Fatalf("synthesis event = %+v, want wall-clock order after spawn", rs.events[2])
	}
	if rs.events[3].Type != EventAgentResultInjected || rs.events[3].ChildRunID != childRunID {
		t.Fatalf("result event = %+v, want completed coder card update", rs.events[3])
	}
	for i, event := range rs.events {
		if event.Seq != int64(i+1) {
			t.Fatalf("event %d seq = %d, want %d", i, event.Seq, i+1)
		}
	}
}

func TestRun1264ComposedSubAgentPromptIsInternal(t *testing.T) {
	prompt := composeAgentSpawnPrompt(&AgentDefinition{Name: "reviewer", Role: "reviewer", SystemPrompt: "You are the review agent."}, "Review the coder result.")
	if !isSystemPrompt(prompt) {
		t.Fatalf("composed sub-agent prompt must not replay as a user message: %q", prompt)
	}
}

func TestRun1264RestoreKeepsAgentLifecycleNarrationOutOfTranscript(t *testing.T) {
	historical := []ProviderEvent{
		{Type: EventMessageCompleted, Text: "Spawned agent **coder**"},
		{Type: EventMessageCompleted, Text: "**[coder]** I'll inspect the change."},
		{Type: EventToolStarted, ToolName: "spawn_agent"},
		{Type: EventMessageCompleted, Text: "Both reviewers approved with no conflicts."},
	}
	got := userFacingTranscriptEvents(historical)
	if len(got) != 2 || got[0].Type != EventToolStarted || got[1].Text != "Both reviewers approved with no conflicts." {
		t.Fatalf("user-facing restore events = %+v, want the spawn anchor and synthesis message", got)
	}
}

func TestRun1264RestoreReplacesSpawnToolAnchorsInPlace(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-1264-anchor-parent"
	for i, name := range []string{"coder", "reviewer", "reviewer-agent"} {
		if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: fmt.Sprintf("run-1264-anchor-child-%d", i), ParentRunID: parentRunID, AgentName: name, Role: name,
			ProviderKey: ProviderKeyGrok, ProviderSessionID: fmt.Sprintf("thread-%d", i), RunKind: "chat", Status: RunStatusCompleted,
			StartedAt: fmt.Sprintf("2026-07-18T14:00:0%dZ", i+1),
		}); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", name, err)
		}
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyGrok, providerSessionID: "thread-parent", createdAt: "2026-07-18T14:00:00Z",
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug", OccurredAt: "2026-07-18T14:00:00Z"},
			{Seq: 2, ID: "spawn-coder", Type: EventToolStarted, ToolName: "spawn_agent", OccurredAt: "2026-07-18T14:00:01Z"},
			{Seq: 3, ID: "spawn-reviewer", Type: EventToolStarted, ToolName: "spawn_agent", OccurredAt: "2026-07-18T14:00:02Z"},
			{Seq: 4, ID: "spawn-local-reviewer", Type: EventToolStarted, ToolName: "spawn_agent", OccurredAt: "2026-07-18T14:00:03Z"},
			{Seq: 5, Type: EventMessageCompleted, Text: "Both reviewers approved", OccurredAt: "2026-07-18T14:00:04Z"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}
	svc.appendResumedParentAnnotations(rs)

	for i, want := range []string{"coder", "reviewer", "reviewer-agent"} {
		event := rs.events[i+1]
		if event.Type != EventAgentSpawnedByUser || event.AgentName != want {
			t.Fatalf("event %d = %+v, want in-place %q agent card", i+1, event, want)
		}
	}
	for i, event := range rs.events {
		if event.Type == EventMessageCompleted && event.Text == "Both reviewers approved" {
			if i < 4 {
				t.Fatalf("synthesis appeared before all three spawn anchors: %+v", rs.events)
			}
			return
		}
	}
	t.Fatalf("missing synthesis event after anchored spawns: %+v", rs.events)
}

func TestRun1264RestorePlacesUnanchoredFlowSpawnsBeforeSynthesis(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	const parentRunID = "run-1264-unanchored-parent"
	for i, name := range []string{"coder", "reviewer", "reviewer-agent"} {
		if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: fmt.Sprintf("run-1264-unanchored-child-%d", i), ParentRunID: parentRunID, AgentName: name, Role: name,
			ProviderKey: ProviderKeyGrok, ProviderSessionID: fmt.Sprintf("thread-%d", i), RunKind: "chat", Status: RunStatusCompleted,
			StartedAt: fmt.Sprintf("2026-07-18T14:47:1%dZ", i), LastMessage: name + " completed",
		}); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", name, err)
		}
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{
		id: parentRunID, providerKey: ProviderKeyGrok, providerSessionID: "thread-parent", createdAt: "2026-07-18T14:46:22Z",
		flowEngineDriven: true,
		events: []ProviderEvent{
			{Seq: 1, Type: EventTurnStarted, Prompt: "fix bug", OccurredAt: "2026-07-18T14:46:23Z"},
			{Seq: 2, Type: EventMessageCompleted, Text: "Both reviewers approved", OccurredAt: "2026-07-18T14:48:04Z"},
		},
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
	}

	svc.appendResumedParentAnnotations(rs)

	// run-5695 fix: each result sits next to its spawn (spawn→result)×N then
	// synthesis — not spawn×N, synthesis, result×N.
	if got, want := len(rs.events), 8; got != want {
		t.Fatalf("restored events = %d, want %d: %+v", got, want, rs.events)
	}
	wantAgents := []string{"coder", "reviewer", "reviewer-agent"}
	for i, want := range wantAgents {
		spawnIdx := 1 + i*2
		resultIdx := spawnIdx + 1
		spawn := rs.events[spawnIdx]
		result := rs.events[resultIdx]
		if spawn.Type != EventAgentSpawnedByUser || spawn.AgentName != want {
			t.Fatalf("event %d = %+v, want spawn for %q", spawnIdx, spawn, want)
		}
		if result.Type != EventAgentResultInjected || result.AgentName != want {
			t.Fatalf("event %d = %+v, want result for %q immediately after spawn", resultIdx, result, want)
		}
		if result.ChildRunID != spawn.ChildRunID {
			t.Fatalf("result child %s != spawn child %s for %q", result.ChildRunID, spawn.ChildRunID, want)
		}
	}
	if got := rs.events[7]; got.Type != EventMessageCompleted || got.Text != "Both reviewers approved" {
		t.Fatalf("synthesis event = %+v, want it after all spawn→result pairs", got)
	}
	for i, event := range rs.events {
		if event.Seq != int64(i+1) {
			t.Fatalf("event %d seq = %d, want %d", i, event.Seq, i+1)
		}
	}
}
