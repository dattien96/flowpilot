package flowgate

import "testing"

// run-23820: pendingGateCodePaths re-check sets WrittenPaths with an empty
// GitDiff. r-ca / r-bug must still see CA / BUG docs held in WrittenPaths so a
// remediation-only turn (declare contract, no new edits) does not re-fire
// missing-doc rules and force another durable reprompt.
//
// Provider-agnostic: Evaluate/checkRule take no providerKey.
func TestRun23820CodeChangedSatisfiedByPendingCAPathWithoutGitDiff(t *testing.T) {
	rule := Rule{ID: "r-ca", Trigger: "code_changed", Action: "reprompt", Enabled: true}
	tr := TurnResult{
		// Prior coding turn's held paths (code + CA already written).
		WrittenPaths: []string{
			"calc.go",
			"change-audit/CA-917-calc-core-add-off-by-one.md",
		},
		GitDiff: nil, // remediation turn: no new diff
	}
	if v := checkRule(rule, tr); v != nil {
		t.Fatalf("r-ca must not fire when WrittenPaths already hold a CA note, got %+v", v)
	}
}

func TestRun23820CodeChangedStillFiresWithoutCAInDiffOrPaths(t *testing.T) {
	rule := Rule{ID: "r-ca", Trigger: "code_changed", Action: "reprompt", Enabled: true}
	tr := TurnResult{
		WrittenPaths: []string{"calc.go"},
		GitDiff:      nil,
	}
	if v := checkRule(rule, tr); v == nil {
		t.Fatal("r-ca must still fire when code changed with no CA in diff or paths")
	}
}

func TestRun23820BugFixedSatisfiedByPendingBugPathWithoutGitDiff(t *testing.T) {
	rule := Rule{ID: "r-bug", Trigger: "bug_fixed", Action: "reprompt", Enabled: true}
	tr := TurnResult{
		ChangeType: "bugfix",
		WrittenPaths: []string{
			"calc.go",
			"requirements/09-BugFix/done/BUG-278-calc-core-add-off-by-one.md",
		},
		GitDiff: nil,
	}
	if v := checkRule(rule, tr); v != nil {
		t.Fatalf("r-bug must not fire when WrittenPaths hold a BugFix doc, got %+v", v)
	}
}

func TestHasChangeAuditNoteInPathsAbsoluteAndRelative(t *testing.T) {
	if !HasChangeAuditNoteInPaths([]string{
		"/Users/x/proj/change-audit/CA-917-calc-core-add-off-by-one.md",
	}) {
		t.Fatal("absolute pending path must count as CA")
	}
	if HasChangeAuditNoteInPaths([]string{"calc.go", "readme.md"}) {
		t.Fatal("code/docs without CA- must not count")
	}
}
