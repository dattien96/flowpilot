package flowgate

import (
	"fmt"
	"strings"
)

// ReproduceRuleID is the CP-64 reproduce-first gate rule. It is deliberately
// NOT part of DefaultRules() — TestDefaultRules stays byte-stable and every
// pre-CP-64 workspace keeps the exact same rule set (same opt-in pattern
// RequirementRule established for r-requirement). The runner appends it for a
// reproduce turn only (see EnabledReproduceRules).
const ReproduceRuleID = "r-reproduce"

// ReproduceRule (CP-64 P-1) forces a BugFix turn to physically demonstrate the
// bug before any production write is allowed: at least one newly written test
// must FAIL ON ITS ASSERTION. Action "reprompt" per T-2 — both failure shapes
// (compile error, all-green) reprompt with a different Detail; the
// assertion-failure shape produces no violation at all (gate passes and the
// runner locks the test file read-only for the coder).
func ReproduceRule() Rule {
	return Rule{
		ID:             ReproduceRuleID,
		Scope:          "step",
		Trigger:        "reproduce_not_demonstrated",
		RequiredOutput: "reproducing_failing_test",
		Action:         "reprompt",
		Enabled:        true,
	}
}

// EnabledReproduceRules returns base with r-reproduce appended when the caller
// decided this turn must reproduce a bug (CP-64 P-1 activation: node behavior
// agent.reproduce, or a bug-flow behavior-change turn) and the reproduce gate
// flag is on. Idempotent — a stored flow-rules.json that already lists the id
// is never duplicated. An empty/disabled flag keeps the caller's base rule set
// byte-identical to pre-CP-64 behavior (CP-64 §8 fallback).
func EnabledReproduceRules(base []Rule, expected bool) []Rule {
	if !expected {
		return base
	}
	for _, r := range base {
		if r.ID == ReproduceRuleID {
			return base
		}
	}
	return append(append([]Rule(nil), base...), ReproduceRule())
}

// SuppressTestRules returns base without the tier-2 test rules (r-tests/r-reg)
// for exactly one reproduce turn. The test written in a reproduce turn exists
// to be RED, so the ordinary "tests failed"/"suite regressed" block would
// reprompt the very turn the reproduce gate wants (CP-64 P-1; same one-turn
// exemption shape the proposal-turn already uses in the runner's gate hook).
// Every other rule is preserved verbatim.
func SuppressTestRules(base []Rule) []Rule {
	out := make([]Rule, 0, len(base))
	for _, r := range base {
		if r.ID == "r-tests" || r.ID == "r-reg" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// ReproduceSatisfied reports whether a reproduce turn actually demonstrated the
// bug: the suite ran, at least one named test failed, and the failure was not a
// compile error. Exposed so the runner's gate hook can log/settle the same
// verdict the rule engine reached.
func ReproduceSatisfied(tr TurnResult) bool {
	return tr.ReproduceExpected && !tr.ReproduceCompileFailed && tr.Tests.Ran && len(tr.Tests.Failed) > 0
}

// ReproduceFailedTests returns the reproduce turn's failing test names (nil
// when the turn is not a reproduce turn) — the evidence a passing r-reproduce
// gate is recorded with.
func ReproduceFailedTests(tr TurnResult) []string {
	if !tr.ReproduceExpected {
		return nil
	}
	return append([]string(nil), tr.Tests.Failed...)
}

// checkReproduceRule is the r-reproduce evaluator. Three outcomes per CP-64
// §3.1:
//
//   - compile error       -> violation (reprompt: make the test compile)
//   - suite green / no run -> violation (reprompt: reproduce the bug)
//   - assertion failure    -> nil (gate passes, runner locks the test file)
func checkReproduceRule(rule Rule, tr TurnResult) *Violation {
	if !tr.ReproduceExpected {
		// Non-reproduce turn: the rule is opt-in, so it never fires outside a
		// reproduce context even if a stored rules file lists it (T-1/T-5).
		return nil
	}
	if tr.ReproduceCompileFailed {
		return &Violation{
			Rule:   rule,
			Detail: "the reproduce test failed to compile — a compile error is not a reproduction; fix the test's syntax/imports so it builds and then fails on its assertion",
		}
	}
	if !tr.Tests.Ran {
		return &Violation{
			Rule:   rule,
			Detail: "no test run was observed for this reproduce turn — write the failing test AND run the suite so the gate can see it fail",
		}
	}
	if len(tr.Tests.Failed) == 0 {
		return &Violation{
			Rule: rule,
			Detail: "the suite passed, so the bug was not reproduced — add an assertion that fails against the current (unfixed) code; " +
				"do not weaken an existing test",
		}
	}
	return nil
}

// ReproduceSatisfiedDetail renders the short evidence line recorded when a
// reproduce gate passes (also used by the runner's reprompt/escalate logs).
func ReproduceSatisfiedDetail(tr TurnResult) string {
	failed := ReproduceFailedTests(tr)
	if len(failed) == 0 {
		return ""
	}
	return fmt.Sprintf("bug reproduced by failing test(s): %s", strings.Join(failed, ", "))
}
