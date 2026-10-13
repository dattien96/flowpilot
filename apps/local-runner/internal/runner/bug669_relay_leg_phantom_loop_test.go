package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-669 (live run-816045/run-816751): a reattach leg inherits
// flowArm=started + flowEngineDriven + chatFlowRef for launch-dedup but
// carries NO activeFlowNodes — only a leg that actually mounted the flow
// holds topology. Every parentRunID=="" && flowEngineDriven consumer then
// treats the leg as a loop owner: a zero-value loop materializes on its id,
// the stall watchdog parks it, and admission rejects every relayed turn
// with flow_awaiting_user — the relay target dies on arrival.
//
// Fix contract: relay-leg checks resolve to the launcher leg's loop; a
// relay leg's own phantom loop must never gate its turns or be parked.
func mkFlowLeg(id, chatID string, seq int, driven bool, nodes int) *interactiveRun {
	rs := &interactiveRun{
		id:               id,
		runKind:          "chat",
		chatID:           chatID,
		legSeq:           seq,
		legState:         LegStateActive,
		flowEngineDriven: driven,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	for i := 0; i < nodes; i++ {
		rs.activeFlowNodes = append(rs.activeFlowNodes, agentpack.FlowNode{ID: "n"})
	}
	return rs
}

func TestBug669_RelayLegAdmissionConsultsLauncherLoop(t *testing.T) {
	svc := bug289Service(t)
	chatID := "cht-669"

	launcher := mkFlowLeg("leg-669-launcher", chatID, 0, true, 3)
	relay := mkFlowLeg("leg-669-relay", chatID, 1, true, 0)
	svc.mu.Lock()
	svc.runs[launcher.id] = launcher
	svc.runs[relay.id] = relay
	// The phantom loop that formed on the relay leg live.
	svc.agentOrchestrator.mutateLoop(relay.id, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		return st
	})
	// The real flow loop on the launcher is running.
	svc.agentOrchestrator.mutateLoop(launcher.id, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})
	got := svc.flowAdmissionLoopStatusLocked(relay)
	svc.mu.Unlock()

	if got != "running" {
		t.Fatalf("admission consulted %q loop — relay leg must see the launcher's running loop, not its own parked phantom", got)
	}

	// And when the REAL loop is blocked, the relay leg must still refuse —
	// the gate is honest, it just consults the right owner.
	svc.agentOrchestrator.mutateLoop(launcher.id, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		return st
	})
	svc.mu.Lock()
	got = svc.flowAdmissionLoopStatusLocked(relay)
	svc.mu.Unlock()
	if got != "blocked" {
		t.Fatalf("admission = %q — a genuinely-blocked launcher loop must still gate the relay leg", got)
	}
}

// The stall watchdog must never park a loop on the relay leg's id — the
// phantom is how the deadlock formed live.
func TestBug669_StallWatchNeverParksRelayLeg(t *testing.T) {
	svc := bug289Service(t)
	chatID := "cht-669b"

	relay := mkFlowLeg("leg-669b-relay", chatID, 1, true, 0)
	svc.mu.Lock()
	svc.runs[relay.id] = relay
	svc.mu.Unlock()
	// No launcher resident → nothing to stall; the leg must come out untouched.
	if svc.checkAndBlockStalledHub(relay.id) {
		t.Fatal("watchdog stalled a relay leg with no launcher — phantom park")
	}
	if st := svc.agentOrchestrator.loopStateFor(relay.id).Status; st == "blocked" {
		t.Fatal("watchdog materialized and parked a loop on the relay leg")
	}
}
