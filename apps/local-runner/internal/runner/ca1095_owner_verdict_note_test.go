package runner

// CA-1095 (owner verdict propagation): owner children are verdict_only —
// their remediation decision rides submit_review_outcome, and the final
// message is thin. The joined cohort note must carry the verdict status AND
// content into debate_synthesis; live run-3362's synthesis escalated
// "joined owner notes carried no verdict content" and continue-remounted.

import (
	"strings"
	"testing"
)

func TestCA1095_CohortNoteCarriesVerdictContent(t *testing.T) {
	entries := []cohortEntry{
		{
			Label:          "owner_1",
			Provider:       "devin",
			Status:         "completed",
			FinalMessage:   "verdict submitted",
			MachineVerdict: "continue",
			VerdictDetail:  "reprompt the coder — fill the three TODO bodies only; leave tests untouched\n  - AC-1: fail — PrivaVaultApp still TODO",
		},
		{
			Label:          "owner_2",
			Provider:       "devin",
			Status:         "completed",
			FinalMessage:   "verdict submitted",
			MachineVerdict: "continue",
			VerdictDetail:  "rescope to MainActivity.kt only",
		},
	}
	note := buildCohortNote("run-parent", "owner_debate", entries, 3)
	if !strings.Contains(note, "verdict=continue") {
		t.Fatalf("joined note must surface each member's machine verdict, got:\n%s", note)
	}
	if !strings.Contains(note, "fill the three TODO bodies") {
		t.Fatalf("joined note must carry the owner's remediation content, got:\n%s", note)
	}
	if !strings.Contains(note, "rescope to MainActivity.kt") {
		t.Fatalf("owner_2 detail missing from joined note:\n%s", note)
	}
}

// The buffered detail flows from recordReviewCohortMemberVerdict through the
// cohort-entry append into the note — the full handoff, not just the render.
func TestCA1095_VerdictDetailBufferToNote(t *testing.T) {
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.recordReviewCohortMemberVerdict(parent.RunID, "owner_1", "continue", "reprompt the coder — fill the TODO bodies")

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	detail := rs.pendingReviewVerdictDetailByLabel["owner_1"]
	svc.mu.Unlock()
	if !strings.Contains(detail, "fill the TODO bodies") {
		t.Fatalf("verdict detail was not buffered, got %q", detail)
	}
}
