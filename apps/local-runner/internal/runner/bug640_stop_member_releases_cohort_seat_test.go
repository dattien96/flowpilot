package runner

import (
	"testing"
)

// BUG-640 (live run-306526): POST /agent-loop/stop on a cohort-member LEG
// terminalizes the leg but never appends its entry to the grandparent's
// cohort — stopAgentLoop only covers the stopped run's OWN children. The
// parent barrier then sits expected>N buffered<N forever and every
// continue/done defers as rejected_cohort_incomplete; the run-306526 runbook
// needed ~a dozen manual interventions for exactly this wedge.
//
// Correct behaviour: stopping a cohort member must buffer the same
// released-seat placeholder (Status "cancelled") the parent-stop and park
// paths already append, so the barrier can join — or stay correctly open
// for a respawned member to claim the seat (BUG-553 replace rule).
func TestStopMemberLegBuffersCancelledEntrySoGrandparentBarrierJoins(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	member, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	sibling, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[member.RunID].parentRunID = parent.RunID
	svc.runs[member.RunID].label = "tdd"
	svc.runs[member.RunID].status = RunStatusRunning
	svc.runs[member.RunID].flowCohortId = "cohort-round5"
	svc.runs[sibling.RunID].parentRunID = parent.RunID
	svc.runs[sibling.RunID].label = "spec_align"
	svc.runs[sibling.RunID].status = RunStatusCompleted
	svc.runs[sibling.RunID].flowCohortId = "cohort-round5"
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, member.RunID)
	svc.agentOrchestrator.registerChild(parent.RunID, sibling.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "cohort-round5", 2)
	// Sibling already delivered — the barrier is only missing the member's seat.
	svc.agentOrchestrator.appendCohortResult(parent.RunID, "cohort-round5", cohortEntry{Label: "spec_align", Status: "completed"})
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit"})

	// Operator stops the stalled leg directly (the run-306526 workaround).
	if _, e := svc.stopAgentLoop(member.RunID); e != nil {
		t.Fatalf("stopAgentLoop: %v", e)
	}

	svc.mu.Lock()
	memberStatus := svc.runs[member.RunID].status
	svc.mu.Unlock()
	if memberStatus != RunStatusCancelled {
		t.Fatalf("member status=%s, want Cancelled", memberStatus)
	}
	// The member's seat must have been filled with the cancelled placeholder:
	// the last expected entry makes the barrier join and drain.
	if svc.agentOrchestrator.cohortExpectedCount(parent.RunID, "cohort-round5") != 0 {
		t.Fatal("cohort-round5 must have drained — the stopped member's seat leaked")
	}
	if svc.agentOrchestrator.hasOpenCohort(parent.RunID) {
		t.Fatal("no open cohort should remain on the parent after the stopped member's seat is released")
	}
}

// Second half of BUG-640: while the barrier still has other open seats, the
// stopped member's cancelled placeholder keeps ITS seat claimable — a
// respawned member's real result must replace the placeholder (BUG-553),
// not dedupe against it.
func TestStopMemberLegKeepsSeatOpenForRespawn(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	member, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[member.RunID].parentRunID = parent.RunID
	svc.runs[member.RunID].label = "reviewer"
	svc.runs[member.RunID].status = RunStatusRunning
	svc.runs[member.RunID].flowCohortId = "cohort-round5"
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, member.RunID)
	// Expected 2 and nothing buffered: stopping the member must NOT complete
	// the barrier — its seat stays claimable by a replacement leg.
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "cohort-round5", 2)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit"})

	if _, e := svc.stopAgentLoop(member.RunID); e != nil {
		t.Fatalf("stopAgentLoop: %v", e)
	}
	if !svc.agentOrchestrator.hasOpenCohort(parent.RunID) {
		t.Fatal("barrier should still be open — the second seat is unclaimed")
	}
	// The stopped member must have left a released-seat placeholder behind —
	// without the fix the buffer stays empty and the seat is indistinguishable
	// from never-claimed.
	svc.agentOrchestrator.mu.Lock()
	entries := append([]cohortEntry(nil), svc.agentOrchestrator.cohort[cohortKey(parent.RunID, "cohort-round5")]...)
	svc.agentOrchestrator.mu.Unlock()
	if len(entries) != 1 || entries[0].Label != "reviewer" || entries[0].Status != "cancelled" {
		t.Fatalf("want one cancelled placeholder for reviewer, got %+v", entries)
	}
	svc.mu.Lock()
	seat := svc.openCohortSeatForLabelLocked(parent.RunID, "reviewer")
	svc.mu.Unlock()
	if seat != "cohort-round5" {
		t.Fatalf("stopped member's seat must stay open for a respawn, got seat=%q", seat)
	}
	// A replacement member's real result replaces the cancelled placeholder
	// rather than double-counting the seat.
	svc.agentOrchestrator.appendCohortResult(parent.RunID, "cohort-round5", cohortEntry{Label: "reviewer", Status: "completed"})
	svc.agentOrchestrator.appendCohortResult(parent.RunID, "cohort-round5", cohortEntry{Label: "spec_align", Status: "completed"})
	if svc.agentOrchestrator.hasOpenCohort(parent.RunID) {
		t.Fatal("barrier must join once the respawned member delivers")
	}
	svc.agentOrchestrator.mu.Lock()
	final := append([]cohortEntry(nil), svc.agentOrchestrator.cohort[cohortKey(parent.RunID, "cohort-round5")]...)
	svc.agentOrchestrator.mu.Unlock()
	if len(final) != 2 {
		t.Fatalf("placeholder must be replaced not double-counted, got %+v", final)
	}
	for _, e := range final {
		if e.Label == "reviewer" && e.Status != "completed" {
			t.Fatalf("reviewer placeholder must be replaced by the real result, got %+v", e)
		}
	}
}

// Guard the other direction: a member whose real result is already buffered
// must not have it overwritten by a later Stop (a duplicate-late stop must be
// a no-op for the barrier, never regressing completed → cancelled).
func TestStopMemberLegDoesNotOverwriteBufferedResult(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	member, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[member.RunID].parentRunID = parent.RunID
	svc.runs[member.RunID].label = "reviewer"
	svc.runs[member.RunID].status = RunStatusRunning
	svc.runs[member.RunID].flowCohortId = "cohort-round5"
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, member.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "cohort-round5", 2)
	svc.agentOrchestrator.appendCohortResult(parent.RunID, "cohort-round5", cohortEntry{Label: "reviewer", Status: "completed"})

	if _, e := svc.stopAgentLoop(member.RunID); e != nil {
		t.Fatalf("stopAgentLoop: %v", e)
	}
	// The buffered "completed" must survive — an extra cancelled entry would
	// either double-count the seat or regress the verdict. The barrier now
	// has exactly the one real entry and still waits for the second member.
	if svc.agentOrchestrator.cohortComplete(parent.RunID, "cohort-round5") {
		t.Fatal("barrier must remain open — completed entry must not be double-counted by the stop")
	}
	svc.agentOrchestrator.mu.Lock()
	entries := append([]cohortEntry(nil), svc.agentOrchestrator.cohort[cohortKey(parent.RunID, "cohort-round5")]...)
	svc.agentOrchestrator.mu.Unlock()
	if len(entries) != 1 || entries[0].Status != "completed" {
		t.Fatalf("stop must not touch the buffered completed entry, got %+v", entries)
	}
}
