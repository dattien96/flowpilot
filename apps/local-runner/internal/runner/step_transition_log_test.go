package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

func testRegWithFakeCodex() *ProviderRegistry {
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
	return reg
}

// Task-239 D-1: each successful setFlowStepStatus/setFlowStepPosture on a
// flow-engine-driven run appends exactly one transition line.
func TestStepTransitionLogAppendsOnStatusAndPosture(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(testRegWithFakeCodex(), newInteractiveCatalog(), store)
	parent, apiErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", DependsOn: []string{"coder"}},
	}
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.markFlowEngineDriven(parent.RunID)
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()

	ctx := context.Background()
	svc.setFlowStepStatus(ctx, parent.RunID, "coder", StepStatusRunning)
	svc.setFlowStepPosture(ctx, parent.RunID, "coder", "codex", "gpt-5")
	svc.setFlowStepStatus(ctx, parent.RunID, "coder", StepStatusDone)
	svc.setFlowStepStatus(ctx, parent.RunID, "synthesis", StepStatusRunning)

	lines, err := store.LoadStepTransitions(ctx, parent.RunID)
	if err != nil {
		t.Fatalf("LoadStepTransitions: %v", err)
	}
	if len(lines) != 4 {
		t.Fatalf("got %d transition lines, want 4", len(lines))
	}
	if lines[0].Status != string(StepStatusRunning) || lines[0].NodeID != "coder" {
		t.Errorf("line0 = %+v", lines[0])
	}
	if lines[1].Status != "" || lines[1].Provider != "codex" || lines[1].Model != "gpt-5" {
		t.Errorf("posture line = %+v", lines[1])
	}
	if lines[2].Status != string(StepStatusDone) {
		t.Errorf("line2 status = %q", lines[2].Status)
	}
}

// Task-239 D-1: non-flow-engine runs do not write a transition log.
func TestStepTransitionLogSkippedWhenNotFlowEngineDriven(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(testRegWithFakeCodex(), newInteractiveCatalog(), store)
	parent, apiErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	// Seed a step so ApplyStepTransition is not a pure no-op on empty list.
	svc.seedFlowStepRuntimeRows(parent.RunID, []RuntimeWorkflowStep{{
		ID: "chat-" + parent.RunID, StepType: "chat", Status: StepStatusPending,
	}})
	svc.setFlowStepStatus(context.Background(), parent.RunID, "chat-"+parent.RunID, StepStatusRunning)
	lines, err := store.LoadStepTransitions(context.Background(), parent.RunID)
	if err != nil {
		t.Fatalf("LoadStepTransitions: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected 0 lines for non-flow-engine run, got %d", len(lines))
	}
}

// Task-239 D-2/D-3: replay restores settled nodes; RUNNING → CANCELED on resume.
func TestStepTransitionReplayRestoresSettledAndCancelsRunning(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	runID := "run-replay-1"
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"coder"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", DependsOn: []string{"reviewer"}},
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, line := range []stepTransitionLine{
		{RunID: runID, NodeID: "coder", Status: string(StepStatusRunning), TS: now},
		{RunID: runID, NodeID: "coder", Status: string(StepStatusDone), TS: now},
		{RunID: runID, NodeID: "reviewer", Status: string(StepStatusRunning), TS: now},
		{RunID: runID, NodeID: "synthesis", Status: string(StepStatusPending), TS: now},
	} {
		if err := store.AppendStepTransition(context.Background(), runID, line); err != nil {
			t.Fatalf("AppendStepTransition: %v", err)
		}
	}
	// Simulate restart: fresh service, same store dir.
	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("restart store: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store2)
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           runID,
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyCodex,
		WorkflowID:      "wf-1",
		RunKind:         "workflow",
		Status:          RunStatusRunning,
		StartedAt:       now,
		UpdatedAt:       now,
		ActiveFlowNodes: nodes,
		AutoOrchestrate: true,
		LoopState:       AgentLoopState{Status: "running", Round: 0, Cap: 3, Mode: "explicit", ActiveNode: "reviewer"},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	_ = rs
	if got := flowStepStatus(t, svc, runID, "coder"); got != StepStatusDone {
		t.Errorf("coder = %q, want DONE", got)
	}
	if got := flowStepStatus(t, svc, runID, "reviewer"); got != StepStatusCanceled {
		t.Errorf("reviewer = %q, want CANCELED (was RUNNING at kill)", got)
	}
}

// Task-239 D-3: WAITING_USER_APPROVAL kept when a pending approval exists.
func TestStepTransitionReplayKeepsWaitingWhenApprovalPending(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	runID := "run-wait-1"
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", DependsOn: []string{"coder"}},
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_ = store.AppendStepTransition(context.Background(), runID, stepTransitionLine{
		RunID: runID, NodeID: "coder", Status: string(StepStatusDone), TS: now,
	})
	_ = store.AppendStepTransition(context.Background(), runID, stepTransitionLine{
		RunID: runID, NodeID: "synthesis", Status: string(StepStatusWaitingUserApr), TS: now,
	})
	_ = store.UpsertApproval(context.Background(), ProviderApprovalState{
		ApprovalID: "appr-1", RunID: runID, Status: "pending",
	})

	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("restart store: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store2)
	_, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID: runID, ProjectID: "proj", ProviderKey: ProviderKeyCodex,
		WorkflowID: "wf-1", RunKind: "workflow", Status: RunStatusWaitingApproval,
		StartedAt: now, UpdatedAt: now, ActiveFlowNodes: nodes, AutoOrchestrate: true,
		LoopState: AgentLoopState{Status: "blocked", ActiveNode: "synthesis", Mode: "explicit", Cap: 3},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if got := flowStepStatus(t, svc, runID, "synthesis"); got != StepStatusWaitingUserApr {
		t.Fatalf("synthesis = %q, want WAITING_USER_APPROVAL (pending approval on disk)", got)
	}
	if got := flowStepStatus(t, svc, runID, "coder"); got != StepStatusDone {
		t.Fatalf("coder = %q, want DONE", got)
	}
}

// Task-239 D-7: FAILED in log is never promoted to DONE when flow reaches done.
func TestStepTransitionReplayKeepsFailedDespiteFlowDone(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	runID := "run-fail-1"
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"},
		{ID: "reviewer-ok", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"coder"}},
		{ID: "reviewer-bad", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"coder"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", DependsOn: []string{"reviewer-ok", "reviewer-bad"}},
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, line := range []stepTransitionLine{
		{RunID: runID, NodeID: "coder", Status: string(StepStatusDone), TS: now},
		{RunID: runID, NodeID: "reviewer-ok", Status: string(StepStatusDone), TS: now},
		{RunID: runID, NodeID: "reviewer-bad", Status: string(StepStatusFailed), TS: now},
		{RunID: runID, NodeID: "synthesis", Status: string(StepStatusDone), TS: now},
	} {
		_ = store.AppendStepTransition(context.Background(), runID, line)
	}
	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store2)
	_, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID: runID, ProjectID: "proj", ProviderKey: ProviderKeyCodex,
		WorkflowID: "wf-1", RunKind: "workflow", Status: RunStatusCompleted,
		StartedAt: now, UpdatedAt: now, ActiveFlowNodes: nodes, AutoOrchestrate: true,
		PendingAgentContext: []string{"Flow completed."},
		LoopState:           AgentLoopState{Status: "done", Round: 0, Cap: 3, Mode: "explicit"},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if got := flowStepStatus(t, svc, runID, "reviewer-bad"); got != StepStatusFailed {
		t.Fatalf("reviewer-bad = %q, want FAILED (I-3)", got)
	}
}

// Task-239 D-5: normalizeResumedStatus covers spawned / waiting_dependency.
func TestNormalizeResumedStatusSpawnedAndWaitingDependency(t *testing.T) {
	if got := normalizeResumedStatus(RunStatus("spawned")); got != RunStatusCancelled {
		t.Errorf("spawned → %q, want cancelled", got)
	}
	if got := normalizeResumedStatus(RunStatus("waiting_dependency")); got != RunStatusCancelled {
		t.Errorf("waiting_dependency → %q, want cancelled", got)
	}
	if got := normalizeResumedStatus(RunStatusCompleted); got != RunStatusCompleted {
		t.Errorf("completed must stay completed, got %q", got)
	}
}

// Task-239 B5: deleteChatSession removes step-transition sidecar.
func TestDeleteChatSessionRemovesStepTransitions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(testRegWithFakeCodex(), newInteractiveCatalog(), store)
	parent, apiErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	nodes := []agentpack.FlowNode{{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"}}
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.markFlowEngineDriven(parent.RunID)
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.setFlowStepStatus(context.Background(), parent.RunID, "coder", StepStatusDone)

	path := filepath.Join(dir, parent.RunID+"-step-transitions.ndjson")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected sidecar before delete: %v", err)
	}
	if apiErr := svc.deleteChatSession(parent.RunID); apiErr != nil {
		t.Fatalf("deleteChatSession: %v", apiErr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected sidecar removed, stat err=%v", err)
	}
}

// Task-239 D-6: sessionRecordFrom must carry required flow snapshot fields.
func TestSessionRecordFromPersistShapeIncludesFlowFields(t *testing.T) {
	st := ProviderSessionState{
		RunID:        "r1",
		Label:        "coder",
		FlowCohortID: "flow-auto-coder-round-0",
		LoopState:    AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, Round: 1},
		ActiveFlowNodes: []agentpack.FlowNode{
			{ID: "coder", Behavior: "agent.delegate"},
		},
		ActiveFlowEdges: []agentpack.FlowEdge{
			{From: "coder", To: "synthesis", When: "done"},
		},
	}
	rec := sessionRecordFrom(st)
	if rec.LoopState == nil {
		t.Fatal("LoopState must be persisted when set")
	}
	if rec.FlowCohortID == "" {
		t.Fatal("FlowCohortID must be persisted")
	}
	if rec.Label == "" {
		t.Fatal("Label must be persisted")
	}
	if len(rec.ActiveFlowNodes) == 0 {
		t.Fatal("ActiveFlowNodes must be persisted")
	}
	if len(rec.ActiveFlowEdges) == 0 {
		t.Fatal("ActiveFlowEdges must be persisted")
	}
}

// Task-239 D-4 matrix (core cells): kill mid-cohort with transition log.
func TestFlowRestoreMatrixKillMidCohort(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	runID := "run-matrix-1"
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"},
		{ID: "r1", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"coder"}},
		{ID: "r2", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"coder"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", DependsOn: []string{"r1", "r2"}},
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, line := range []stepTransitionLine{
		{RunID: runID, NodeID: "coder", Status: string(StepStatusDone), TS: now},
		{RunID: runID, NodeID: "r1", Status: string(StepStatusDone), TS: now},
		{RunID: runID, NodeID: "r2", Status: string(StepStatusRunning), TS: now},
	} {
		_ = store.AppendStepTransition(context.Background(), runID, line)
	}
	// Child session evidence for r1 also FAILED would conflict — log wins for r2 running.
	_ = store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "child-r1", ParentRunID: runID, ProviderKey: ProviderKeyCodex, RunKind: "chat",
		Label: "r1", AgentName: "reviewer-agent", Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now,
	})

	store2, _ := NewLocalFileSessionStore(dir)
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store2)
	_, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID: runID, ProjectID: "proj", ProviderKey: ProviderKeyCodex,
		WorkflowID: "wf", RunKind: "workflow", Status: RunStatusRunning,
		StartedAt: now, UpdatedAt: now, ActiveFlowNodes: nodes, AutoOrchestrate: true,
		LoopState: AgentLoopState{Status: "running", Mode: "explicit", Cap: 3},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	cases := map[string]RuntimeWorkflowStepStatus{
		"coder":     StepStatusDone,
		"r1":        StepStatusDone,
		"r2":        StepStatusCanceled,
		"synthesis": StepStatusPending,
	}
	for node, want := range cases {
		if got := flowStepStatus(t, svc, runID, node); got != want {
			t.Errorf("%s = %q, want %q", node, got, want)
		}
	}
}

// Task-239: traversal guard rejects path separators in runID.
func TestStepTransitionsPathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	err = store.AppendStepTransition(context.Background(), "../evil", stepTransitionLine{NodeID: "x", TS: "t"})
	if err == nil {
		t.Fatal("expected path traversal rejection")
	}
	if !strings.Contains(err.Error(), "path separators") {
		t.Errorf("err = %v", err)
	}
}

// Task-239: malformed lines skipped, valid kept.
func TestStepTransitionsSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	runID := "run-mal"
	path := filepath.Join(dir, runID+"-step-transitions.ndjson")
	content := "not-json\n" + `{"run_id":"run-mal","node_id":"coder","status":"DONE","ts":"t"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	lines, err := store.LoadStepTransitions(context.Background(), runID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(lines) != 1 || lines[0].NodeID != "coder" {
		t.Fatalf("got %+v", lines)
	}
}
