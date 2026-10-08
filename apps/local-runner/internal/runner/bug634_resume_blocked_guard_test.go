package runner

import "testing"

// BUG-634 (live run-306526): agent-loop/resume unconditionally flipped a
// `blocked` loop to `running` and wiped GateReason while the durable
// decision-form park stayed armed — the run reported live while frozen, and
// agent-loop/amend (which requires loop blocked) became unreachable on the
// very drift park it exists to discharge. Resume may only flip `paused`.
func TestBug634_ResumeKeepsBlockedPark(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-634a"
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status:      "blocked",
		GateReason:  "scope drift detected — amend the contract or stop",
		BlockReason: "escalate",
		Cap:         3,
		RoundCap:    3,
	})

	svc.agentOrchestrator.resume(runID)

	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "blocked" {
		t.Fatalf("resume clobbered blocked park: status=%s", st.Status)
	}
	if st.GateReason == "" {
		t.Fatal("resume wiped GateReason of a decision-form park")
	}
}

// paused is the state resume exists for — it must still flip to running.
func TestBug634_ResumeStillUnpauses(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-634b"
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status:     "paused",
		GateReason: "operator pause",
		Cap:        3,
		RoundCap:   3,
	})

	svc.agentOrchestrator.resume(runID)

	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "running" {
		t.Fatalf("resume failed to unpause: status=%s", st.Status)
	}
	if st.GateReason != "" {
		t.Fatalf("resume kept stale pause reason: %q", st.GateReason)
	}
}

// `done` is terminal — never resumable. `stopped` IS resumable: the
// agent-loop/resume API on a stopped (or crash-stopped) run is the restart
// discharge path — resumePendingLoopWork drains durable intents behind it
// (CA-1090 contract). Only `blocked`, `done`, and `tournament_escalation`
// are preserved.
func TestBug634_ResumeNeverResurrectsTerminal(t *testing.T) {
	svc := bug289Service(t)
	svc.agentOrchestrator.setLoop("run-634c-done", AgentLoopState{Status: "done", Cap: 3, RoundCap: 3})
	svc.agentOrchestrator.resume("run-634c-done")
	if st := svc.agentOrchestrator.loopStateFor("run-634c-done"); st.Status != "done" {
		t.Fatalf("resume resurrected terminal loop done → %s", st.Status)
	}
}

// CA-1090 counterpart: a stopped loop resumes — otherwise the post-crash
// restart drain (pending restart intents, stop-fence release) is dead and
// every crash-stopped run wedges permanently.
func TestBug634_ResumeRevivesStopped(t *testing.T) {
	svc := bug289Service(t)
	svc.agentOrchestrator.setLoop("run-634d", AgentLoopState{Status: "stopped", GateReason: "stopped", Cap: 3, RoundCap: 3})
	svc.agentOrchestrator.resume("run-634d")
	if st := svc.agentOrchestrator.loopStateFor("run-634d"); st.Status != "running" {
		t.Fatalf("resume failed to revive stopped loop: status=%s — crash-stop restart drain dead", st.Status)
	}
}
