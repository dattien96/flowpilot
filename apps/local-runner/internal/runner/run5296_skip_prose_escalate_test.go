package runner

import (
	"testing"
)

// TestHubShouldSkipProseEscalateWhenOpenCohort is run-5296: after continue,
// round-1 reviewers are in-flight; a gate/hub prose turn must NOT BUG-226
// escalate (which park-cancels those reviewers and shows a stale form).
func TestHubShouldSkipProseEscalateWhenOpenCohort(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-hub"
	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		autoOrchestrate:  true,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3})

	// Open cohort of 2, zero members buffered yet.
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-coder-round-1", 2)
	if !svc.hubShouldSkipProseEscalate(parentID) {
		t.Fatal("open cohort must skip prose escalate")
	}
}

func TestHubShouldSkipProseEscalateWhenChildRunning(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-hub2"
	childID := "run-child"
	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id: parentID, flowEngineDriven: true, status: RunStatusRunning,
		subs: map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id: childID, parentRunID: parentID, status: RunStatusRunning, turnInFlight: true,
		subs: map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	if !svc.hubShouldSkipProseEscalate(parentID) {
		t.Fatal("running child must skip prose escalate")
	}
}

func TestHubShouldNotSkipProseEscalateWhenIdle(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-hub3"
	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id: parentID, flowEngineDriven: true, status: RunStatusRunning,
		subs: map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	if svc.hubShouldSkipProseEscalate(parentID) {
		t.Fatal("idle hub with no children must allow BUG-226 escalate")
	}
}
