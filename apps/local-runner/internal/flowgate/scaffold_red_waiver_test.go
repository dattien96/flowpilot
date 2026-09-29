package flowgate

import (
	"strings"
	"testing"
)

// Live run-15525/run-17384 (vibe sprint, debate-verify): a pre-existing
// implementation planted before the scaffold phase was adjudicated by
// owner-debate remediation — the node's declared contract records
// "red_tests: []" and "failure_type: none" with status scaffold_ready. The
// gate nevertheless flagged the GREEN suite as smuggled implementation and
// reprompted "revert every body to the stub whitelist", contradicting the
// adjudicated contract: reverting the accepted artifact would break the
// green baseline. r-scaffold-red must honor the durable waiver instead.
func TestScaffoldRedWaived_GreenSuiteIsSatisfied(t *testing.T) {
	tr := TurnResult{
		ScaffoldExpected:    true,
		ScaffoldRedWaived:   true,
		ScaffoldBodyNonStub: true,
		NonStubSymbols:      []string{"ToolValue:5"},
		Tests:               TestOutcome{Ran: true, Failed: []string{}},
	}
	for _, v := range Evaluate(tr, []Rule{scaffoldRule()}) {
		t.Fatalf("waived scaffold must not flag the accepted green artifact: %s", v.Detail)
	}
}

// The waiver does not lull the rule: a RED suite under the declared
// "red_tests: []" contract means the accepted pre-existing implementation is
// broken — flag it so the reprompt fixes the implementation (not the tests).
func TestScaffoldRedWaived_RedSuiteStillViolates(t *testing.T) {
	tr := TurnResult{
		ScaffoldExpected:    true,
		ScaffoldRedWaived:   true,
		ScaffoldBodyNonStub: true,
		Tests:               TestOutcome{Ran: true, Failed: []string{"TestToolValue"}},
	}
	violations := Evaluate(tr, []Rule{scaffoldRule()})
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation for broken accepted artifact, got %d", len(violations))
	}
	if !strings.Contains(violations[0].Detail, "accepted") && !strings.Contains(violations[0].Detail, "Tests failed") {
		t.Fatalf("violation must describe the broken artifact, got %q", violations[0].Detail)
	}
}

// Compile failure and an absent suite run still violate under the waiver —
// only the "must be RED + stub bodies" requirements are waived.
func TestScaffoldRedWaived_CompileAndNoRunStillViolate(t *testing.T) {
	for _, tr := range []TurnResult{
		{ScaffoldExpected: true, ScaffoldRedWaived: true, ScaffoldCompileFailed: true, Tests: TestOutcome{Ran: true}},
		{ScaffoldExpected: true, ScaffoldRedWaived: true, Tests: TestOutcome{Ran: false}},
	} {
		if got := Evaluate(tr, []Rule{scaffoldRule()}); len(got) != 1 {
			t.Fatalf("expected compile/no-run to keep violating under waiver, got %+v", got)
		}
	}
}
