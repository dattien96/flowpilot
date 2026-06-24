package runner

import "strings"

func normalizeChangeType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "task":
		return "task"
	case "bugfix", "bug":
		return "bugfix"
	default:
		return ""
	}
}

func buildModePrefix(changeType, sourceDocID string) string {
	id := strings.TrimSpace(sourceDocID)
	switch normalizeChangeType(changeType) {
	case "task":
		if id == "" {
			id = "Task-<NNN>"
		}
		return "[Flow context: You are completing " + id + ". " +
			"When your work is done you MUST create a NEW file " +
			"`requirements/08-Task/done/" + id + "-<short-title>.md` " +
			"following the structure in `requirements/08-Task/FORMAT-REFERENCE-TASK.md`. " +
			"Do NOT use the FORMAT-REFERENCE file itself as the artifact. " +
			"The gate re-checks after your turn.]\n\n"
	case "bugfix":
		if id == "" {
			id = "BUG-<NNN>"
		}
		return "[Flow context: You are fixing " + id + ". " +
			"When your fix is done you MUST create a NEW file " +
			"`requirements/09-BugFix/done/" + id + "-<short-title>.md` " +
			"following the structure in `requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md`. " +
			"Do NOT edit the change-audit note to satisfy this. " +
			"The gate re-checks after your turn.]\n\n"
	default:
		return ""
	}
}

func prependModePrefix(prompt string, turnCount int, changeType, sourceDocID string) string {
	if turnCount != 1 {
		return prompt
	}
	if prefix := buildModePrefix(changeType, sourceDocID); prefix != "" {
		return prefix + prompt
	}
	return prompt
}
