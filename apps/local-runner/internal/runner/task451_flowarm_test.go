package runner

import (
	"context"
	"encoding/json"
	"flowpilot-runner/internal/agentpack"
	"strings"
	"testing"
)

// Task-451 (CP-89): run/chat-level flowArm latch — immediate|pending|started.
// `pending` pins a flow without starting it: chat turns stay plain chat until
// an explicit forwardFlow turn (Task-452). The latch is run-scoped, must
// round-trip through sessions.ndjson + Drive manifest + reconstruct, and a
// corrupt value fails closed rather than defaulting silently.

func task451Service(t *testing.T) *InteractiveService {
	t.Helper()
	return task451ServiceWithStore(t, newFakeWorkflowStore())
}

func task451ServiceWithStore(t *testing.T, store WorkflowStore) *InteractiveService {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	reg.register(ProviderRegistration{
		Key: ProviderKeyDevin, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	return newInteractiveService(reg, newInteractiveCatalog(), store)
}

// Default: a client that sends no flowArm behaves exactly as today — the run
// arms immediate and a flowRef on turn 1 starts the flow.
func TestTask451_DefaultAbsentArmIsImmediate(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[h.RunID].flowArm
	svc.mu.Unlock()
	if got != FlowArmImmediate {
		t.Fatalf("absent flowArm must resolve immediate, got %q", got)
	}
}

// pending without any flow pin is a contract violation — fail closed.
func TestTask451_PendingRequiresFlowPin(t *testing.T) {
	svc := task451Service(t)
	for _, arm := range []string{"pending", "chat_then_forward"} {
		_, err := svc.createRun(StartRunInput{
			ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
			FlowArm: arm,
		})
		if err == nil || err.code != "flow_arm_requires_flow" {
			t.Fatalf("flowArm=%q with no pin must 400 flow_arm_requires_flow, got %v", arm, err)
		}
	}
}

// Pin-vs-arm split (F-3): chatFlowRef stamps but vibe start markers stay off
// while the run is pending. chat_then_forward aliases to pending.
func TestTask451_PendingCreateKeepsVibeMarkersOff(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef:     "flowpilot-core-flow-pack/vibe-cp-ingest",
		WorkingMode: "vibe", Client: "tui",
		FlowArm: "chat_then_forward",
	})
	if err != nil {
		t.Fatalf("createRun pending: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	ref, arm, lock, budget := rs.chatFlowRef, rs.flowArm, rs.vibeAwaitingLock, rs.vibeSprintBudget
	svc.mu.Unlock()
	if ref == "" {
		t.Fatal("pending pin must still stamp chatFlowRef")
	}
	if arm != FlowArmPending {
		t.Fatalf("chat_then_forward must alias to pending, got %q", arm)
	}
	if lock || budget != 0 {
		t.Fatalf("pending must keep vibe markers off: awaitingLock=%v sprintBudget=%d", lock, budget)
	}
}

// Working-mode fence runs at PIN time for both modes — pending never defers
// FlowAllowedForWorkingMode.
func TestTask451_WorkingModeFenceAtPinTime(t *testing.T) {
	for _, arm := range []string{"", "pending"} {
		svc := task451Service(t)
		_, err := svc.createRun(StartRunInput{
			ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
			FlowRef:     "flowpilot-core-flow-pack/vibe-cp-ingest",
			WorkingMode: "dev", Client: "tui",
			FlowArm: arm,
		})
		if err == nil || !strings.Contains(err.code, "working_mode") {
			t.Fatalf("flowArm=%q: dev pinning vibe flow must be forbidden, got %v", arm, err)
		}
	}
}

// The latch round-trips through the durable session row.
func TestTask451_SessionRowRoundTrip(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness",
		FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	st := sessionStateOf(svc.runs[h.RunID])
	svc.mu.Unlock()
	if st.FlowArm != "pending" {
		t.Fatalf("session row must carry flowArm=pending, got %q", st.FlowArm)
	}
	// Reconstruct from the row — latch survives, no flow machinery armed.
	rs, recErr := svc.reconstructRun(st)
	if recErr != nil {
		t.Fatalf("reconstructRun: %v", recErr)
	}
	if rs.flowArm != FlowArmPending {
		t.Fatalf("reconstructed flowArm = %q, want pending", rs.flowArm)
	}
}

// Restart while pending → the run comes back as a chat run with the pin
// intact; no startResolvedFlow, no vibe markers, no flowEngineDriven.
func TestTask451_RestartPendingReconstructsAsChat(t *testing.T) {
	svc := task451Service(t)
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-p1", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusIdle,
		ChatFlowRef: "task-harness",
		FlowArm:     "pending", TurnCount: 3,
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if rs.flowArm != FlowArmPending {
		t.Fatalf("flowArm = %q, want pending", rs.flowArm)
	}
	if rs.flowEngineDriven {
		t.Fatal("pending reconstruct must not arm flowEngineDriven")
	}
	if rs.chatFlowRef == "" {
		t.Fatal("pending reconstruct must keep the flow pin")
	}
}

// A started run never re-arms: flowArm stays started, vibe markers stay off
// (BUG-315 shape preserved — reconstruction never calls startResolvedFlow).
// A legitimately started flow always carries its persisted topology AND the
// durable entry child row the launch produced (child rows are never pruned) —
// started + topology + no child evidence is the never-launched crash row and
// heals pending (R4-2).
func TestTask451_RestartStartedDoesNotReArm(t *testing.T) {
	store := newFakeWorkflowStore()
	// The entry child row the real launch wrote: ParentRunID + Label = the
	// flow node id. This is what distinguishes a launched flow from a torn
	// started write.
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-s1-entry", ProjectID: "proj", ParentRunID: "run-s1",
		Label: "n1", Status: RunStatusCompleted,
	}); err != nil {
		t.Fatal(err)
	}
	svc := task451ServiceWithStore(t, store)
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-s1", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusRunning,
		ChatFlowRef:     "flowpilot-core-flow-pack/vibe-cp-ingest",
		FlowArm:         "started", TurnCount: 5,
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "n1"}},
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if rs.flowArm != FlowArmStarted {
		t.Fatalf("flowArm = %q, want started", rs.flowArm)
	}
	if rs.vibeAwaitingLock {
		t.Fatal("started run must not re-arm vibeAwaitingLock")
	}
}

// Crash-window heal: arm=started persisted but no flow topology — the
// forward's flip landed before the durable-first topology commit (or a
// pre-fix row hit the spawn-before-persist window). The flow never durably
// established, so the honest state is pending: a retry forwardFlow can
// launch cleanly instead of wedging on flow_already_started.
func TestTask451_StartedWithoutTopologyHealsToPending(t *testing.T) {
	svc := task451Service(t)
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-h1", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusIdle,
		ChatFlowRef: "task-harness",
		FlowArm:     "started", TurnCount: 1,
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if rs.flowArm != FlowArmPending {
		t.Fatalf("flowArm = %q, want pending (never-launched started row must heal)", rs.flowArm)
	}
	if rs.chatFlowRef == "" {
		t.Fatal("healed pending run must keep the flow pin")
	}
}

// Corrupt arm value on disk fails closed — repair surface, never silent
// immediate.
func TestTask451_CorruptArmValueFailsClosed(t *testing.T) {
	svc := task451Service(t)
	_, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-c1", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusIdle,
		FlowArm: "half_armed",
	})
	if err == nil {
		t.Fatal("corrupt flowArm value must fail closed, not default to immediate")
	}
}

// A provider switch mints a new leg but the run-scoped latch rides over:
// the new leg keeps flowArm=pending AND the chatFlowRef pin.
func TestTask451_ProviderSwitchKeepsPending(t *testing.T) {
	svc := task451Service(t)
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness",
		FlowArm: "pending", ChatID: "cht-arm1",
	})
	if err != nil {
		t.Fatalf("createRun pending: %v", err)
	}
	resp, swErr := svc.switchChatProvider(context.Background(), "cht-arm1", chatSwitchRequest{
		TargetProviderKey: ProviderKeyDevin,
	})
	if swErr != nil {
		t.Fatalf("switchChatProvider: %v", swErr)
	}
	if resp.Handle.RunID == h.RunID {
		t.Fatal("switch must mint a new leg")
	}
	svc.mu.Lock()
	nl := svc.runs[resp.Handle.RunID]
	arm, ref := nl.flowArm, nl.chatFlowRef
	svc.mu.Unlock()
	if arm != FlowArmPending {
		t.Fatalf("new leg flowArm = %q, want pending (run-scoped latch)", arm)
	}
	if ref == "" {
		t.Fatal("new leg lost the chatFlowRef pin — forward could never resolve it")
	}
}

// The pending latch must survive the REAL sessions.ndjson file — not just the
// in-memory store list. A process restart reloads through ndjsonSessionRecord;
// if flow_arm is not on the durable row the latch silently drops to the
// immediate default and the pinned flow auto-starts on the next turn.
func TestTask451_FlowArmSurvivesSessionsNDJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyCodex,
		DisplayName:  "Codex",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter:   func() ProviderRuntimeAdapter { return &childCompleteAdapter{} },
	})
	svc := NewInteractiveServiceWithStore(reg, nil, store)
	h, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if apiErr != nil {
		t.Fatalf("createRun pending: %v", apiErr)
	}

	reloaded, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	st, ok, getErr := reloaded.GetProviderSession(context.Background(), h.RunID)
	if getErr != nil || !ok {
		t.Fatalf("reloaded session row missing for %s (ok=%v, err=%v)", h.RunID, ok, getErr)
	}
	if st.FlowArm != "pending" {
		t.Fatalf("flowArm after file reload = %q, want pending — latch must ride sessions.ndjson", st.FlowArm)
	}
	if st.ChatFlowRef == "" {
		t.Fatal("chatFlowRef pin did not survive the file reload")
	}
}

// Corrupt row: pending latch + armed vibe/flow markers must normalize toward
// the latch — a pending run can never have legitimately locked, checkpointed,
// parked a sprint, or accumulated flow-engine state. Every persisted derived
// field is cleared so the reconstructed run is plain chat with the pin kept.
func TestTask451_PendingReconstructClearsStaleVibeMarkers(t *testing.T) {
	svc := task451Service(t)
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-bad1", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusIdle,
		WorkingMode: "vibe",
		ChatFlowRef: "flowpilot-core-flow-pack/vibe-cp-ingest",
		FlowArm:     "pending", TurnCount: 2,
		VibeAwaitingLock:           true,
		VibeTaskPlan:               []string{"task-1"},
		VibeRequirementFromNode:    "tdd",
		VibeSprintIndex:            2,
		VibeSprintBudget:           7,
		VibeSprintBoundaryDeclined: true,
		VibeLockedCP:               "CP-09",
		VibeLockedSS:               "SS-09",
		VibeLockNodeID:             "cp_lock",
		VibeLockPath:               "docs/CP-09.md",
		VibeCheckpointNode:         "cp",
		VibeCheckpointArtifacts:    []string{"docs/CP-09.md"},
		VibeTaskIndex:              3, VibeTaskTotal: 5, VibeTaskName: "t",
		VibeParkedFlowRef:       "flowpilot-core-flow-pack/vibe-sprint",
		VibeParkedAcceptance:    []string{"a"},
		PendingFlowGateSettle:   true,
		PendingFlowGateTurnID:   "turn-9",
		PendingFlowGateFinalMsg: "stale",
		ActiveFlowNodes:         []agentpack.FlowNode{{ID: "n1"}},
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if rs.flowArm != FlowArmPending {
		t.Fatalf("flowArm = %q, want pending", rs.flowArm)
	}
	if rs.chatFlowRef == "" {
		t.Fatal("pin must be retained")
	}
	bad := []struct {
		name string
		got  bool
	}{
		{"vibeAwaitingLock", rs.vibeAwaitingLock},
		{"vibeTaskPlan", len(rs.vibeTaskPlan) > 0},
		{"vibeRequirementFromNode", rs.vibeRequirementFromNode != ""},
		{"vibeSprintIndex", rs.vibeSprintIndex != 0},
		{"vibeSprintBudget", rs.vibeSprintBudget != 0},
		{"vibeSprintBoundaryDeclined", rs.vibeSprintBoundaryDeclined},
		{"vibeLockedCP", rs.vibeLockedCP != ""},
		{"vibeLockedSS", rs.vibeLockedSS != ""},
		{"vibeLockNodeID", rs.vibeLockNodeID != ""},
		{"vibeLockPath", rs.vibeLockPath != ""},
		{"vibeCheckpointNode", rs.vibeCheckpointNode != ""},
		{"vibeCheckpointArtifacts", len(rs.vibeCheckpointArtifacts) > 0},
		{"vibeTaskIndex", rs.vibeTaskIndex != 0},
		{"vibeTaskTotal", rs.vibeTaskTotal != 0},
		{"vibeTaskName", rs.vibeTaskName != ""},
		{"vibeSSSealed", rs.vibeSSSealed},
		{"vibeCPSealed", rs.vibeCPSealed},
		{"vibeParkedNodes", len(rs.vibeParkedNodes) > 0},
		{"vibeParkedEdges", len(rs.vibeParkedEdges) > 0},
		{"vibeParkedAcceptance", len(rs.vibeParkedAcceptance) > 0},
		{"vibeParkedFlowRef", rs.vibeParkedFlowRef != ""},
		{"vibeSprintBoundaryPending", rs.vibeSprintBoundaryPending},
		{"vibeResumeConfirm", rs.vibeResumeConfirm},
		{"vibeResumeFromNode", rs.vibeResumeFromNode != ""},
		{"activeFlowNodes", len(rs.activeFlowNodes) > 0},
		{"flowEngineDriven", rs.flowEngineDriven},
		{"pendingFlowGateSettle", rs.pendingFlowGateSettle},
		{"pendingFlowGateTurnID", rs.pendingFlowGateTurnID != ""},
		{"pendingFlowGateFinalMsg", rs.pendingFlowGateFinalMsg != ""},
	}
	for _, b := range bad {
		if b.got {
			t.Fatalf("corrupt pending row leaked stale %s", b.name)
		}
	}
}

// The Supabase session_runtime JSONB blob is a production persistence surface
// (durable-state contract: the latch must survive restart on EVERY backend —
// BUG-500 series). FlowArm must ride the blob both directions; a run stored
// through Supabase that drops the latch would reconstruct as immediate and
// auto-start a flow the user pinned but never forwarded.
func TestTask451_FlowArmSurvivesSupabaseRuntimeBlob(t *testing.T) {
	raw, err := json.Marshal(sessionRuntimeFromState(ProviderSessionState{
		RunID: "run-sb1", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusIdle,
		ChatFlowRef: "task-harness",
		FlowArm:     "pending", TurnCount: 1,
	}))
	if err != nil {
		t.Fatalf("marshal runtime blob: %v", err)
	}
	sess := ProviderSessionState{RunID: "run-sb1"}
	if err := applySessionRuntimeV2(context.Background(), nil, &sess, raw); err != nil {
		t.Fatalf("applySessionRuntimeV2: %v", err)
	}
	if sess.FlowArm != "pending" {
		t.Fatalf("flowArm lost through supabase runtime blob: got %q, want pending", sess.FlowArm)
	}
}
