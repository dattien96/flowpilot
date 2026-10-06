package runner

import (
	"testing"
	"time"
)

// BUG-639 (live run-306526, 2026-10-06): sprint rounds re-use node labels —
// by round 5 the run had three scaffold-architect children labelled "tdd",
// six spec-aligner children labelled "spec_align" and seven reviewer
// children labelled "reviewer". handleMemberAction used to take the FIRST
// child whose label matched (oldest registration order), so Retry/Skip
// dead-lettered onto a historical sibling while the actually-stalled member
// stayed untouched — the hub re-parked member_stalled within seconds and
// the card actions could never discharge it. The operator had to find and
// stop the live leg by run-id via agent-loop/stop instead.
//
// Correct behaviour: resolve the member the OPEN cohort barrier is waiting
// on — newest-first, non-terminal, seated in the cohort that still holds
// the label's open seat — and append the skip entry to THAT cohort, not to
// a drained sibling cohort.
func TestMemberActionSkipTargetsLiveOpenCohortMember(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	oldLeg, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	liveLeg, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].flowEngineDriven = true
	// Historical duplicate: same label, terminal, seated in a drained cohort.
	svc.runs[oldLeg.RunID].parentRunID = parent.RunID
	svc.runs[oldLeg.RunID].label = "tdd"
	svc.runs[oldLeg.RunID].status = RunStatusFailed
	svc.runs[oldLeg.RunID].flowCohortId = "cohort-round4"
	// Live member: same label, running, silent, seated in the open cohort.
	svc.runs[liveLeg.RunID].parentRunID = parent.RunID
	svc.runs[liveLeg.RunID].label = "tdd"
	svc.runs[liveLeg.RunID].status = RunStatusRunning
	svc.runs[liveLeg.RunID].flowCohortId = "cohort-round5"
	svc.runs[liveLeg.RunID].lastProviderEventAt = time.Now().UTC().Add(-time.Hour)
	svc.mu.Unlock()
	// Registration order mirrors live state: the old leg registered first.
	svc.agentOrchestrator.registerChild(parent.RunID, oldLeg.RunID)
	svc.agentOrchestrator.registerChild(parent.RunID, liveLeg.RunID)
	// Round-4 cohort already delivered its tdd entry and drained.
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "cohort-round4", 1)
	svc.agentOrchestrator.appendCohortResult(parent.RunID, "cohort-round4", cohortEntry{Label: "tdd", Status: "completed"})
	svc.agentOrchestrator.drainCohort(parent.RunID, "cohort-round4")
	// Round-5 cohort waits on spec_align (already arrived) + tdd (open seat).
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "cohort-round5", 2)
	svc.agentOrchestrator.appendCohortResult(parent.RunID, "cohort-round5", cohortEntry{Label: "spec_align", Status: "completed"})

	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "member_stalled", ActiveNode: "tdd", Cap: 3, Mode: "explicit",
	})

	_, handled, aerr := svc.handleMemberAction(parent.RunID, MemberAction{Action: "skip", Node: "tdd"})
	if aerr != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, aerr)
	}
	svc.mu.Lock()
	liveStatus := svc.runs[liveLeg.RunID].status
	svc.mu.Unlock()
	if liveStatus != RunStatusFailed {
		t.Fatalf("live stalled member status=%s, want Failed — the action dead-lettered onto the historical duplicate", liveStatus)
	}
	// The skip entry must consume the seat in the OPEN cohort so the join
	// can complete — a completed cohort drains (its expected entry is
	// deleted), so success is observable as: no open cohort remains and the
	// round5 barrier is gone. Appending to the drained sibling cohort would
	// leave round5 open forever.
	if svc.agentOrchestrator.cohortExpectedCount(parent.RunID, "cohort-round5") != 0 {
		t.Fatal("round-5 cohort must have drained — skip did not append to the open seat")
	}
	if svc.agentOrchestrator.hasOpenCohort(parent.RunID) {
		t.Fatal("no open cohort should remain after the skip fills the tdd seat")
	}
}

// BUG-639 retry half: the retry turn must re-drive the LIVE stalled member,
// not resurrect a terminal sibling (live run-309960 flipped back to running
// and re-entered the stall pool).
func TestMemberActionRetryTargetsLiveOpenCohortMember(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	oldLeg, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	liveLeg, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[oldLeg.RunID].parentRunID = parent.RunID
	svc.runs[oldLeg.RunID].label = "reviewer"
	svc.runs[oldLeg.RunID].status = RunStatusCompleted
	svc.runs[oldLeg.RunID].flowCohortId = "cohort-round4"
	svc.runs[liveLeg.RunID].parentRunID = parent.RunID
	svc.runs[liveLeg.RunID].label = "reviewer"
	svc.runs[liveLeg.RunID].status = RunStatusRunning
	svc.runs[liveLeg.RunID].flowCohortId = "cohort-round5"
	svc.runs[liveLeg.RunID].lastProviderEventAt = time.Now().UTC().Add(-time.Hour)
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, oldLeg.RunID)
	svc.agentOrchestrator.registerChild(parent.RunID, liveLeg.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "cohort-round4", 1)
	svc.agentOrchestrator.appendCohortResult(parent.RunID, "cohort-round4", cohortEntry{Label: "reviewer", Status: "completed"})
	svc.agentOrchestrator.drainCohort(parent.RunID, "cohort-round4")
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "cohort-round5", 1)

	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "member_stalled", ActiveNode: "reviewer", Cap: 3, Mode: "explicit",
	})

	before := time.Now().UTC()
	_, handled, aerr := svc.handleMemberAction(parent.RunID, MemberAction{Action: "retry", Node: "reviewer"})
	if aerr != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, aerr)
	}
	svc.mu.Lock()
	liveStamp := svc.runs[liveLeg.RunID].lastProviderEventAt
	svc.mu.Unlock()
	if liveStamp.Before(before) {
		t.Fatalf("retry must refresh the live member (lastProviderEventAt=%s before action at %s)", liveStamp, before)
	}
	if loop := svc.agentOrchestrator.loopStateFor(parent.RunID); loop.Status != "running" {
		t.Fatalf("loop status=%s, want running after retry", loop.Status)
	}
}
