package promptpacker

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// WriteAuditLog writes the prompt context audit record (prompt_context_audit,
// CP-23 Phase 1) as one JSON line: selected/dropped tokens, dropped items with
// their reasons, the per-section token usage map, and any over-budget
// warnings. The workflow-run log writer is the usual destination.
func WriteAuditLog(w io.Writer, report PromptAuditReport) error {
	if w == nil {
		return errors.New("audit writer is nil")
	}
	if report.DroppedItems == nil {
		report.DroppedItems = []string{}
	}
	if report.SectionUsage == nil {
		report.SectionUsage = map[string]int{}
	}
	if report.Warnings == nil {
		report.Warnings = []string{}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = w.Write(append(encoded, '\n'))
	return err
}

const (
	// maxCompactSkillLines caps the Core Rule bullet lines on a compact card
	// (Task-334 T-2: 3-5 dòng Core Rule).
	maxCompactSkillLines = 5
	// maxCompactSkillChars keeps a compact card under ~200 tokens at the
	// ~4 chars/token heuristic (Task-334 §10: token < 200 for a typical
	// ~3000-token skill).
	maxCompactSkillChars = 780
)

// CompactSkillCard extracts the Core Rule lines from a full SKILL.md
// (Task-334 T-2): the bullet lines under "## Always Do" / "## Core Rules"
// (both headings accepted, case-insensitive), falling back to the first 3-5
// bullet lines of the document when neither heading exists. The card is
// capped so it stays < 200 tokens for a typical ~3000-token skill.
func CompactSkillCard(fullSkillContent string) string {
	lines := strings.Split(fullSkillContent, "\n")
	bullets := coreRuleBullets(lines)
	if len(bullets) == 0 {
		bullets = firstBulletLines(lines)
	}
	if len(bullets) == 0 {
		return ""
	}
	card := strings.Join(bullets, "\n")
	if len(card) > maxCompactSkillChars {
		card = truncateToTokens(card, maxCompactSkillChars/4)
	}
	return card
}

// coreRuleBullets collects the bullet lines under the first "## Always Do" or
// "## Core Rules" heading, up to maxCompactSkillLines bullets; the next H1/H2
// heading ends the section.
func coreRuleBullets(lines []string) []string {
	start := -1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if isHeadingLevel(t, 2) && isCoreRulesHeading(t) {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	var bullets []string
	for _, line := range lines[start+1:] {
		t := strings.TrimSpace(line)
		if isHeadingLevel(t, 1) || isHeadingLevel(t, 2) {
			break
		}
		if isBulletLine(t) {
			bullets = append(bullets, t)
			if len(bullets) >= maxCompactSkillLines {
				break
			}
		}
	}
	return bullets
}

// firstBulletLines is the fallback when no Core Rules heading exists: the
// first bullet lines of the document, up to maxCompactSkillLines.
func firstBulletLines(lines []string) []string {
	var bullets []string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if isBulletLine(t) {
			bullets = append(bullets, t)
			if len(bullets) >= maxCompactSkillLines {
				break
			}
		}
	}
	return bullets
}

// isCoreRulesHeading reports whether an H2 heading names the Core Rules
// section ("Always Do" and "Core Rules" both accepted, case-insensitive).
func isCoreRulesHeading(heading string) bool {
	h := strings.ToLower(strings.TrimSpace(heading))
	return strings.Contains(h, "always do") || strings.Contains(h, "core rules")
}

// isHeadingLevel reports whether a trimmed line is exactly the given markdown
// heading level ("## " for level 2 — a deeper "### " line does not match).
func isHeadingLevel(line string, level int) bool {
	if level < 1 || level > 6 {
		return false
	}
	return strings.HasPrefix(line, strings.Repeat("#", level)+" ")
}

// isBulletLine reports whether a trimmed line is a list bullet (dash, star,
// plus, or ordered "N.").
func isBulletLine(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "+ ") {
		return true
	}
	dot := strings.Index(line, ". ")
	if dot <= 0 || dot > 4 {
		return false
	}
	for _, r := range line[:dot] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
