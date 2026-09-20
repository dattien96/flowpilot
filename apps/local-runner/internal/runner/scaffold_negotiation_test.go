package runner

import (
	"testing"
)

// CP-67 P-5 (Task-382 B-10): the phase-scoped renegotiation budget.

func negotiationTestRun(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	return svc, parent.RunID
}

func TestScaffoldNegotiationCapEnforcedAtFive(t *testing.T) {
	svc, runID := negotiationTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 5, NegotiationCap: 5})
	svc.mu.Lock()
	svc.runs[runID].stepID = "synthesis_negotiation"
	svc.mu.Unlock()

	// Rounds 1..4: continue (the loop keeps negotiating).
	for i := 1; i <= 4; i++ {
		result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "continue"})
		if err != nil {
			t.Fatalf("round %d: %v", i, err)
		}
		if result.Status != "continue" {
			t.Fatalf("round %d: status = %q, want continue", i, result.Status)
		}
		st := svc.agentOrchestrator.loopStateFor(runID)
		if st.NegotiationRound != i {
			t.Fatalf("round counter = %d, want %d", st.NegotiationRound, i)
		}
	}
	// Round 5 hits the phase cap: ESCALATE (blocked), never an extend.
	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "continue"})
	if err != nil {
		t.Fatalf("round 5: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("round 5: status = %q, want blocked (negotiation cap exhausted)", result.Status)
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.NegotiationRound != 5 || st.BlockReason != "escalate" {
		t.Fatalf("cap state = round %d reason %q, want round 5 + escalate", st.NegotiationRound, st.BlockReason)
	}
}

func TestScaffoldNegotiationCapDefaultsToFive(t *testing.T) {
	st := AgentLoopState{}
	if got := effectiveNegotiationCap(st); got != 5 {
		t.Fatalf("default cap = %d, want 5", got)
	}
	st.NegotiationCap = 3
	if got := effectiveNegotiationCap(st); got != 3 {
		t.Fatalf("declared cap = %d, want 3", got)
	}
}

func TestSynthesisNegotiationNodeRouting(t *testing.T) {
	if !isNegotiationHubNode("synthesis_negotiation") {
		t.Fatal("the negotiation hub step must be recognized")
	}
	if isNegotiationHubNode("synthesis") || isNegotiationHubNode("coder") {
		t.Fatal("unrelated steps must not be treated as the negotiation phase")
	}

	// The negotiation hub's DONE closes the phase: NegotiationRound resets.
	svc, runID := negotiationTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 5, NegotiationCap: 5, NegotiationRound: 3})
	svc.mu.Lock()
	svc.runs[runID].stepID = "synthesis_negotiation"
	svc.mu.Unlock()

	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "done"})
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	if result.Status != "done" {
		t.Fatalf("status = %q, want done", result.Status)
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.NegotiationRound != 0 {
		t.Fatalf("NegotiationRound = %d, want 0 after phase close", st.NegotiationRound)
	}
}

func TestScaffoldNegotiationReviewLoopCapUnshared(t *testing.T) {
	// B-10: the negotiation budget is PHASE-SCOPED — a negotiation round
	// must not consume the review loop's Round, and vice versa.
	svc, runID := negotiationTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 5, NegotiationCap: 5})
	svc.mu.Lock()
	svc.runs[runID].stepID = "synthesis_negotiation"
	svc.mu.Unlock()

	if _, err := svc.applyFlowControl(runID, FlowControlInput{Status: "continue"}); err != nil {
		t.Fatalf("negotiation continue: %v", err)
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.NegotiationRound != 1 || st.Round != 0 {
		t.Fatalf("negotiation round consumed the wrong budget: NegotiationRound=%d Round=%d", st.NegotiationRound, st.Round)
	}
}
