package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/workingmode"
)

// BUG-659 (live run-523131, sprint-4): Resume OK at tdd abandoned BOTH
// active frozen contracts ("vibe-sprint resume at tdd") but the
// preflight_contract_freeze node is lifecycle:once and already DONE — the
// resume enters at tdd and never re-freezes. Every downstream writer then
// hard-blocked on "no frozen contract found" until the operator hand-
// appended reactivation rows.

// Resume at tdd keeps the sprint's frozen contracts — the same task's scope
// still governs. The abandon-then-refreeze shape belongs ONLY to the
// new-sprint path (maybeStartNextVibeSprint), which re-runs the freeze node.
func TestBug659_ResumeAtTddKeepsFrozenContract(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)

	freezeP4Contract(t, dir, runID, "coder", head, []string{"src/calc.go"})

	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.workingMode = workingmode.Vibe
	rs.vibeTaskPlan = []string{"requirements/08-Task/todo/Task-910.md"}
	rs.vibeSprintIndex = 1
	svc.mu.Unlock()

	svc.forceStartVibeSprintAtTdd(runID)

	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.GetFrozenForStep(runID, "coder"); !ok {
		t.Fatal("resume-at-tdd abandoned the frozen contract with no re-freeze path — the permanent gate wall")
	}
}

// When contracts exist for the step but none are active (abandoned/
// superseded), the gate must say so — "no frozen contract found" told the
// operator nothing about WHY and offered no remedy.
func TestBug659_AbandonWithoutRefreezeHonestGateMessage(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	rec := freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})

	// Abandon the only contract for the step — the post-resume wreck shape.
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus(rec.ContractID, changecontract.ContractStatusAbandoned, "vibe-sprint resume at tdd", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	rs := newP4ChildRun(svc, "child-659", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("expected block: the only contract for this step is abandoned")
	}

	svc.mu.Lock()
	var msg string
	for _, ev := range rs.events {
		if ev.Type == EventFlowGateViolation {
			msg = ev.Error
		}
	}
	svc.mu.Unlock()
	if !strings.Contains(msg, "abandoned") {
		t.Fatalf("gate message must say the contract was ABANDONED (resume without re-freeze), got %q", msg)
	}
	if strings.Contains(msg, "no frozen contract found") {
		t.Fatalf("bare 'no frozen contract found' hides the real defect — want the abandoned/re-freeze phrasing, got %q", msg)
	}
}
