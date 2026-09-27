package runner

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// BUG-538: a spawned child's async first-turn dispatch can hit the
// flow_awaiting_user fence when the parent loop re-blocks between
// spawnChildRun's blocked-guard and the goroutine's startTurn admission (live
// run-1651: quota-route successor spawned, its turn refused on a re-blocked
// loop, handler marked step FAILED — the successor stayed `spawned`/`idle`
// with leg_state=active forever, wedging the parent run on an undispatchable
// live leg claim). The refusal is retryable: the child must park on a durable
// resume intent and re-drive on unblock, not fail.
func TestBug538_SpawnedChildBlockedDispatchParksDurableIntent(t *testing.T) {
	svc, runID := clusterFService(t)

	// Spawned-child shape exactly as spawnChildRun leaves it before the async
	// goroutine runs: leg active, agent_status=spawned, cohort label set.
	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun child: %v", apiErr)
	}
	svc.mu.Lock()
	child := svc.runs[handle.RunID]
	child.parentRunID = runID
	child.agentStatus = "spawned"
	child.legState = LegStateActive
	child.stepID = handle.StepID
	child.label = "candidate-a"
	child.flowCohortId = "flow-auto-parallel_rollout-attempt-0"
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, handle.RunID)

	// The parent loop re-blocks before the goroutine's admission — the exact
	// live race (vetoed sibling's escalate lands in between).
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", BlockReason: "escalate"})
	svc.handleSpawnedChildTurnFailure(handle.RunID, runID, "spawn first-turn prompt", handle.StepID,
		newAPIErr(http.StatusConflict, "flow_awaiting_user", "parent flow is waiting for your decision"))

	svc.mu.Lock()
	if child.status == RunStatusFailed {
		t.Fatal("retryable blocked-loop refusal must not fail the spawned leg")
	}
	if child.legState != LegStateActive {
		t.Fatalf("parked successor must keep its leg claim, got legState=%q", child.legState)
	}
	if strings.TrimSpace(child.pendingResumePrompt) == "" || strings.TrimSpace(child.pendingResumeStepID) == "" {
		t.Fatal("parked successor must carry a durable resume intent")
	}
	if child.pendingResumeGen == 0 {
		t.Fatal("pendingResumeGen must be bumped so the intent is claimable")
	}
	svc.mu.Unlock()

	// Durable: the session row must carry the intent so a restart can re-drive.
	store := svc.workflowStore
	if idx, ok := store.(SessionIndexReader); ok {
		sessions, err := idx.ListAllProviderSessions(context.Background())
		if err != nil {
			t.Fatalf("ListAllProviderSessions: %v", err)
		}
		found := false
		for _, sess := range sessions {
			if sess.RunID == handle.RunID {
				found = true
				if strings.TrimSpace(sess.PendingResumePrompt) == "" {
					t.Fatal("durable session row lost the resume intent — restart would strand the leg")
				}
			}
		}
		if !found {
			t.Fatal("child session row not persisted")
		}
	}

	// Unblock → resumePendingLoopWork must flush the child's durable intent.
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running"})
	svc.resumePendingLoopWork(runID)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		done := child.turnCount > 0
		svc.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	svc.mu.Lock()
	dispatched := child.turnCount
	svc.mu.Unlock()
	if dispatched == 0 {
		t.Fatal("unblock never re-drove the parked successor leg")
	}
	// The gen-claim + clear-on-accept path must consume the intent exactly
	// once — a second turn would double-dispatch the same spawn prompt.
	time.Sleep(150 * time.Millisecond)
	svc.mu.Lock()
	if child.turnCount != 1 {
		svc.mu.Unlock()
		t.Fatalf("intent re-drove the successor %d times — durable intent not consumed once", child.turnCount)
	}
	if child.pendingResumePrompt != "" {
		svc.mu.Unlock()
		t.Fatal("accepted intent still armed — a later flush would re-drive it")
	}
	svc.mu.Unlock()
}

// BUG-538 live path: the parked successor must also re-drive when the parent
// unblocks through agent-loop/continue (resumeFlowWithFeedback) — the real
// entry the R9 drill used — not only resumeAgentLoop.
func TestBug538_ParkedSuccessorFlushedOnFlowResume(t *testing.T) {
	svc, runID := clusterFService(t)

	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun child: %v", apiErr)
	}
	svc.mu.Lock()
	child := svc.runs[handle.RunID]
	child.parentRunID = runID
	child.agentStatus = "spawned"
	child.legState = LegStateActive
	child.stepID = handle.StepID
	child.label = "candidate-a"
	child.flowCohortId = "flow-auto-parallel_rollout-attempt-0"
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, handle.RunID)

	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", BlockReason: "escalate"})
	svc.handleSpawnedChildTurnFailure(handle.RunID, runID, "spawn first-turn prompt", handle.StepID,
		newAPIErr(http.StatusConflict, "flow_awaiting_user", "parent flow is waiting for your decision"))

	svc.mu.Lock()
	parked := child.status == RunStatusWaitingUserApr && child.pendingResumePrompt != ""
	svc.mu.Unlock()
	if !parked {
		t.Fatal("setup: successor did not park on a durable resume intent")
	}

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		done := child.turnCount > 0
		svc.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	svc.mu.Lock()
	dispatched := child.turnCount
	svc.mu.Unlock()
	if dispatched == 0 {
		t.Fatal("continue never re-drove the parked successor leg")
	}
	time.Sleep(150 * time.Millisecond)
	svc.mu.Lock()
	if child.turnCount != 1 {
		svc.mu.Unlock()
		t.Fatalf("continue re-drove the successor %d times — intent not consumed once", child.turnCount)
	}
	svc.mu.Unlock()
}

// BUG-538 residual: the flow verdict settles while the successor is still
// parked — the settle must close its leg claim and drop the armed intent,
// not leave leg_state=active forever (live run-1651 residue).
func TestBug538_ParkedSuccessorLegClosedOnFlowDone(t *testing.T) {
	svc, runID := clusterFService(t)

	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun child: %v", apiErr)
	}
	svc.mu.Lock()
	child := svc.runs[handle.RunID]
	child.parentRunID = runID
	child.agentStatus = "spawned"
	child.legState = LegStateActive
	child.stepID = handle.StepID
	child.label = "candidate-a"
	child.flowCohortId = "flow-auto-parallel_rollout-attempt-0"
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, handle.RunID)

	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", BlockReason: "escalate"})
	svc.handleSpawnedChildTurnFailure(handle.RunID, runID, "spawn first-turn prompt", handle.StepID,
		newAPIErr(http.StatusConflict, "flow_awaiting_user", "parent flow is waiting for your decision"))

	svc.reconcileChildRunsOnFlowDone(runID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if child.status != RunStatusCompleted {
		t.Fatalf("flow-done settle must complete the parked child, got %q", child.status)
	}
	if child.legState == LegStateActive {
		t.Fatal("flow-done settle leaked an active leg/worktree claim")
	}
	if child.legClosedReason != LegClosedReasonFlowDone {
		t.Fatalf("legClosedReason = %q, want %q", child.legClosedReason, LegClosedReasonFlowDone)
	}
	if child.pendingResumePrompt != "" {
		t.Fatal("settled child kept a live resume intent — restart would re-drive a completed run")
	}
}

// BUG-538 twin: a hard dispatch failure (non-retryable refusal) must close the
// leg claim — a spawned leg that never ran must not hold leg_state=active
// against its worktree forever (live run-1651 residue).
func TestBug538_SpawnedChildHardFailureClosesLeg(t *testing.T) {
	svc, runID := clusterFService(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "stopped"})

	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun child: %v", apiErr)
	}
	svc.mu.Lock()
	child := svc.runs[handle.RunID]
	child.parentRunID = runID
	child.agentStatus = "spawned"
	child.legState = LegStateActive
	child.stepID = handle.StepID
	child.label = "candidate-a"
	child.flowCohortId = "flow-auto-parallel_rollout-attempt-0"
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, handle.RunID)

	svc.handleSpawnedChildTurnFailure(handle.RunID, runID, "spawn first-turn prompt", handle.StepID,
		newAPIErr(http.StatusConflict, "flow_stopped", "parent flow loop is stopped"))

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if child.status != RunStatusFailed {
		t.Fatalf("terminal refusal must fail the child, got %q", child.status)
	}
	if child.legState == LegStateActive {
		t.Fatal("failed spawned leg leaked an active worktree claim")
	}
}
