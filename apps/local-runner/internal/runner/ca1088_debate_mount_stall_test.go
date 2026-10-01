package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// seedVibeDebateStarvedRun reproduces the live run-3362 wedge: a vibe sprint
// run whose post-turn gate routed to vibe-owner-debate. stashVibeFlowForDebate
// parked the sprint topology and startResolvedFlow swapped the debate graph in
// — but the hub stall watchdog blocked the shared loop mid-mount, so
// child_spawn_refused_blocked_loop starved owner_1/owner_2 before they ever
// existed (flow_start_no_entry). Steps: owner steps seeded PENDING, zero
// owner children, hub silent past the stall timeout.
func seedVibeDebateStarvedRun(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.chatFlowRef = workingmode.PackPrefix + vibeOwnerDebateFlowID
	rs.activeFlowNodes = vibeOwnerDebateNodes()
	rs.vibeParkedNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	rs.activeHubNodeID = "synthesis" // stale sprint hub pointer, live shape
	rs.hubLastProgressAt = time.Now().UTC().Add(-3 * time.Minute)
	rs.stallTimeout = 2 * time.Minute
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, vibeOwnerDebateNodes())
	return svc, parent.RunID
}

// Live run-3362: the watchdog converted the mid-mount silence into hub_stalled
// while owner_1/owner_2 spawn was in flight — the refusal left a dead debate
// that escalated/re-parked forever. A mounted debate IS the remediation in
// flight; the shared loop must not be stall-blocked under it.
func TestCA1088_StallWatchdogNeverBlocksMountedDebate(t *testing.T) {
	svc, runID := seedVibeDebateStarvedRun(t)
	if svc.checkAndBlockStalledHub(runID) {
		st := svc.agentOrchestrator.loopStateFor(runID)
		t.Fatalf("hub_stalled blocked a mounted owner debate: %+v", st)
	}
	if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "blocked" {
		t.Fatalf("loop blocked under mounted debate: %+v", st)
	}
}

// Same guard, different shape: the mount is still dispatching
// (debate_trigger RUNNING) so the settle helper must skip it — the watchdog
// guard alone owns the no-block contract here.
func TestCA1088_StallWatchdogSkipsInFlightDebateMount(t *testing.T) {
	svc, runID := seedVibeDebateStarvedRun(t)
	svc.setFlowStepStatus(context.Background(), runID, "debate_trigger", StepStatusRunning)
	if svc.checkAndBlockStalledHub(runID) {
		st := svc.agentOrchestrator.loopStateFor(runID)
		t.Fatalf("hub_stalled blocked a mid-mount debate: %+v", st)
	}
}

// The starved-mount shape (owners PENDING, zero owner children) is the same
// wedge class as both-owners-failed: the settle must retry the debate through
// the existing retry/cap ladder instead of returning false and letting the
// watchdog park it.
func TestCA1088_StarvedDebateRetriesNotStalls(t *testing.T) {
	svc, runID := seedVibeDebateStarvedRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status: "blocked", BlockReason: "hub_stalled", Cap: 5, RoundCap: 5,
	})
	if !svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("starved owner mount must retry the debate, not stall")
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status == "blocked" && st.BlockReason == "hub_stalled" {
		t.Fatalf("starved retry must clear hub_stalled: %+v", st)
	}
}

// stashVibeFlowForDebate must drop the sprint's hub pointer: a stale
// activeHubNodeID resolved post-mount reinvokes onto sprint `synthesis` —
// stamping RUNNING transitions and feeding the escalate loop while the debate
// topology was active (live run-3362 synthesis reinvoke churn).
func TestCA1088_StashClearsStaleHubPointer(t *testing.T) {
	svc, runID := seedVibeDebateStarvedRun(t)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.vibeParkedNodes = nil // force the real stash path (not the early return)
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	rs.chatFlowRef = workingmode.PackPrefix + "vibe-sprint"
	rs.vibeParkedNodes = nil
	svc.mu.Unlock()
	svc.stashVibeFlowForDebate(runID, "")
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if got := svc.runs[runID].activeHubNodeID; got != "" {
		t.Fatalf("activeHubNodeID=%q survived debate stash; stale hub outcomes resolve on the wrong topology", got)
	}
}

// The settle must not double-restart while the mount is still dispatching:
// debate_trigger RUNNING with owner steps PENDING is in-flight work, not a
// starved mount.
func TestCA1088_SettleSkipsInFlightDebateMount(t *testing.T) {
	svc, runID := seedVibeDebateStarvedRun(t)
	svc.setFlowStepStatus(context.Background(), runID, "debate_trigger", StepStatusRunning)
	if svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("in-flight debate mount must not be treated as starved")
	}
}
