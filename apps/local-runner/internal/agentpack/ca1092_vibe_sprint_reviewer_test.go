package agentpack

import (
	"strings"
	"testing"
)

// CA-1092 (live run-3362 review): vibe-sprint ran 5 tasks with NO dedicated
// review step — coder -> validate -> synthesis -> audit meant the hub
// synthesized straight off validation output. task-harness/cp-harness both
// carry a read-only reviewer cohort between validate and synthesis; the vibe
// sprint must too. Review failure routes back to coder through the existing
// synthesis -> coder continue back-edge (round-capped).
func TestPack_CA1092_VibeSprintReviewerTopology(t *testing.T) {
	def := loadVibeSprint(t)

	rev := nodeByID(def, "reviewer")
	if rev.ID == "" {
		t.Fatal("vibe-sprint missing reviewer node — code lands unreviewed")
	}
	if rev.Run != "delegate" {
		t.Fatalf("reviewer run=%q, want delegate", rev.Run)
	}
	if canonical, ok := NormalizeBehaviorID(rev.Behavior); !ok || canonical != "agent.delegate" {
		t.Fatalf("reviewer behavior=%q, want agent.delegate", rev.Behavior)
	}
	if rev.Agent != "agents/reviewer.md" {
		t.Fatalf("reviewer agent=%q, want agents/reviewer.md", rev.Agent)
	}
	if !strings.Contains(rev.PromptTemplate, "review") {
		t.Fatalf("reviewer promptTemplate=%q, want a review prompt", rev.PromptTemplate)
	}
	if rev.Posture != "read_only" {
		t.Fatalf("reviewer posture=%q, want read_only (may never edit code or tests)", rev.Posture)
	}
	if rev.Cohort != "review" {
		t.Fatalf("reviewer cohort=%q, want review — hubDoneVerdictError gates synthesis done on it", rev.Cohort)
	}
	if rev.Join != "all" {
		t.Fatalf("reviewer join=%q, want all", rev.Join)
	}
	foundDepends := false
	for _, d := range rev.DependsOn {
		if d == "validate" {
			foundDepends = true
		}
	}
	if !foundDepends {
		t.Fatalf("reviewer dependsOn=%v, want [validate]", rev.DependsOn)
	}
	if def.ContextProfiles != nil {
		if _, ok := def.ContextProfiles["reviewer"]; !ok {
			t.Fatal("contextProfiles missing reviewer profile")
		}
	}
	hasTool := false
	for _, tool := range def.Tools {
		if strings.Contains(tool, "submit-review-outcome") {
			hasTool = true
		}
	}
	if !hasTool {
		t.Fatal("vibe-sprint tools missing submit-review-outcome face for the reviewer")
	}

	var (
		validateToReviewer  bool
		reviewerToSynthesis bool
		validateToSynthesis bool
		validateToCoder     bool
		synthesisToCoder    bool
	)
	for _, e := range def.Edges {
		switch {
		case e.From == "validate" && e.To == "reviewer" && e.When == "done":
			validateToReviewer = true
		case e.From == "reviewer" && e.To == "synthesis" && e.When == "done":
			reviewerToSynthesis = true
		case e.From == "validate" && e.To == "synthesis" && e.When == "done":
			validateToSynthesis = true
		case e.From == "validate" && e.To == "coder" && e.When == "continue" && e.Kind == "back":
			validateToCoder = true
		case e.From == "synthesis" && e.To == "coder" && e.When == "continue" && e.Kind == "back":
			synthesisToCoder = true
		}
	}
	if !validateToReviewer {
		t.Fatal("missing validate -> reviewer done edge")
	}
	if !reviewerToSynthesis {
		t.Fatal("missing reviewer -> synthesis done edge")
	}
	if validateToSynthesis {
		t.Fatal("validate -> synthesis done edge bypasses the reviewer — remove it")
	}
	if !validateToCoder {
		t.Fatal("missing validate -> coder continue back-edge (validate retry must not escalate to the user)")
	}
	if !synthesisToCoder {
		t.Fatal("missing synthesis -> coder continue back-edge (review changes_requested must re-enter coder)")
	}
	if err := ValidateFlowSafetyTopology(def); err != nil {
		t.Fatalf("ValidateFlowSafetyTopology: %v", err)
	}
}
