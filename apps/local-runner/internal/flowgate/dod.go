package flowgate

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Task-330 (CP-47 P-1): deterministic markdown parsing of the
// "## Definition of Done" section (SS-13 §10) — pure Go, offline,
// 0 LLM tokens. Shared by r-dod-present (Task-330) and r-dod-complete
// (Task-331).

type DodStatus struct {
	Present   bool     `json:"present"`
	Total     int      `json:"total"`
	Checked   int      `json:"checked"`
	OpenItems []string `json:"open_items,omitempty"`
}

var (
	dodHeadingRegex   = regexp.MustCompile(`(?i)^##\s+(definition\s+of\s+done|dod)\b`)
	anyHeadingRegex   = regexp.MustCompile(`^##+\s+`)
	checkboxItemRegex = regexp.MustCompile(`^\s*-\s*\[([ xX])\]\s*(.+)$`)
)

// ParseDefinitionOfDone quét nội dung markdown để bóc tách checklist DOD:
// tìm heading "## Definition of Done" (hoặc "## DoD", không phân biệt hoa
// thường), đọc từng dòng cho đến khi gặp heading markdown kế tiếp, đếm
// "- [ ]" (chưa xong) và "- [x]"/"- [X]" (đã xong). Checkbox thụt đầu dòng
// vẫn được đếm. Section có heading nhưng 0 checkbox → Total = 0 (coi như
// thiếu), và Present = (Total > 0).
func ParseDefinitionOfDone(content string) DodStatus {
	var status DodStatus
	inSection := false
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if dodHeadingRegex.MatchString(line) {
			inSection = true
			continue
		}
		if anyHeadingRegex.MatchString(line) {
			if inSection {
				break // heading markdown kế tiếp đóng section DOD
			}
			continue
		}
		if !inSection {
			continue
		}
		m := checkboxItemRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		status.Total++
		if m[1] == "x" || m[1] == "X" {
			status.Checked++
			continue
		}
		status.OpenItems = append(status.OpenItems, strings.TrimSpace(m[2]))
	}
	status.Present = status.Total > 0
	return status
}

// isTaskOrBugDocPath reports whether path is a markdown document targeting a
// Task or BugFix document: base name "Task-*.md" or "BUG-*.md" (Task-330 T-3).
// FORMAT-REFERENCE-* scaffolds never carry either prefix, so they are excluded
// implicitly (same rationale as HasTaskDoc / BUG-141).
func isTaskOrBugDocPath(path string) bool {
	base := filepath.Base(filepath.FromSlash(strings.TrimSpace(path)))
	if !strings.HasSuffix(base, ".md") {
		return false
	}
	return strings.HasPrefix(base, "Task-") || strings.HasPrefix(base, "BUG-")
}

// MissingDodDocs returns the writtenPaths entries that are Task-*/BUG-*.md
// documents whose on-disk content has no Definition of Done checklist
// (DodStatus.Total == 0) — Task-330 (CP-47 P-2). Mirrors
// MissingRequiredFileArtifactOutputs: empty workspace or empty list yields
// nil, and files that cannot be read are skipped gracefully (never panic —
// an unreadable doc must not hard-fail the reprompt-only gate).
func MissingDodDocs(workspaceCwd string, writtenPaths []string) []string {
	workspaceCwd = strings.TrimSpace(workspaceCwd)
	if workspaceCwd == "" || len(writtenPaths) == 0 {
		return nil
	}
	root, err := filepath.Abs(workspaceCwd)
	if err != nil {
		return nil
	}
	var missing []string
	seen := make(map[string]bool, len(writtenPaths))
	for _, p := range writtenPaths {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] || !isTaskOrBugDocPath(p) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			continue // unreadable/missing file → graceful skip
		}
		if ParseDefinitionOfDone(string(data)).Total == 0 {
			seen[p] = true
			missing = append(missing, p)
		}
	}
	return missing
}
