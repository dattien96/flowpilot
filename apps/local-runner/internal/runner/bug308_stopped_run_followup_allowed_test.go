package runner

import (
	"context"
	"testing"
)

// BUG-308: a Stop-ped flow run is now continuable like a "done" one (reverses
// BUG-302's V-1 scope, which kept "stopped" sealed). Found live on run-19845:
// after Stop, a follow-up got 409 flow_stopped both live and after restart, and
// the optimistically-shown prompt vanished on reopen because a rejected turn is
// never persisted. These tests pin the new admission behavior; the flip of the
// original BUG-302 guard lives in TestChatRunAllowsNewTurnAfterStopped
// (bug302_chat_followup_after_flow_done_test.go).
//
// cross-provider-parity: the admission gate and offerReviewOutcomeTool scoping
// read only AgentLoopState.Status, never providerKey (Case 1, same as BUG-302).
// TestChatFollowUpAllowedAfterFlowLoopStoppedGrok exercises the Grok adapter to
// confirm no provider-specific branch was introduced.
//
// additive-tests-only: this file only adds tests; the single existing-test edit
// (the BUG-302 stopped-seal guard) is the deliberate behavior reversal itself.

func bug308CompletingCodexRegistry() *ProviderRegistry {
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
	return reg
}

// The exact run-19845 shape: a Stop-ped run that came back from disk (restart /
// reopen) must accept the next ordinary chat turn instead of 409 flow_stopped.
func TestChatFollowUpAllowedAfterFlowLoopStoppedResumedFromDisk(t *testing.T) {
	svc := newInteractiveService(bug308CompletingCodexRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.resumedFromDisk = true // reconstructed after a server restart (run-19845)
	if rs.runKind == "" {
		rs.runKind = "chat"
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "stopped", Round: 1, Cap: 3, Mode: "explicit"})

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "lan truoc fix gi the"}, "", ""); apiErr != nil {
		t.Fatalf("post-restart follow-up on a Stop-ped run was rejected (BUG-308 should admit it): %s (code=%s)", apiErr.msg, apiErr.code)
	}
}

// The BUG-305 plain-chat carve-out must also arm for a Stop-ped follow-up, so
// emitLocked/runTurn treat it as chat (no post-turn flow gate, no
// submit_review_outcome offer that would trip BUG-226's escalate fallback).
// Mirrors TestTurnStartedAfterLoopDoneFlagReflectsAdmissionLoopState's "done"
// case for the newly-added "stopped" case.
func TestTurnStartedAfterLoopDoneFlagTrueForStoppedFollowUp(t *testing.T) {
	svc := newInteractiveService(bug308CompletingCodexRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	if rs.runKind == "" {
		rs.runKind = "chat"
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "stopped", Round: 1, Cap: 3, Mode: "explicit"})

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "anything"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn (loop stopped): %s", apiErr.msg)
	}
	svc.mu.Lock()
	flag := svc.runs[parent.RunID].turnStartedAfterLoopDone
	svc.mu.Unlock()
	if !flag {
		t.Fatal("turnStartedAfterLoopDone must be TRUE for a follow-up admitted onto a Stop-ped loop (plain-chat carve-out must arm, else the post-turn gate force-blocks and BUG-226 can misfire)")
	}
}

// cross-provider-parity: same behavior through the Grok adapter path.
func TestChatFollowUpAllowedAfterFlowLoopStoppedGrok(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.resumedFromDisk = true
	if rs.runKind == "" {
		rs.runKind = "chat"
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "stopped", Round: 1, Cap: 3, Mode: "explicit"})

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "anything"}, "", ""); apiErr != nil {
		t.Fatalf("Grok follow-up on a Stop-ped run was rejected (BUG-308 should admit it): %s (code=%s)", apiErr.msg, apiErr.code)
	}
}

// Guard the scope of BUG-308: only the ROOT branch was relaxed. A CHILD turn
// whose PARENT flow loop is stopped must still be rejected — children must not
// run under a stopped parent flow (intent-race / orphan-work protection). This
// pins that the "stopped" carve-out did not leak into the child admission
// branch.
func TestChildTurnStillRejectedWhenParentStopped(t *testing.T) {
	svc := newInteractiveService(bug308CompletingCodexRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun child: %v", err)
	}
	svc.mu.Lock()
	svc.runs[child.RunID].parentRunID = parent.RunID
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "stopped", Round: 1, Cap: 3, Mode: "explicit"})

	_, apiErr := svc.startTurn(child.RunID, TurnInput{StepID: "s1", Prompt: "child work"}, "", "")
	if apiErr == nil {
		t.Fatal("child turn under a Stop-ped parent must still be rejected, got success")
	}
	if apiErr.code != "flow_stopped" {
		t.Fatalf("apiErr.code = %q, want flow_stopped for child under stopped parent", apiErr.code)
	}
}
