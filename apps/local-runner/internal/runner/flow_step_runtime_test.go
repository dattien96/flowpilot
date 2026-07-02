package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// TestReviewOutcomeToolOfferedOnlyOnHubSynthesisTurn proves the BUG-179 fix: the
// flow control tool is withheld from the hub's first turn (so it cannot finalize
// the flow before the review cohort runs) and offered on its later synthesis
// turn.
func TestReviewOutcomeToolOfferedOnlyOnHubSynthesisTurn(t *testing.T) {
	var mu sync.Mutex
	var offers []bool
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				offers = append(offers, req.OfferReviewOutcomeTool)
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// Mark it a flow hub, the way startResolvedFlow's coder spawn does.
	svc.mu.Lock()
	svc.runs[parent.RunID].autoOrchestrate = true
	svc.mu.Unlock()

	runTurnAndWait := func(prompt string, wantOffers int) {
		if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: prompt}, "", ""); apiErr != nil {
			t.Fatalf("startTurn(%q): %s", prompt, apiErr.msg)
		}
		waitLoop(t, "turn settled", 3*time.Second, func() bool {
			mu.Lock()
			n := len(offers)
			mu.Unlock()
			svc.mu.Lock()
			inflight := svc.runs[parent.RunID].turnInFlight
			svc.mu.Unlock()
			return n == wantOffers && !inflight
		})
	}

	runTurnAndWait("start", 1)     // hub's first turn
	runTurnAndWait("synthesize", 2) // hub's synthesis turn

	mu.Lock()
	defer mu.Unlock()
	if offers[0] {
		t.Fatal("the hub's FIRST turn must NOT be offered the flow control tool (it could finalize before the cohort runs)")
	}
	if !offers[1] {
		t.Fatal("the hub's synthesis turn (turnCount > 1) must be offered the flow control tool")
	}
}

// reviewLoopTestNodes mirrors review-loop.yaml's topology for step-wiring tests
// without depending on the embedded pack loader.
func reviewLoopTestNodes() []agentpack.FlowNode {
	return []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "reviewer_correctness", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"coder"}},
		{ID: "reviewer_security", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"coder"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
	}
}

func stepByID(steps []RuntimeWorkflowStep, id string) (RuntimeWorkflowStep, bool) {
	for _, st := range steps {
		if st.ID == id {
			return st, true
		}
	}
	return RuntimeWorkflowStep{}, false
}

// TestReseedFlowStepRuntimeSeedsOneStepPerNode proves the timeline is rebuilt
// from the flow's real nodes (with node ids), replacing the generic catalog
// steps a workflow-picker launch seeds (BUG-174).
func TestReseedFlowStepRuntimeSeedsOneStepPerNode(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	svc.reseedFlowStepRuntime(parent.RunID, reviewLoopTestNodes())

	steps, loadErr := svc.workflowStore.LoadRunSteps(context.Background(), parent.RunID)
	if loadErr != nil {
		t.Fatalf("LoadRunSteps: %v", loadErr)
	}
	if len(steps) != 4 {
		t.Fatalf("steps = %d, want 4 (one per node): %#v", len(steps), steps)
	}
	for _, want := range []string{"coder", "reviewer_correctness", "reviewer_security", "synthesis"} {
		st, ok := stepByID(steps, want)
		if !ok {
			t.Fatalf("no step for node %q; steps=%#v", want, steps)
		}
		if st.NodeID != want {
			t.Errorf("step %q NodeID = %q, want %q", want, st.NodeID, want)
		}
		if st.Status != StepStatusPending {
			t.Errorf("step %q status = %q, want PENDING", want, st.Status)
		}
	}
	if syn, _ := stepByID(steps, "synthesis"); syn.BehaviorID != "hub.inline" {
		t.Errorf("synthesis BehaviorID = %q, want hub.inline", syn.BehaviorID)
	}
	if coder, _ := stepByID(steps, "coder"); coder.AgentRef != "coder" {
		t.Errorf("coder AgentRef = %q, want coder", coder.AgentRef)
	}
}

// TestSetFlowStepStatusTransitionsByNodeID proves a single node's step can be
// advanced RUNNING → DONE by node id, with timestamps stamped.
func TestSetFlowStepStatusTransitionsByNodeID(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.reseedFlowStepRuntime(parent.RunID, reviewLoopTestNodes())

	svc.setFlowStepStatus(context.Background(), parent.RunID, "coder", StepStatusRunning)
	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), parent.RunID)
	coder, _ := stepByID(steps, "coder")
	if coder.Status != StepStatusRunning || coder.StartedAt == "" {
		t.Fatalf("after RUNNING: coder = %+v, want RUNNING with StartedAt", coder)
	}

	svc.setFlowStepStatus(context.Background(), parent.RunID, "coder", StepStatusDone)
	steps, _ = svc.workflowStore.LoadRunSteps(context.Background(), parent.RunID)
	coder, _ = stepByID(steps, "coder")
	if coder.Status != StepStatusDone || coder.FinishedAt == "" {
		t.Fatalf("after DONE: coder = %+v, want DONE with FinishedAt", coder)
	}
	// A sibling untouched node must remain PENDING — no bulk completion.
	if rc, _ := stepByID(steps, "reviewer_correctness"); rc.Status != StepStatusPending {
		t.Fatalf("reviewer_correctness = %q, want still PENDING (no bulk completion)", rc.Status)
	}
}

// TestTryAdvanceFlowMarksCompletedDoneAndTargetsRunning proves the coder →
// reviewer-cohort transition drives the step timeline node-by-node: the coder
// step becomes DONE and each reviewer step leaves PENDING (RUNNING, or DONE once
// it finishes), instead of the whole list flipping at once (BUG-174).
func TestTryAdvanceFlowMarksCompletedDoneAndTargetsRunning(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].activeFlowEdges = []agentpack.FlowEdge{
		{From: "coder", To: "reviewer_correctness", When: "done", Kind: "forward"},
		{From: "coder", To: "reviewer_security", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "coder", StepStatusRunning)

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "coder", "fixed the bug") {
		t.Fatal("tryAdvanceFlowFromNode returned false; expected it to advance to the reviewer cohort")
	}

	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), parent.RunID)
	if coder, _ := stepByID(steps, "coder"); coder.Status != StepStatusDone {
		t.Fatalf("coder = %q, want DONE after advancing", coder.Status)
	}
	for _, rid := range []string{"reviewer_correctness", "reviewer_security"} {
		st, _ := stepByID(steps, rid)
		// RUNNING (just spawned) or DONE (already completed via the fake adapter's
		// fast cohort join) are both correct — the point is it left PENDING.
		if st.Status == StepStatusPending {
			t.Fatalf("%s = PENDING, want RUNNING or DONE after coder advanced", rid)
		}
	}
}

// TestFlowEngineDrivenRunSkipsBulkProgress proves F-3: a flow-engine-driven run
// does NOT get its steps bulk-completed by PlanWorkflowProgress after a hub
// turn, while an identical run that is NOT flow-engine-driven does (the control).
func TestFlowEngineDrivenRunSkipsBulkProgress(t *testing.T) {
	seedTwoPendingSteps := func(svc *InteractiveService, runID string) {
		seeder := svc.workflowStore.(workflowRunSeeder)
		seeder.seed(runID, []RuntimeWorkflowStep{
			{ID: "s1", StepType: "s1", Status: StepStatusPending},
			{ID: "s2", StepType: "s2", Status: StepStatusPending},
		})
	}
	startAndWaitTurn := func(t *testing.T, svc *InteractiveService, runID string) {
		if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "s1", Prompt: "do the thing"}, "", ""); apiErr != nil {
			t.Fatalf("startTurn: %s", apiErr.msg)
		}
		waitLoop(t, "turn settled", 3*time.Second, func() bool {
			svc.mu.Lock()
			defer svc.mu.Unlock()
			rs := svc.runs[runID]
			return rs != nil && !rs.turnInFlight
		})
	}

	// Control: not flow-engine-driven → Progress runs → both steps bulk-completed.
	t.Run("control_bulk_completes", func(t *testing.T) {
		svc, _ := newTestServer(t)
		parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
		if err != nil {
			t.Fatalf("createRun: %v", err)
		}
		seedTwoPendingSteps(svc, parent.RunID)
		startAndWaitTurn(t, svc, parent.RunID)
		waitLoop(t, "bulk completion by Progress", 2*time.Second, func() bool {
			steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), parent.RunID)
			s2, _ := stepByID(steps, "s2")
			return s2.Status == StepStatusDone
		})
	})

	// Flow-engine-driven → Progress skipped → the untouched s2 stays PENDING.
	t.Run("flow_engine_driven_skips", func(t *testing.T) {
		svc, _ := newTestServer(t)
		parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
		if err != nil {
			t.Fatalf("createRun: %v", err)
		}
		seedTwoPendingSteps(svc, parent.RunID)
		svc.markFlowEngineDriven(parent.RunID)
		startAndWaitTurn(t, svc, parent.RunID)
		steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), parent.RunID)
		if s2, _ := stepByID(steps, "s2"); s2.Status != StepStatusPending {
			t.Fatalf("s2 = %q, want still PENDING (bulk Progress must be skipped for flow-engine-driven runs)", s2.Status)
		}
	})
}

// TestResolveWorkflowFlowRefAdoptsMirroredFlow proves F-1's bridge: a run whose
// workflowID resolves (via the flow definition store, keyed by the mirror row's
// UUID) to a flow-engine flow yields that flow's canonical flowRef, so the
// executor engages for a workflow-picker launch that sent no flowRef.
func TestResolveWorkflowFlowRefAdoptsMirroredFlow(t *testing.T) {
	svc, _ := newTestServer(t)

	// A valid review-loop record from the embedded pack, stored under a UUID key
	// exactly as a mirrored built-in row would be (GetByRef(uuid) returns it).
	rec, err := NewFlowDefinitionResolver(nil).ResolveBuiltin(context.Background(), "flowpilot-core-flow-pack", "review-loop")
	if err != nil {
		t.Fatalf("ResolveBuiltin(review-loop): %v", err)
	}
	const workflowUUID = "11111111-1111-1111-1111-111111111111"
	store := newFakeFlowDefinitionStore()
	store.byRef[workflowUUID] = rec
	svc.SetFlowDefinitionStore(store)

	parent, apiErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].workflowID = workflowUUID
	svc.mu.Unlock()

	ref, ok := svc.resolveWorkflowFlowRef(context.Background(), parent.RunID)
	if !ok {
		t.Fatal("resolveWorkflowFlowRef returned false; want it to adopt the mirrored flow's ref")
	}
	if ref != rec.FlowRef {
		t.Fatalf("resolved flowRef = %q, want %q", ref, rec.FlowRef)
	}
}

// TestSubmitFlowControlRejectsCohortMemberButAllowsHub proves the BUG-176
// join-barrier: a cohort member (reviewer) calling the flow control tool is
// rejected and does NOT advance/terminate the flow, while the hub's own call
// (a non-cohort run) still finalizes it — so "synthesis done while a reviewer
// is still running" can no longer happen via a reviewer short-circuiting the
// cohort join.
func TestSubmitFlowControlRejectsCohortMemberButAllowsHub(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, apiErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	// A cohort-member (reviewer) child run.
	const childID = "child-reviewer-1"
	svc.mu.Lock()
	svc.runs[childID] = &interactiveRun{
		id:           childID,
		parentRunID:  parent.RunID,
		flowCohortId: "cohort-1",
		subs:         map[int64]chan ProviderEvent{},
		idempotency:  map[string]string{},
	}
	child := svc.runs[childID]
	hub := svc.runs[parent.RunID]
	svc.mu.Unlock()

	childBridge := &turnBridge{svc: svc, rs: child}
	if _, err := childBridge.SubmitFlowControl(FlowControlInput{Status: "done"}); err == nil {
		t.Fatal("expected a cohort member's flow_control call to be rejected")
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Status == "done" {
		t.Fatal("a cohort member's flow_control must NOT terminate the flow before the join")
	}

	// The hub (the parent run itself, not a cohort member) may still finalize.
	hubBridge := &turnBridge{svc: svc, rs: hub}
	if _, err := hubBridge.SubmitFlowControl(FlowControlInput{Status: "done"}); err != nil {
		t.Fatalf("the hub's own flow_control must succeed: %v", err)
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Status != "done" {
		t.Fatalf("hub flow_control(done) should mark the loop done, got %q", st.Status)
	}
}

// TestMarkFlowRunCompleteSettlesEveryStep proves the BUG-181 fix: on flow
// completion every non-terminal step is settled to DONE, so a reviewer whose
// DONE write lagged or a synthesis node left RUNNING can't leave the timeline
// inconsistent after the run is done.
func TestMarkFlowRunCompleteSettlesEveryStep(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, apiErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	// Simulate an inconsistent mid-finalization state: coder done, one reviewer
	// still RUNNING, synthesis still RUNNING.
	svc.setFlowStepStatus(context.Background(), parent.RunID, "coder", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "reviewer_correctness", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "reviewer_security", StepStatusRunning)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "synthesis", StepStatusRunning)

	svc.markFlowRunComplete(context.Background(), parent.RunID)

	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), parent.RunID)
	for _, st := range steps {
		if st.Status != StepStatusDone {
			t.Errorf("step %q status = %q, want DONE after markFlowRunComplete", st.ID, st.Status)
		}
	}
}

// TestReconstructWorkflowRunRestoresStepTimeline proves the BUG-178 fix: a
// completed flow run reopened from history after a server restart (its
// in-memory step store now empty) rebuilds its step timeline from the persisted
// flow nodes, with every node shown DONE — instead of "No step-runtime data".
func TestReconstructWorkflowRunRestoresStepTimeline(t *testing.T) {
	svc, _ := newTestServer(t)

	nodes := reviewLoopTestNodes()
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-restored",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyClaude,
		WorkflowID:      "wf-1",
		RunKind:         "workflow",
		Status:          RunStatusCompleted,
		StartedAt:       "2026-07-02T00:00:00Z",
		UpdatedAt:       "2026-07-02T00:05:00Z",
		ActiveFlowNodes: nodes,
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if rs == nil || rs.id != "run-restored" {
		t.Fatalf("reconstructRun returned unexpected run: %+v", rs)
	}

	steps, err := svc.workflowStore.LoadRunSteps(context.Background(), "run-restored")
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	if len(steps) != len(nodes) {
		t.Fatalf("restored steps = %d, want %d (one per flow node): %#v", len(steps), len(nodes), steps)
	}
	for _, want := range []string{"coder", "reviewer_correctness", "reviewer_security", "synthesis"} {
		st, ok := stepByID(steps, want)
		if !ok {
			t.Fatalf("no restored step for node %q; steps=%#v", want, steps)
		}
		if st.Status != StepStatusDone {
			t.Errorf("restored step %q status = %q, want DONE for a completed run", want, st.Status)
		}
		if st.NodeID != want {
			t.Errorf("restored step %q NodeID = %q, want %q", want, st.NodeID, want)
		}
	}
}

// TestReconstructNonCompletedFlowRunRestoresPendingSteps proves a non-completed
// (e.g. restart-cancelled) flow run restores its step structure as PENDING —
// the timeline is shown, without falsely claiming completion.
func TestReconstructNonCompletedFlowRunRestoresPendingSteps(t *testing.T) {
	svc, _ := newTestServer(t)
	nodes := reviewLoopTestNodes()
	if _, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-mid",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyClaude,
		WorkflowID:      "wf-1",
		RunKind:         "workflow",
		Status:          RunStatusRunning, // normalized to cancelled on resume
		StartedAt:       "2026-07-02T00:00:00Z",
		UpdatedAt:       "2026-07-02T00:05:00Z",
		ActiveFlowNodes: nodes,
	}); apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), "run-mid")
	if len(steps) != len(nodes) {
		t.Fatalf("restored steps = %d, want %d", len(steps), len(nodes))
	}
	for _, st := range steps {
		if st.Status != StepStatusPending {
			t.Errorf("step %q status = %q, want PENDING for a non-completed run", st.ID, st.Status)
		}
	}
}

// TestResolveWorkflowFlowRefBailsForPlainRun proves the safe-bail contract: a
// run with no workflowID (a plain chat/normal launch) is never forced onto the
// executor.
func TestResolveWorkflowFlowRefBailsForPlainRun(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if _, ok := svc.resolveWorkflowFlowRef(context.Background(), parent.RunID); ok {
		t.Fatal("expected resolveWorkflowFlowRef to bail (false) for a run with no workflowID")
	}
}
