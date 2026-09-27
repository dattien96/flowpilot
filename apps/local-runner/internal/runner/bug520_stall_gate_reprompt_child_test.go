package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-520 (live run-60145): the hub stall watchdog fired while a cohort
// member sat in its post-turn gate window with an armed gate-reprompt
// intent — hasActiveFlowChild did not count pendingGateRepromptPrompt /
// pendingFlowGateSettle as activity, read the child as a ghost, and
// parkFlowForAwaitingUser wiped the armed reprompt, orphaning the child
// (waiting_user_approval forever) and parking a healthy tournament.
func TestBug520_StallDoesNotFireOnArmedGateReprompt(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-520p"
	childID := "run-520c"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:                parentID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		hubLastProgressAt: time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:      time.Second,
		subs:              map[int64]chan ProviderEvent{},
	}
	// Exact post-turn shape observed live: turn over (turnInFlight=false),
	// gate evaluated and armed a reprompt intent, but the reprompt turn has
	// not dispatched yet (postTurnGateCancel already cleared).
	svc.runs[childID] = &interactiveRun{
		id:                        childID,
		parentRunID:               parentID,
		label:                     "candidate-b",
		status:                    RunStatusRunning,
		agentStatus:               string(RunStatusRunning),
		flowCohortId:              "flow-auto-parallel_rollout-round-0",
		pendingGateRepromptPrompt: "[flow-gate] fix your change-contract scope",
		pendingGateRepromptStepID: "step-x",
		pendingGateRepromptGen:    1,
		subs:                      map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-parallel_rollout-round-0", 2)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 3
		return st
	})

	if svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("hub_stalled must not fire while a child has an armed gate-reprompt intent")
	}
	if !svc.hasActiveFlowChild(parentID) {
		t.Fatal("child with armed pendingGateRepromptPrompt must count as active")
	}
}

// BUG-520 guard: a child that is genuinely a ghost (running, no turn, no
// gate, no armed reprompt — the BUG-354 shape) still must NOT shield the
// hub — the watchdog has to keep firing for real stalls.
func TestBug520_TrueGhostChildStillStalls(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-522p"
	childID := "run-522c"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:                parentID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		hubLastProgressAt: time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:      time.Second,
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:           childID,
		parentRunID:  parentID,
		label:        "candidate-b",
		status:       RunStatusRunning,
		agentStatus:  string(RunStatusRunning),
		flowCohortId: "flow-auto-parallel_rollout-round-0",
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-parallel_rollout-round-0", 2)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 3
		return st
	})

	if !svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("a true ghost child (no armed work of any kind) must still let hub_stalled fire")
	}
}

// BUG-520 residual (a) — orphan-cure: a child parked waiting_user_approval by
// the pre-fix watchdog had its reprompt fields wiped but kept the durable
// pendingGateCodePaths seed. The cure seam must hold:
//  1. parkFlowForAwaitingUser clears the armed reprompt intent but PRESERVES
//     pendingGateCodePaths (the gate-fail evidence for the forced re-check).
//  2. reinvokeMatchingFlowChild (the Continue resume path, gated on
//     lastEscalatedInlineNodeID / continue back-edge) revives a parked
//     waiting_user_approval child — no status guard must strand it.
//  3. The revived child still carries pendingGateCodePaths, so its next
//     post-turn gate re-checks the held paths and re-arms the reprompt if
//     the diff still fails (forced re-check itself is pinned in
//     bug439_440_gate_contract_test.go).
func TestBug520_ParkedChildRepromptWipedButCodePathsSurviveAndReDrive(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-524p"
	childID := "run-524c"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:                        childID,
		parentRunID:               parentID,
		label:                     "candidate-b",
		status:                    RunStatusRunning,
		agentStatus:               string(RunStatusRunning),
		stepID:                    "step-rollout-b",
		pendingGateRepromptPrompt: "[flow-gate] fix scope drift",
		pendingGateRepromptStepID: "step-rollout-b",
		pendingGateRepromptGen:    2,
		pendingGateCodePaths:      []string{"src/calc.go"},
		subs:                      map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)

	// (1) The park still wipes the armed reprompt (BUG-354 contract: parked
	// flow holds no live auto-intents) — but the code-paths seed survives.
	svc.parkFlowForAwaitingUser(parentID)

	svc.mu.Lock()
	child := svc.runs[childID]
	if child.status != RunStatusWaitingUserApr {
		svc.mu.Unlock()
		t.Fatalf("parked child status = %q, want waiting_user_approval", child.status)
	}
	if child.pendingGateRepromptPrompt != "" || child.pendingGateRepromptStepID != "" {
		svc.mu.Unlock()
		t.Fatalf("park must wipe the armed reprompt intent, got prompt=%q step=%q",
			child.pendingGateRepromptPrompt, child.pendingGateRepromptStepID)
	}
	if len(child.pendingGateCodePaths) != 1 || child.pendingGateCodePaths[0] != "src/calc.go" {
		svc.mu.Unlock()
		t.Fatalf("park must preserve pendingGateCodePaths (re-check seed), got %#v", child.pendingGateCodePaths)
	}
	svc.mu.Unlock()

	// (2) Re-drive: the Continue resume path's reinvoke must un-park the
	// waiting child — a parked orphan is not a terminal member.
	ok := svc.reinvokeMatchingFlowChild(parentID, "[flow-engine] resume: fix the gate violations", func(c *interactiveRun) bool {
		return c.label == "candidate-b"
	})
	if !ok {
		t.Fatal("reinvokeMatchingFlowChild must match and re-drive a parked waiting_user_approval child")
	}

	svc.mu.Lock()
	child = svc.runs[childID]
	if child.status != RunStatusRunning {
		svc.mu.Unlock()
		t.Fatalf("re-driven child status = %q, want running", child.status)
	}
	if child.activationSeq < 1 {
		svc.mu.Unlock()
		t.Fatalf("re-drive must bump activationSeq, got %d", child.activationSeq)
	}
	// (3) The re-check seed survived the revive — the child's next post-turn
	// gate still owes a forced re-check on the held paths.
	if len(child.pendingGateCodePaths) != 1 || child.pendingGateCodePaths[0] != "src/calc.go" {
		svc.mu.Unlock()
		t.Fatalf("pendingGateCodePaths must survive re-drive for the post-turn re-check, got %#v", child.pendingGateCodePaths)
	}
	svc.mu.Unlock()
}

// BUG-520 residual (a), real entry: POST /agent-loop/continue on a parent
// whose flow child was parked waiting_user_approval by the watchdog/escalate
// park — armed reprompt wiped, pendingGateCodePaths still armed, and NO
// escalated/failed node stamped (the hub_stalled shape: the park was about
// the parent, not a writer). The generic resume tail only reinvokes the hub
// — the orphan child must be re-driven itself, or it waits on a human
// decision nobody can answer (run-60145).
func TestBug520_ContinueReDrivesParkedOrphanChild(t *testing.T) {
	reg := newProviderRegistry()
	adapterCh := make(chan TurnRequest, 2)
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				adapterCh <- req
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc, srv := newTestServerWith(t, reg, nil, nil)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	childID := "run-520-orphan"

	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.status = RunStatusRunning
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "rollout", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke"},
		{ID: "candidate-b", Behavior: "agent.delegate", Agent: "agents/coder.md", Lifecycle: "spawn"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "rollout", To: "candidate-b", When: "done", Kind: "forward"},
	}
	svc.runs[childID] = &interactiveRun{
		id:                childID,
		parentRunID:       parentID,
		label:             "candidate-b",
		providerKey:       ProviderKeyCodex,
		providerAccountID: rs.providerAccountID, // children inherit the run's account stamp at spawn
		status:            RunStatusWaitingUserApr,
		agentStatus:       "waiting_user_approval",
		stepID:            "step-candidate-b",
		pendingGateCodePaths: []string{"src/calc.go"},
		// Post-park shape: the armed reprompt is gone — only the durable
		// code-paths seed remains.
		subs: map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.upsertSummary(parentID, AgentRunSummary{
		RunID: childID, ParentRunID: parentID, AgentName: "coder", Label: "candidate-b",
		Status: RunStatusWaitingUserApr, AgentStatus: "waiting_user_approval",
	})
	// Parked parent: blocked loop with the hub_stalled reason (watchdog park)
	// — no escalated/failed node to target the resume at.
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "blocked", Cap: 5, RoundCap: 5, BlockReason: "hub_stalled"})

	body, _ := json.Marshal(map[string]string{"feedback": "continue"})
	resp, herr := http.Post(srv.URL+"/client/workflow-runs/"+parentID+"/agent-loop/continue", "application/json", strings.NewReader(string(body)))
	if herr != nil {
		t.Fatalf("continue POST: %v", herr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("continue POST status=%d", resp.StatusCode)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		st := svc.runs[childID].status
		svc.mu.Unlock()
		if st != RunStatusWaitingUserApr {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	svc.mu.Lock()
	child := svc.runs[childID]
	if child.status == RunStatusWaitingUserApr {
		svc.mu.Unlock()
		t.Fatal("Continue on the parent must re-drive the parked orphan child — still waiting_user_approval")
	}
	if len(child.pendingGateCodePaths) != 1 || child.pendingGateCodePaths[0] != "src/calc.go" {
		svc.mu.Unlock()
		t.Fatalf("orphan re-drive must preserve pendingGateCodePaths for the gate re-check, got %#v", child.pendingGateCodePaths)
	}
	svc.mu.Unlock()
	// The re-drive must reach the provider — a scheduled turn carrying a
	// resume prompt is the real effect, not just a status flip.
	select {
	case req := <-adapterCh:
		if !strings.Contains(req.Prompt, "gate") && !strings.Contains(req.Prompt, "flow-engine") {
			t.Fatalf("re-drive prompt missing resume context, got %q", req.Prompt)
		}
	case <-time.After(3 * time.Second):
		_, aerr := svc.startTurn(childID, TurnInput{StepID: "step-candidate-b", Prompt: "debug"}, "", "")
		t.Fatalf("orphan re-drive never dispatched a provider turn; manual startTurn err=%v", aerr)
	}
}
