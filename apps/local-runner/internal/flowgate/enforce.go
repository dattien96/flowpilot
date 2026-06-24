package flowgate

import "strings"

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
	return trigger == "regression_test_broke" || trigger == "tests_failed"
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
	return "The flow gate is asking you to add a required document before this step can complete:\n\n" +
		strings.Join(parts, "\n\n") +
		"\n\nCreate the file(s) above now. The gate re-checks automatically after your next turn."
}

// remediationFor maps a reprompt violation to a concrete, file-level instruction.
func remediationFor(v Violation) string {
	switch v.Rule.Trigger {
	case "bug_fixed":
		return "• Missing BugFix document. You fixed a bug but did not add its BugFix doc. " +
			"Create a NEW file `requirements/09-BugFix/done/BUG-<NNN>.md` " +
			"(use the next available zero-padded number) following the structure in " +
			"`requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md`. " +
			"Do NOT edit the change-audit note to satisfy this — the BugFix document is a separate, required artifact."
	case "task_referenced":
		return "• Missing Task document. Your final message references a Task-NNN but you did not add its Task doc. " +
			"Create a NEW file `requirements/08-Task/done/Task-<NNN>.md` " +
			"(use the same Task number you referenced; use the next available zero-padded number if new) " +
			"following the structure in `requirements/08-Task/FORMAT-REFERENCE-TASK.md`. " +
			"Do NOT edit the change-audit note to satisfy this — the Task document is a separate, required artifact."
	case "code_changed":
		return "• Missing change-audit note. You changed code but did not add a change-audit note. " +
			"Create a NEW file `change-audit/CA-<NNN>.md` recording what changed and why."
	default:
		return "• " + v.Detail
	}
}
