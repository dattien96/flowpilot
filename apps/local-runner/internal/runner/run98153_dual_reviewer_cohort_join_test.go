package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// run-98153 / run-91842: grok-flow dual reviewers (builtin grok-review +
// custom my-reviewer / reviewer-agent) both complete and buffer, but
// cohort_join_complete never fires when RAM cohortExpected is 0.
//
// Additive only. Provider-agnostic settle path; matrix Claude/Codex/Grok.

func run98153FakeAdapter() ProviderRuntimeAdapter {
	return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
		b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "reviewed ok"})
		return nil
	})
}

func run98153Service(t *testing.T, pk ProviderKey) *InteractiveService {
	t.Helper()
	reg := newProviderRegistry()
	for _, k := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		k := k
		reg.register(ProviderRegistration{
			Key: k, Status: ProviderStatusAvailable,
			Capabilities: ProviderCapabilities{Streaming: true},
			newAdapter:   func() ProviderRuntimeAdapter { return run98153FakeAdapter() },
		})
	}
	_ = pk
	return newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
}

func run98153ArmGrokFlow(t *testing.T, svc *InteractiveService, pk ProviderKey) *interactiveRun {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := []agentpack.FlowNode{
		{ID: "grok-coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "grok-review", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
		{ID: "my-reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
		{ID: "grok-synthesis", Behavior: "hub.inline", Agent: "synthesizer"},
	}
	edges := []agentpack.FlowEdge{
		{From: "grok-coder", To: "grok-review", When: "done", Kind: "forward"},
		{From: "grok-coder", To: "my-reviewer", When: "done", Kind: "forward"},
		{From: "grok-review", To: "grok-synthesis", When: "done", Kind: "forward"},
		{From: "my-reviewer", To: "grok-synthesis", When: "done", Kind: "forward"},
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Mode: "explicit"})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	return rs
}

func run98153StampDualReviewers(t *testing.T, svc *InteractiveService, parentID string, pk ProviderKey, registerExpected bool) (cohortID string) {
	t.Helper()
	cohortID = "flow-auto-grok-coder-round-0"
	for _, label := range []string{"grok-review", "my-reviewer"} {
		child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
		if err != nil {
			t.Fatalf("create %s: %v", label, err)
		}
		svc.mu.Lock()
		rs := svc.runs[child.RunID]
		rs.parentRunID = parentID
		rs.label = label
		rs.flowCohortId = cohortID
		rs.providerKey = pk
		svc.mu.Unlock()
		svc.agentOrchestrator.registerChild(parentID, child.RunID)
	}
	if registerExpected {
		svc.agentOrchestrator.preRegisterCohort(parentID, cohortID, 2)
	}
	return cohortID
}

func run98153FindChild(t *testing.T, svc *InteractiveService, parentID, label string) *interactiveRun {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, r := range svc.runs {
		if r != nil && r.parentRunID == parentID && r.label == label {
			return r
		}
	}
	t.Fatalf("child %q not found", label)
	return nil
}

func run98153Settle(svc *InteractiveService, child *interactiveRun, msg string) {
	svc.mu.Lock()
	svc.settleFlowChildTurnCompletedLocked(child, msg, ProviderEvent{
		Type: EventTurnCompleted, FinalMessage: msg,
	})
	svc.mu.Unlock()
}

func TestRun98153DualReviewerJoinAfterBothComplete(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		pk := pk
		t.Run(string(pk), func(t *testing.T) {
			svc := run98153Service(t, pk)
			parent := run98153ArmGrokFlow(t, svc, pk)
			cohort := run98153StampDualReviewers(t, svc, parent.id, pk, true)
			if got := svc.agentOrchestrator.cohortExpectedCount(parent.id, cohort); got != 2 {
				t.Fatalf("expected after stamp = %d, want 2", got)
			}
			run98153Settle(svc, run98153FindChild(t, svc, parent.id, "grok-review"), "ok grok-review")
			if note := svc.lastCohortNoteFor(parent.id); note != "" {
				t.Fatalf("join after first member only: %q", note)
			}
			run98153Settle(svc, run98153FindChild(t, svc, parent.id, "my-reviewer"), "ok my-reviewer")
			note := svc.lastCohortNoteFor(parent.id)
			if !strings.Contains(note, "grok-review") || !strings.Contains(note, "my-reviewer") {
				t.Fatalf("join note missing both labels: %q", note)
			}
			if svc.agentOrchestrator.hasOpenCohort(parent.id) {
				t.Fatal("cohort still open after 2/2")
			}
		})
	}
}

func TestRun98153JoinHealsWhenExpectedLost(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		pk := pk
		t.Run(string(pk), func(t *testing.T) {
			svc := run98153Service(t, pk)
			parent := run98153ArmGrokFlow(t, svc, pk)
			cohort := run98153StampDualReviewers(t, svc, parent.id, pk, false)
			// Live hang: children tagged, RAM expected never set / wiped.
			if svc.agentOrchestrator.cohortExpectedCount(parent.id, cohort) != 0 {
				t.Fatal("expected must start at 0 for this hang shape")
			}
			run98153Settle(svc, run98153FindChild(t, svc, parent.id, "grok-review"), "ok a")
			if note := svc.lastCohortNoteFor(parent.id); note != "" {
				t.Fatalf("must not join after first member when expected was lost: %q", note)
			}
			run98153Settle(svc, run98153FindChild(t, svc, parent.id, "my-reviewer"), "ok b")
			note := svc.lastCohortNoteFor(parent.id)
			if strings.TrimSpace(note) == "" {
				t.Fatal("run-98153 hang: 2/2 buffered but join did not fire after expected=0 heal")
			}
			if !strings.Contains(note, "grok-review") || !strings.Contains(note, "my-reviewer") {
				t.Fatalf("note=%q", note)
			}
		})
	}
}

func TestRun98153OneFailedOneCompletedStillJoinsAfterExpectedLost(t *testing.T) {
	svc := run98153Service(t, ProviderKeyGrok)
	parent := run98153ArmGrokFlow(t, svc, ProviderKeyGrok)
	_ = run98153StampDualReviewers(t, svc, parent.id, ProviderKeyGrok, false)

	a := run98153FindChild(t, svc, parent.id, "grok-review")
	run98153Settle(svc, a, "ok")
	b := run98153FindChild(t, svc, parent.id, "my-reviewer")
	svc.mu.Lock()
	svc.emitLocked(b, ProviderEvent{Type: EventTurnFailed, Error: "boom"})
	svc.mu.Unlock()

	note := svc.lastCohortNoteFor(parent.id)
	if strings.TrimSpace(note) == "" {
		t.Fatal("mixed completed+failed must still join after expected heal")
	}
}

func TestRun98153SingleReviewerCohortJoins(t *testing.T) {
	svc := run98153Service(t, ProviderKeyCodex)
	parent := run98153ArmGrokFlow(t, svc, ProviderKeyCodex)
	cohort := "flow-auto-grok-coder-round-0"
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	rs := svc.runs[child.RunID]
	rs.parentRunID = parent.id
	rs.label = "grok-review"
	rs.flowCohortId = cohort
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.id, child.RunID)
	run98153Settle(svc, run98153FindChild(t, svc, parent.id, "grok-review"), "solo ok")
	if strings.TrimSpace(svc.lastCohortNoteFor(parent.id)) == "" {
		t.Fatal("size-1 cohort must join after heal")
	}
}

func TestRun98153InferDoesNotShrinkPreDeclaredSize(t *testing.T) {
	o := newAgentOrchestrator()
	o.preRegisterCohort("p", "c", 3)
	o.inferCohortExpectedIfMissing("p", "c", 2)
	if got := o.cohortExpectedCount("p", "c"); got != 3 {
		t.Fatalf("expected=%d, want 3 (must not shrink)", got)
	}
	o.appendCohortResult("p", "c", cohortEntry{Label: "a", Status: "completed"})
	o.appendCohortResult("p", "c", cohortEntry{Label: "b", Status: "completed"})
	if o.cohortComplete("p", "c") {
		t.Fatal("2/3 must stay open")
	}
}

func TestRun98153AdvanceSpawnDualThenJoin(t *testing.T) {
	svc := run98153Service(t, ProviderKeyGrok)
	parent := run98153ArmGrokFlow(t, svc, ProviderKeyGrok)
	if !svc.tryAdvanceFlowFromNode(parent.id, "grok-coder", "coder done") {
		t.Fatal("tryAdvanceFlowFromNode returned false")
	}
	waitLoop(t, "both reviewers spawned", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		var gr, mr bool
		for _, r := range svc.runs {
			if r == nil || r.parentRunID != parent.id {
				continue
			}
			if r.label == "grok-review" {
				gr = true
			}
			if r.label == "my-reviewer" {
				mr = true
			}
		}
		return gr && mr
	})
	waitLoop(t, "hub joined after both fake turns", 3*time.Second, func() bool {
		return strings.TrimSpace(svc.lastCohortNoteFor(parent.id)) != ""
	})
}
