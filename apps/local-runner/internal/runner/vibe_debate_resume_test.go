package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Live run-136 / run-9597: after the owner-debate resolved, the sprint
// topology was restored but nothing re-armed a continuation — the gate
// diverted the gated child's node completion before its done-edge fired, so
// the loop sat "running" with nothing in flight until hub_stalled parked it
// and every Continue only re-ran another debate round. The fix arms the
// standard gate-reprompt intent on the gated child (the child session never
// saw the debate — it ran on the parent hub — so the verdict is carried
// inline), so its re-completion advances the restored sprint chain through
// the normal completion path.
func TestVibeDebateRestoreRepromptsGatedChild(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.chatFlowRef = workingmode.PackPrefix + vibeSprintFlowID
	rs.autoOrchestrate = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.delegate"},
		{ID: "coder", Behavior: "agent.code"},
	}
	rs.lastFlowVerdicts = []VerdictRow{{ACID: "remediation", Verdict: "fail", Note: "revert impl to stubs"}}
	svc.runs["child-gated"] = &interactiveRun{
		id: "child-gated", parentRunID: parent.RunID, label: "tdd",
		status: RunStatusRunning, lastTurnStepID: "step-tdd",
	}
	svc.mu.Unlock()

	svc.stashVibeFlowForDebate(parent.RunID, "child-gated")
	svc.mu.Lock()
	rs = svc.runs[parent.RunID]
	rs.chatFlowRef = workingmode.PackPrefix + vibeOwnerDebateFlowID
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: vibeDebateSynthesisNodeID, Behavior: "hub.inline"},
	}
	rs.activeFlowAcceptanceNodes = nil
	svc.mu.Unlock()

	svc.onVibeCpNodeDone(parent.RunID, vibeDebateSynthesisNodeID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	child := svc.runs["child-gated"]
	if child.pendingGateRepromptPrompt == "" {
		t.Fatal("restore must arm the gated child's reprompt intent so its node completion re-fires the sprint edge")
	}
	if !strings.Contains(child.pendingGateRepromptPrompt, "revert impl to stubs") {
		t.Fatalf("reprompt prompt must carry the debate verdict inline, got %q", child.pendingGateRepromptPrompt)
	}
	if child.pendingGateRepromptStepID != "step-tdd" || child.pendingGateRepromptGen != 1 {
		t.Fatalf("reprompt step/gen wrong: step=%q gen=%d", child.pendingGateRepromptStepID, child.pendingGateRepromptGen)
	}
	rs = svc.runs[parent.RunID]
	if rs.pendingHubReinvoke {
		t.Fatal("hub reinvoke must not double-drive when a gated child reprompt is armed")
	}
	if len(rs.vibeParkedGatedRunIDs) != 0 {
		t.Fatal("gated run IDs must clear after restore")
	}
}

// When no gated child was recorded (drift-only debate on the hub, or the
// child row is gone) the restore still owes the loop a continuation — it
// falls back to a hub reinvoke so the restored sprint does not sit "running"
// with nothing in flight until hub_stalled parks it.
func TestVibeDebateRestoreArmsHubContinuation(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.chatFlowRef = workingmode.PackPrefix + vibeSprintFlowID
	rs.autoOrchestrate = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.delegate"},
		{ID: "coder", Behavior: "agent.code"},
	}
	svc.mu.Unlock()

	svc.stashVibeFlowForDebate(parent.RunID, "")
	svc.mu.Lock()
	rs = svc.runs[parent.RunID]
	rs.chatFlowRef = workingmode.PackPrefix + vibeOwnerDebateFlowID
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: vibeDebateSynthesisNodeID, Behavior: "hub.inline"},
	}
	rs.activeFlowAcceptanceNodes = nil
	// The debate_synthesis turn is still finishing when its done verdict lands —
	// the continuation must defer via pendingHubReinvoke, not drop.
	rs.turnInFlight = true
	svc.mu.Unlock()

	svc.onVibeCpNodeDone(parent.RunID, vibeDebateSynthesisNodeID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[parent.RunID]
	if !rs.pendingHubReinvoke {
		t.Fatal("debate restore must arm pendingHubReinvoke so the restored sprint resumes")
	}
	if strings.TrimSpace(rs.pendingHubReinvokePrompt) == "" {
		t.Fatal("debate restore must carry a resume prompt for the re-invoked hub turn")
	}
}

// The armed continuation must not resurrect a parked flow that is still
// mid-debate, and must not overwrite an already-armed intent.
func TestVibeDebateRestoreNoParkedGraphNoArm(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.autoOrchestrate = true
	rs.turnInFlight = true
	svc.mu.Unlock()

	// No parked sprint graph → restore is a no-op and must not arm.
	svc.onVibeCpNodeDone(parent.RunID, vibeDebateSynthesisNodeID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[parent.RunID]
	if rs.pendingHubReinvoke || rs.pendingHubReinvokePrompt != "" {
		t.Fatalf("no parked graph must not arm a reinvoke (pending=%v prompt=%q)",
			rs.pendingHubReinvoke, rs.pendingHubReinvokePrompt)
	}
}
