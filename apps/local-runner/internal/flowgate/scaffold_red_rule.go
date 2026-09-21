package flowgate

import (
	"fmt"
	"strings"
)

// ScaffoldRedRuleID is the CP-67 P-2 (Task-379, B-4) scaffold gate. Not part
// of DefaultRules() — the runner appends it for a scaffold turn only
// (EnabledScaffoldRules), suppressing r-tests/r-reg for exactly that turn
// because the suite is MEANT to be red.
const ScaffoldRedRuleID = "r-scaffold-red"

// ScaffoldRedRule forces a scaffold turn to deliver exactly what CP-67
// promises: production stubs that compile plus a test suite that runs RED.
// Three detect signals (CP-67 §3.2):
//
//  1. GREEN  — the suite passed, so the "stubs" contain real logic.
//  2. COMPILE — the code does not compile.
//  3. STATIC (B-11, Task-383) — a stub body sits outside the whitelist,
//     caught by static AST validation regardless of the suite's outcome.
func ScaffoldRedRule() Rule {
	return Rule{
		ID:             ScaffoldRedRuleID,
		Scope:          "step",
		Trigger:        "scaffold_not_red",
		RequiredOutput: "compiling_stubs_with_red_suite",
		Action:         "reprompt",
		Enabled:        true,
	}
}

// EnabledScaffoldRules returns base with r-scaffold-red appended (idempotent)
// and the tier-2 test rules removed for this one turn — the same one-turn
// exemption shape EnabledReproduceRules/SuppressTestRules established.
func EnabledScaffoldRules(base []Rule, expected bool) []Rule {
	if !expected {
		return base
	}
	filtered := SuppressTestRules(base)
	for _, r := range filtered {
		if r.ID == ScaffoldRedRuleID {
			return filtered
		}
	}
	return append(filtered, ScaffoldRedRule())
}

// ScaffoldSatisfied reports whether a scaffold turn meets the gate shape:
// suite ran, compiled, at least one RED test, and every stub body inside the
// static whitelist.
func ScaffoldSatisfied(tr TurnResult) bool {
	return tr.ScaffoldExpected && tr.Tests.Ran && !tr.ScaffoldCompileFailed && len(tr.Tests.Failed) > 0 && !tr.ScaffoldBodyNonStub
}

// checkScaffoldRedRule is the r-scaffold-red evaluator. Opt-in — it never
// fires outside a scaffold turn even if a stored rules file lists it.
func checkScaffoldRedRule(rule Rule, tr TurnResult) *Violation {
	if !tr.ScaffoldExpected {
		return nil
	}
	// Signal 3 (B-11): static whitelist. Deterministic — fires even when the
	// suite is conveniently red because the smuggled logic is wrong.
	if tr.ScaffoldBodyNonStub {
		detail := "stub bodies carry real implementation logic (static whitelist violation) — strip every body back to signature-only stubs; the suite staying red does not make hidden logic legal"
		if len(tr.NonStubSymbols) > 0 {
			detail += "; offending symbols: " + strings.Join(tr.NonStubSymbols, ", ")
		}
		return &Violation{Rule: rule, Detail: detail}
	}
	// Signal 2: compile failure.
	if tr.ScaffoldCompileFailed {
		return &Violation{
			Rule:   rule,
			Detail: "the scaffold failed to compile — a compile error is not a red suite; fix the stubs'/tests' syntax and imports so the suite builds and then fails on its assertions",
		}
	}
	if !tr.Tests.Ran {
		return &Violation{
			Rule:   rule,
			Detail: "no test run was observed for this scaffold turn — create the stubs AND run the suite so the gate can see it RED",
		}
	}
	// Signal 1: all-green means the stubs were implemented.
	if len(tr.Tests.Failed) == 0 {
		return &Violation{
			Rule:   rule,
			Detail: "the suite passed, so the stubs contain real implementation — revert every body to the stub whitelist (TODO / throw not-implemented / return zero sentinel) and keep the signature surface unchanged",
		}
	}
	return nil
}

// ScaffoldSatisfiedDetail renders the evidence line recorded on a passing
// scaffold gate.
func ScaffoldSatisfiedDetail(tr TurnResult) string {
	if !tr.ScaffoldExpected {
		return ""
	}
	red := tr.Tests.Failed
	if len(red) == 0 {
		return ""
	}
	detail := fmt.Sprintf("scaffold ready: %d red test(s): %s", len(red), strings.Join(red, ", "))
	if tr.ScaffoldBodyNonStub {
		// unreachable on the pass path; kept for symmetry with the runner log.
		return ""
	}
	return detail
}
