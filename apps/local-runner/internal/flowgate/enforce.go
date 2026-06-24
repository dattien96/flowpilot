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
