package flowgate

import "testing"

// BUG-389 (live runs 4501/5307/6008): checkReproduceRule accepted ANY named
// assertion failure — including fabricated self-contained failures that never
// touch the reported symbol (simulatedBuggy := 3 + 4). When the runner can
// resolve the failing test's source and the step's declared production
// symbols, a failing test that exercises no declared symbol is a violation.
func TestBug389_FabricatedFailureNotExercisingTargetIsViolation(t *testing.T) {
	rule := Rule{ID: "r-reproduce", Trigger: "reproduce_not_demonstrated", Action: "block", Enabled: true}
	tr := TurnResult{
		ReproduceExpected:        true,
		ReproduceTargetChecked:   true,
		ReproduceExercisesTarget: false,
		Tests:                    TestOutcome{Ran: true, Failed: []string{"TestSimulatedBuggy"}},
	}
	v := checkReproduceRule(rule, tr)
	if v == nil {
		t.Fatal("a failing test that exercises no declared symbol must not satisfy r-reproduce")
	}
}

func TestBug389_RealFailureExercisingTargetPasses(t *testing.T) {
	rule := Rule{ID: "r-reproduce", Trigger: "reproduce_not_demonstrated", Action: "block", Enabled: true}
	tr := TurnResult{
		ReproduceExpected:        true,
		ReproduceTargetChecked:   true,
		ReproduceExercisesTarget: true,
		Tests:                    TestOutcome{Ran: true, Failed: []string{"TestAdd_Negative"}},
	}
	if v := checkReproduceRule(rule, tr); v != nil {
		t.Fatalf("genuine reproduction must still pass: %v", v.Detail)
	}
}

// Uncheckable signal (non-Go targets, unresolvable test file) degrades
// gracefully: the named-failure verdict stands — typed degradation, not a
// silent block of every reproduce turn.
func TestBug389_UncheckedTargetKeepsNamedFailureVerdict(t *testing.T) {
	rule := Rule{ID: "r-reproduce", Trigger: "reproduce_not_demonstrated", Action: "block", Enabled: true}
	tr := TurnResult{
		ReproduceExpected:      true,
		ReproduceTargetChecked: false,
		Tests:                  TestOutcome{Ran: true, Failed: []string{"TestWhatever"}},
	}
	if v := checkReproduceRule(rule, tr); v != nil {
		t.Fatalf("uncheckable target must not block: %v", v.Detail)
	}
}
