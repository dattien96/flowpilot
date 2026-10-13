package runner

import (
	"context"
	"testing"
	"time"
)

// BUG-670 Gap A (live run-752742 legs waiting_question 4h+): expireQuestion
// clears rs.pendingQuestionID and flips waiting_question→running in memory,
// and persists the QUESTION snapshot — but never persists the run row. A
// restart before any other session write resurrects the stale waiting status
// from the durable row. clearPendingQuestion already persists its run
// snapshot; expireQuestion must match.
func TestBug670_ExpireQuestionPersistsClearedRunStatus(t *testing.T) {
	svc, _ := newTestServer(t)
	ctx := context.Background()

	runID := "run-670a"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                runID,
		status:            RunStatusWaitingQuestion,
		agentStatus:       string(RunStatusWaitingQuestion),
		pendingQuestionID: "q-670a",
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.questions["q-670a"] = &questionRecord{
		id: "q-670a", runID: runID, prompt: "pick a route",
		status: "pending", resolve: make(chan questionResolveResult, 1),
		expiresAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
	}
	svc.mu.Unlock()

	// Durable row shaped like a mint-time persist: waiting status on file.
	if err := svc.workflowStore.(InteractiveStateStore).UpsertProviderSession(ctx, ProviderSessionState{
		RunID: runID, ProviderKey: ProviderKeyCodex,
		Status:      RunStatusWaitingQuestion,
		AgentStatus: string(RunStatusWaitingQuestion),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc.expireQuestion("q-670a")

	// Persist may be scheduled async — poll the durable row.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		row := findSession(t, svc, runID)
		if row.Status != RunStatusWaitingQuestion {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	row := findSession(t, svc, runID)
	if row.Status == RunStatusWaitingQuestion {
		t.Fatalf("durable run row still carries status=%q after expiry — restart resurrects a dead wait", row.Status)
	}
}

// BUG-670 Gap B: healOrphanedWait bails the moment pendingQuestionID is set
// (wedge_sweep.go: "a card may have been armed") WITHOUT verifying the
// record exists — a pointer to an expired/lost record reads as a live card
// forever. clearDanglingGateIDsLocked was written for exactly this class
// ("dangling gate id is residue, not a live card") but is only invoked on
// the mirror-heal path. A run waiting on a dead record must orphan-heal.
func TestBug670_OrphanSweepClearsWaitOnDeadQuestionRecord(t *testing.T) {
	svc, _ := newTestServer(t)
	ctx := context.Background()

	runID := "run-670b"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                runID,
		status:            RunStatusWaitingQuestion,
		agentStatus:       string(RunStatusWaitingQuestion),
		pendingQuestionID: "q-670b-dead",
		subs:              map[int64]chan ProviderEvent{},
	}
	// The record exists but is already expired — no answer can ever land.
	svc.questions["q-670b-dead"] = &questionRecord{
		id: "q-670b-dead", runID: runID, prompt: "pick a route",
		status: "expired", resolve: make(chan questionResolveResult, 1),
	}
	svc.mu.Unlock()

	svc.healOrphanedWait(runID)

	svc.mu.Lock()
	rs := svc.runs[runID]
	svc.mu.Unlock()
	if rs.pendingQuestionID != "" {
		t.Fatalf("pendingQuestionID=%q still points at an expired record — run stays 409-locked forever", rs.pendingQuestionID)
	}
	if rs.status == RunStatusWaitingQuestion {
		t.Fatalf("status still %q on a dead card — orphan sweep must heal the wait", rs.status)
	}
	if err := ctx.Err(); err != nil {
		t.Fatal(err)
	}
}

// BUG-670 parity: expireApproval shares the same gap — it cleared the
// in-memory pointer/status without persisting the run row. Same
// durable-resurrection class, approval side.
func TestBug670_ExpireApprovalPersistsClearedRunStatus(t *testing.T) {
	svc, _ := newTestServer(t)
	ctx := context.Background()

	runID := "run-670c"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                runID,
		status:            RunStatusWaitingApproval,
		agentStatus:       string(RunStatusWaitingApproval),
		pendingApprovalID: "appr-670c",
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.approvals["appr-670c"] = &approvalRecord{
		id: "appr-670c", runID: runID,
		status: "pending",
	}
	svc.mu.Unlock()

	if err := svc.workflowStore.(InteractiveStateStore).UpsertProviderSession(ctx, ProviderSessionState{
		RunID: runID, ProviderKey: ProviderKeyCodex,
		Status:      RunStatusWaitingApproval,
		AgentStatus: string(RunStatusWaitingApproval),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc.expireApproval("appr-670c")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		row := findSession(t, svc, runID)
		if row.Status != RunStatusWaitingApproval {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	row := findSession(t, svc, runID)
	if row.Status == RunStatusWaitingApproval {
		t.Fatalf("durable run row still carries status=%q after approval expiry — parity with expireQuestion persist", row.Status)
	}
}
