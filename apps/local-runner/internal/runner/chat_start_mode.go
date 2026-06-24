package runner

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var sourceDocIDRegex = regexp.MustCompile(`\b(?:Task|BUG)-(\d+)\b`)

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
			id = "Task-<next-available>"
		}
		return "[Flow context: You are completing " + id + ". " +
			"When your work is done you MUST create a NEW file " +
			"`requirements/08-Task/done/" + id + "-<short-title>.md` " +
			"following the structure in `requirements/08-Task/FORMAT-REFERENCE-TASK.md`. " +
			"If this id was auto-assigned, keep using exactly that Task id. " +
			"Do NOT use the FORMAT-REFERENCE file itself as the artifact. " +
			"The gate re-checks after your turn.]\n\n"
	case "bugfix":
		if id == "" {
			id = "BUG-<next-available>"
		}
		return "[Flow context: You are fixing " + id + ". " +
			"When your fix is done you MUST create a NEW file " +
			"`requirements/09-BugFix/done/" + id + "-<short-title>.md` " +
			"following the structure in `requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md`. " +
			"If this id was auto-assigned, keep using exactly that Bug id. " +
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

func resolveSourceDocID(workspaceCwd, changeType, sourceDocID string) string {
	id := strings.TrimSpace(sourceDocID)
	if id != "" {
		return id
	}
	switch normalizeChangeType(changeType) {
	case "task":
		return nextAvailableSourceDocID(workspaceCwd, "requirements/08-Task", "Task")
	case "bugfix":
		return nextAvailableSourceDocID(workspaceCwd, "requirements/09-BugFix", "BUG")
	default:
		return ""
	}
}

func nextAvailableSourceDocID(workspaceCwd, relRoot, prefix string) string {
	if workspaceCwd == "" {
		return prefix + "-001"
	}
	root := filepath.Join(workspaceCwd, relRoot)
	maxValue := 0
	maxWidth := 3
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		for _, match := range sourceDocIDRegex.FindAllStringSubmatch(d.Name(), -1) {
			if len(match) < 2 || !strings.HasPrefix(match[0], prefix+"-") {
				continue
			}
			value, convErr := strconv.Atoi(match[1])
			if convErr != nil {
				continue
			}
			if value > maxValue {
				maxValue = value
			}
			if len(match[1]) > maxWidth {
				maxWidth = len(match[1])
			}
		}
		return nil
	})
	nextValue := maxValue + 1
	width := maxWidth
	if width < 3 {
		width = 3
	}
	return fmt.Sprintf("%s-%0*d", prefix, width, nextValue)
}
