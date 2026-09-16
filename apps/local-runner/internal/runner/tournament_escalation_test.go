package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// CP-65 P-4 (Task-371): escalation fallback wiring. New file — no
// pre-existing test is modified. The flag defaults OFF, so every legacy
// suite already proves the byte-identical fallback (AC-4); these tests pin
// the flag-ON rescue paths.

const tournamentEscalationFlag = "FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION"

// tournamentEscalationFixture builds a stuck parent run: task-harness-like
// topology with a writer, loop parked at blocked/cap.
func tournamentEscalationFixture(t *testing.T, flagOn bool) (*InteractiveService, string) {
	t.Helper()
	if flagOn {
		t.Setenv(tournamentEscalationFlag, "1")
	} else {
		t.Setenv(tournamentEscalationFlag, "")
	}
	svc := newFreezeTestService(t)
	workspace := t.TempDir()
	parentID := "run-tournament-cap"
	svc.runs[parentID] = &interactiveRun{
		id: parentID, projectID: "proj", workspaceCwd: workspace,
		providerKey: ProviderKeyCodex, modelName: "gpt-5.4-mini",
		workflowID: workingmode.PackPrefix + "task-harness",
		activeFlowNodes: []agentpack.FlowNode{
			{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md"},
			{ID: "validate", Behavior: "command.validate"},
		},
		activeFlowEdges: []agentpack.FlowEdge{
			{From: "implement", To: "validate", When: "done", Kind: "forward"},
		},
	}
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{
		Status: "blocked", BlockReason: "cap", Round: 3, Cap: 3, RoundCap: 3,
		GateReason: "cap 3 reached with 2 open issue(s)",
	})
	return svc, parentID
}

func tournamentChildOf(t *testing.T, svc *InteractiveService, parentID string) *interactiveRun {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, rs := range svc.runs {
		if rs.parentRunID == parentID {
			return rs
		}
	}
	t.Fatalf("no tournament child run for parent %q", parentID)
	return nil
}

func assertTournamentChild(t *testing.T, svc *InteractiveService, parentID, reasonFrag string) *interactiveRun {
	t.Helper()
	child := tournamentChildOf(t, svc, parentID)
	if child.workflowID != workingmode.PackPrefix+"tournament-harness" {
		t.Fatalf("child workflow = %q, want pack tournament-harness", child.workflowID)
	}
	found := false
	for _, n := range child.activeFlowNodes {
		if n.ID == "tournament_arbiter" && n.Behavior == "tournament.arbiter" {
			found = true
		}
	}
	if !found {
		t.Fatal("child must carry the tournament-harness topology (tournament_arbiter)")
	}
	if !strings.Contains(child.pendingTurnPrompt, reasonFrag) {
		t.Fatalf("child intent must carry %q, got %q", reasonFrag, child.pendingTurnPrompt)
	}
	if child.workspaceCwd == "" || child.providerKey == "" {
		t.Fatal("child must inherit workspace + provider (contract handoff)")
	}
	loop := svc.agentOrchestrator.loopStateFor(parentID)
	if loop.Status != LoopStatusTournamentEscalation {
		t.Fatalf("parent loop status = %q, want tournament_escalation", loop.Status)
	}
	if !strings.Contains(loop.GateReason, child.id) {
		t.Fatalf("parent gate reason must name the child, got %q", loop.GateReason)
	}
	return child
}

// Scenario: review loop hits its round cap with the flag on — the parent
// flips to tournament_escalation and a tournament child carries the intent.
func TestReviewLoopTriggersTournamentOnCapExceeded(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	if !svc.maybeEscalateCapToTournament(parentID, "review cap 3 reached with 2 open issue(s)") {
		t.Fatal("flag on + blocked cap run must escalate")
	}
	assertTournamentChild(t, svc, parentID, "review cap 3")
}

// Scenario: vibe owner-debate parks its owner-fail cap with the flag on —
// the same rescue leg fires (stall reason preserved in the intent).
func TestVibeDebateTriggersTournamentOnStall(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	svc.mu.Lock()
	svc.runs[parentID].workingMode = workingmode.Vibe
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.GateReason = "vibe owner debate members failed"
		return st
	})
	if !svc.maybeEscalateCapToTournament(parentID, "vibe owner debate stalled (owner-fail cap)") {
		t.Fatal("flag on + debate-cap run must escalate")
	}
	assertTournamentChild(t, svc, parentID, "owner debate stalled")
}

func TestTournamentEscalationFlagOffKeepsLegacyPark(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, false)
	if svc.maybeEscalateCapToTournament(parentID, "review cap 3 reached") {
		t.Fatal("flag off must never escalate")
	}
	loop := svc.agentOrchestrator.loopStateFor(parentID)
	if loop.Status != "blocked" || loop.BlockReason != "cap" {
		t.Fatalf("flag off must leave the legacy park untouched, got %+v", loop)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, rs := range svc.runs {
		if rs.parentRunID == parentID {
			t.Fatalf("flag off must spawn no child, found %q", rs.id)
		}
	}
}

func TestTournamentEscalationRefusesTournamentRuns(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	// AC-5: a tournament flow hitting its own cap escalates normally — no
	// tournament-of-tournament. Both identity shapes refuse.
	svc.mu.Lock()
	svc.runs[parentID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "tournament_arbiter", Behavior: "tournament.arbiter"},
	}
	svc.mu.Unlock()
	if svc.maybeEscalateCapToTournament(parentID, "tournament cap reached") {
		t.Fatal("tournament run must never re-escalate (topology identity)")
	}
	svc.mu.Lock()
	svc.runs[parentID].activeFlowNodes = nil
	svc.runs[parentID].workflowID = workingmode.PackPrefix + "tournament-harness"
	svc.mu.Unlock()
	if svc.maybeEscalateCapToTournament(parentID, "tournament cap reached") {
		t.Fatal("tournament run must never re-escalate (workflow identity)")
	}
	// A rescued parent keeps its single child: second trigger is a no-op.
	svc2, parent2 := tournamentEscalationFixture(t, true)
	if !svc2.maybeEscalateCapToTournament(parent2, "review cap 3 reached") {
		t.Fatal("first escalation must fire")
	}
	if svc2.maybeEscalateCapToTournament(parent2, "review cap 3 reached") {
		t.Fatal("second escalation on a rescued parent must be a no-op")
	}
	count := 0
	svc2.mu.Lock()
	for _, rs := range svc2.runs {
		if rs.parentRunID == parent2 {
			count++
		}
	}
	svc2.mu.Unlock()
	if count != 1 {
		t.Fatalf("rescued parent must own exactly 1 tournament child, got %d", count)
	}
}

func TestTournamentEscalationRefusesLiveLoops(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Round: 1, Cap: 3, RoundCap: 3})
	if svc.maybeEscalateCapToTournament(parentID, "review cap 3 reached") {
		t.Fatal("a running loop must never escalate")
	}
	if _, err := svc.escalateToTournament("run-missing", "x"); err == nil {
		t.Fatal("unknown parent must error")
	}
}

func TestResumeParentAfterTournament(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)
	if !svc.maybeEscalateCapToTournament(parentID, "review cap 3 reached") {
		t.Fatal("escalation must fire")
	}
	// AC-3 merge path: winner landed — loop reopens for validation.
	if err := svc.resumeParentAfterTournament(parentID, "candidate-a", true); err != nil {
		t.Fatalf("resume after merge: %v", err)
	}
	loop := svc.agentOrchestrator.loopStateFor(parentID)
	if loop.Status != "running" || loop.BlockReason != "" {
		t.Fatalf("merged parent must reopen running/unblocked, got %+v", loop)
	}
	if !strings.Contains(loop.GateReason, "candidate-a") {
		t.Fatalf("resume must record the winner, got %q", loop.GateReason)
	}
	// AC-3 refuse path: tie + human refused — fail-closed to manual escalate.
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: LoopStatusTournamentEscalation, Round: 3, Cap: 3, RoundCap: 3})
	if err := svc.resumeParentAfterTournament(parentID, "", false); err != nil {
		t.Fatalf("resume after refuse: %v", err)
	}
	loop = svc.agentOrchestrator.loopStateFor(parentID)
	if loop.Status != "blocked" || loop.BlockReason != "cap" {
		t.Fatalf("refused parent must fail closed to blocked/cap, got %+v", loop)
	}
	if err := svc.resumeParentAfterTournament("run-missing", "a", true); err == nil {
		t.Fatal("unknown parent must error")
	}
}
