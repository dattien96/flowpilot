package runner

import (
	"testing"
)

// CA-1151 (vibe spec-alignment gate): a green suite is not proof of done —
// the TDD leg may have written tests that assert the wrong thing versus the
// requirement chain. The spec_align node re-derives requirements from the
// SS/SD/CP/Task docs and its cohort verdict gates synthesis done, exactly
// like the reviewer's: a misaligned or missing verdict blocks the sprint.
//
// These tests pin the pack wiring (topology + cohort membership), not the
// leg's prose quality.

func TestCA1151_SpecAlignNodeDeclaredInVibeSprint(t *testing.T) {
	def := loadVibeSprintDefForTest(t)

	node, ok := findFlowNode(def.Nodes, "spec_align")
	if !ok {
		t.Fatal("spec_align node missing from vibe-sprint")
	}
	if node.Cohort != "review" {
		t.Fatalf("spec_align cohort=%q want review — its verdict must join the synthesis done gate", node.Cohort)
	}
	if node.Posture != "read_only" {
		t.Fatalf("spec_align posture=%q want read_only — a verifier must not mutate the tree", node.Posture)
	}
	if got := node.Behavior; got != "agent.delegate" {
		t.Fatalf("spec_align behavior=%q want agent.delegate", got)
	}
	if node.Agent != "agents/spec-aligner.md" || node.PromptTemplate != "prompts/spec-align-check.md" {
		t.Fatalf("spec_align agent=%q prompt=%q want spec-aligner/spec-align-check", node.Agent, node.PromptTemplate)
	}

	hasEdge := func(from, to, when string) bool {
		for _, e := range def.Edges {
			if e.From == from && e.To == to && e.When == when {
				return true
			}
		}
		return false
	}
	// Cohort members settle into the cohort join, not a per-node done edge —
	// so validate fans both review verifiers out in parallel and their
	// verdicts are what gate synthesis. No serial spec_align->reviewer edge.
	if !hasEdge("validate", "spec_align", "done") {
		t.Fatal("edge validate->spec_align (done) missing")
	}
	if !hasEdge("validate", "reviewer", "done") {
		t.Fatal("edge validate->reviewer (done) missing — reviewer fans out in parallel")
	}
	if hasEdge("spec_align", "reviewer", "done") || hasEdge("spec_align", "synthesis", "done") {
		t.Fatal("spec_align must not own a done edge — its verdict reaches synthesis through the cohort join")
	}
}

// The user's contract: pass = tests green AND aligned with the spec chain.
// A missing spec_align verdict must block synthesis done even when the
// reviewer's verdict is approved.
func TestCA1151_SpecAlignVerdictGatesSynthesisDone(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armVibeSprintRun(t, svc)

	// Reviewer approved but spec_align verdict absent → done must stay blocked.
	svc.snapshotReviewCohortVerdicts(runID, []cohortEntry{
		{Label: "reviewer", Status: "completed", MachineVerdict: "approved"},
	})
	err := svc.synthesisDoneVerdictError(runID)
	if err == nil {
		t.Fatal("synthesis done must be blocked while spec_align verdict is missing")
	}

	// spec_align changes_requested (test drifted from spec) → still blocked.
	svc.snapshotReviewCohortVerdicts(runID, []cohortEntry{
		{Label: "reviewer", Status: "completed", MachineVerdict: "approved"},
		{Label: "spec_align", Status: "completed", MachineVerdict: "changes_requested"},
	})
	if err := svc.synthesisDoneVerdictError(runID); err == nil {
		t.Fatal("synthesis done must be blocked while spec_align reports drift")
	}

	// Both approved → unblocked.
	svc.snapshotReviewCohortVerdicts(runID, []cohortEntry{
		{Label: "reviewer", Status: "completed", MachineVerdict: "approved"},
		{Label: "spec_align", Status: "completed", MachineVerdict: "approved"},
	})
	if err := svc.synthesisDoneVerdictError(runID); err != nil {
		t.Fatalf("synthesis done must pass with both verdicts approved: %v", err)
	}
}
