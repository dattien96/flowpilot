package runner

import (
	"testing"
	"time"
)

// CP-51 A1 residual (run-9437 class): after flow_control continue delegates a
// writer, the hub must not start a concurrent gate-reprompt / write turn while
// that child is still RUNNING or waiting.
func TestRun9437HubParkedWhileActiveFlowChildBlocksStartTurn(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-9437-hub"
	childID := "run-9437-coder"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		stepID:           "synthesis",
		lastTurnStepID:   "synthesis",
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:           childID,
		parentRunID:  parentID,
		label:        "coder",
		role:         "coder",
		agentName:    "coder",
		status:       RunStatusRunning,
		agentStatus:  string(RunStatusRunning),
		turnInFlight: true,
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 3
		return st
	})

	if !svc.shouldParkHubWriteTurn(parentID) {
		t.Fatal("shouldParkHubWriteTurn = false, want true while coder is RUNNING")
	}

	_, apiErr := svc.startTurn(parentID, TurnInput{
		StepID: "synthesis",
		Prompt: "Flow gate remediation: create missing BUG doc",
	}, "", "")
	if apiErr == nil {
		t.Fatal("startTurn succeeded on hub while coder is RUNNING; want hub_parked")
	}
	if apiErr.code != "hub_parked" {
		t.Fatalf("startTurn code = %q, want hub_parked (msg=%s)", apiErr.code, apiErr.msg)
	}
}

func TestRun9437ContinueDelegatedSuppressesSameTurnHubGateReprompt(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-9437-hub-continue"
	childID := "run-9437-coder-r1"
	const turnID = "turn-9437-synthesis"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:                         parentID,
		flowEngineDriven:           true,
		status:                     RunStatusRunning,
		currentTurnID:              turnID,
		hubContinueDelegatedTurnID: turnID,
		pendingGateRepromptPrompt:  "Flow gate: create missing requirements/09-BugFix/BUG-908.md",
		pendingGateRepromptStepID:  "synthesis",
		pendingGateRepromptGen:     3,
		subs:                       map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:           childID,
		parentRunID:  parentID,
		label:        "coder",
		role:         "coder",
		status:       RunStatusRunning,
		turnInFlight: true,
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)

	svc.scheduleRootGateRepromptOrPark(parentID, "synthesis",
		"Flow gate: create missing requirements/09-BugFix/BUG-908.md", turnID, 3)

	// Allow any accidental async startTurn to race (must not start).
	time.Sleep(50 * time.Millisecond)

	svc.mu.Lock()
	rs := svc.runs[parentID]
	promptLeft := rs.pendingGateRepromptPrompt
	hubInFlight := rs.turnInFlight
	svc.mu.Unlock()
	if promptLeft != "" {
		t.Fatalf("pendingGateRepromptPrompt still set after continue-suppress: %q", promptLeft)
	}
	if hubInFlight {
		t.Fatal("hub turnInFlight true: gate reprompt must not start after continue-delegate")
	}
}

func TestRun9437HubParkDefersGateRepromptWithoutStartingWhileChildWaiting(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-9437-hub-park"
	childID := "run-9437-coder-wait"
	const turnID = "turn-9437-gate"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:                        parentID,
		flowEngineDriven:          true,
		status:                    RunStatusRunning,
		pendingGateRepromptPrompt: "remediate missing CA note",
		pendingGateRepromptStepID: "hub",
		pendingGateRepromptGen:    1,
		subs:                      map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:                childID,
		parentRunID:       parentID,
		label:             "coder",
		status:            RunStatusWaitingApproval,
		agentStatus:       string(RunStatusWaitingApproval),
		pendingApprovalID: "appr-1",
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)

	svc.scheduleRootGateRepromptOrPark(parentID, "hub", "remediate missing CA note", turnID, 1)
	time.Sleep(50 * time.Millisecond)

	svc.mu.Lock()
	rs := svc.runs[parentID]
	if rs.pendingGateRepromptPrompt == "" {
		t.Fatal("parked path must keep durable gate reprompt intent for later flush")
	}
	if rs.turnInFlight {
		t.Fatal("hub must not start gate reprompt while child is waiting_approval")
	}
	svc.mu.Unlock()
}

func TestHubParkedIsTransientStartTurnError(t *testing.T) {
	if isPermanentStartTurnError(newAPIErr(409, "hub_parked", "parked")) {
		t.Fatal("hub_parked must be transient so durable intents can flush after children settle")
	}
}
