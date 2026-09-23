package flowgate

import "testing"

// BUG-398 (live run-2290): a scaffold/tdd step's contract is compile-green /
// runtime-RED by design — stubs plus failing tests. The generic r-tests/r-reg
// rules must not treat that contracted RED as a violation; r-scaffold-red is
// the rule that owns the RED semantics for this turn.
func scaffoldRule() Rule {
	return Rule{ID: "r-scaffold-red", Trigger: "scaffold_not_red", Action: "block", Enabled: true}
}

func TestBug398_ScaffoldExpectedSuppressesRTestsAndRReg(t *testing.T) {
	tr := TurnResult{
		ScaffoldExpected: true,
		Tests:            TestOutcome{Ran: true, Failed: []string{"TestReverse_Empty"}, Regressed: []string{"TestOld"}},
	}
	rules := []Rule{
		{ID: "r-tests", Trigger: "tests_failed", Action: "block", Enabled: true},
		{ID: "r-reg", Trigger: "regression_test_broke", Action: "block", Enabled: true},
	}
	for _, v := range Evaluate(tr, rules) {
		if v.Rule.ID == "r-tests" || v.Rule.ID == "r-reg" {
			t.Fatalf("contracted RED must not fire %s on a scaffold turn: %v", v.Rule.ID, v.Detail)
		}
	}
}

// Non-scaffold turns keep the rules armed — suppression is scoped to the
// contracted-RED turn only.
func TestBug398_NonScaffoldTurnStillFiresRTests(t *testing.T) {
	tr := TurnResult{Tests: TestOutcome{Ran: true, Failed: []string{"TestX"}}}
	rules := []Rule{{ID: "r-tests", Trigger: "tests_failed", Action: "block", Enabled: true}}
	found := false
	for _, v := range Evaluate(tr, rules) {
		if v.Rule.ID == "r-tests" {
			found = true
		}
	}
	if !found {
		t.Fatal("r-tests must still fire on a non-scaffold failing turn")
	}
}
