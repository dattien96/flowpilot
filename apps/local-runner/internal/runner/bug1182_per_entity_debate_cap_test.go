package runner

import (
	"fmt"
	"testing"
)

// BUG-1182 — the owner-debate mount budget was a single per-sprint counter:
// three debates resolving member A's gate violations consumed the whole
// sprint budget, so the FIRST gate block on member B hit the mount cap and
// escalated instead of remediating. The cap exists to stop sequential
// debates on the SAME gated entity (BUG-595, live run-100368) — mounts must
// be accounted per gated entity (its node label, stable across leg
// respawns), while a sprint-wide ceiling still bounds total mounts.

func bug1182GatedChild(t *testing.T, svc *InteractiveService, parentID, childID, label string) {
	t.Helper()
	svc.mu.Lock()
	svc.runs[childID] = &interactiveRun{
		id:          childID,
		parentRunID: parentID,
		label:       label,
		status:      RunStatusRunning,
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
}

// bug1182Stash mounts + restores once so successive mounts stay legal.
func bug1182Stash(t *testing.T, svc *InteractiveService, runID, gatedID string) {
	t.Helper()
	if !svc.stashVibeFlowForDebate(runID, gatedID) {
		t.Fatalf("stash for %s refused a fresh park", gatedID)
	}
	svc.mu.Lock()
	svc.runs[runID].vibeParkedNodes = nil // simulate mounted-then-restored
	svc.mu.Unlock()
}

// (a) Mounts on different entities each get their own budget; the same
// entity still caps at maxVibeDebateMountsPerSprint.
func TestBug1182_DebatesOnDifferentEntitiesDoNotShareBudget(t *testing.T) {
	svc, runID := bug595SeedSprintRun(t)
	bug1182GatedChild(t, svc, runID, "run-gated-a", "coder")
	bug1182GatedChild(t, svc, runID, "run-gated-b", "reviewer")

	for i := 0; i < maxVibeDebateMountsPerSprint; i++ {
		bug1182Stash(t, svc, runID, "run-gated-a")
	}

	// Fourth mount on the SAME entity → cap.
	svc.startVibeOwnerDebate(runID, "run-gated-a", "gate block: r-tests")
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.BlockReason != "cap" {
		t.Fatalf("4th mount on the same entity must cap, loop=%+v", loop)
	}

	// First mount on a different entity on a fresh run → still mounts.
	svc2, runID2 := bug595SeedSprintRun(t)
	bug1182GatedChild(t, svc2, runID2, "run-gated-a2", "coder")
	bug1182GatedChild(t, svc2, runID2, "run-gated-b2", "reviewer")
	for i := 0; i < maxVibeDebateMountsPerSprint; i++ {
		bug1182Stash(t, svc2, runID2, "run-gated-a2")
	}
	if !svc2.stashVibeFlowForDebate(runID2, "run-gated-b2") {
		t.Fatal("first mount on a different gated entity must not hit the cap")
	}
}

// (b) The sprint reset clears the per-entity ledger too.
func TestBug1182_SprintResetClearsEntityBudget(t *testing.T) {
	svc, runID := bug595SeedSprintRun(t)
	bug1182GatedChild(t, svc, runID, "run-gated-a", "coder")

	bug1182Stash(t, svc, runID, "run-gated-a")
	svc.mu.Lock()
	rs := svc.runs[runID]
	if len(rs.vibeDebateMountsByEntity) == 0 {
		svc.mu.Unlock()
		t.Fatal("per-entity mount ledger not recorded")
	}
	d := svc.takeNextVibeSprintLocked(rs)
	svc.mu.Unlock()
	if !d.Start {
		t.Fatal("takeNextVibeSprintLocked did not start the next sprint")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.runs[runID].vibeDebateMountsByEntity) != 0 {
		t.Fatalf("per-entity ledger must reset on sprint take, got %v", svc.runs[runID].vibeDebateMountsByEntity)
	}
	if svc.runs[runID].vibeDebateMounts != 0 {
		t.Fatalf("total mounts must reset on sprint take, got %d", svc.runs[runID].vibeDebateMounts)
	}
}

// (c) A sprint-wide ceiling still bounds total mounts across distinct
// entities — per-entity accounting must not make the debate loop
// unbounded in aggregate.
func TestBug1182_SprintCeilingBoundsTotalMounts(t *testing.T) {
	svc, runID := bug595SeedSprintRun(t)
	for i := 0; i < maxVibeDebateMountsSprintCeiling; i++ {
		childID := fmt.Sprintf("run-gated-x%d", i)
		bug1182GatedChild(t, svc, runID, childID, fmt.Sprintf("member-%d", i))
		bug1182Stash(t, svc, runID, childID)
	}
	svc.startVibeOwnerDebate(runID, "run-gated-y", "gate block")
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.BlockReason != "cap" && loop.Status != LoopStatusTournamentEscalation {
		t.Fatalf("sprint ceiling must still escalate, loop=%+v", loop)
	}
}
