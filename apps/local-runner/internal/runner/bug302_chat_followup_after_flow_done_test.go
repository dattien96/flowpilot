package runner

import (
	"context"
	"testing"
	"time"
)

// BUG-302: a Chat-mode Review Loop hub whose flow finished ("done") could
// never receive another message — startTurn rejected every subsequent turn
// with 409 flow_stopped, identically to a genuinely Stop-ped run. This
// contradicts CP-36's own design ("loop ends, no further spawns" — not "the
// chat is sealed") and the desktop's own "Chat Intent locked after the first
// message" contract, which only makes sense if the SAME run stays usable for
// follow-up chat. Confirmed live against run-18997 (CP-51 A10 test): loop_state
// was "done", and "hello, turn trước bạn fix gì thế" was rejected outright.
//
// cross-provider-parity: both the admission-gate check (startTurn) and the
// offerReviewOutcomeTool/BUG-226 fallback scoping fix touch no `providerKey` —
// confirmed by reading both call sites (interactive_service.go's loop-status
// gate and the offerReviewOutcomeTool computation) — provider-agnostic (Case
// 1). One representative provider (Codex, matching this file's sibling
// TestRun1264StartTurnDoesNotOverwriteTitleWithJoinedResultNote) is sufficient.
//
// additive-tests-only: this file only adds new tests; no existing test file
// is modified.
func TestChatFollowUpAllowedAfterFlowLoopDone(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				// Plain prose reply, no submit_review_outcome call — exactly what a
				// genuine "what did the previous turn fix?" follow-up looks like.
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "I fixed the Add offset bug."})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	stepID := "chat-" + parent.RunID
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: stepID, Prompt: "fix bug 1+1 != 2"}, "", ""); apiErr != nil {
		t.Fatalf("first startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "first turn completes", time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[parent.RunID]
		return !rs.turnInFlight && !rs.pendingFlowGateSettle && rs.postTurnGateCancel == nil
	})

	// Simulate the Review Loop having already finished successfully (the exact
	// run-18997 precondition): flow-engine-driven, auto-orchestrated, loop done.
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	if rs.runKind == "" {
		rs.runKind = "chat"
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "done", Round: 1, Cap: 3, Mode: "explicit"})

	_, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: stepID, Prompt: "hello, turn trước bạn fix gì thế"}, "", "")
	if apiErr != nil {
		t.Fatalf("follow-up startTurn after loop done was rejected: %s (code=%s) — BUG-302 not fixed", apiErr.msg, apiErr.code)
	}
	waitLoop(t, "follow-up turn completes", time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})

	// The follow-up must not be mistaken for the hub's own review-decision turn:
	// the fake adapter answered in plain prose with no submit_review_outcome
	// call, so if offerReviewOutcomeTool/BUG-226's fallback fired anyway, this
	// would have escalated ("completed without calling submit_review_outcome"),
	// flipping the loop to "blocked" and the run to WaitingApproval.
	snap := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if snap.Status != "done" {
		t.Fatalf("loop status after plain follow-up = %q, want unchanged %q (BUG-226 fallback misfired on a non-decision turn)", snap.Status, "done")
	}
	svc.mu.Lock()
	finalStatus := svc.runs[parent.RunID].status
	svc.mu.Unlock()
	if finalStatus == RunStatusWaitingApproval {
		t.Fatalf("run status after plain follow-up = %q, want not waiting_approval (BUG-226 fallback misfired)", finalStatus)
	}
}

// A workflow-kind run must ALSO keep accepting a follow-up turn once its flow
// loop reaches "done" — the desktop's composer (ChatInput) is the same
// component regardless of Chat vs Workflow mode, with no distinct "closed"
// affordance for a workflow run either, so the carve-out is not chat-only.
func TestWorkflowRunAlsoAllowsNewTurnAfterFlowLoopDone(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.runKind = "workflow"
	rs.flowEngineDriven = true
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "done", Round: 1, Cap: 3, Mode: "explicit"})

	_, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "anything"}, "", "")
	if apiErr != nil {
		t.Fatalf("follow-up startTurn on a workflow-kind run after loop done was rejected: %s (code=%s)", apiErr.msg, apiErr.code)
	}
}

// Regression guard: a genuinely Stop-ped chat run must still be sealed —
// BUG-302's fix only carves out "done", never "stopped".
func TestChatRunStillRejectsNewTurnWhenStopped(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	if rs.runKind == "" {
		rs.runKind = "chat"
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "stopped", Round: 1, Cap: 3, Mode: "explicit"})

	_, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "anything"}, "", "")
	if apiErr == nil {
		t.Fatal("expected flow_stopped rejection for a stopped chat run, got success")
	}
	if apiErr.code != "flow_stopped" {
		t.Fatalf("apiErr.code = %q, want flow_stopped", apiErr.code)
	}
}
