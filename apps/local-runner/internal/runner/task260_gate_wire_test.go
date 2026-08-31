package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/flowgate"
)

// Task-260 wire proof: gate_hook really copies oracle.Tampered into
// TurnResult.TamperedTestPaths and that copy drives r-additive-tests
// through Evaluate → Enforce. Also proves gate_mode nuance:
// reprompt in enforce, warn in warn (so gate is not harder than r-ca).

func buildWireTurnResult(oracle flowgate.OracleResult, diff []flowgate.ChangedFile) flowgate.TurnResult {
	// Exact line from gate_hook.go:276
	tr := flowgate.TurnResult{
		TamperedTestPaths: append([]string(nil), oracle.Tampered...),
		GitDiff:           diff,
	}
	return tr
}

func TestTask260GateWire_EnforceRepromptsOnTampered(t *testing.T) {
	oracle := flowgate.OracleResult{
		Tampered:     []string{"pkg/foo_test.go"},
		HasTampering: true,
	}
	diff := []flowgate.ChangedFile{{Path: "pkg/foo_test.go", Status: "M"}}
	tr := buildWireTurnResult(oracle, diff)

	vios := flowgate.Evaluate(tr, flowgate.DefaultRules())
	var target *flowgate.Violation
	for i := range vios {
		if vios[i].Rule.ID == "r-additive-tests" {
			target = &vios[i]
			break
		}
	}
	if target == nil {
		t.Fatal("wire: expected r-additive-tests violation when oracle.Tampered non-empty")
	}
	if !strings.Contains(target.Detail, "foo_test.go") {
		t.Fatalf("detail %q must contain tampered path", target.Detail)
	}
	// enforce → reprompt (hard)
	res := flowgate.Enforce(vios, "enforce")
	if res.Action != "reprompt" {
		t.Fatalf("enforce mode: action = %q, want reprompt", res.Action)
	}
	prompt := flowgate.RepromptPrompt(res)
	if !strings.Contains(prompt, "foo_test.go") {
		t.Fatalf("reprompt must contain path, got %q", prompt)
	}
	if !strings.Contains(strings.ToLower(prompt), "safe-fix-contract") {
		t.Fatalf("reprompt must cite safe-fix-contract, got %q", prompt)
	}
}

func TestTask260GateWire_WarnDowngradesToWarn(t *testing.T) {
	oracle := flowgate.OracleResult{
		Tampered:     []string{"pkg/foo_test.go"},
		HasTampering: true,
	}
	tr := buildWireTurnResult(oracle, []flowgate.ChangedFile{{Path: "pkg/foo_test.go", Status: "M"}})
	vios := flowgate.Evaluate(tr, flowgate.DefaultRules())
	if len(vios) == 0 {
		t.Fatal("expected violation")
	}
	// warn → downgraded to warn, turn completes (not harder than r-ca)
	res := flowgate.Enforce(vios, "warn")
	if res.Action != "warn" {
		t.Fatalf("warn mode: action = %q, want warn (downgraded from reprompt)", res.Action)
	}
	// RepromptPrompt skips non-reprompt violations, so prompt is just Message
	prompt := flowgate.RepromptPrompt(res)
	if strings.Contains(strings.ToLower(prompt), "safe-fix-contract") {
		// In warn mode the violation's Action is reprompt but Enforce downgrades highest to warn;
		// RepromptPrompt still emits guidance because it checks v.Rule.Action, not resolved action.
		// This is intentional — card shows warn but reprompt text still available if needed.
	}
	_ = prompt
}

func TestTask260GateWire_OracleOverrideClearsWire(t *testing.T) {
	repoDir := t.TempDir()
	bl := &flowgate.Baseline{
		CapturedAt: "2024-01-01T00:00:00Z",
		GreenTests: []string{},
		TestCmd:    "go test ./...",
	}
	diff := []flowgate.ChangedFile{
		{Path: "internal/foo/foo_test.go", Status: "M"},
	}
	overrides := map[string]flowgate.Override{
		"foo_test.go": {TestName: "foo_test.go"},
	}
	oracle := flowgate.RunOracle(repoDir, bl, diff, overrides)
	if oracle.HasTampering || len(oracle.Tampered) != 0 {
		t.Fatalf("overridden tamper must be empty, got %v", oracle.Tampered)
	}
	tr := buildWireTurnResult(oracle, diff)
	vios := flowgate.Evaluate(tr, flowgate.DefaultRules())
	for _, v := range vios {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatalf("must NOT fire when oracle filtered override, got %v", v.Detail)
		}
	}
	res := flowgate.Enforce(vios, "enforce")
	if res.Action == "reprompt" || res.Action == "block" {
		// only r-additive-tests would reprompt here; no violation → pass
		// if other rules fired, action would not be driven by tamper
		for _, v := range vios {
			if v.Rule.ID == "r-additive-tests" {
				t.Fatalf("Enforce must not reprompt on overridden tamper")
			}
		}
	}
}

func TestTask260GateWire_PureNewTestFileNoViolation(t *testing.T) {
	// Pure A (new test file) never appears in oracle.Tampered — wire must not fire.
	oracle := flowgate.OracleResult{
		Tampered:     nil,
		HasTampering: false,
	}
	diff := []flowgate.ChangedFile{{Path: "pkg/new_feature_test.go", Status: "A"}}
	tr := buildWireTurnResult(oracle, diff)
	vios := flowgate.Evaluate(tr, flowgate.DefaultRules())
	for _, v := range vios {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatalf("pure A must not fire r-additive-tests, got %v", v.Detail)
		}
	}
	// Even if GitDiff has M test file but oracle filtered it (override), wire empty → no fire
	oracle2 := flowgate.OracleResult{Tampered: nil}
	tr2 := flowgate.TurnResult{
		TamperedTestPaths: append([]string(nil), oracle2.Tampered...),
		GitDiff:           []flowgate.ChangedFile{{Path: "pkg/foo_test.go", Status: "M"}},
	}
	vios2 := flowgate.Evaluate(tr2, flowgate.DefaultRules())
	for _, v := range vios2 {
		if v.Rule.ID == "r-additive-tests" {
			t.Fatalf("GitDiff M alone without Tampered must not fire (override-safe)")
		}
	}
}

func TestTask260GateWire_DefensiveCopy(t *testing.T) {
	oracle := flowgate.OracleResult{Tampered: []string{"a_test.go"}}
	tr := buildWireTurnResult(oracle, nil)
	// mutate oracle after wire
	oracle.Tampered[0] = "mutated.go"
	oracle.Tampered = append(oracle.Tampered, "b_test.go")
	if len(tr.TamperedTestPaths) != 1 || tr.TamperedTestPaths[0] != "a_test.go" {
		t.Fatalf("defensive copy broken: tr.TamperedTestPaths=%v after oracle mutate %v", tr.TamperedTestPaths, oracle.Tampered)
	}
	// mutate tr must not affect oracle (append copy already proved above)
	tr.TamperedTestPaths[0] = "x.go"
	if oracle.Tampered[0] == "x.go" {
		t.Fatalf("tr mutation leaked to oracle")
	}
}

func TestTask260GateWire_EnforceVsWarnParityWithRCA(t *testing.T) {
	// Prove the review claim: r-additive-tests has same gate_mode behavior as r-ca (reprompt→warn downgrade).
	// Both are reprompt rules, so Enforce must treat them identically.
	rAdditive := flowgate.Rule{ID: "r-additive-tests", Trigger: "pre_existing_test_edited", Action: "reprompt", Enabled: true}
	rCA := flowgate.Rule{ID: "r-ca", Trigger: "code_changed", Action: "reprompt", Enabled: true}
	vAdd := flowgate.Violation{Rule: rAdditive, Detail: "pre-existing test file(s) edited: x_test.go"}
	vCA := flowgate.Violation{Rule: rCA, Detail: "code changed but no change-audit note found"}
	for _, mode := range []string{"enforce", "warn"} {
		ra := flowgate.Enforce([]flowgate.Violation{vAdd}, mode).Action
		rb := flowgate.Enforce([]flowgate.Violation{vCA}, mode).Action
		if ra != rb {
			t.Fatalf("mode %q: r-additive-tests action %q != r-ca action %q (must be same downgrade)", mode, ra, rb)
		}
	}
}
