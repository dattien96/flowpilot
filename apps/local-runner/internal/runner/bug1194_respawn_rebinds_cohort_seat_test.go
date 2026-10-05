package runner

import (
	"context"
	"testing"
)

// BUG-1194 (live run-225691 / replacement leg run-260972): a member
// re-dispatch that reaches spawnChildRun WITHOUT an explicit cohort id —
// the member_stalled Continue respawn, the failed-delegate fresh spawn, or
// a hub ad-hoc spawn_agent — must still fill the seat an OPEN cohort
// barrier is holding for its label. Without the rebind the replacement
// leg's completion can never join (the settle keys off flowCohortId): the
// live barrier sat at 1/2 while owner_1's replacement ran and settled
// cleanly, so debate_synthesis never re-invoked and the member_stalled →
// respawn loop fired every round instead.
func TestBug1194_RespawnRebindsOpenCohortSeat(t *testing.T) {
	svc, parentID := clusterFService(t)
	cohortID := "flow-auto-debate_trigger-round-4"
	svc.agentOrchestrator.preRegisterCohort(parentID, cohortID, 2)

	// Prior owner_1 leg: cohort-bound, terminal (the live leg's settle was
	// swallowed, so no buffered entry — the seat for label owner_1 is still
	// held open). owner_2 already reported: the barrier waits at 1/2.
	prior, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun prior leg: %v", apiErr)
	}
	svc.mu.Lock()
	p := svc.runs[prior.RunID]
	p.parentRunID = parentID
	p.label = "owner_1"
	p.flowCohortId = cohortID
	p.status = RunStatusCompleted
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, prior.RunID)
	svc.agentOrchestrator.appendCohortResult(parentID, cohortID, cohortEntry{
		Label: "owner_2", Provider: "devin", Status: "completed",
	})

	// The live spawn shape: no cohort fields — exactly what the member_stalled
	// Continue respawn and the hub's ad-hoc spawn_agent produce.
	res, err := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
		Agent: "owner", Prompt: "retry the owner leg", Label: "owner_1", Wait: false,
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}

	svc.mu.Lock()
	replacement := svc.runs[res.RunID]
	svc.mu.Unlock()
	if replacement == nil {
		t.Fatal("replacement run missing")
	}
	if replacement.flowCohortId != cohortID {
		t.Fatalf("respawned member lost the cohort seat: flowCohortId=%q, want %q — its completion can never join the open barrier",
			replacement.flowCohortId, cohortID)
	}
	if got := svc.agentOrchestrator.cohortExpectedCount(parentID, cohortID); got != 2 {
		t.Fatalf("cohort expected=%d, want 2 — the seat must be inherited, not grown", got)
	}

	// The replacement's own completion must close the open barrier.
	svc.agentOrchestrator.appendCohortResult(parentID, cohortID, cohortEntry{
		Label: replacement.label, Provider: "devin", Status: "completed",
	})
	if !svc.agentOrchestrator.cohortComplete(parentID, cohortID) {
		t.Fatal("barrier never completed — the rebound member must join the cohort it replaced")
	}
}

// The seat key is the EFFECTIVE label: a spawn carrying no Label at all
// falls back to its Agent name — the same fallback the prior leg's stored
// label went through — so a label-less re-dispatch still finds its seat.
func TestBug1194_RespawnAgentNameFallbackBindsSeat(t *testing.T) {
	svc, parentID := clusterFService(t)
	cohortID := "flow-auto-debate_trigger-round-4"
	svc.agentOrchestrator.preRegisterCohort(parentID, cohortID, 2)

	// Prior leg spawned label-less: its stored label is the agent name.
	prior, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun prior leg: %v", apiErr)
	}
	svc.mu.Lock()
	p := svc.runs[prior.RunID]
	p.parentRunID = parentID
	p.label = "owner"
	p.flowCohortId = cohortID
	p.status = RunStatusCompleted
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, prior.RunID)
	svc.agentOrchestrator.appendCohortResult(parentID, cohortID, cohortEntry{
		Label: "owner_2", Provider: "devin", Status: "completed",
	})

	res, err := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
		Agent: "owner", Prompt: "retry the owner leg", Wait: false,
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	svc.mu.Lock()
	replacement := svc.runs[res.RunID]
	svc.mu.Unlock()
	if replacement == nil {
		t.Fatal("replacement run missing")
	}
	if replacement.flowCohortId != cohortID {
		t.Fatalf("label-less respawn did not fall back to the agent name: flowCohortId=%q, want %q",
			replacement.flowCohortId, cohortID)
	}
	if got := svc.agentOrchestrator.cohortExpectedCount(parentID, cohortID); got != 2 {
		t.Fatalf("cohort expected=%d, want 2 — the seat must be inherited, not grown", got)
	}
}

// A spawn whose label holds NO open seat must stay unbound: a drained
// barrier is tombstoned, and binding a fresh spawn to it would either
// re-open a delivered cohort on a phantom member or drop its completion on
// a dead key. Explicit re-drives that intend a rejoin use
// rearmCohortIfDrainedLocked — a plain spawn must not.
func TestBug1194_SpawnWithoutHeldSeatStaysUnbound(t *testing.T) {
	svc, parentID := clusterFService(t)
	cohortID := "flow-auto-debate_trigger-round-4"
	svc.agentOrchestrator.preRegisterCohort(parentID, cohortID, 2)

	// Prior owner_1 leg reported already — its seat is CONSUMED, not held.
	prior, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun prior leg: %v", apiErr)
	}
	svc.mu.Lock()
	p := svc.runs[prior.RunID]
	p.parentRunID = parentID
	p.label = "owner_1"
	p.flowCohortId = cohortID
	p.status = RunStatusCompleted
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, prior.RunID)
	svc.agentOrchestrator.appendCohortResult(parentID, cohortID, cohortEntry{
		Label: "owner_1", Provider: "devin", Status: "completed",
	})

	res, err := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
		Agent: "owner", Prompt: "fresh owner leg", Label: "owner_1", Wait: false,
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	svc.mu.Lock()
	replacement := svc.runs[res.RunID]
	svc.mu.Unlock()
	if replacement == nil {
		t.Fatal("replacement run missing")
	}
	if replacement.flowCohortId != "" {
		t.Fatalf("spawn bound a consumed seat: flowCohortId=%q — a second entry for the same label is deduped away, so binding is a lie", replacement.flowCohortId)
	}
}

// A spawn whose label has nothing to do with the open barrier — no prior
// leg ever carried it — must not inherit a seat meant for a different
// member.
func TestBug1194_UnrelatedLabelSpawnStaysUnbound(t *testing.T) {
	svc, parentID := clusterFService(t)
	cohortID := "flow-auto-debate_trigger-round-4"
	svc.agentOrchestrator.preRegisterCohort(parentID, cohortID, 2)

	prior, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun prior leg: %v", apiErr)
	}
	svc.mu.Lock()
	p := svc.runs[prior.RunID]
	p.parentRunID = parentID
	p.label = "owner_1"
	p.flowCohortId = cohortID
	p.status = RunStatusCompleted
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, prior.RunID)
	svc.agentOrchestrator.appendCohortResult(parentID, cohortID, cohortEntry{
		Label: "owner_2", Provider: "devin", Status: "completed",
	})

	res, err := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
		Agent: "reviewer", Prompt: "unrelated review leg", Label: "reviewer_1", Wait: false,
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	svc.mu.Lock()
	replacement := svc.runs[res.RunID]
	svc.mu.Unlock()
	if replacement == nil {
		t.Fatal("replacement run missing")
	}
	if replacement.flowCohortId != "" {
		t.Fatalf("unrelated label stole owner_1's seat: flowCohortId=%q", replacement.flowCohortId)
	}
}
