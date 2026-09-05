package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Task-325: conditional plan-approval park. A plan loop that already churned
// (plan_writer re-entered >= 1) parks for human read + approve after
// plan_synthesis approves and BEFORE anything freezes; clean first-pass
// plans run straight through. New file; no pre-existing test is modified.

// task325PlanTopology extends the run-201295 freeze topology with the plan
// loop back-edge + plan_writer node (task-harness / bug-plan-harness shape).
func task325PlanTopology() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "plan_synthesis", To: "preflight_contract_freeze", When: "done", Kind: "forward"},
		{From: "preflight_contract_freeze", To: "test_signatures", When: "done", Kind: "forward"},
		{From: "plan_synthesis", To: "plan_writer", When: "continue", Kind: "back"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze", Lifecycle: "once"},
		{ID: "test_signatures", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "once"},
		{ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md", Lifecycle: "reinvoke"},
	}
	return edges, nodes
}

// task325PlanService builds a workspace-backed run with the plan topology,
// a seeded planner draft (real task-harness shape: freeze reuses it), and a
// running loop.
func task325PlanService(t *testing.T, pk ProviderKey) (*InteractiveService, string) {
	t.Helper()
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestServiceForProvider(t, pk)
	edges, nodes := task325PlanTopology()
	runID := newFreezeTestRunForProvider(t, svc, pk, dir, edges, nodes, head)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeHubNodeID = "plan_synthesis"
	rs.autoOrchestrate = true
	rs.currentTurnID = "turn-325"
	// Seeded planner draft so freeze can run (prose-test pattern).
	planner := &interactiveRun{
		id:          "run-planner-325",
		parentRunID: runID,
		label:       "preflight_contract_plan",
		agentName:   "contract-planner",
		status:      RunStatusCompleted,
		providerKey: pk,
		subs:        map[int64]chan ProviderEvent{},
		events:      []ProviderEvent{{Type: EventTurnCompleted, FinalMessage: validPlannerDraft}},
	}
	svc.runs[planner.id] = planner
	svc.agentOrchestrator.registerChild(runID, planner.id)
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	svc.reseedFlowStepRuntime(runID, nodes)
	return svc, runID
}

// task325SeedWriter inserts a plan_writer child with the given activationSeq
// (deterministic churn simulator: 0 = first pass, >=1 = re-entered).
func task325SeedWriter(t *testing.T, svc *InteractiveService, runID string, pk ProviderKey, seq int) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	w := &interactiveRun{
		id:            "run-writer-325",
		parentRunID:   runID,
		label:         "plan_writer",
		agentName:     "doc-writer",
		status:        RunStatusCompleted,
		providerKey:   pk,
		activationSeq: seq,
		subs:          map[int64]chan ProviderEvent{},
	}
	svc.runs[w.id] = w
	svc.agentOrchestrator.registerChild(runID, w.id)
}

func task325WriterSeq(t *testing.T, svc *InteractiveService, runID string) int {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, r := range svc.runs {
		if r != nil && r.parentRunID == runID && r.label == planWriterNodeID {
			return r.activationSeq
		}
	}
	return -1
}

// planLoopChurned is loop-specific: implement/code churn and the shared
// Round counter never count — only plan_writer re-entries.
func TestPlanLoopChurned_Specificity(t *testing.T) {
	newSvc := func() (*InteractiveService, string) {
		svc := &InteractiveService{runs: map[string]*interactiveRun{}}
		svc.runs["parent"] = &interactiveRun{id: "parent"}
		return svc, "parent"
	}
	// No writer at all → no churn.
	svc, parent := newSvc()
	if churned, _ := svc.planLoopChurned(parent); churned {
		t.Fatal("no writer child must not churn")
	}
	// First-pass writer (seq 0) → no churn.
	svc, parent = newSvc()
	svc.runs["w"] = &interactiveRun{id: "w", parentRunID: parent, label: "plan_writer", activationSeq: 0}
	if churned, _ := svc.planLoopChurned(parent); churned {
		t.Fatal("first-pass writer (seq 0) must not churn")
	}
	// Re-entered writer (seq 1) → churned, rounds reported.
	svc, parent = newSvc()
	svc.runs["w"] = &interactiveRun{id: "w", parentRunID: parent, label: "plan_writer", activationSeq: 1}
	if churned, rounds := svc.planLoopChurned(parent); !churned || rounds != 1 {
		t.Fatalf("churned=%v rounds=%d, want true/1", churned, rounds)
	}
	// Code-loop churn only (implement seq 5, shared Round irrelevant) → no churn.
	svc, parent = newSvc()
	svc.runs["c"] = &interactiveRun{id: "c", parentRunID: parent, label: "implement", activationSeq: 5}
	if churned, _ := svc.planLoopChurned(parent); churned {
		t.Fatal("implement re-entries must never count as plan churn")
	}
	// Other parent's writer → no churn here.
	svc, parent = newSvc()
	svc.runs["w"] = &interactiveRun{id: "w", parentRunID: "other", label: "plan_writer", activationSeq: 3}
	if churned, _ := svc.planLoopChurned(parent); churned {
		t.Fatal("another parent's writer must not churn this run")
	}
	if churned, _ := (*InteractiveService)(nil).planLoopChurned("x"); churned {
		t.Fatal("nil service must not churn")
	}
}

// Reported repro: churned plan parks (blocked/plan_approval, WAITING step,
// nothing frozen, guard stamped) — across Claude/Codex/Grok (R2: the path is
// provider-agnostic; the matrix proves it rather than claims it).
func TestPlanApprovalPark_ChurnedPlanParks(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := task325PlanService(t, pk)
			task325SeedWriter(t, svc, runID, pk, 1)

			res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
			if !handled {
				t.Fatalf("%s: churned plan done must be claimed (parked), got handled=false", pk)
			}
			if res.Status != "blocked" || res.NextAction != "awaiting_user" {
				t.Fatalf("%s: result = %+v, want blocked/awaiting_user", pk, res)
			}
			st := svc.agentOrchestrator.loopStateFor(runID)
			if st.Status != "blocked" || st.BlockReason != planApprovalBlockReason {
				t.Fatalf("%s: loop = %+v, want blocked/plan_approval", pk, st)
			}
			if !strings.Contains(st.GateReason, "writer round 1") {
				t.Fatalf("%s: GateReason must name the churn: %q", pk, st.GateReason)
			}
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusWaitingUserApr {
				t.Fatalf("%s: plan_synthesis = %v, want WAITING (not RUNNING-as-hang, not FAILED)", pk, got)
			}
			if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got == StepStatusDone {
				t.Fatalf("%s: freeze must NOT run before human approval", pk)
			}
			if n := countChildrenWithLabel(svc, runID, "test_signatures"); n != 0 {
				t.Fatalf("%s: code chain must not start behind the park, test_signatures children = %d", pk, n)
			}
			if !svc.flowControlSubmittedForTurn(runID, "turn-325") {
				t.Fatalf("%s: approving turn must count as a decision (BUG-226 guard)", pk)
			}
			if st.BlockReason == "hub_stalled" {
				t.Fatalf("%s: park must never read as a stall", pk)
			}
		})
	}
}

// Near-miss: clean first-pass plan runs straight through unattended.
func TestPlanApprovalPark_CleanPlanPassesThrough(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := task325PlanService(t, pk)
			task325SeedWriter(t, svc, runID, pk, 0)

			res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
			if !handled {
				t.Fatalf("%s: clean plan done must dispatch freeze, got handled=false", pk)
			}
			if res.NextAction == "awaiting_user" {
				t.Fatalf("%s: clean plan must not park: %+v", pk, res)
			}
			if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
				t.Fatalf("%s: freeze = %v, want DONE (unattended path intact)", pk, got)
			}
			waitLoop(t, string(pk)+": writer spawned after freeze", 3*time.Second, func() bool {
				return countChildrenWithLabel(svc, runID, "test_signatures") == 1
			})
			if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == planApprovalBlockReason {
				t.Fatalf("%s: clean plan must never carry the approval reason", pk)
			}
		})
	}
}

// Near-miss: code-loop churn alone never parks the plan gate.
func TestPlanApprovalPark_CodeChurnDoesNotPark(t *testing.T) {
	svc, runID := task325PlanService(t, ProviderKeyCodex)
	svc.mu.Lock()
	svc.runs["run-coder-325"] = &interactiveRun{
		id: "run-coder-325", parentRunID: runID, label: "implement",
		agentName: "coder", status: RunStatusCompleted, activationSeq: 4,
		subs: map[int64]chan ProviderEvent{},
	}
	svc.agentOrchestrator.registerChild(runID, "run-coder-325")
	svc.mu.Unlock()
	// Shared Round bumped by the code loop — still not plan churn.
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Round = 2
		return st
	})

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
	if !handled || res.NextAction == "awaiting_user" {
		t.Fatalf("code-only churn must not park the plan gate: handled=%v %+v", handled, res)
	}
	if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
		t.Fatalf("freeze = %v, want DONE", got)
	}
}

// Degraded: writer never spawned (CA-732 shape) → fail open, never stall.
func TestPlanApprovalPark_MissingWriterNoPark(t *testing.T) {
	svc, runID := task325PlanService(t, ProviderKeyCodex)
	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
	if !handled || res.NextAction == "awaiting_user" {
		t.Fatalf("missing writer must fail open to freeze: handled=%v %+v", handled, res)
	}
	if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
		t.Fatalf("freeze = %v, want DONE", got)
	}
}

// Near-miss: a code-loop hub done never parks even with a churned writer.
func TestPlanApprovalPark_CodeHubDoneDoesNotPark(t *testing.T) {
	svc, runID := task325PlanService(t, ProviderKeyCodex)
	task325SeedWriter(t, svc, runID, ProviderKeyCodex, 2)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeHubNodeID = "synthesis"
	rs.activeFlowEdges = append(rs.activeFlowEdges,
		agentpack.FlowEdge{From: "synthesis", To: "audit", When: "done", Kind: "forward"})
	rs.activeFlowNodes = append(rs.activeFlowNodes,
		agentpack.FlowNode{ID: "synthesis", Behavior: "hub.inline", Lifecycle: "reinvoke"},
		agentpack.FlowNode{ID: "audit", Behavior: "artifact.audit_draft", Lifecycle: "once"})
	svc.mu.Unlock()

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "code done"})
	if !handled {
		t.Fatal("code hub done must still dispatch audit (handled)")
	}
	if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == planApprovalBlockReason {
		t.Fatalf("code hub done must never park plan_approval: %+v", res)
	}
}

// Approve (empty feedback) advances FORWARD to freeze — never retries writer.
func TestPlanApprovalPark_ApproveAdvancesToFreeze(t *testing.T) {
	svc, runID := task325PlanService(t, ProviderKeyCodex)
	task325SeedWriter(t, svc, runID, ProviderKeyCodex, 1)
	if _, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft}); !handled {
		t.Fatal("must park first")
	}
	snap, err := svc.resumeFlowWithFeedback(runID, "")
	if err != nil {
		t.Fatalf("approve resume: %v", err)
	}
	if snap.LoopState.Status == "blocked" {
		t.Fatalf("approve must unblock the loop: %+v", snap.LoopState)
	}
	if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
		t.Fatalf("freeze = %v, want DONE after approve", got)
	}
	if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusDone {
		t.Fatalf("plan_synthesis = %v, want DONE after approve", got)
	}
	if seq := task325WriterSeq(t, svc, runID); seq != 1 {
		t.Fatalf("approve must not touch the writer (seq=%d, want 1)", seq)
	}
	waitLoop(t, "writer spawned after approved freeze", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "test_signatures") == 1
	})
}

// Feedback re-enters the SAME writer with the human note (no second child).
func TestPlanApprovalPark_FeedbackReentersWriter(t *testing.T) {
	svc, runID := task325PlanService(t, ProviderKeyCodex)
	task325SeedWriter(t, svc, runID, ProviderKeyCodex, 1)
	if _, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft}); !handled {
		t.Fatal("must park first")
	}
	snap, err := svc.resumeFlowWithFeedback(runID, "human: split T-2 into two paths")
	if err != nil {
		t.Fatalf("feedback resume: %v", err)
	}
	if snap.LoopState.Status == "blocked" {
		t.Fatalf("feedback must unblock the loop: %+v", snap.LoopState)
	}
	// Same child re-entered (session reuse), not a second writer.
	svc.mu.Lock()
	children := 0
	for _, r := range svc.runs {
		if r != nil && r.parentRunID == runID && r.label == planWriterNodeID {
			children++
		}
	}
	svc.mu.Unlock()
	if children != 1 {
		t.Fatalf("writer children = %d, want 1 (same-session re-entry)", children)
	}
	if seq := task325WriterSeq(t, svc, runID); seq != 2 {
		t.Fatalf("writer seq = %d, want 2 (re-entered once more)", seq)
	}
	if got := flowStepStatus(t, svc, runID, "plan_writer"); got != StepStatusRunning {
		t.Fatalf("plan_writer = %v, want RUNNING", got)
	}
	if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got == StepStatusDone {
		t.Fatal("feedback must not freeze (plan still under revision)")
	}
}

// Ordering/lifecycle: the park survives a restart as blocked/plan_approval
// with the turn sealed — never stranded, never auto-run.
func TestPlanApprovalPark_RestartKeepsParkSealed(t *testing.T) {
	svc, runID := task325PlanService(t, ProviderKeyCodex)
	task325SeedWriter(t, svc, runID, ProviderKeyCodex, 1)
	if _, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft}); !handled {
		t.Fatal("must park first")
	}
	svc.mu.Lock()
	live := svc.runs[runID]
	st := sessionStateOf(live)
	st.LoopState = svc.agentOrchestrator.loopStateFor(runID)
	svc.mu.Unlock()

	root := t.TempDir()
	store, err := NewLocalFileSessionStore(root)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := store.UpsertProviderSession(context.Background(), st); err != nil {
		t.Fatalf("upsert park snap: %v", err)
	}
	svc2 := NewInteractiveService()
	loaded, ok, getErr := store.GetProviderSession(context.Background(), st.RunID)
	if getErr != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, getErr)
	}
	if _, apiErr := svc2.reconstructRun(loaded); apiErr != nil {
		t.Fatalf("reconstruct after plan park: %s", apiErr.msg)
	}
	loop := svc2.agentOrchestrator.loopStateFor(st.RunID)
	if loop.Status != "blocked" || loop.BlockReason != planApprovalBlockReason {
		t.Fatalf("after restart loop=%+v, want blocked/plan_approval", loop)
	}
	if !strings.Contains(loop.GateReason, "writer round 1") {
		t.Fatalf("GateReason must survive restart: %q", loop.GateReason)
	}
	if _, turnErr := svc2.startTurn(st.RunID, TurnInput{StepID: "chat", Prompt: "hi"}, "", ""); turnErr == nil {
		t.Fatal("startTurn after plan park restart must stay sealed")
	} else if turnErr.code != "flow_awaiting_user" {
		t.Fatalf("startTurn seal = %q, want flow_awaiting_user", turnErr.code)
	}
}
