package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/workingmode"
)

// BUG-1196 (live run-225691, turn-254226 @12:51:57 and turn-260646
// @14:14:40 — both dead-ended invisibly): a requirement-class gate block on
// a vibe hub turn parks via vibeGateRequirement — loop blocked +
// BlockReason=requirement — but never stamps the flow step WAITING, never
// emits agent_graph_updated, never calls parkFlowForAwaitingUser, and arms
// no durable decision (pendingGateBlock only covers r-reg options). The
// park is real but invisible: the only push is a fire-and-forget
// flow_gate_violation event, so a client that missed it (or polled
// steps-runtime, where the hub step still reads RUNNING) sees a silent
// freeze. SS-18 BR-4 makes this class user-only — a decision surface is
// the entire point of the park.
func TestBug1196_RequirementParkStampsAndFreezes(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	nodes := []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.activeFlowNodes = nodes
	rs.activeHubNodeID = "synthesis"
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	blocked := svc.applyVibeGateResolver(parent.RunID, "", "turn-x", rs, flowgate.EnforceResult{
		Action:  "block",
		Message: "requirement signatures drifted from the locked SS",
		Violations: []flowgate.Violation{{
			Rule:   flowgate.RequirementRule(),
			Detail: "signature drift on §6 importVault",
		}},
	})
	if !blocked {
		t.Fatal("requirement-class violation on a vibe hub turn must consume the gate")
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "blocked" || loop.BlockReason != "requirement" {
		t.Fatalf("loop = %+v, want blocked/requirement", loop)
	}
	svc.mu.Lock()
	st := rs.status
	svc.mu.Unlock()
	if st != RunStatusWaitingUserApr {
		t.Fatalf("gated run status = %s, want waiting_user_approval", st)
	}
	if got := flowStepStatus(t, svc, parent.RunID, "synthesis"); got != StepStatusWaitingUserApr {
		t.Fatalf("hub step = %s, want WAITING_USER_APPROVAL — the park must be visible in the step runtime, not an invisible freeze", got)
	}
}

// The child-divert shape: a spawned member's requirement block must park
// the PARENT's loop — mutating the child's (nonexistent) loop state leaves
// the hub running while the member silently waits, same invisible freeze
// one level down. The escalated node must be recorded so Continue retries
// this child, not a generic hub reinvoke.
func TestBug1196_ChildRequirementParkTargetsParentLoop(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	nodes := []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.workingMode = workingmode.Vibe
	prs.activeFlowNodes = nodes
	crs := svc.runs[child.RunID]
	crs.parentRunID = parent.RunID
	crs.label = "implement"
	crs.workingMode = workingmode.Vibe
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	blocked := svc.applyVibeGateResolver(child.RunID, parent.RunID, "turn-c", crs, flowgate.EnforceResult{
		Action:  "block",
		Message: "requirement signatures drifted from the locked SS",
		Violations: []flowgate.Violation{{
			Rule:   flowgate.RequirementRule(),
			Detail: "signature drift on §6 importVault",
		}},
	})
	if !blocked {
		t.Fatal("requirement-class violation on a vibe child turn must consume the gate")
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "blocked" || loop.BlockReason != "requirement" {
		t.Fatalf("parent loop = %+v, want blocked/requirement — the gated child parked a nonexistent child loop and the hub kept running", loop)
	}
	if got := flowStepStatus(t, svc, parent.RunID, "implement"); got != StepStatusWaitingUserApr {
		t.Fatalf("gated child's flow step = %s, want WAITING_USER_APPROVAL", got)
	}
	svc.mu.Lock()
	escalated := prs.lastEscalatedInlineNodeID
	childStatus := crs.status
	svc.mu.Unlock()
	if escalated != "implement" {
		t.Fatalf("lastEscalatedInlineNodeID = %q, want implement — Continue would retry the wrong node", escalated)
	}
	if childStatus != RunStatusWaitingUserApr {
		t.Fatalf("gated child status = %s, want waiting_user_approval", childStatus)
	}
}
