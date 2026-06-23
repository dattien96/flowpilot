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
		details = append(details, v.Detail)
	}

	return EnforceResult{
		Action:     highest,
		Violations: violations,
		Message:    "Flow gate: " + strings.Join(details, "; "),
	}
}
