package flowgate

import (
	"strings"
	"testing"
)

// CP-64 P-1 (Task-364): r-reproduce rule engine. New file — do not edit
// existing flowgate tests (additive-tests-only).
//
// The rule is signal-driven (TurnResult.ReproduceExpected +
// ReproduceCompileFailed + Tests), so every scenario below pins the pure
// evaluator the runner's gate hook feeds.

func reproduceTurn(failed []string, ran bool, compileFailed bool) TurnResult {
	return TurnResult{
		ReproduceExpected:      true,
		ReproduceCompileFailed: compileFailed,
		Tests:                  TestOutcome{Ran: ran, Failed: failed},
	}
}

// Scenario: reproduce turn chạy XANH (bug không tái hiện) -> r-reproduce reprompt.
func TestRuleReproduceFailsWhenAllTestsPass(t *testing.T) {
	tr := reproduceTurn(nil, true, false)
	v := checkReproduceRule(ReproduceRule(), tr)
	if v == nil {
		t.Fatal("expected r-reproduce violation when the suite passed (bug not reproduced)")
	}
	if v.Rule.ID != ReproduceRuleID {
		t.Fatalf("violation rule = %q want %q", v.Rule.ID, ReproduceRuleID)
	}
	if !strings.Contains(strings.ToLower(v.Detail), "was not reproduced") {
		t.Fatalf("detail should state the bug was not reproduced, got %q", v.Detail)
	}
	// Full evaluation path: the rule must be appended, not part of defaults.
	if got := Evaluate(tr, EnabledReproduceRules(DefaultRules(), true)); !hasRuleViolation(got, ReproduceRuleID) {
		t.Fatalf("expected %s in evaluation, got %+v", ReproduceRuleID, got)
	}
}

// Scenario: test reproduce lỗi cú pháp (compile error) -> KHÔNG tính là tái hiện.
func TestRuleReproduceFailsOnCompileError(t *testing.T) {
	tr := reproduceTurn(nil, true, true)
	v := checkReproduceRule(ReproduceRule(), tr)
	if v == nil {
		t.Fatal("expected r-reproduce violation on a compile error")
	}
	if !strings.Contains(strings.ToLower(v.Detail), "compile") {
		t.Fatalf("detail should mention the compile failure, got %q", v.Detail)
	}

	// Classifier provenance: a real `go test` compile error is classified as
	// such, so the runner's wiring sets ReproduceCompileFailed for it.
	goOut := "# flowpilot-runner/internal/calc\ncalc/calc_test.go:9:8: undefined: Add\nFAIL\tflowpilot-runner/internal/calc [build failed]\n"
	if !ClassifySuiteOutput("go test ./calc/...", goOut) {
		t.Fatal("expected go compile error to be classified as compile failure")
	}
	// An assertion failure must NOT be classified as a compile error.
	assertOut := "=== RUN   TestAdd\n    calc_test.go:9: expected 3 got 4\n--- FAIL: TestAdd (0.00s)\nFAIL\n"
	if ClassifySuiteOutput("go test ./calc/...", assertOut) {
		t.Fatal("assertion failure output must not classify as a compile error")
	}
}

// Scenario: test ĐỎ vì assertion -> pass gate (không violation).
func TestRuleReproducePassesOnAssertionFailure(t *testing.T) {
	tr := reproduceTurn([]string{"TestDivideByZeroReturnsError"}, true, false)
	if v := checkReproduceRule(ReproduceRule(), tr); v != nil {
		t.Fatalf("expected no violation for an assertion failure, got %+v", v)
	}
	if !ReproduceSatisfied(tr) {
		t.Fatal("ReproduceSatisfied should be true for a named assertion failure")
	}
	if got := ReproduceFailedTests(tr); len(got) != 1 || got[0] != "TestDivideByZeroReturnsError" {
		t.Fatalf("ReproduceFailedTests = %v", got)
	}
	if got := Evaluate(tr, EnabledReproduceRules(DefaultRules(), true)); hasRuleViolation(got, ReproduceRuleID) {
		t.Fatalf("assertion failure must not produce %s, got %+v", ReproduceRuleID, got)
	}
	// A reproduce turn that never ran the suite is also a failure (no evidence).
	if v := checkReproduceRule(ReproduceRule(), reproduceTurn(nil, false, false)); v == nil {
		t.Fatal("expected a violation when the reproduce turn never ran the suite")
	}
}

// Scenario: turn thường (task tính năng mới) -> r-reproduce im lặng, kể cả khi
// rule vô tình nằm trong file rules của workspace.
func TestRuleReproduceSkipsOnNonBugFlow(t *testing.T) {
	tr := TurnResult{Tests: TestOutcome{Ran: true}}
	if v := checkReproduceRule(ReproduceRule(), tr); v != nil {
		t.Fatalf("non-reproduce turn must never fire %s, got %+v", ReproduceRuleID, v)
	}
	// Even a red suite on a non-reproduce turn is r-tests' business, not ours.
	red := TurnResult{Tests: TestOutcome{Ran: true, Failed: []string{"TestOther"}}}
	if v := checkReproduceRule(ReproduceRule(), red); v != nil {
		t.Fatalf("non-reproduce turn with failures must not fire %s, got %+v", ReproduceRuleID, v)
	}
	if got := Evaluate(red, EnabledReproduceRules(DefaultRules(), false)); hasRuleViolation(got, ReproduceRuleID) {
		t.Fatalf("activation off must not append %s, got %+v", ReproduceRuleID, got)
	}
}

// Scenario: DefaultRules() byte-stable (r-reproduce opt-in, T-1) và suppression
// r-tests/r-reg chỉ áp dụng cho đúng turn reproduce.
func TestReproduceRuleOptInAndTestRuleSuppression(t *testing.T) {
	for _, r := range DefaultRules() {
		if r.ID == ReproduceRuleID {
			t.Fatal("r-reproduce must not be in DefaultRules() (TestDefaultRules byte-stability)")
		}
	}
	base := EnabledReproduceRules(DefaultRules(), true)
	count := 0
	for _, r := range base {
		if r.ID == ReproduceRuleID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("r-reproduce appended %d times, want exactly 1", count)
	}
	// Idempotent: a stored flow-rules.json already listing it never duplicates.
	again := EnabledReproduceRules(base, true)
	count = 0
	for _, r := range again {
		if r.ID == ReproduceRuleID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("EnabledReproduceRules is not idempotent: %d entries", count)
	}
	// A reproduce turn keeps everything except r-tests/r-reg (the red test is
	// the point of the turn).
	suppressed := SuppressTestRules(base)
	for _, r := range suppressed {
		if r.ID == "r-tests" || r.ID == "r-reg" {
			t.Fatalf("%s must be suppressed on a reproduce turn", r.ID)
		}
	}
	if !hasRuleViolation(Evaluate(reproduceTurn(nil, true, false), suppressed), ReproduceRuleID) {
		t.Fatal("suppression must not drop r-reproduce itself")
	}
	if hasRuleViolation(Evaluate(reproduceTurn(nil, true, false), suppressed), "r-tests") {
		t.Fatal("r-tests must not fire on a reproduce turn")
	}
	if len(suppressed) != len(base)-2 {
		t.Fatalf("SuppressTestRules dropped %d rules, want 2", len(base)-len(suppressed))
	}
}

// Scenario: classifier nhận diện compile signature của từng runner family
// (T-3) và bỏ qua output rỗng.
func TestClassifySuiteOutputPerRunner(t *testing.T) {
	cases := []struct {
		name    string
		testCmd string
		output  string
		want    bool
	}{
		{"go build failed", "go test ./...", "FAIL\tpkg [build failed]", true},
		{"go undefined", "go test ./pkg/...", "pkg/x_test.go:3:2: undefined: Missing\n", true},
		{"go syntax", "go test ./...", "pkg/x.go:1:1: syntax error: unexpected }\n", true},
		{"go assertion", "go test ./...", "--- FAIL: TestX (0.00s)\n    expected 2 got 3\n", false},
		{"pytest syntax", "pytest -q", "E   SyntaxError: invalid syntax\n", true},
		{"pytest collection", "pytest -q", "ERROR collecting tests/test_x.py\n", true},
		{"pytest assertion", "pytest -q", "FAILED tests/test_x.py::test_add - assert 1 == 2\n", false},
		{"npm ts error", "npm test", "error TS2345: Argument of type 'string' is not assignable\n", true},
		{"npm assertion", "npm test", "x adds numbers (4 ms)\n  expect(received).toBe(expected)\n", false},
		{"empty output", "go test ./...", "   ", false},
		{"unknown runner syntax", "./run_tests.sh", "SyntaxError: Unexpected token '}'", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifySuiteOutput(tc.testCmd, tc.output); got != tc.want {
				t.Fatalf("ClassifySuiteOutput(%q, %q) = %v want %v", tc.testCmd, tc.output, got, tc.want)
			}
		})
	}
}

func hasRuleViolation(vs []Violation, id string) bool {
	for _, v := range vs {
		if v.Rule.ID == id {
			return true
		}
	}
	return false
}
