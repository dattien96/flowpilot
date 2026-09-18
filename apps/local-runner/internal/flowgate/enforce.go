package flowgate

import "strings"

// GateRepromptPrefix is the leading sentence of every gate reprompt (see
// RepromptPrompt). The runner matches this prefix to recognise a system reprompt
// turn so feature resolution treats it as a continuation of the conversation's
// established feature, rather than resolving on the reprompt's own
// process-describing text (which names feature keys and writes change-audit files
// and would otherwise mis-resolve the turn).
const GateRepromptPrefix = "The flow gate is asking you to add a required document before this step can complete:"

type EnforceResult struct {
	Action     string      `json:"action"`
	Violations []Violation `json:"violations,omitempty"`
	Message    string      `json:"message,omitempty"`
}

var actionSeverity = map[string]int{
	"approve":  0,
	"warn":     1,
	"reprompt": 2,
	"block":    3,
}

func isAlwaysBlock(trigger string) bool {
	return trigger == "regression_test_broke" || trigger == "tests_failed" || trigger == "requirement_signature_drift"
}

func Enforce(violations []Violation, gateMode string) EnforceResult {
	if len(violations) == 0 {
		return EnforceResult{Action: "pass"}
	}

	highest := "approve"
	var details []string
	seen := map[string]bool{}

	for _, v := range violations {
		action := v.Rule.Action

		if isAlwaysBlock(v.Rule.Trigger) {
			action = "block"
		} else if gateMode == "warn" {
			if action == "block" || action == "reprompt" {
				action = "warn"
			}
		}

		if actionSeverity[action] > actionSeverity[highest] {
			highest = action
		}
		// Dedupe identical details: r-tests and r-reg are coupled in v1 (both fire on
		// the same failing-test condition with an identical Detail), which would
		// otherwise render the message twice ("Tests failed: X; Tests failed: X").
		if !seen[v.Detail] {
			seen[v.Detail] = true
			details = append(details, v.Detail)
		}
	}

	return EnforceResult{
		Action:     highest,
		Violations: violations,
		Message:    "Flow gate: " + strings.Join(details, "; "),
	}
}

// RepromptPrompt builds the actionable instruction sent to the AI when the gate
// resolves to "reprompt". The inline desktop card uses the terse EnforceResult.Message
// (a symptom, e.g. "bug fix detected but no bugfix doc found"), but that is NOT enough
// for the AI to self-correct — observed in E2E-11, where the AI edited the change-audit
// note instead of creating the required BugFix document. This returns explicit,
// per-rule remediation steps naming the exact file to create. (BUG-140)
func RepromptPrompt(result EnforceResult) string {
	var parts []string
	for _, v := range result.Violations {
		// Only the auto-remediable (reprompt) rules get guidance; block/warn rules
		// never reach the reprompt branch.
		if v.Rule.Action != "reprompt" {
			continue
		}
		parts = append(parts, remediationFor(v))
	}
	if len(parts) == 0 {
		return result.Message
	}
	return GateRepromptPrefix + "\n\n" +
		strings.Join(parts, "\n\n") +
		"\n\nCreate the file(s) above now. The gate re-checks automatically after your next turn."
}

// remediationFor maps a reprompt violation to a concrete, file-level instruction.
func remediationFor(v Violation) string {
	switch v.Rule.Trigger {
	case "bug_fixed":
		if v.Declared {
			if v.SourceDocID != "" {
				return "• Missing BugFix document. This chat started in Bug mode, so the gate expects its BugFix doc even if the final message does not say \"bug fix\". " +
					"Create a NEW file `requirements/09-BugFix/done/" + v.SourceDocID + "-<short-title>.md` " +
					"following the structure in `requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md`. " +
					"Do NOT edit the change-audit note to satisfy this — the BugFix document is a separate, required artifact."
			}
			return "• Missing BugFix document. This chat started in Bug mode, so the gate expects its BugFix doc even if the final message does not say \"bug fix\". " +
				"Create a NEW file `requirements/09-BugFix/done/BUG-<next-available>-<short-title>.md` " +
				"(use the next available zero-padded BUG number) following the structure in " +
				"`requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md`. " +
				"Do NOT edit the change-audit note to satisfy this — the BugFix document is a separate, required artifact."
		}
		return "• Missing BugFix document. You fixed a bug but did not add its BugFix doc. " +
			"Create a NEW file `requirements/09-BugFix/done/BUG-<NNN>.md` " +
			"(use the next available zero-padded number) following the structure in " +
			"`requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md`. " +
			"Do NOT edit the change-audit note to satisfy this — the BugFix document is a separate, required artifact."
	case "task_referenced":
		if v.Declared {
			if v.SourceDocID != "" {
				return "• Missing Task document. This chat started in Task mode, so the gate expects its Task doc even if the final message does not mention a Task id. " +
					"Create a NEW file `requirements/08-Task/done/" + v.SourceDocID + "-<short-title>.md` " +
					"following the structure in `requirements/08-Task/FORMAT-REFERENCE-TASK.md`. " +
					"Do NOT edit the change-audit note to satisfy this — the Task document is a separate, required artifact."
			}
			return "• Missing Task document. This chat started in Task mode, so the gate expects its Task doc even if the final message does not mention a Task id. " +
				"Create a NEW file `requirements/08-Task/done/Task-<next-available>-<short-title>.md` " +
				"(use the next available zero-padded Task number) following the structure in `requirements/08-Task/FORMAT-REFERENCE-TASK.md`. " +
				"Do NOT edit the change-audit note to satisfy this — the Task document is a separate, required artifact."
		}
		return "• Missing Task document. Your final message references a Task-NNN but you did not add its Task doc. " +
			"Create a NEW file `requirements/08-Task/done/Task-<NNN>.md` " +
			"(use the same Task number you referenced; use the next available zero-padded number if new) " +
			"following the structure in `requirements/08-Task/FORMAT-REFERENCE-TASK.md`. " +
			"Do NOT edit the change-audit note to satisfy this — the Task document is a separate, required artifact."
	case "code_changed":
		return "• Missing change-audit note. You changed code but did not add a change-audit note. " +
			"Create a NEW file `change-audit/CA-<NNN>.md` recording what changed and why."
	case "commit_feature_key_missing":
		shortlist := ""
		if len(v.Options) > 0 {
			shortlist = "Suggested feature keys: " + strings.Join(v.Options, ", ") + ". "
		}
		return "• Missing or unverified feature key. " + shortlist +
			"Use a verified key from `change-audit/FEATURE-KEYS.md`, or register a new one there first if none fits. " +
			"Then update the commit subject to use `[Type][feature][layer?]` before retrying."
	case "required_artifact_output_missing":
		// Detail already names the missing paths from Evaluate.
		return "• Missing required file artifact output(s). " + v.Detail + ". " +
			"Create or update each listed workspace-relative path with your tools now " +
			"(this is a file_artifact.v1 OUTPUT write contract — chat text alone is not enough). " +
			"Do not invent other paths; write exactly the bound artifact path(s)."
	case "code_changed_no_contract":
		return "• Missing Change Contract. Before your next edit, start your response with:\n\n" +
			"  [Change Contract]\n  feature: <feature_key>\n  intent: <one-line intended behavior change>\n  files: <comma-separated paths you expect to touch>\n\n" +
			"This does not block the current turn — it lets the gate flag edits that land outside what you declared."
	case "required_artifact_output_structure_missing":
		// Task-225: files exist but lack declared structure.sections headings.
		return "• Required file artifact structure incomplete. " + v.Detail + ". " +
			"Update each listed existing file in place and add the missing markdown section headings " +
			"(ATX headings such as `## SectionName` — level and casing may vary). " +
			"Do not invent new paths; edit the bound artifact path(s) only."
	case "production_change_no_new_test":
		return "• Missing new test coverage. You changed production code but did not ADD a new test file in this turn. " +
			"Create a NEW `*_test.go` / `*.test.ts` file with a failing-or-asserting case derived from the acceptance criteria. " +
			"Do NOT edit or delete existing tests to make the gate pass (oracle-rule / additive-tests-only)."
	case "pre_existing_test_edited":
		// Task-260: actionable guidance for additive-tests-only + oracle-rule. Detail already
		// lists the tampered path(s) from Evaluate.
		return "• " + v.Detail + ". " +
			"safe-fix-contract / additive-tests-only / oracle-rule: do not modify legacy tests without user approval. " +
			"1. Revert edits to those files (git checkout / git restore). " +
			"2. Add NEW regression tests in a new file (or new cases in a new file only). " +
			"3. Fix production code so old + new tests pass. " +
			"4. If an old test is truly wrong vs AC: stop and ask the user — do not silent-edit. " +
			"Do NOT edit change-audit solely to dismiss this rule."
	case "reproduce_not_demonstrated":
		// CP-64 P-1: reproduce-first remediation. The Detail already states
		// which failure shape was seen (compile error / green suite / no run).
		return "• Reproduce-first gate: " + v.Detail + ". " +
			"Write (or fix) ONE new test file that reproduces the reported bug " +
			"and run the test suite yourself before finishing this turn. " +
			"The gate only passes when the suite COMPILES and the new test FAILS " +
			"on its assertion (expected X, got Y). " +
			"Do not touch production code yet, do not weaken or edit existing tests, " +
			"and do not delete the failing assertion to make the suite green."
	default:
		return "• " + v.Detail
	}
}
