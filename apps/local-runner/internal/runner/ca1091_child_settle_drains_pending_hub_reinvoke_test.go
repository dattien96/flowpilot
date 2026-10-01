package runner

// CA-1091 — live run-3362 (vibe-tasks, PrivateVault, 2026-10-01 ~19:47):
// debate_synthesis armed a hub reinvoke via cohort join; the first startTurn
// hit hub_parked (owners still settling) → re-armed pendingHubReinvoke. When
// the LAST child went terminal, notifyTurnIdle(child) only ran
// flushDurableTurnIntents(parent) — which has no pendingHubReinvoke leg — so
// the armed reinvoke was stranded forever: loop "running", debate_synthesis
// RUNNING, zero hub turns for ~10min. The stall watchdog could not surface it
// either: CA-1088's debateMounted guard re-arms during a mounted debate, and
// pendingHubReinvoke is deliberately NOT busy, so the tick never blocks nor
// drains. Silent wedge until operator surgery.
//
// Contract: an armed-but-undrained pendingHubReinvoke is progress OWED, not a
// stall. Every settle surface must re-fire it — (a) last-child-settle, and
// (b) the F-0 stall tick when nothing is live.

import (
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// setupArmedHubReinvoke builds a flow parent with a stranded pendingHubReinvoke
// (the shape hub_reinvoke_start_failed leaves behind) plus one settled child.
func setupArmedHubReinvoke(t *testing.T) (svc *InteractiveService, parentID, childID string, sends *int32) {
	t.Helper()
	var sendCount int32
	var cap atomic.Value
	svc = newInteractiveService(bug308FullAFRegistry(ProviderKeyCodex, &cap, &sendCount), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.SetDispatchStore(NewMemoryDispatchStore())

	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	if rs.runKind == "" {
		rs.runKind = "chat"
	}
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "debate_trigger", Behavior: "hub.inline"},
		{ID: "owner_1"},
		{ID: "owner_2"},
		{ID: "debate_synthesis", Behavior: "hub.inline"},
	}
	// The armed reinvoke is owed the debate_synthesis join turn.
	rs.activeHubNodeID = "debate_synthesis"
	rs.pendingHubReinvoke = true
	rs.pendingHubReinvokePrompt = "synthesize the joined owner results"
	rs.hubLastProgressAt = time.Now().UTC().Add(-3 * time.Minute)
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, Mode: "explicit", Round: 1})

	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun child: %v", err)
	}
	svc.mu.Lock()
	ch := svc.runs[child.RunID]
	ch.parentRunID = parent.RunID
	ch.label = "owner_1"
	ch.status = RunStatusCompleted
	ch.agentStatus = string(RunStatusCompleted)
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)
	return svc, parent.RunID, child.RunID, &sendCount
}

func pendingHubReinvokeArmed(svc *InteractiveService, runID string) bool {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	return rs != nil && rs.pendingHubReinvoke
}

// (a) Last-child-settle must drain the parent's armed hub reinvoke. Before the
// fix the parent branch only flushed gate-reprompt/resume intents, leaving the
// armed reinvoke stranded — the exact 19:46:57→19:57:26 silent park.
func TestCA1091_LastChildSettleDrainsParentPendingHubReinvoke(t *testing.T) {
	svc, parentID, childID, sends := setupArmedHubReinvoke(t)
	sendsBefore := atomic.LoadInt32(sends)

	svc.notifyTurnIdle(childID)

	waitLoop(t, "parent pendingHubReinvoke drains to adapter", 3*time.Second, func() bool {
		return atomic.LoadInt32(sends) > sendsBefore
	})
	if pendingHubReinvokeArmed(svc, parentID) {
		t.Fatal("pendingHubReinvoke still armed after last child settled — stranded wedge (run-3362)")
	}
}

// (b) The F-0 stall tick must drain an armed pendingHubReinvoke instead of
// blocking hub_stalled (or re-arming forever under a mounted debate) while the
// owed hub turn never fires.
func TestCA1091_StallTickDrainsPendingHubReinvoke(t *testing.T) {
	svc, parentID, _, sends := setupArmedHubReinvoke(t)
	sendsBefore := atomic.LoadInt32(sends)

	if svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("watchdog hub_stalled-blocked a run whose only gap was an undrained pendingHubReinvoke")
	}
	waitLoop(t, "watchdog-drained reinvoke reaches adapter", 3*time.Second, func() bool {
		return atomic.LoadInt32(sends) > sendsBefore
	})
	if pendingHubReinvokeArmed(svc, parentID) {
		t.Fatal("pendingHubReinvoke still armed after stall tick drain")
	}
	if st := svc.agentOrchestrator.loopStateFor(parentID).Status; st == "blocked" {
		t.Fatalf("loop blocked=%q after drain — must stay running", st)
	}
}

// Guard: an armed reinvoke must NOT drain while flow children are still live —
// hub_parked is the real blocker there and the drain belongs to their settle.
func TestCA1091_NoDrainWhileFlowChildActive(t *testing.T) {
	svc, parentID, childID, sends := setupArmedHubReinvoke(t)
	svc.mu.Lock()
	svc.runs[childID].status = RunStatusRunning
	svc.runs[childID].agentStatus = string(RunStatusRunning)
	svc.runs[childID].turnInFlight = true // live provider turn — real activity
	svc.mu.Unlock()

	sendsBefore := atomic.LoadInt32(sends)
	if svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("hub_stalled while a live child turn is running")
	}
	time.Sleep(150 * time.Millisecond)
	if atomic.LoadInt32(sends) > sendsBefore {
		t.Fatal("armed reinvoke drained while a flow child was still active — must stay parked for settle")
	}
	if !pendingHubReinvokeArmed(svc, parentID) {
		t.Fatal("pendingHubReinvoke dropped while children active — the owed turn would be lost")
	}
}
