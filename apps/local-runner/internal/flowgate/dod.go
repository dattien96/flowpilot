package flowgate

import (
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
	// dodHeadingRegex recognizes the DOD heading variants real Task-*/BUG-*
	// documents use (review rounds 2-3, real-repo scans): unnumbered h2
	// ("## Definition of Done" / "## DoD"), numbered h2 ("## 9. Definition of
	// Done"), and h3 sub-section form with multi-part numbering
	// ("### 6.1 Definition of Done (DOD)", "### 6.2 Definition of Done").
	dodHeadingRegex = regexp.MustCompile("(?i)^#{2,3}\\s+(?:\\d+(?:\\.\\d+)*[.)]?\\s*)?(definition\\s+of\\s+done|dod)\\b")
	// dodParenRegex additionally accepts h2/h3 headings that carry the DOD
	// phrase parenthesized ("## 6. Acceptance Check (Definition of Done)").
	dodParenRegex = regexp.MustCompile("(?i)^#{2,3}\\s+.*\\(definition\\s+of\\s+done\\)")
	// acceptanceCheckHeadingRegex accepts the spec-mandated acceptance
	// checklist section (BUG-395): plan-task.md, task-splitter.md and
	// FORMAT-REFERENCE-TASK.md all define the Task/BUG checkbox list under
	// "## 6. Acceptance Check" (numbered h2/h3 variants) — never a literal
	// "Definition of Done" heading. The acceptance checklist IS the document's
	// definition-of-done contract, so its checkboxes satisfy r-dod-present.
	// A heading with zero checkboxes still reports Present=false (Total==0).
	acceptanceCheckHeadingRegex = regexp.MustCompile("(?i)^#{2,3}\\s+(?:\\d+(?:\\.\\d+)*[.)]?\\s*)?acceptance\\s+(check|criteria)\\b")
	// anyHeadingRegex closes the DOD section: only level-1/level-2 headings
	// end it. Real repo DOD sections legitimately contain ### sub-headings
	// between checklist items (review round 2, real-repo scan) — those must
	// NOT terminate the section.
	anyHeadingRegex   = regexp.MustCompile(`^#{1,2}\s+`)
	checkboxItemRegex = regexp.MustCompile(`^\s*-\s*\[([ xX])\]\s*(.+)$`)
)

// ParseDefinitionOfDone quét nội dung markdown để bóc tách checklist DOD:
// tìm heading "## Definition of Done" (hoặc "## DoD", không phân biệt hoa
// thường), đọc từng dòng cho đến khi gặp heading markdown kế tiếp, đếm
// "- [ ]" (chưa xong) và "- [x]"/"- [X]" (đã xong). Checkbox thụt đầu dòng
// vẫn được đếm. Section có heading nhưng 0 checkbox → Total = 0 (coi như
// thiếu), và Present = (Total > 0).
//
// Hardening (review CA-833 follow-up): các dòng nằm trong fenced code block
// (``` hoặc ~~~) bị bỏ qua — một ví dụ DOD quote trong fence không được tính
// là DOD thật và không thể "game" cổng; heading/checkbox trong fence cũng
// không mở/đếm section. Nội dung dùng strings.Split (không còn giới hạn
// 64KB/line của bufio.Scanner từng gây truncate im lặng).
func ParseDefinitionOfDone(content string) DodStatus {
	var status DodStatus
	inSection := false
	var fenceMarker string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			if fenceMarker == "" {
				fenceMarker = marker
			} else if strings.HasPrefix(marker, fenceMarker) {
				fenceMarker = ""
			}
			continue
		}
		if fenceMarker != "" {
			continue // dòng trong fenced code block: bỏ qua hoàn toàn
		}
		if dodHeadingRegex.MatchString(line) || dodParenRegex.MatchString(line) || acceptanceCheckHeadingRegex.MatchString(line) {
			inSection = true
			continue
		}
		if anyHeadingRegex.MatchString(line) {
			// BUG-395: close the current section but keep scanning — a doc may
			// carry the checklist under a LATER matching heading (e.g.
			// "## 6. Acceptance Check" prose followed by "## Definition of
			// Done" checkboxes, or the reverse). Items across all matching
			// sections union into one DodStatus.
			inSection = false
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
// nil, and files that cannot be read are skipped gracefully (no panic) —
// an unreadable doc must not hard-fail the reprompt-only gate. Absolute
// paths and `..`/symlink escapes are rejected up-front (defense-in-depth,
// same guard as MissingRequiredFileArtifactOutputs) and treated as missing.
func MissingDodDocs(workspaceCwd string, writtenPaths []string) []string {
	workspaceCwd = strings.TrimSpace(workspaceCwd)
	if workspaceCwd == "" || len(writtenPaths) == 0 {
		return nil
	}
	root, err := filepath.Abs(workspaceCwd)
	if err != nil {
		return nil
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	var missing []string
	seen := make(map[string]bool, len(writtenPaths))
	for _, p := range writtenPaths {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] || !isTaskOrBugDocPath(p) {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(p))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			seen[p] = true
			missing = append(missing, p)
			continue
		}
		abs := filepath.Join(root, clean)
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
		if !strings.HasPrefix(abs, root+string(filepath.Separator)) && abs != root {
			seen[p] = true
			missing = append(missing, p)
			continue
		}
		data, err := os.ReadFile(abs)
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
