package workingmode

import (
	"path"
	"regexp"
	"strings"
)

var cpDocumentID = regexp.MustCompile("(?i)Document ID:\\s*`?CP-[0-9]+")

// IsCodingPlanCPPath reports whether p is requirements/07-Coding-Plan/**/CP-*.md.
func IsCodingPlanCPPath(p string) bool {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	if p == "" || !strings.HasSuffix(strings.ToLower(p), ".md") {
		return false
	}
	base := strings.ToUpper(path.Base(p))
	if !strings.HasPrefix(base, "CP-") {
		return false
	}
	lower := strings.ToLower(p)
	return strings.Contains(lower, "/07-coding-plan/") || strings.HasPrefix(lower, "07-coding-plan/") ||
		strings.Contains(lower, "requirements/07-coding-plan/")
}

// HasCPDocumentID reports SS-13 CP frontmatter Document ID: CP-*.
func HasCPDocumentID(content string) bool {
	return cpDocumentID.MatchString(content)
}

// RejectNonCP is the deterministic /vibe-cp rejection (path + optional body).
func RejectNonCP(p, content string) error {
	if !IsCodingPlanCPPath(p) {
		return errCode(CodeInvalidFlowRef, "vibe-cp requires requirements/07-Coding-Plan/**/CP-*.md")
	}
	if strings.TrimSpace(content) != "" && !HasCPDocumentID(content) {
		return errCode(CodeInvalidFlowRef, "vibe-cp requires Document ID: CP-*")
	}
	return nil
}

// DetectVibeEntry chooses vibe-cp-ingest vs vibe-ingest from a path or prompt.
func DetectVibeEntry(pathOrPrompt string) (flowID, source string) {
	raw := strings.TrimSpace(pathOrPrompt)
	if raw == "" {
		return "vibe-ingest", ""
	}
	if IsCodingPlanCPPath(raw) {
		return "vibe-cp-ingest", raw
	}
	first := strings.Fields(raw)
	if len(first) > 0 && IsCodingPlanCPPath(first[0]) {
		return "vibe-cp-ingest", first[0]
	}
	return "vibe-ingest", raw
}
