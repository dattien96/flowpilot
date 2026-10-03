package runner

import (
	"testing"

	"flowpilot-runner/internal/workingmode"
)

// BUG-595 (live run-100368): gate-triggered owner-debate mounts have no
// bound. The run mounted the debate SEVEN times on the same gated child
// over ~8h — each debate concluded, the reprompted child failed the gate
// again, and a fresh debate re-parked the sprint. The debate lifecycle
// itself worked; the missing bound turned a remediation mechanism into an
// unbounded loop. Contract §3: retry loops are capped; hitting the cap
// escalates with a structured status — never an unbounded reprompt.

// bug595SeedSprintRun seeds a live vibe run on the sprint topology (no
// debate claim).
func bug595SeedSprintRun(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	runID := "run-bug595"
	rs := &interactiveRun{
		id:               runID,
		projectID:        "proj",
		status:           RunStatusRunning,
		agentStatus:      string(RunStatusRunning),
		workingMode:      workingmode.Vibe,
		chatFlowRef:      workingmode.PackPrefix + vibeSprintFlowID,
		autoOrchestrate:  true,
		flowEngineDriven: true,
		activeFlowNodes:  bug567SprintGraph(),
		vibeTaskPlan:     []string{"task-a"},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()
	return svc, runID
}

// At the mount cap the next divert escalates instead of mounting an Nth
// debate: the loop parks blocked/cap with the violation carried in the
// gate reason, and no topology is stashed.
func TestBUG595DebateMountCapEscalates(t *testing.T) {
	svc, runID := bug595SeedSprintRun(t)
	svc.mu.Lock()
	svc.runs[runID].vibeDebateMounts = maxVibeDebateMountsPerSprint
	svc.mu.Unlock()

	svc.startVibeOwnerDebate(runID, "run-gated-coder", "gate block: r-tests")

	loop := svc.agentOrchestrator.loopStateFor(runID)
	// The cap stamp is BlockReason=cap; the escalate surface is either the
	// tournament rescue (flag on, always-on in tests) or the legacy park.
	if loop.BlockReason != "cap" ||
		(loop.Status != "blocked" && loop.Status != LoopStatusTournamentEscalation) {
		t.Fatalf("cap-exceeded divert did not escalate: loop=%+v", loop)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if len(rs.vibeParkedNodes) != 0 {
		t.Fatalf("cap-exceeded divert still stashed topology: %d parked nodes", len(rs.vibeParkedNodes))
	}
	if workingmode.BareFlowID(rs.chatFlowRef) == vibeOwnerDebateFlowID {
		t.Fatal("cap-exceeded divert mounted another debate")
	}
}

// Below the cap a divert mounts and counts; the count resets when the next
// sprint's task is taken (fresh remediation budget per sprint).
func TestBUG595DebateMountCountIncrementsAndResets(t *testing.T) {
	svc, runID := bug595SeedSprintRun(t)

	if !svc.stashVibeFlowForDebate(runID, "run-gated-coder") {
		t.Fatal("stash refused a fresh park")
	}
	svc.mu.Lock()
	mounts := svc.runs[runID].vibeDebateMounts
	// Simulate the mounted-then-restored lifecycle so the next take is legal.
	rs := svc.runs[runID]
	rs.vibeParkedNodes = nil
	rs.vibeParkedFlowRef = ""
	svc.mu.Unlock()
	if mounts != 1 {
		t.Fatalf("vibeDebateMounts=%d want 1 after a fresh park", mounts)
	}

	svc.mu.Lock()
	d := svc.takeNextVibeSprintLocked(svc.runs[runID])
	svc.mu.Unlock()
	if !d.Start {
		t.Fatal("takeNextVibeSprintLocked did not start the next sprint")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[runID].vibeDebateMounts != 0 {
		t.Fatalf("vibeDebateMounts=%d want 0 after the next sprint take", svc.runs[runID].vibeDebateMounts)
	}
}
