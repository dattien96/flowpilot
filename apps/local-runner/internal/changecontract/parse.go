package changecontract

import (
	"regexp"
	"strings"
)

// declarationMarker is the sentinel line the context-discipline skill (T-5)
// instructs the AI to emit before it starts editing. Matched case-insensitively,
// with or without a surrounding ```change-contract fenced block.
var declarationMarker = regexp.MustCompile(`(?i)^\s*\[change contract\]\s*$`)

var (
	featureLineRe = regexp.MustCompile(`(?i)^\s*feature\s*:\s*(.+?)\s*$`)
	intentLineRe  = regexp.MustCompile(`(?i)^\s*intent\s*:\s*(.+?)\s*$`)
	filesLineRe   = regexp.MustCompile(`(?i)^\s*files\s*:\s*(.+?)\s*$`)
)

// ParseDeclaration extracts a Change Contract declaration block from text
// (an assistant turn's message). The block is a `[Change Contract]` marker
// line (optionally inside a ```change-contract fenced block) followed by
// `feature:`/`intent:`/`files:` lines in any order; the block ends at the
// first blank line, closing code fence, or end of text.
//
// Returns ok=false (not an error) when no marker is present. A present block
// with some lines missing is tolerated — a partial Contract is still
// returned with Confidence=ConfidenceDeclared, since the AI did declare.
func ParseDeclaration(text string) (Contract, bool) {
	lines := strings.Split(text, "\n")

	markerIdx := -1
	for i, line := range lines {
		if declarationMarker.MatchString(line) {
			markerIdx = i
			break
		}
	}
	if markerIdx == -1 {
		return Contract{}, false
	}

	c := Contract{Confidence: ConfidenceDeclared}
	for _, line := range lines[markerIdx+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "```" {
			break // blank line or closing fence ends the block
		}
		if m := featureLineRe.FindStringSubmatch(line); m != nil {
			c.FeatureKey = m[1]
			continue
		}
		if m := intentLineRe.FindStringSubmatch(line); m != nil {
			c.Intent = m[1]
			continue
		}
		if m := filesLineRe.FindStringSubmatch(line); m != nil {
			c.DeclaredPaths = splitAndTrim(m[1])
			continue
		}
		// Unrecognized line inside the block (e.g. a stray fence opener) —
		// tolerate it and keep scanning rather than aborting the parse.
	}
	return c, true
}

// splitAndTrim splits a comma-separated `files:` value into trimmed,
// non-empty entries.
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
