package flowgate

import "testing"

func TestRequirementRuleNotInDefaultRules(t *testing.T) {
	for _, r := range DefaultRules() {
		if r.ID == RequirementRuleID {
			t.Fatal("DefaultRules must not include r-requirement (vibe-only via EnabledRulesFor)")
		}
	}
}

func TestEnabledRulesFor_VibeIncludesRequirement(t *testing.T) {
	rules := EnabledRulesFor("vibe", DefaultRules())
	found := false
	for _, r := range rules {
		if r.ID == RequirementRuleID {
			found = true
			if r.Trigger != "requirement_signature_drift" || r.Action != "block" {
				t.Fatalf("rule = %+v", r)
			}
		}
	}
	if !found {
		t.Fatal("vibe must enable r-requirement")
	}
}

func TestEnabledRulesFor_DevOmitsRequirement(t *testing.T) {
	injected := append(DefaultRules(), RequirementRule())
	for _, r := range EnabledRulesFor("dev", injected) {
		if r.ID == RequirementRuleID {
			t.Fatal("dev must omit r-requirement")
		}
	}
	if n := len(EnabledRulesFor("dev", DefaultRules())); n != len(DefaultRules()) {
		t.Fatalf("dev rules %d, want %d", n, len(DefaultRules()))
	}
}

func TestRRequirement_GreenDriftBlocks(t *testing.T) {
	tr := TurnResult{
		Tests:                  TestOutcome{Ran: true},
		RequirementDrift:       true,
		RequirementDriftDetail: "AC-1 vs TestFoo",
	}
	vs := Evaluate(tr, EnabledRulesFor("vibe", DefaultRules()))
	found := false
	for _, v := range vs {
		if v.Rule.ID == RequirementRuleID {
			found = true
			if v.Detail != "AC-1 vs TestFoo" {
				t.Fatalf("detail=%q", v.Detail)
			}
		}
	}
	if !found {
		t.Fatal("r-requirement must fire on green+drift")
	}
	en := Enforce(vs, "enforce")
	if en.Action != "block" {
		t.Fatalf("action=%q, want block (always-block)", en.Action)
	}
}

func TestRRequirement_DevIgnoresDrift(t *testing.T) {
	tr := TurnResult{Tests: TestOutcome{Ran: true}, RequirementDrift: true}
	for _, v := range Evaluate(tr, EnabledRulesFor("dev", DefaultRules())) {
		if v.Rule.ID == RequirementRuleID {
			t.Fatal("dev must not fire r-requirement")
		}
	}
}

func TestRRequirement_TamperedGreenCoerced(t *testing.T) {
	tr := TurnResult{
		Tests:             TestOutcome{Ran: true},
		TamperedTestPaths: []string{"foo_test.go"},
	}
	CoerceVibeRequirementDrift(&tr)
	if !tr.RequirementDrift {
		t.Fatal("tampered green must coerce RequirementDrift")
	}
	vs := Evaluate(tr, EnabledRulesFor("vibe", DefaultRules()))
	found := false
	for _, v := range vs {
		if v.Rule.ID == RequirementRuleID {
			found = true
		}
	}
	if !found {
		t.Fatal("coerced tamper must fire r-requirement")
	}
}

func TestRRequirement_NotGreenNoFire(t *testing.T) {
	tr := TurnResult{
		Tests:            TestOutcome{Ran: true, Failed: []string{"TestX"}},
		RequirementDrift: true,
	}
	for _, v := range Evaluate(tr, EnabledRulesFor("vibe", DefaultRules())) {
		if v.Rule.ID == RequirementRuleID {
			t.Fatal("failed suite must not fire r-requirement")
		}
	}
}
