package flowgate

import (
	"regexp"
	"strings"
)

var taskIDRegex = regexp.MustCompile(`\bTask-\d+\b`)

func Evaluate(tr TurnResult, rules []Rule) []Violation {
	var violations []Violation
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		v := checkRule(rule, tr)
		if v != nil {
			violations = append(violations, *v)
		}
	}
	return violations
}

func checkRule(rule Rule, tr TurnResult) *Violation {
	switch rule.Trigger {
	case "code_changed":
		if HasCodeChanges(tr.GitDiff) && !HasChangeAuditNote(tr.GitDiff) {
			return &Violation{Rule: rule, Detail: "code changed but no change-audit note found"}
		}

	case "bug_fixed":
		msgLower := strings.ToLower(tr.FinalMessage)
		isBugFix := tr.ChangeType == "bugfix" ||
			strings.Contains(msgLower, "fixed bug") ||
			strings.Contains(msgLower, "bug fix")
		if isBugFix && !HasBugFixDoc(tr.GitDiff) {
			return &Violation{Rule: rule, Detail: "bug fix detected but no bugfix doc found"}
		}

	case "task_referenced":
		// Fires when the AI's final message references a Task-NNN ID (meaning the AI
		// is completing a tracked task) but no Task document was added to the diff.
		// ChangeType == "task" is reserved for a future explicit signal; the message
		// heuristic covers v1. (SD-20 §2.7, Task-113)
		hasTaskRef := taskIDRegex.MatchString(tr.FinalMessage) || tr.ChangeType == "task"
		if hasTaskRef && !HasTaskDoc(tr.GitDiff) {
			return &Violation{Rule: rule, Detail: "task reference detected but no task document found"}
		}

	case "tests_failed":
		if tr.Tests.Ran && len(tr.Tests.Failed) > 0 {
			return &Violation{Rule: rule, Detail: "Tests failed: " + strings.Join(tr.Tests.Failed, ", ")}
		}

	case "regression_test_broke":
		if tr.Tests.Ran && len(tr.Tests.Failed) > 0 {
			return &Violation{Rule: rule, Detail: "Tests failed: " + strings.Join(tr.Tests.Failed, ", ")}
		}

	case "removed_referenced_code":
		for _, f := range tr.GitDiff {
			if f.Status == "D" && strings.HasSuffix(f.Path, ".go") {
				return &Violation{Rule: rule, Detail: "Removed: " + f.Path}
			}
		}
	}
	return nil
}
