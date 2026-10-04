package runner

import (
	"context"
	"testing"
)

// BUG-1180 (live run-204891, Task-039 sprint): at 00:07:00 a child's post-turn
// gate finished and the vibe resolver escalated (debate reprompt budget
// exhausted); at 00:07:01 the lifecycle manager entered idle_grace
// (clients=0 work=0); at 00:07:15 the owed settle finalized; at 00:07:32 the
// idle deadline fired draining_shutdown and stop-all CANCELLED the root
// mid-sprint. LiveWorkSnapshot counted work=0 for the entire 31s grace
// window even though the child still owed a settle disposition and armed
// resume/reprompt intents — none of the five counted markers
// (turnInFlight, turnCancel, postTurnGateCancel, flowInlineCancel,
// running/paused loop) are armed between gate-done and settle-finalize, and
// none are armed while a durable intent waits for re-drive. Owed
// dispositions are protected work: a runner that drains while they exist
// converts "re-drive on next client" into a terminal cancellation.

func TestBug1180_PendingFlowGateSettleCountsAsWork(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, cerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if cerr != nil {
		t.Fatalf("createRun parent: %v", cerr.msg)
	}
	rs := &interactiveRun{
		id:           "child-settle",
		parentRunID:  parent.RunID,
		status:       RunStatusRunning,
		providerKey:  ProviderKeyCodex,
		subs:         map[int64]chan ProviderEvent{},
		idempotency:  map[string]string{},
		workspaceCwd: t.TempDir(),
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	rs.pendingFlowGateSettle = true
	rs.pendingFlowGateTurnID = "turn-owed"
	svc.mu.Unlock()

	snap, err := svc.LiveWorkSnapshot(context.Background())
	if err != nil {
		t.Fatalf("LiveWorkSnapshot: %v", err)
	}
	if snap.ActiveCount() == 0 {
		t.Fatal("armed pendingFlowGateSettle counted as zero work — idle_grace would drain mid-settle (live run-204891)")
	}
}

func TestBug1180_ArmedResumeIntentCountsAsWork(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, cerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if cerr != nil {
		t.Fatalf("createRun parent: %v", cerr.msg)
	}
	rs := &interactiveRun{
		id:           "child-resume",
		parentRunID:  parent.RunID,
		status:       RunStatusWaitingUserApr,
		agentStatus:  "waiting_user_approval",
		providerKey:  ProviderKeyCodex,
		subs:         map[int64]chan ProviderEvent{},
		idempotency:  map[string]string{},
		workspaceCwd: t.TempDir(),
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	// The flow_awaiting_user park shape (CA-1179): parked on a durable resume
	// intent with no in-flight markers. This is the exact inventory state a
	// parked vibe sprint holds while it waits for the gate answer.
	rs.pendingResumePrompt = "reprompt after debate"
	rs.pendingResumeStepID = "tdd"
	rs.pendingResumeGen = 1
	svc.mu.Unlock()

	snap, err := svc.LiveWorkSnapshot(context.Background())
	if err != nil {
		t.Fatalf("LiveWorkSnapshot: %v", err)
	}
	if snap.ActiveCount() == 0 {
		t.Fatal("armed pendingResume* intent counted as zero work — drain would cancel a re-drivable park")
	}
}

func TestBug1180_ArmedGateRepromptCountsAsWork(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, cerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if cerr != nil {
		t.Fatalf("createRun parent: %v", cerr.msg)
	}
	rs := &interactiveRun{
		id:           "child-reprompt",
		parentRunID:  parent.RunID,
		status:       RunStatusRunning,
		providerKey:  ProviderKeyCodex,
		subs:         map[int64]chan ProviderEvent{},
		idempotency:  map[string]string{},
		workspaceCwd: t.TempDir(),
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	rs.pendingGateRepromptPrompt = "fix the failing gate"
	rs.pendingGateRepromptStepID = "coder"
	rs.pendingGateRepromptGen = 1
	svc.mu.Unlock()

	snap, err := svc.LiveWorkSnapshot(context.Background())
	if err != nil {
		t.Fatalf("LiveWorkSnapshot: %v", err)
	}
	if snap.ActiveCount() == 0 {
		t.Fatal("armed pendingGateReprompt* counted as zero work — drain would cancel mid-remediation")
	}
}
