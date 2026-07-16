package runner

import (
	"context"
	"path/filepath"
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
			rs := svc.runs[parent.RunID]
			inflight := rs.turnInFlight
			// V10R4 P2: production rejects new turns while post-turn gate is
			// active — wait for full settle, not only turnInFlight=false.
			pendingGate := rs.pendingFlowGateSettle || rs.postTurnGateCancel != nil
			svc.mu.Unlock()
			return n == wantOffers && !inflight && !pendingGate
		})
	}

	runTurnAndWait("start", 1)      // hub's first turn
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

	// Task-241 D-10 / BUG-259: flow-engine-driven + failed turn must also skip bulk Progress.
	t.Run("flow_engine_driven_failed_turn_skips", func(t *testing.T) {
		reg := newProviderRegistry()
		reg.register(ProviderRegistration{
			Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
			Capabilities: ProviderCapabilities{Streaming: true},
			newAdapter: func() ProviderRuntimeAdapter {
				return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
					b.Emit(ProviderEvent{Type: EventTurnFailed, Error: "provider failed"})
					return nil
				})
			},
		})
		svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
		parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
		if err != nil {
			t.Fatalf("createRun: %v", err)
		}
		seedTwoPendingSteps(svc, parent.RunID)
		svc.markFlowEngineDriven(parent.RunID)
		startAndWaitTurn(t, svc, parent.RunID)
		steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), parent.RunID)
		if s2, _ := stepByID(steps, "s2"); s2.Status == StepStatusDone {
			t.Fatalf("s2 must not bulk-complete on failed flow-engine turn, got DONE")
		}
	})

	// BUG-259 regression: some adapters (codex_adapter.go's real SendTurn) emit
	// EventTurnFailed but still return a nil error — mirrored here by a fake
	// adapter doing exactly that. A NON-flow-engine-driven run (a plain
	// workflow-attached chat, not a real flow-engine run) must still skip the
	// bulk Progress() call when the turn actually failed, even though err==nil,
	// or a provider error (e.g. hitting a usage limit) falsely bulk-completes
	// every still-PENDING workflow step.
	t.Run("failed_turn_with_nil_err_skips_despite_not_flow_engine_driven", func(t *testing.T) {
		reg := newProviderRegistry()
		reg.register(ProviderRegistration{
			Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
			Capabilities: ProviderCapabilities{Streaming: true},
			newAdapter: func() ProviderRuntimeAdapter {
				return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
					b.Emit(ProviderEvent{Type: EventTurnFailed, Error: "You've hit your usage limit."})
					return nil // the exact lie codex_adapter.go's SendTurn tells on failure
				})
			},
		})
		svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
		parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
		if err != nil {
			t.Fatalf("createRun: %v", err)
		}
		seedTwoPendingSteps(svc, parent.RunID)
		startAndWaitTurn(t, svc, parent.RunID)

		svc.mu.Lock()
		gotStatus := svc.runs[parent.RunID].status
		svc.mu.Unlock()
		if gotStatus != RunStatusFailed {
			t.Fatalf("run status = %q, want failed", gotStatus)
		}
		steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), parent.RunID)
		if s1, _ := stepByID(steps, "s1"); s1.Status != StepStatusFailed {
			t.Errorf("s1 = %q, want FAILED — the run's own turn failed while this step was in flight; it must settle to FAILED, not hang at RUNNING or get bulk-marked DONE", s1.Status)
		}
		if s2, _ := stepByID(steps, "s2"); s2.Status != StepStatusPending {
			t.Fatalf("s2 = %q, want still PENDING (bulk Progress must be skipped when the turn actually failed, regardless of err==nil)", s2.Status)
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

func TestMarkFlowRunCompletePreservesFailedAndSkipsUnstartedSteps(t *testing.T) {
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

	svc.setFlowStepStatus(context.Background(), parent.RunID, "coder", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "reviewer_correctness", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "reviewer_security", StepStatusFailed)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "synthesis", StepStatusPending)

	svc.markFlowRunComplete(context.Background(), parent.RunID)

	if got := flowStepStatus(t, svc, parent.RunID, "reviewer_security"); got != StepStatusFailed {
		t.Errorf("failed reviewer status = %v, want FAILED after markFlowRunComplete", got)
	}
	if got := flowStepStatus(t, svc, parent.RunID, "synthesis"); got != StepStatusSkipped {
		t.Errorf("pending synthesis status = %v, want SKIPPED after markFlowRunComplete", got)
	}
}

// TestMarkFlowRunCompleteSettlesLingeringChildAgentRun is the regression test
// for BUG-235: markFlowRunComplete settled the flow's STEP timeline but never
// touched a child agent RUN whose own status update never landed, so it stayed
// "running" in the Agents panel and the main chat's inline run card even after
// the flow that spawned it was done. Constructs a child run directly (rather
// than driving spawnChildRun's async turn machinery, whose internal retry
// behavior on a non-terminating adapter is orthogonal to what this test proves)
// to deterministically simulate a lingering "running" child with no turn in
// flight — the orphan markFlowRunComplete must now clean up.
func TestMarkFlowRunCompleteSettlesLingeringChildAgentRun(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, apiErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)

	childID := "run-child-lingering"
	svc.mu.Lock()
	svc.runs[childID] = &interactiveRun{
		id:          childID,
		parentRunID: parent.RunID,
		agentName:   "reviewer_correctness",
		role:        "reviewer",
		label:       "reviewer_correctness",
		status:      RunStatusRunning,
		providerKey: ProviderKeyCodex,
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, childID)
	svc.agentOrchestrator.upsertSummary(parent.RunID, AgentRunSummary{
		RunID: childID, AgentName: "reviewer_correctness", Role: "reviewer",
		Status: RunStatusRunning, ParentRunID: parent.RunID,
	})

	svc.markFlowRunComplete(context.Background(), parent.RunID)

	svc.mu.Lock()
	gotStatus := svc.runs[childID].status
	svc.mu.Unlock()
	if gotStatus != RunStatusCompleted {
		t.Errorf("child run status after markFlowRunComplete = %q, want completed", gotStatus)
	}
	summary, ok := svc.agentOrchestrator.currentSummary(parent.RunID, childID)
	if !ok || summary.Status != RunStatusCompleted {
		t.Errorf("child summary status = %+v, want completed", summary)
	}
}

func TestMarkFlowRunCompleteSettlesParentWaitingQuestion(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, apiErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowNodes = nodes
	rs.status = RunStatusWaitingQuestion
	rs.agentStatus = string(RunStatusWaitingQuestion)
	rec := &questionRecord{
		id:      "q-runtime-drive",
		runID:   parent.RunID,
		prompt:  "Select Google Drive context",
		status:  "pending",
		resolve: make(chan questionResolveResult, 1),
	}
	svc.questions[rec.id] = rec
	rs.pendingQuestionID = rec.id
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)

	svc.markFlowRunComplete(context.Background(), parent.RunID)

	svc.mu.Lock()
	gotStatus := svc.runs[parent.RunID].status
	gotAgentStatus := svc.runs[parent.RunID].agentStatus
	gotPendingQuestionID := svc.runs[parent.RunID].pendingQuestionID
	gotQuestionStatus := svc.questions[rec.id].status
	svc.mu.Unlock()
	if gotStatus != RunStatusCompleted {
		t.Fatalf("parent status = %q, want completed after flow done", gotStatus)
	}
	if gotAgentStatus != string(RunStatusCompleted) {
		t.Fatalf("parent agentStatus = %q, want completed after flow done", gotAgentStatus)
	}
	if gotPendingQuestionID != "" {
		t.Fatalf("pendingQuestionID = %q, want cleared after flow done", gotPendingQuestionID)
	}
	if gotQuestionStatus != "expired" {
		t.Fatalf("question status = %q, want expired so history reopen cannot replay the gate", gotQuestionStatus)
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
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-mid",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyClaude,
		WorkflowID:      "wf-1",
		RunKind:         "workflow",
		Status:          RunStatusRunning, // normalized to cancelled on resume
		StartedAt:       "2026-07-02T00:00:00Z",
		UpdatedAt:       "2026-07-02T00:05:00Z",
		ActiveFlowNodes: nodes,
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if rs.status != RunStatusCancelled {
		t.Fatalf("reconstructed flow status = %q, want cancelled after restart", rs.status)
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

func TestReconstructIncompleteCompletedFlowRunCancelsResumeAndDisablesAutoOrchestrate(t *testing.T) {
	svc, _ := newTestServer(t)
	nodes := reviewLoopTestNodes()
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:               "run-mid-completed",
		ProjectID:           "proj",
		ProviderKey:         ProviderKeyCodex,
		WorkflowID:          "wf-1",
		RunKind:             "workflow",
		Status:              RunStatusCompleted,
		StartedAt:           "2026-07-02T00:00:00Z",
		UpdatedAt:           "2026-07-02T00:05:00Z",
		ActiveFlowNodes:     nodes,
		AutoOrchestrate:     true,
		PendingAgentContext: []string{"[flow-engine joined result note] something pending"},
		LoopState:           AgentLoopState{Status: "running", Round: 1, RoundCap: 3, Cap: 3, Mode: "explicit"},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if rs.status != RunStatusCancelled {
		t.Fatalf("status = %q, want cancelled for an incomplete flow restored after restart", rs.status)
	}
	if rs.autoOrchestrate {
		t.Fatalf("autoOrchestrate = true, want false after restart-cancel normalization")
	}
}

func TestReconstructCompletedAutoFlowWithoutLoopStateKeepsCompleted(t *testing.T) {
	svc, _ := newTestServer(t)
	nodes := reviewLoopTestNodes()
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-complete-legacy",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyCodex,
		WorkflowID:      "wf-1",
		RunKind:         "workflow",
		Status:          RunStatusCompleted,
		StartedAt:       "2026-07-02T00:00:00Z",
		UpdatedAt:       "2026-07-02T00:05:00Z",
		ActiveFlowNodes: nodes,
		AutoOrchestrate: true,
		LoopState:       AgentLoopState{},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if rs.status != RunStatusCompleted {
		t.Fatalf("status = %q, want completed for a settled flow missing legacy loop-state metadata", rs.status)
	}
	if !rs.autoOrchestrate {
		t.Fatalf("autoOrchestrate = false, want preserved for a completed flow")
	}
}

func TestReconstructResumeKeepsCompletedFlowNodesAndCancelsSyntheticHub(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	ctx := context.Background()
	nodes := []agentpack.FlowNode{
		{ID: "coder-gpt", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"},
		{ID: "review-gpt", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"coder-gpt"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", DependsOn: []string{"review-gpt"}},
	}
	now := "2026-07-07T15:44:07Z"
	for _, session := range []ProviderSessionState{
		{
			RunID: "run-coder-1", ParentRunID: "run-parent-1",
			ProviderKey: ProviderKeyCodex, RunKind: "chat",
			AgentName: "coder-agent", Role: "coder-agent",
			Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now,
		},
		{
			RunID: "run-review-1", ParentRunID: "run-parent-1",
			ProviderKey: ProviderKeyCodex, RunKind: "chat",
			AgentName: "reviewer-agent", Role: "reviewer-agent",
			Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertProviderSession(ctx, session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}

	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-parent-1",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyCodex,
		WorkflowID:      "wf-1",
		RunKind:         "workflow",
		Status:          RunStatusCompleted,
		StartedAt:       now,
		UpdatedAt:       now,
		ActiveFlowNodes: nodes,
		AutoOrchestrate: true,
		PendingAgentContext: []string{
			"[flow-engine joined result note]\nFlow round 0 — joined.",
		},
		LoopState: AgentLoopState{Status: "running", ActiveNode: "synthesis", Round: 0, Cap: 3, RoundCap: 3},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if rs.status != RunStatusCancelled {
		t.Fatalf("status = %q, want cancelled", rs.status)
	}
	steps, err := svc.workflowStore.LoadRunSteps(ctx, "run-parent-1")
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	if got := flowStepStatus(t, svc, "run-parent-1", "coder-gpt"); got != StepStatusDone {
		t.Fatalf("coder-gpt = %q, want DONE", got)
	}
	if got := flowStepStatus(t, svc, "run-parent-1", "review-gpt"); got != StepStatusDone {
		t.Fatalf("review-gpt = %q, want DONE", got)
	}
	if got := flowStepStatus(t, svc, "run-parent-1", "synthesis"); got != StepStatusCanceled {
		t.Fatalf("synthesis = %q, want CANCELED", got)
	}
	if len(steps) != len(nodes) {
		t.Fatalf("restored steps = %d, want %d", len(steps), len(nodes))
	}
}

// TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone is the
// BUG-260 regression: CA-251/BUG-254 let the hub declare a round done off the
// surviving cohort members even when one member genuinely FAILED — a live
// Claude repro (run-10239) hit a cohort of two reviewers where one failed
// ("claude-review-fake-model") and one completed ("my-reviewer"); the hub still
// escalated, got resolved, and reached loop_state.status "done". On restart,
// resumedFlowStepRows's old fast path treated ANY terminal, non-blocked loop
// state as "every node genuinely succeeded" and bulk-marked all nodes DONE,
// silently overwriting the failed reviewer's real outcome. It must instead
// still read FAILED from that reviewer's own persisted session.
func TestReconstructResumeKeepsFailedCohortMemberDespiteFlowReachingDone(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	ctx := context.Background()
	nodes := []agentpack.FlowNode{
		{ID: "my-coder", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"},
		{ID: "my-reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"my-coder"}},
		{ID: "claude-review-fake-model", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"my-coder"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", DependsOn: []string{"my-reviewer", "claude-review-fake-model"}},
	}
	now := "2026-07-08T01:39:31Z"
	for _, session := range []ProviderSessionState{
		{
			RunID: "run-10244", ParentRunID: "run-10239",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			Label: "my-coder", AgentName: "coder-agent", Role: "coder-agent",
			Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now,
		},
		{
			RunID: "run-10331", ParentRunID: "run-10239",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			Label: "my-reviewer", AgentName: "reviewer-agent", Role: "reviewer-agent",
			FlowCohortID: "flow-auto-my-coder-round-0",
			Status:       RunStatusCompleted, StartedAt: now, UpdatedAt: now,
		},
		{
			RunID: "run-10339", ParentRunID: "run-10239",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			Label: "claude-review-fake-model", AgentName: "reviewer-agent", Role: "reviewer-agent",
			FlowCohortID: "flow-auto-my-coder-round-0",
			Status:       RunStatusFailed, StartedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertProviderSession(ctx, session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}

	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:               "run-10239",
		ProjectID:           "proj",
		ProviderKey:         ProviderKeyClaude,
		WorkflowID:          "wf-1",
		RunKind:             "workflow",
		Status:              RunStatusCompleted,
		StartedAt:           now,
		UpdatedAt:           now,
		ActiveFlowNodes:     nodes,
		AutoOrchestrate:     true,
		PendingAgentContext: []string{"Flow completed."},
		LoopState:           AgentLoopState{Status: "done", Round: 0, Cap: 3, RoundCap: 3, Mode: "explicit"},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	_ = rs
	if got := flowStepStatus(t, svc, "run-10239", "my-coder"); got != StepStatusDone {
		t.Errorf("my-coder = %q, want DONE", got)
	}
	if got := flowStepStatus(t, svc, "run-10239", "my-reviewer"); got != StepStatusDone {
		t.Errorf("my-reviewer = %q, want DONE", got)
	}
	if got := flowStepStatus(t, svc, "run-10239", "claude-review-fake-model"); got != StepStatusFailed {
		t.Fatalf("claude-review-fake-model = %q, want FAILED — this reviewer genuinely failed; the flow reaching loop_state.status=done afterward must not overwrite it to DONE", got)
	}
	if got := flowStepStatus(t, svc, "run-10239", "synthesis"); got != StepStatusDone {
		t.Errorf("synthesis = %q, want DONE (no child session evidence for the inline hub node — falls back to DONE because the flow genuinely completed)", got)
	}
}

func TestReconstructResumeCancelsRunningReviewerButKeepsCompletedCoder(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	ctx := context.Background()
	nodes := []agentpack.FlowNode{
		{ID: "coder-gpt", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"},
		{ID: "review-gpt", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"coder-gpt"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", DependsOn: []string{"review-gpt"}},
	}
	now := "2026-07-07T15:49:33Z"
	for _, session := range []ProviderSessionState{
		{
			RunID: "run-coder-2", ParentRunID: "run-parent-2",
			ProviderKey: ProviderKeyCodex, RunKind: "chat",
			AgentName: "coder-agent", Role: "coder-agent",
			Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now,
		},
		{
			RunID: "run-review-2", ParentRunID: "run-parent-2",
			ProviderKey: ProviderKeyCodex, RunKind: "chat",
			AgentName: "reviewer-agent", Role: "reviewer-agent",
			Status: RunStatusRunning, StartedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertProviderSession(ctx, session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}

	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-parent-2",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyCodex,
		WorkflowID:      "wf-1",
		RunKind:         "workflow",
		Status:          RunStatusCompleted,
		StartedAt:       now,
		UpdatedAt:       now,
		ActiveFlowNodes: nodes,
		AutoOrchestrate: true,
		PendingAgentContext: []string{
			"[flow-engine] An agent has already been spawned to work on this request.",
		},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if rs.status != RunStatusCancelled {
		t.Fatalf("status = %q, want cancelled", rs.status)
	}
	if got := flowStepStatus(t, svc, "run-parent-2", "coder-gpt"); got != StepStatusDone {
		t.Fatalf("coder-gpt = %q, want DONE", got)
	}
	if got := flowStepStatus(t, svc, "run-parent-2", "review-gpt"); got != StepStatusCanceled {
		t.Fatalf("review-gpt = %q, want CANCELED", got)
	}
	if got := flowStepStatus(t, svc, "run-parent-2", "synthesis"); got != StepStatusPending {
		t.Fatalf("synthesis = %q, want PENDING because synthesis never started", got)
	}
}

func TestReconstructResumeRestoresDistinctReviewerStatusesByPersistedLabel(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".flowpilot", "chats")
	store, err := NewLocalFileSessionStore(dataDir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	ctx := context.Background()
	nodes := []agentpack.FlowNode{
		{ID: "coder-gpt", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"},
		{ID: "review-gpt", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"coder-gpt"}},
		{ID: "review-security-gpt", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"coder-gpt"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", DependsOn: []string{"review-gpt", "review-security-gpt"}},
	}
	now := "2026-07-07T22:12:36Z"
	for _, session := range []ProviderSessionState{
		{
			RunID: "run-coder-3", ParentRunID: "run-parent-3",
			ProjectID:   "proj",
			ProviderKey: ProviderKeyCodex, RunKind: "chat",
			AgentName: "coder-agent", Label: "coder-gpt", Role: "coder-agent",
			Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now,
		},
		{
			RunID: "run-review-done-3", ParentRunID: "run-parent-3",
			ProjectID:   "proj",
			ProviderKey: ProviderKeyCodex, RunKind: "chat",
			AgentName: "reviewer-agent", Label: "review-gpt", Role: "reviewer-agent",
			Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now,
		},
		{
			RunID: "run-review-failed-3", ParentRunID: "run-parent-3",
			ProjectID:   "proj",
			ProviderKey: ProviderKeyCodex, RunKind: "chat",
			AgentName: "reviewer-agent", Label: "review-security-gpt", Role: "reviewer-agent",
			Status: RunStatusFailed, StartedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertProviderSession(ctx, session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}

	restartedStore, err := NewLocalFileSessionStore(dataDir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore(restart): %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), restartedStore)

	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-parent-3",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyCodex,
		WorkflowID:      "wf-1",
		RunKind:         "workflow",
		Status:          RunStatusCompleted,
		StartedAt:       now,
		UpdatedAt:       now,
		ActiveFlowNodes: nodes,
		AutoOrchestrate: true,
		PendingAgentContext: []string{
			"[flow-engine joined result note]\nFlow round 0 — joined.",
		},
		LoopState: AgentLoopState{Status: "running", ActiveNode: "synthesis", Round: 0, Cap: 3, RoundCap: 3},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if rs.status != RunStatusCancelled {
		t.Fatalf("status = %q, want cancelled", rs.status)
	}
	if got := flowStepStatus(t, svc, "run-parent-3", "review-gpt"); got != StepStatusDone {
		t.Fatalf("review-gpt = %q, want DONE", got)
	}
	if got := flowStepStatus(t, svc, "run-parent-3", "review-security-gpt"); got != StepStatusFailed {
		t.Fatalf("review-security-gpt = %q, want FAILED", got)
	}
	if got := flowStepStatus(t, svc, "run-parent-3", "synthesis"); got != StepStatusCanceled {
		t.Fatalf("synthesis = %q, want CANCELED", got)
	}
}

func TestReconstructResumeInfersLegacyReviewerStatusesFromCohortOrder(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), ".flowpilot", "chats")
	store, err := NewLocalFileSessionStore(dataDir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	ctx := context.Background()
	nodes := []agentpack.FlowNode{
		{ID: "coder-gpt", Behavior: "agent.delegate", Agent: "agents/coder-agent.md"},
		{ID: "review-gpt", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"coder-gpt"}},
		{ID: "review-security-gpt", Behavior: "agent.delegate", Agent: "agents/reviewer-agent.md", DependsOn: []string{"coder-gpt"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", DependsOn: []string{"review-gpt", "review-security-gpt"}},
	}
	edges := []agentpack.FlowEdge{
		{From: "coder-gpt", To: "review-gpt", When: "done", Kind: "forward"},
		{From: "coder-gpt", To: "review-security-gpt", When: "done", Kind: "forward"},
		{From: "review-gpt", To: "synthesis", When: "done", Kind: "forward"},
		{From: "review-security-gpt", To: "synthesis", When: "done", Kind: "forward"},
	}
	for _, session := range []ProviderSessionState{
		{
			RunID: "run-review-done-legacy", ParentRunID: "run-parent-legacy",
			ProjectID: "proj", ProviderKey: ProviderKeyCodex, RunKind: "chat",
			AgentName: "reviewer-agent", Role: "reviewer-agent", FlowCohortID: "flow-auto-coder-gpt-round-2",
			Status: RunStatusCompleted, StartedAt: "2026-07-07T22:21:10Z", UpdatedAt: "2026-07-07T22:23:20Z",
		},
		{
			RunID: "run-review-failed-legacy", ParentRunID: "run-parent-legacy",
			ProjectID: "proj", ProviderKey: ProviderKeyCodex, RunKind: "chat",
			AgentName: "reviewer-agent", Role: "reviewer-agent", FlowCohortID: "flow-auto-coder-gpt-round-2",
			Status: RunStatusFailed, StartedAt: "2026-07-07T22:21:11Z", UpdatedAt: "2026-07-07T22:21:14Z",
		},
	} {
		if err := store.UpsertProviderSession(ctx, session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}

	restartedStore, err := NewLocalFileSessionStore(dataDir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore(restart): %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), restartedStore)
	_, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-parent-legacy",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyCodex,
		WorkflowID:      "wf-1",
		RunKind:         "workflow",
		Status:          RunStatusCompleted,
		StartedAt:       "2026-07-07T22:08:01Z",
		UpdatedAt:       "2026-07-07T22:23:27Z",
		ActiveFlowNodes: nodes,
		ActiveFlowEdges: edges,
		AutoOrchestrate: true,
		PendingAgentContext: []string{
			"[flow-engine joined result note]\nFlow round 2 — joined.",
		},
		LoopState: AgentLoopState{Status: "blocked", ActiveNode: "synthesis", Round: 3, Cap: 3, RoundCap: 3},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if got := flowStepStatus(t, svc, "run-parent-legacy", "review-gpt"); got != StepStatusDone {
		t.Fatalf("review-gpt = %q, want DONE", got)
	}
	if got := flowStepStatus(t, svc, "run-parent-legacy", "review-security-gpt"); got != StepStatusFailed {
		t.Fatalf("review-security-gpt = %q, want FAILED", got)
	}
}

// stepWriteBeforeEmitProbe wraps a *fakeWorkflowStore (embedded by pointer, not
// by interface, so it still satisfies workflowRunSeeder — reseedFlowStepRuntime
// silently no-ops against anything that doesn't) to observe, at the instant
// hubNodeID's step is transitioned to StepStatusDone, how many "done"
// agent_graph_updated events have already been recorded on the parent run's
// timeline. Used by TestApplyFlowControlDoneSettlesStepsBeforeEmittingAgentGraph
// (BUG-242) to prove step settlement happens BEFORE the desktop-facing emit,
// not after.
type stepWriteBeforeEmitProbe struct {
	*fakeWorkflowStore
	svc         *InteractiveService
	parentRunID string
	hubNodeID   string

	sawHubDoneWrite                 bool
	doneAgentGraphEventsAtWriteTime int
}

func (p *stepWriteBeforeEmitProbe) ApplyStepTransition(ctx context.Context, runID string, t WorkflowStepTransition) error {
	if runID == p.parentRunID && t.StepID == p.hubNodeID && t.Patch.Status == StepStatusDone {
		p.svc.mu.Lock()
		count := 0
		if rs := p.svc.runs[p.parentRunID]; rs != nil {
			for _, ev := range rs.events {
				if ev.Type == EventAgentGraphUpdated && ev.AgentGraphSnapshot != nil && ev.AgentGraphSnapshot.LoopState.Status == "done" {
					count++
				}
			}
		}
		p.svc.mu.Unlock()
		p.sawHubDoneWrite = true
		p.doneAgentGraphEventsAtWriteTime = count
	}
	return p.fakeWorkflowStore.ApplyStepTransition(ctx, runID, t)
}

// TestApplyFlowControlDoneSettlesStepsBeforeEmittingAgentGraph is the
// regression test for BUG-242 (Bug 3): the flow's "done" control result used
// to call emitAgentGraph — which fires the agent_graph_updated SSE event the
// desktop reacts to by refreshing its step-runtime snapshot
// (refreshWorkflowStepRuntime) — BEFORE markFlowRunComplete settled the
// flow's own step timeline (the hub/synthesis node -> DONE). That let the
// desktop's refresh race ahead of the settlement and read the still-RUNNING
// hub node; since setFlowStepStatus emits no event of its own, nothing
// corrected the stale display until an unrelated event (e.g. a manual agent
// focus switch) triggered another refresh. BUG-233 already fixed this exact
// ordering for the "continue"/looping branch; this proves "done" now does the
// same: by the time the hub node's step is actually written to DONE, no
// "done" agent_graph_updated event has been emitted yet.
func TestApplyFlowControlDoneSettlesStepsBeforeEmittingAgentGraph(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}

	fake, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("newTestServer's workflowStore = %T, want *fakeWorkflowStore", svc.workflowStore)
	}
	probe := &stepWriteBeforeEmitProbe{fakeWorkflowStore: fake, svc: svc, parentRunID: parent.RunID, hubNodeID: "synthesis"}
	svc.workflowStore = probe

	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.setFlowStepStatus(context.Background(), parent.RunID, "synthesis", StepStatusRunning)

	if _, err := svc.applyFlowControl(parent.RunID, FlowControlInput{Status: "done", Summary: "all approved"}); err != nil {
		t.Fatalf("applyFlowControl(done): %v", err)
	}

	if !probe.sawHubDoneWrite {
		t.Fatal("expected markFlowRunComplete to write the synthesis node's step status to DONE")
	}
	if probe.doneAgentGraphEventsAtWriteTime != 0 {
		t.Errorf("%d \"done\" agent_graph_updated event(s) already emitted by the time the synthesis node's step was settled to DONE — "+
			"the desktop's refresh can race ahead of the settlement and read a stale RUNNING status", probe.doneAgentGraphEventsAtWriteTime)
	}

	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), parent.RunID)
	synth, ok := stepByID(steps, "synthesis")
	if !ok || synth.Status != StepStatusDone {
		t.Fatalf("synthesis step status = %+v, want DONE after applyFlowControl(done)", synth)
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
