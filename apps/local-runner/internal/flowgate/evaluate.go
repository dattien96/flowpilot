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
		// Use WrittenPaths (files the AI tool-called) not GitDiff: the git working tree may
		// contain pre-existing dirty files or runner-internal state (e.g. test_baseline.json)
		// that are not changes the AI made and must not trigger a change-audit requirement.
		if HasCodeChangesInList(tr.WrittenPaths) && !HasChangeAuditNote(tr.GitDiff) {
			return &Violation{Rule: rule, Detail: "code changed but no change-audit note found"}
		}

	case "commit_feature_key_missing":
		if len(tr.CommitSubjects) == 0 {
			return nil
		}
		if !HasCodeChanges(tr.GitDiff) && !HasCodeChangesInList(tr.WrittenPaths) {
			return nil
		}
		if missingFeatureKey(tr.CommitSubjects, tr.KnownFeatureKeys) {
			return &Violation{
				Rule:        rule,
				Detail:      "code-changing turn committed without a verified feature key",
				Options:     append([]string(nil), tr.SuggestedFeatureKeys...),
				SourceDocID: tr.SourceDocID,
			}
		}

	case "bug_fixed":
		isBugFix := tr.ChangeType == "bugfix"
		if !isBugFix {
			msgLower := strings.ToLower(tr.FinalMessage)
			isBugFix = strings.Contains(msgLower, "fixed bug") ||
				strings.Contains(msgLower, "bug fix")
		}
		if isBugFix && !HasBugFixDoc(tr.GitDiff) {
			if tr.ChangeType == "bugfix" {
				return &Violation{
					Rule:        rule,
					Detail:      "declared bug mode but no bugfix document found",
					SourceDocID: tr.SourceDocID,
					Declared:    true,
				}
			}
			return &Violation{Rule: rule, Detail: "bug fix detected but no bugfix doc found"}
		}

	case "task_referenced":
		// Explicitly declared Task mode wins first. If the run already carries
		// ChangeType == "task", we do not consult the final-message regex at all.
		// Otherwise we fall back to the v1 Task-ID heuristic. (SD-20 §2.7, Task-113)
		hasTaskRef := tr.ChangeType == "task"
		if !hasTaskRef {
			hasTaskRef = taskIDRegex.MatchString(tr.FinalMessage)
		}
		if hasTaskRef && !HasTaskDoc(tr.GitDiff) {
			if tr.ChangeType == "task" {
				return &Violation{
					Rule:        rule,
					Detail:      "declared task mode but no task document found",
					SourceDocID: tr.SourceDocID,
					Declared:    true,
				}
			}
			return &Violation{Rule: rule, Detail: "task reference detected but no task document found"}
		}

	case "tests_failed":
		if tr.Tests.Ran && len(tr.Tests.Failed) > 0 {
			return &Violation{Rule: rule, Detail: "Tests failed: " + strings.Join(tr.Tests.Failed, ", ")}
		}

	case "regression_test_broke":
		if tr.Tests.Ran && len(tr.Tests.Failed) > 0 {
			return &Violation{
				Rule:           rule,
				Detail:         "Tests failed: " + strings.Join(tr.Tests.Failed, ", "),
				Options:        []string{"keep-test-fix-code", "suggest-requirement-change", "custom"},
				RegressedTests: tr.Tests.Failed,
			}
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

func missingFeatureKey(commitSubjects []string, knownKeys []string) bool {
	if len(commitSubjects) == 0 {
		return true
	}
	for _, subject := range commitSubjects {
		key, ok := declaredFeatureKey(subject)
		if !ok || !isKnownFeatureKey(key, knownKeys) {
			return true
		}
	}
	return false
}

func declaredFeatureKey(subject string) (string, bool) {
	subject = strings.TrimSpace(subject)
	if !strings.HasPrefix(subject, "[") {
		return "", false
	}
	endType := strings.Index(subject, "]")
	if endType < 0 {
		return "", false
	}
	rest := strings.TrimSpace(subject[endType+1:])
	if !strings.HasPrefix(rest, "[") {
		return "", false
	}
	endFeature := strings.Index(rest, "]")
	if endFeature < 0 {
		return "", false
	}
	key := strings.ToLower(strings.TrimSpace(rest[1:endFeature]))
	key = strings.ReplaceAll(key, " ", "-")
	key = strings.ReplaceAll(key, "_", "-")
	for strings.Contains(key, "--") {
		key = strings.ReplaceAll(key, "--", "-")
	}
	return strings.Trim(key, "-"), strings.Trim(key, "-") != ""
}

func isKnownFeatureKey(key string, knownKeys []string) bool {
	for _, known := range knownKeys {
		if known == key {
			return true
		}
	}
	return false
}
