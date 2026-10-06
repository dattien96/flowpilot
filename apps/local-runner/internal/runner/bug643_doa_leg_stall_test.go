package runner

import (
	"strings"
	"testing"
	"time"
)

// BUG-643 (live run-306526, debate owner_2 leg run-460096): a leg can be
// spawned and registered into an open cohort while its first turn is never
// dispatched — lastProviderEventAt stays zero AND turnInFlight stays false.
// checkAndBlockStalledMembers skipped exactly that shape (`else { continue }`),
// and deadDispatchNonTerminalChildExists counted the leg as live, so the
// dead-dispatch sweep never re-drove it either. run-460096 held the debate
// cohort seat ~17min producing zero records until an operator stopped it.
// The sweep must age a zero-event, non-in-flight member from createdAt the
// same way it ages in-flight ones.
func TestBug643_DOAMemberLegStallsOpenCohort(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].stallTimeout = time.Minute
	c := svc.runs[child.RunID]
	c.parentRunID = parent.RunID
	c.label = "owner_2"
	c.status = RunStatusRunning
	c.legState = LegStateActive
	c.flowCohortId = "flow-auto-debate-round-0"
	c.turnInFlight = false
	c.lastProviderEventAt = time.Time{} // never dispatched — zero events
	c.createdAt = time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339Nano)
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "flow-auto-debate-round-0", 2)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running"})

	if !svc.checkAndBlockStalledMembers(parent.RunID) {
		t.Fatal("DOA member leg (0 events, not in-flight, aged out) must park the hub member_stalled — run-460096 held the debate cohort seat 17min live")
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.BlockReason != "member_stalled" || !strings.Contains(loop.GateReason, "owner_2") {
		t.Fatalf("loop.BlockReason=%q GateReason=%q, want member_stalled naming owner_2", loop.BlockReason, loop.GateReason)
	}
}

// A freshly spawned member whose createdAt is still inside the timeout is in
// the dispatch window — not yet provably dead.
func TestBug643_FreshDOAMemberLegDoesNotStall(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].stallTimeout = time.Minute
	c := svc.runs[child.RunID]
	c.parentRunID = parent.RunID
	c.label = "owner_2"
	c.status = RunStatusRunning
	c.legState = LegStateActive
	c.flowCohortId = "flow-auto-debate-round-0"
	c.turnInFlight = false
	c.lastProviderEventAt = time.Time{}
	c.createdAt = time.Now().UTC().Format(time.RFC3339Nano) // just spawned
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "flow-auto-debate-round-0", 2)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running"})

	if svc.checkAndBlockStalledMembers(parent.RunID) {
		t.Fatal("fresh DOA leg inside the dispatch window must not stall — age is unproven")
	}
}

// A member whose turn is queued (pendingTurnPrompt) is held work — serial
// dependency ordering can legitimately keep it queued past the timeout while
// a sibling runs. It is not silence.
func TestBug643_QueuedTurnMemberIsNotStalled(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].stallTimeout = time.Minute
	c := svc.runs[child.RunID]
	c.parentRunID = parent.RunID
	c.label = "owner_2"
	c.status = RunStatusRunning
	c.legState = LegStateActive
	c.flowCohortId = "flow-auto-debate-round-0"
	c.turnInFlight = false
	c.lastProviderEventAt = time.Time{}
	c.createdAt = time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339Nano)
	c.pendingTurnPrompt = "[flow-engine] run when dependencies settle"
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "flow-auto-debate-round-0", 2)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running"})

	if svc.checkAndBlockStalledMembers(parent.RunID) {
		t.Fatal("member with an armed turn queue is held work, not silence — dependency ordering owns it")
	}
}

// waiting_* statuses are parks owned by the orphan/mirror heal sweeps
// (BUG-641 heal class) — the DOA arm must not double-own them.
func TestBug643_WaitingMemberIsNotDOAStalled(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].stallTimeout = time.Minute
	c := svc.runs[child.RunID]
	c.parentRunID = parent.RunID
	c.label = "owner_2"
	c.status = RunStatusWaitingUserApr
	c.legState = LegStateActive
	c.flowCohortId = "flow-auto-debate-round-0"
	c.turnInFlight = false
	c.lastProviderEventAt = time.Time{}
	c.createdAt = time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339Nano)
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "flow-auto-debate-round-0", 2)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running"})

	if svc.checkAndBlockStalledMembers(parent.RunID) {
		t.Fatal("waiting_* member is a park owned by the orphan/mirror heal sweeps — DOA arm must not double-own it")
	}
}

// The stall timer is armed off the same age attribution — a DOA leg the
// checker can flag must also schedule a sweep, otherwise the wedge only
// resolves on the next provider event (which never comes for a DOA leg).
func TestBug643_DOAMemberLegSchedulesStallTimer(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].stallTimeout = time.Minute
	c := svc.runs[child.RunID]
	c.parentRunID = parent.RunID
	c.label = "owner_2"
	c.status = RunStatusRunning
	c.legState = LegStateActive
	c.flowCohortId = "flow-auto-debate-round-0"
	c.turnInFlight = false
	c.lastProviderEventAt = time.Time{}
	c.createdAt = time.Now().UTC().Add(-30 * time.Second).Format(time.RFC3339Nano)
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "flow-auto-debate-round-0", 2)

	key := stallTimerKey(svc, parent.RunID)
	stallTimerMu.Lock()
	if t0, ok := stallTimers[key]; ok {
		t0.Stop()
		delete(stallTimers, key)
	}
	stallTimerMu.Unlock()

	svc.maybeScheduleStallCheck(parent.RunID)

	stallTimerMu.Lock()
	_, armed := stallTimers[key]
	stallTimerMu.Unlock()
	if !armed {
		t.Fatal("DOA member leg did not arm a stall sweep — the wedge would only resolve on a provider event that never comes")
	}
	stallTimerMu.Lock()
	if t0, ok := stallTimers[key]; ok {
		t0.Stop()
		delete(stallTimers, key)
	}
	stallTimerMu.Unlock()
}
