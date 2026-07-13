package flowgate

import (
	"strings"
	"testing"
)

// TestCheckRuleCodeChangedNoContractFires verifies Task-185's r-contract:
// a code-changing turn without a declared Change Contract (ContractDeclared
// false) raises a violation.
func TestCheckRuleCodeChangedNoContractFires(t *testing.T) {
	rule := Rule{ID: "r-contract", Trigger: "code_changed_no_contract", Action: "reprompt", Enabled: true}
	tr := TurnResult{
		WrittenPaths:     []string{"calc.go"},
		ContractDeclared: false,
	}
	v := checkRule(rule, tr)
	if v == nil {
		t.Fatal("expected r-contract to fire for a code change with no declared contract")
	}
}

func TestCheckRuleCodeChangedNoContractSkipsWhenDeclared(t *testing.T) {
	rule := Rule{ID: "r-contract", Trigger: "code_changed_no_contract", Action: "reprompt", Enabled: true}
	tr := TurnResult{
		WrittenPaths:     []string{"calc.go"},
		ContractDeclared: true,
	}
	if v := checkRule(rule, tr); v != nil {
		t.Fatalf("expected no violation when a Change Contract was declared, got %+v", v)
	}
}

func TestCheckRuleCodeChangedNoContractSkipsWhenNoCodeChanged(t *testing.T) {
	rule := Rule{ID: "r-contract", Trigger: "code_changed_no_contract", Action: "reprompt", Enabled: true}
	tr := TurnResult{WrittenPaths: nil, ContractDeclared: false}
	if v := checkRule(rule, tr); v != nil {
		t.Fatalf("expected no violation when nothing code-changing was written, got %+v", v)
	}
}

// TestCheckRuleEditOutsideDeclaredScopeWarnsByDefault verifies r-scope fires
// with the offending paths and stays "warn" when not high-severity.
func TestCheckRuleEditOutsideDeclaredScopeWarnsByDefault(t *testing.T) {
	rule := Rule{ID: "r-scope", Trigger: "edit_outside_declared_scope", Action: "warn", Enabled: true}
	tr := TurnResult{ScopeOutOfScopePaths: []string{"user.go"}, ScopeHighSeverity: false}

	v := checkRule(rule, tr)
	if v == nil {
		t.Fatal("expected r-scope to fire for an out-of-scope edit")
	}
	if v.Rule.Action != "warn" {
		t.Errorf("Action = %q, want warn", v.Rule.Action)
	}
	if !strings.Contains(v.Detail, "user.go") {
		t.Errorf("Detail = %q, want it to name the offending path", v.Detail)
	}
}

func TestCheckRuleEditOutsideDeclaredScopeNoViolationWhenEmpty(t *testing.T) {
	rule := Rule{ID: "r-scope", Trigger: "edit_outside_declared_scope", Action: "warn", Enabled: true}
	tr := TurnResult{ScopeOutOfScopePaths: nil}
	if v := checkRule(rule, tr); v != nil {
		t.Fatalf("expected no violation with an empty out-of-scope set, got %+v", v)
	}
}

// TestCheckRuleEditOutsideDeclaredScopeBlocksOnlyWhenHighSeverity verifies
// SD-21 D-5/Q-2: a configured "block" action is only honored when the
// caller marked the violation high-severity (structure available + has
// dependents); otherwise it is downgraded to "warn".
func TestCheckRuleEditOutsideDeclaredScopeBlocksOnlyWhenHighSeverity(t *testing.T) {
	blockRule := Rule{ID: "r-scope", Trigger: "edit_outside_declared_scope", Action: "block", Enabled: true}

	lowSeverity := TurnResult{ScopeOutOfScopePaths: []string{"user.go"}, ScopeHighSeverity: false}
	v := checkRule(blockRule, lowSeverity)
	if v == nil || v.Rule.Action != "warn" {
		t.Fatalf("expected a configured block to downgrade to warn without high severity, got %+v", v)
	}

	highSeverity := TurnResult{ScopeOutOfScopePaths: []string{"user.go"}, ScopeHighSeverity: true}
	v = checkRule(blockRule, highSeverity)
	if v == nil || v.Rule.Action != "block" {
		t.Fatalf("expected a configured block to be honored when high severity, got %+v", v)
	}
}
