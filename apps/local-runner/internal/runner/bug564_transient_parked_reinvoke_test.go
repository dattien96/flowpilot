package runner

// BUG-564 — live run-69320 (PrivateVault Task-024 sprint, 2026-10-02
// 13:55–13:58): a cohort join armed pendingHubReinvoke; startTurn rejected
// with hub_parked (transient — children still settling). The transient reject
// burned hubReinvokeStartFailCount and the +25ms refire looped 5 times inside
// ~1s, exhausting the <=3 drain budget while the busy window could not
// possibly clear. The armed pending then outlived the settle with no drain —
// the stall watchdog parked the run 3m later even though the reinvoke was
// owed. Worse, when a late drain finally fired at 15:05 the re-driven hub
// read stale state and dispatched a duplicate coder fix-leg racing the live
// reviewer.
//
// Contract:
//  a) hub_parked is not a startTurn failure — it must not burn
//     hubReinvokeStartFailCount nor spin the +25ms refire; the CA-1091
//     child-settle drain and the stall tick own the re-fire.
//  b) the stall tick drains an armed pendingHubReinvoke on ANY flow (not only
//     while a debate is mounted) — progress owed beats park — bounded by the
//     fail counter so a permanently-rejected reinvoke still converges to
//     hub_stalled.

import (
	"sync/atomic"
	"testing"
	"time"
)

// (a) A hub_parked reject is transient settle-window busy: keep pending armed,
// leave the fail counter at 0, and do not tight-loop notifyTurnIdle.
func TestBug564_HubParkedStartFailDoesNotBurnRetryBudget(t *testing.T) {
	var sendCount int32
	var cap atomic.Value
	svc := newInteractiveService(bug308FullAFRegistry(ProviderKeyCodex, &cap, &sendCount), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.SetDispatchStore(NewMemoryDispatchStore())

	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.reinvokeInFlight = true // scheduler armed this before dispatch
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, Mode: "explicit", Round: 1})

	// A live flow child forces shouldParkHubWriteTurn → hub_parked.
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun child: %v", err)
	}
	svc.mu.Lock()
	ch := svc.runs[child.RunID]
	ch.parentRunID = parent.RunID
	ch.label = "owner_1"
	ch.status = RunStatusRunning
	ch.agentStatus = string(RunStatusRunning)
	ch.turnInFlight = true
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)

	svc.scheduleChildTurn(parent.RunID, "step-reinvoke", "synthesize joined results")

	waitLoop(t, "hub_parked reject processed", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !rs.reinvokeInFlight && rs.pendingHubReinvoke
	})

	svc.mu.Lock()
	fails := rs.hubReinvokeStartFailCount
	svc.mu.Unlock()
	if fails != 0 {
		t.Fatalf("hub_parked burned the consecutive-fail counter: hubReinvokeStartFailCount=%d want 0 (transient busy is not a failure)", fails)
	}
	// No immediate refire: the busy window cannot clear in 25ms — the +25ms
	// drain stormed the counter in the live run. Settle/watchdog drains own it.
	time.Sleep(200 * time.Millisecond)
	svc.mu.Lock()
	stillArmed := rs.pendingHubReinvoke
	fails = rs.hubReinvokeStartFailCount
	svc.mu.Unlock()
	if !stillArmed {
		t.Fatal("pendingHubReinvoke was drained while children still settling — the owed hub turn would fire into hub_parked again")
	}
	if fails != 0 {
		t.Fatalf("transient refire loop still burning the counter: fails=%d", fails)
	}
	if atomic.LoadInt32(&sendCount) != 0 {
		t.Fatalf("provider adapter was invoked while hub parked — sends=%d", sendCount)
	}
}

// (a continued) When the last child settles, the armed pending drains and the
// hub turn actually dispatches — no watchdog park needed.
func TestBug564_ArmedReinvokeFiresAfterTransientSettle(t *testing.T) {
	var sendCount int32
	var cap atomic.Value
	svc := newInteractiveService(bug308FullAFRegistry(ProviderKeyCodex, &cap, &sendCount), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.SetDispatchStore(NewMemoryDispatchStore())

	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.reinvokeInFlight = true
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
	ch.status = RunStatusRunning
	ch.agentStatus = string(RunStatusRunning)
	ch.turnInFlight = true
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, child.RunID)

	// Transient reject arms the pending reinvoke.
	svc.scheduleChildTurn(parent.RunID, "step-reinvoke", "synthesize joined results")
	waitLoop(t, "pending armed after hub_parked", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !rs.reinvokeInFlight && rs.pendingHubReinvoke
	})

	// Child settles — the CA-1091 settle drain must fire the owed turn.
	svc.mu.Lock()
	ch.status = RunStatusCompleted
	ch.agentStatus = string(RunStatusCompleted)
	ch.turnInFlight = false
	svc.mu.Unlock()
	svc.notifyTurnIdle(child.RunID)

	waitLoop(t, "settle drain dispatches owed hub turn", 3*time.Second, func() bool {
		return atomic.LoadInt32(&sendCount) > 0
	})
}

// (b) The stall tick drains an armed pendingHubReinvoke on a plain sprint flow
// (no debate mounted) instead of parking — live run-69320 parked hub_stalled
// at 13:58:49 while the synthesis reinvoke sat armed.
func TestBug564_StallTickDrainsArmedPendingNoDebate(t *testing.T) {
	var sendCount int32
	var cap atomic.Value
	svc := newInteractiveService(bug308FullAFRegistry(ProviderKeyCodex, &cap, &sendCount), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.SetDispatchStore(NewMemoryDispatchStore())

	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.pendingHubReinvoke = true
	rs.pendingHubReinvokePrompt = "synthesize the joined owner results"
	rs.hubLastProgressAt = time.Now().UTC().Add(-3 * time.Minute)
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, Mode: "explicit", Round: 1})

	if svc.checkAndBlockStalledHub(parent.RunID) {
		t.Fatal("watchdog parked a run whose only gap was an undrained armed pendingHubReinvoke (no debate mounted)")
	}
	waitLoop(t, "watchdog-drained reinvoke reaches adapter", 3*time.Second, func() bool {
		return atomic.LoadInt32(&sendCount) > 0
	})
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID).Status; st == "blocked" {
		t.Fatalf("loop blocked=%q after drain — must stay running", st)
	}
}

// (b-bounded) A permanently-failing reinvoke (drain budget already spent)
// still parks — F-0's stranded-pending contract is preserved.
func TestBug564_StallTickParksAfterDrainBudgetSpent(t *testing.T) {
	var sendCount int32
	var cap atomic.Value
	svc := newInteractiveService(bug308FullAFRegistry(ProviderKeyCodex, &cap, &sendCount), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.SetDispatchStore(NewMemoryDispatchStore())

	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.pendingHubReinvoke = true
	rs.hubReinvokeStartFailCount = 4 // real (non-transient) rejects already exhausted the drain budget
	rs.hubLastProgressAt = time.Now().UTC().Add(-3 * time.Minute)
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, Mode: "explicit", Round: 1})

	if !svc.checkAndBlockStalledHub(parent.RunID) {
		t.Fatal("drain budget exhausted + armed pending + nothing live must still surface hub_stalled")
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Status != "blocked" || st.BlockReason != "hub_stalled" {
		t.Fatalf("loop = %+v, want blocked/hub_stalled", st)
	}
}
