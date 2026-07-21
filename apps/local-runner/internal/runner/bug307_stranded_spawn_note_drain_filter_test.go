package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

// BUG-307 (extended, run-21028): the entry-spawn wait-notice
// ("[flow-engine] An agent has already been spawned … wait for its result; you
// will be reinvoked automatically once this step of the flow completes") is
// appended to the hub's pendingAgentContext at flow START (spawnChildRun's
// ParentContextNote) while the loop is still RUNNING — so the append-site
// guards cannot catch it; only a later Stop strands it. When the user then sends
// a follow-up on the stopped run, that stale "wait for reinvoke" instruction
// must NOT ride into the prompt: on a sealed loop no reinvoke will ever come, so
// the model sits waiting and the hub stalls (live repro run-21028). It is
// dropped at the drain point for a sealed-loop (turnStartedAfterLoopDone)
// follow-up.
//
// cross-provider-parity: the drain-time filter (interactive_service.go startTurn)
// reads only rs.turnStartedAfterLoopDone + isFlowEnginePrompt(note); no
// providerKey branch. One representative provider (Codex) suffices, consistent
// with BUG-302/BUG-308.
//
// additive-tests-only: only new tests added.

func bug307bCapturingRegistry(captured *string) *ProviderRegistry {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				*captured = req.Prompt
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
				return nil
			})
		},
	})
	return reg
}

const bug307bSpawnNote = "[flow-engine] An agent has already been spawned to work on this request. " +
	"Do not duplicate that work yourself. Wait for its result; you will be reinvoked automatically once this step of the flow completes."

func TestSealedLoopFollowUpDropsStrandedFlowEngineSpawnNote(t *testing.T) {
	var capturedPrompt string
	svc := newInteractiveService(bug307bCapturingRegistry(&capturedPrompt), newInteractiveCatalog(), newFakeWorkflowStore())
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
	// The spawn wait-notice left stranded from flow start (children were spawned,
	// hub never drained it because the user Stop-ped before synthesis).
	rs.pendingAgentContext = []string{bug307bSpawnNote}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "stopped", Round: 1, Cap: 3, Mode: "explicit"})

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "vay la done chua tra loi ok"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn on stopped run: %s (code=%s)", apiErr.msg, apiErr.code)
	}
	waitLoop(t, "follow-up completes", 2*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})

	if capturedPrompt == "" {
		t.Fatal("adapter never received a prompt (turn not dispatched)")
	}
	for _, bad := range []string{"[flow-engine]", "reinvoked automatically", "Wait for its result", "FlowPilot system note"} {
		if strings.Contains(capturedPrompt, bad) {
			t.Fatalf("stale flow-engine spawn note leaked into a stopped-run follow-up prompt (contains %q):\n%s", bad, capturedPrompt)
		}
	}
	if !strings.Contains(capturedPrompt, "vay la done chua tra loi ok") {
		t.Fatalf("user prompt missing from turn:\n%s", capturedPrompt)
	}
}

// Guard against over-filtering: on a still-RUNNING loop the same note MUST be
// preserved — that is a legitimate synthesis reinvoke's context (the hub needs
// to know children are running). Only a sealed-loop follow-up strips it.
func TestRunningLoopTurnKeepsFlowEngineNote(t *testing.T) {
	var capturedPrompt string
	svc := newInteractiveService(bug307bCapturingRegistry(&capturedPrompt), newInteractiveCatalog(), newFakeWorkflowStore())
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
	rs.pendingAgentContext = []string{bug307bSpawnNote}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Round: 0, Cap: 3, Mode: "explicit"})

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "synthesize now"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn on running loop: %s (code=%s)", apiErr.msg, apiErr.code)
	}
	waitLoop(t, "turn completes", 2*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})

	if !strings.Contains(capturedPrompt, "Wait for its result") {
		t.Fatalf("running-loop turn must KEEP the flow-engine note (over-filtering regression); prompt:\n%s", capturedPrompt)
	}
}
