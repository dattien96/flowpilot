package flowgate

import (
	"strings"
	"testing"
)

// CP-67 P-2 (Task-379 B-4): r-scaffold-red evaluator contract — the scaffold
// turn's suite MUST be red, compiled, and (B-11) body-clean.

func scaffoldTurnResult() TurnResult {
	return TurnResult{
		ScaffoldExpected: true,
		Tests: TestOutcome{
			Ran:    true,
			Failed: []string{"TestGetUser_ReturnsNotFoundError"},
		},
	}
}

func TestRuleScaffoldRedRequiresFailingTests(t *testing.T) {
	tr := scaffoldTurnResult()
	tr.Tests.Failed = nil
	if v := checkScaffoldRedRule(ScaffoldRedRule(), tr); v == nil {
		t.Fatal("a scaffold turn with no failing test must violate r-scaffold-red")
	}
}

func TestRuleScaffoldRedRejectsCompileFailure(t *testing.T) {
	tr := scaffoldTurnResult()
	tr.ScaffoldCompileFailed = true
	v := checkScaffoldRedRule(ScaffoldRedRule(), tr)
	if v == nil {
		t.Fatal("compile failure must violate r-scaffold-red")
	}
	if !strings.Contains(v.Detail, "compile") {
		t.Fatalf("compile detail must say so, got %q", v.Detail)
	}
}

func TestRuleScaffoldRedRejectsAllGreen(t *testing.T) {
	// Signal 1: all-green means the "stubs" were implemented.
	tr := scaffoldTurnResult()
	tr.Tests.Failed = nil
	tr.Tests.Regressed = nil
	v := checkScaffoldRedRule(ScaffoldRedRule(), tr)
	if v == nil {
		t.Fatal("all-green scaffold must violate r-scaffold-red")
	}
	if !strings.Contains(strings.ToLower(v.Detail), "implementation") {
		t.Fatalf("all-green detail must call out implemented bodies, got %q", v.Detail)
	}
}

func TestRuleScaffoldRedSuppressesTestRules(t *testing.T) {
	// The scaffold suite is MEANT to be red: r-tests/r-reg must be suppressed
	// for exactly this turn while r-scaffold-red is armed.
	base := append(DefaultRules(), Rule{ID: "r-tests", Trigger: "tests_failed", Enabled: true}, Rule{ID: "r-reg", Trigger: "regression_test_broke", Enabled: true})
	armed := EnabledScaffoldRules(base, true)
	hasScaffold := false
	for _, r := range armed {
		if r.ID == "r-tests" || r.ID == "r-reg" {
			t.Fatal("r-tests/r-reg must be suppressed for the scaffold turn")
		}
		if r.ID == ScaffoldRedRuleID {
			hasScaffold = true
		}
	}
	if !hasScaffold {
		t.Fatal("r-scaffold-red must be appended")
	}
	// And the rule never fires outside a scaffold turn.
	tr := TurnResult{Tests: TestOutcome{Ran: true}}
	if v := checkScaffoldRedRule(ScaffoldRedRule(), tr); v != nil {
		t.Fatal("r-scaffold-red is opt-in — a non-scaffold turn is a no-signal pass")
	}
}

func TestRuleScaffoldRedDetectsBodyLogicAsGreenTests(t *testing.T) {
	// Signal 1 end-to-end: implemented bodies make the suite green, which the
	// rule reads as implemented logic.
	tr := scaffoldTurnResult()
	tr.Tests.Failed = nil
	if v := checkScaffoldRedRule(ScaffoldRedRule(), tr); v == nil {
		t.Fatal("green suite on a scaffold turn must be detected as implemented bodies")
	}
}

func TestRuleScaffoldRedForcesStubAfterBodyViolation(t *testing.T) {
	// The reprompt loop: violating turn → clean stub turn passes.
	dirty := scaffoldTurnResult()
	dirty.Tests.Failed = nil
	if v := checkScaffoldRedRule(ScaffoldRedRule(), dirty); v == nil {
		t.Fatal("first turn (all green) must violate")
	}
	clean := scaffoldTurnResult()
	clean.Tests.Failed = []string{"TestCreateUser_NotImplemented"}
	if v := checkScaffoldRedRule(ScaffoldRedRule(), clean); v != nil {
		t.Fatalf("clean red stub turn must pass, got %v", v.Detail)
	}
}

func TestRuleScaffoldRedFiresOnNonStubBody(t *testing.T) {
	// Signal 3 (B-11): the suite is red, but the static whitelist catches
	// smuggled body logic anyway — closing the "wrote half-right logic so the
	// suite stayed red" hole.
	tr := scaffoldTurnResult()
	tr.ScaffoldBodyNonStub = true
	tr.NonStubSymbols = []string{"GetUser:12"}
	v := checkScaffoldRedRule(ScaffoldRedRule(), tr)
	if v == nil {
		t.Fatal("non-stub body must violate r-scaffold-red even with a red suite")
	}
	if !strings.Contains(v.Detail, "GetUser:12") {
		t.Fatalf("violation must name the offending symbol:line, got %q", v.Detail)
	}
}

func TestScaffoldSatisfiedShape(t *testing.T) {
	good := scaffoldTurnResult()
	if !ScaffoldSatisfied(good) {
		t.Fatal("red compiled suite with clean bodies satisfies the scaffold gate")
	}
	bad := scaffoldTurnResult()
	bad.ScaffoldBodyNonStub = true
	if ScaffoldSatisfied(bad) {
		t.Fatal("non-stub body must fail ScaffoldSatisfied")
	}
}
