package runner

import (
	"context"
	"testing"
	"time"
)

// BUG-507 (live run-22241): two stacked defects —
//
//  1. A provider permission request that expired unanswered dropped the
//     requested effect with no run-level trace: the approval record flipped
//     to expired, the run flipped WAITING→RUNNING, and nothing surfaced
//     that the model's intended action never ran.
//  2. A chat run could report status=completed while its mounted flow
//     loop was still open (loop_status=running) — the final turn's verdict
//     submission had been dropped via (1), so the run looked done while the
//     vibe-owner-debate loop dead-stopped.

// TestBug507_ApprovalExpiryEmitsDurableEvent pins that an approval expiry
// leaves a durable, broadcast run event recording the dropped effect.
func TestBug507_ApprovalExpiryEmitsDurableEvent(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.approvals["appr-x"] = &approvalRecord{
		id:        "appr-x",
		runID:     run.RunID,
		details:   ApprovalDetails{Kind: "file", Command: "write /tmp/wordfreq/out.txt"},
		status:    "pending",
		expiresAt: time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
	}
	svc.mu.Unlock()

	svc.expireApproval("appr-x")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	var found *ProviderEvent
	for i := range svc.runs[run.RunID].events {
		if svc.runs[run.RunID].events[i].Type == EventApprovalExpired {
			found = &svc.runs[run.RunID].events[i]
		}
	}
	if found == nil {
		t.Fatal("expiry must emit a durable approval_expired run event — the dropped effect must be visible at run level")
	}
	if found.ApprovalID != "appr-x" {
		t.Fatalf("approval_expired event must carry the approval id, got %q", found.ApprovalID)
	}
}

// TestBug507_QuestionExpiryEmitsDurableEvent pins the same for ask_user
// questions expiring unanswered.
func TestBug507_QuestionExpiryEmitsDurableEvent(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.questions["q-x"] = &questionRecord{
		id:        "q-x",
		runID:     run.RunID,
		prompt:    "pick",
		status:    "pending",
		expiresAt: time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		resolve:   make(chan questionResolveResult, 1),
	}
	svc.mu.Unlock()

	svc.expireQuestion("q-x")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	found := false
	for _, ev := range svc.runs[run.RunID].events {
		if ev.Type == EventQuestionExpired && ev.QuestionID == "q-x" {
			found = true
		}
	}
	if !found {
		t.Fatal("expiry must emit a durable question_expired run event")
	}
}

// TestBug507_RunDoesNotReportCompletedWhileLoopOpen pins that a turn ending
// with no code changes still withholds `completed` while the run's mounted
// flow loop is open — the loop is still driving work (scheduled hub
// reinvokes / parked escalations), so `completed` is a dishonest terminal.
func TestBug507_RunDoesNotReportCompletedWhileLoopOpen(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "verdict pending"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// The run hosts a mounted flow whose loop is still open (running) — the
	// live shape of run-22241's vibe-owner-debate host.
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	if _, apiErr := svc.startTurn(run.RunID, TurnInput{StepID: "turn-1", Prompt: "synthesize"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "turn settled", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[run.RunID]
		return !rs.turnInFlight
	})

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[run.RunID].status == RunStatusCompleted {
		t.Fatal("run must not report completed while its mounted flow loop is still open (running)")
	}
}

// TestBug507_SealedLoopStillCompletes guards the regression direction: a
// sealed (done) loop keeps the original plain-chat completion shape.
func TestBug507_SealedLoopStillCompletes(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "done", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	svc.runs[run.RunID].turnStartedAfterLoopDone = true
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn(run.RunID, TurnInput{StepID: "turn-1", Prompt: "hi"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "turn settled", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[run.RunID].turnInFlight
	})
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[run.RunID].status != RunStatusCompleted {
		t.Fatalf("sealed-loop follow-up must still complete, got %q", svc.runs[run.RunID].status)
	}
}
