package flowgate

import "testing"

// TestCheckRuleGoverningSpecChangedFires verifies Task-186's r-spec-drift:
// the caller-computed HeadSpecDrifted signal raises a violation.
func TestCheckRuleGoverningSpecChangedFires(t *testing.T) {
	rule := Rule{ID: "r-spec-drift", Trigger: "governing_spec_changed", Action: "warn", Enabled: true}
	tr := TurnResult{HeadSpecDrifted: true}
	if v := checkRule(rule, tr); v == nil {
		t.Fatal("expected r-spec-drift to fire when HeadSpecDrifted is true")
	}
}

func TestCheckRuleGoverningSpecChangedSkipsWhenNotDrifted(t *testing.T) {
	rule := Rule{ID: "r-spec-drift", Trigger: "governing_spec_changed", Action: "warn", Enabled: true}
	tr := TurnResult{HeadSpecDrifted: false}
	if v := checkRule(rule, tr); v != nil {
		t.Fatalf("expected no violation when HeadSpecDrifted is false, got %+v", v)
	}
}

// TestCheckRuleCodeDivergedFromIntentFires verifies Task-186's r-code-drift.
func TestCheckRuleCodeDivergedFromIntentFires(t *testing.T) {
	rule := Rule{ID: "r-code-drift", Trigger: "code_diverged_from_intent", Action: "warn", Enabled: true}
	tr := TurnResult{HeadCodeDrifted: true}
	if v := checkRule(rule, tr); v == nil {
		t.Fatal("expected r-code-drift to fire when HeadCodeDrifted is true")
	}
}

func TestCheckRuleCodeDivergedFromIntentSkipsWhenNotDrifted(t *testing.T) {
	rule := Rule{ID: "r-code-drift", Trigger: "code_diverged_from_intent", Action: "warn", Enabled: true}
	tr := TurnResult{HeadCodeDrifted: false}
	if v := checkRule(rule, tr); v != nil {
		t.Fatalf("expected no violation when HeadCodeDrifted is false, got %+v", v)
	}
}

// TestCheckRuleGoverningSpecAddedToSpecLessFires verifies Task-186's
// r-attach-spec: a spec_less feature gaining a governing doc surfaces for
// human confirmation, never auto-applied (BR-2).
func TestCheckRuleGoverningSpecAddedToSpecLessFires(t *testing.T) {
	rule := Rule{ID: "r-attach-spec", Trigger: "governing_spec_added_to_spec_less", Action: "approve", Enabled: true}
	tr := TurnResult{HeadAttachSpecPending: true}
	if v := checkRule(rule, tr); v == nil {
		t.Fatal("expected r-attach-spec to fire when HeadAttachSpecPending is true")
	}
}

func TestCheckRuleGoverningSpecAddedToSpecLessSkipsWhenNotPending(t *testing.T) {
	rule := Rule{ID: "r-attach-spec", Trigger: "governing_spec_added_to_spec_less", Action: "approve", Enabled: true}
	tr := TurnResult{HeadAttachSpecPending: false}
	if v := checkRule(rule, tr); v != nil {
		t.Fatalf("expected no violation when HeadAttachSpecPending is false, got %+v", v)
	}
}

// TestCheckRuleFeatureRenameMergeOrDeprecateFires verifies Task-187's
// r-retire: a detected retire intent surfaces for human confirmation of
// targets, never auto-applied (BR-2).
func TestCheckRuleFeatureRenameMergeOrDeprecateFires(t *testing.T) {
	rule := Rule{ID: "r-retire", Trigger: "feature_rename_merge_or_deprecate", Action: "approve", Enabled: true}
	tr := TurnResult{HeadRetirePending: true}
	if v := checkRule(rule, tr); v == nil {
		t.Fatal("expected r-retire to fire when HeadRetirePending is true")
	}
}

func TestCheckRuleFeatureRenameMergeOrDeprecateSkipsWhenNotPending(t *testing.T) {
	rule := Rule{ID: "r-retire", Trigger: "feature_rename_merge_or_deprecate", Action: "approve", Enabled: true}
	tr := TurnResult{HeadRetirePending: false}
	if v := checkRule(rule, tr); v != nil {
		t.Fatalf("expected no violation when HeadRetirePending is false, got %+v", v)
	}
}
