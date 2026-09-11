package skilllearn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// SkillExporter renders a LessonCandidate as a standard SKILL.md (YAML
// frontmatter with name + description, a "#" title, rule sections and an
// example — Task-336 DOD item 4) and writes it to the destination selected by
// SkillExportOptions. Rendering is deterministic: identical candidate →
// byte-identical content (provider-agnostic parity evidence).
//
// The zero value is ready to use.
type SkillExporter struct{}

// maxSlugLen caps generated skill directory names.
const maxSlugLen = 48

// skillFileName is the standard FlowPilot skill file name.
const skillFileName = "SKILL.md"

// Export renders the candidate and writes SKILL.md to every configured
// destination, returning the written file paths. Duplicate slugs are versioned
// (<slug>-v2, -v3, ...) per destination — an existing file is never
// overwritten. Conflict detection (DOD item 10) runs first: a candidate whose
// content contradicts the safe-fix-contract / additive-tests-only core skills
// fails with *ConflictError unless opts.Force acknowledges it.
func (SkillExporter) Export(candidate LessonCandidate, opts SkillExportOptions) ([]string, error) {
	if err := validateExportInput(candidate, opts); err != nil {
		return nil, err
	}
	if !opts.Force {
		if reason, conflict := DetectCoreSkillConflict(candidate); conflict {
			return nil, &ConflictError{CandidateID: candidate.ID, Reason: reason}
		}
	}

	baseDirs, err := exportBaseDirs(opts)
	if err != nil {
		return nil, err
	}

	group := strings.TrimSpace(candidate.Group)
	if group == "" {
		group = "common"
	}
	content := ""

	written := make([]string, 0, len(baseDirs))
	for _, baseDir := range baseDirs {
		slug, dir, err := reserveSkillDir(baseDir, group, candidate.Title)
		if err != nil {
			return written, err
		}
		if content == "" {
			content = RenderSkillMarkdownWithSlug(candidate, slug)
		}
		if err := os.WriteFile(filepath.Join(dir, skillFileName), []byte(content), 0o644); err != nil {
			return written, fmt.Errorf("skilllearn: write %s: %w", filepath.Join(dir, skillFileName), err)
		}
		written = append(written, filepath.Join(dir, skillFileName))
	}
	return written, nil
}

// validateExportInput enforces the preconditions: a non-empty existing
// workspace root (clear error, never a panic — DOD item 7 test) and an
// exportable candidate status (T-1: a rejected or already-promoted candidate
// must not be exported by callers that bypass PromoterService).
func validateExportInput(candidate LessonCandidate, opts SkillExportOptions) error {
	root := strings.TrimSpace(opts.WorkspaceRoot)
	if root == "" {
		return fmt.Errorf("skilllearn: workspace root is required")
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("skilllearn: workspace root %q does not exist or is not accessible", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("skilllearn: workspace root %q is not a directory", root)
	}
	if strings.TrimSpace(candidate.Title) == "" {
		return fmt.Errorf("skilllearn: candidate %q has no title", candidate.ID)
	}
	switch candidate.Status {
	case "", StatusCandidate, StatusApproved:
		return nil
	case StatusRejected:
		return fmt.Errorf("skilllearn: candidate %q was rejected and cannot be exported", candidate.ID)
	case StatusPromoted:
		return fmt.Errorf("skilllearn: candidate %q was already promoted", candidate.ID)
	default:
		return fmt.Errorf("skilllearn: candidate %q has unknown status %q", candidate.ID, candidate.Status)
	}
}

// exportBaseDirs resolves the destination base directories:
//   - target project mode: <WorkspaceRoot>/<skill-dir> for each entry of
//     opts.SkillDirs (default [".agents/skills"]);
//   - core mode: opts.FlowPackRoot, or
//     <WorkspaceRoot>/internal/skillpack/flow-pack when unset.
func exportBaseDirs(opts SkillExportOptions) ([]string, error) {
	if !opts.ExportToCore {
		skillDirs := opts.SkillDirs
		if len(skillDirs) == 0 {
			skillDirs = []string{defaultProjectSkillDir}
		}
		dirs := make([]string, 0, len(skillDirs))
		for _, skillDir := range skillDirs {
			clean := filepath.Clean(strings.TrimSpace(skillDir))
			if clean == "." || filepath.IsAbs(clean) {
				return nil, fmt.Errorf("skilllearn: invalid skill dir %q", skillDir)
			}
			dirs = append(dirs, filepath.Join(opts.WorkspaceRoot, clean))
		}
		return dirs, nil
	}

	coreRoot := strings.TrimSpace(opts.FlowPackRoot)
	if coreRoot == "" {
		coreRoot = filepath.Join(opts.WorkspaceRoot, "internal", "skillpack", "flow-pack")
	}
	return []string{coreRoot}, nil
}

// reserveSkillDir picks the destination directory for the skill inside
// <baseDir>/<group>/<slug> versioning the slug when the SKILL.md already
// exists (never overwrite — Task-336 Open Question) and creating the
// directory. Returns the reserved slug (with any -vN suffix so the rendered
// frontmatter name matches the directory).
func reserveSkillDir(baseDir, group, title string) (string, string, error) {
	groupDir := filepath.Join(baseDir, filepath.Clean(group))
	slug := Slugify(title)
	for version := 1; ; version++ {
		candidateSlug := slug
		if version > 1 {
			candidateSlug = fmt.Sprintf("%s-v%d", slug, version)
		}
		dir := filepath.Join(groupDir, candidateSlug)
		free, err := skillDirFree(dir)
		if err != nil {
			return "", "", err
		}
		if !free {
			continue
		}
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return "", "", fmt.Errorf("skilllearn: mkdir %s: %w", dir, mkErr)
		}
		return candidateSlug, dir, nil
	}
}

// skillDirFree reports whether dir can receive a new SKILL.md: the directory
// (or its SKILL.md) does not exist yet. Any stat error other than NotExist is
// returned (never silently treated as free — no overwrites, no hangs).
func skillDirFree(dir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, skillFileName))
	switch {
	case err == nil:
		return false, nil
	case os.IsNotExist(err):
		return true, nil
	default:
		return false, fmt.Errorf("skilllearn: stat %s: %w", dir, err)
	}
}

// RenderSkillMarkdown renders the standard SKILL.md for the candidate using
// Slugify(candidate.Title) as the frontmatter name.
func RenderSkillMarkdown(candidate LessonCandidate) string {
	return RenderSkillMarkdownWithSlug(candidate, Slugify(candidate.Title))
}

// RenderSkillMarkdownWithSlug renders the standard SKILL.md with an explicit
// frontmatter name (the exporter passes the possibly versioned slug so the
// name always matches the directory). Deterministic: same inputs → same
// bytes.
//
// Layout (Task-336 DOD item 4 — strict format: YAML frontmatter name +
// description, "#" title, rule sections, example):
//
//	---
//	name: <slug>
//	description: <one-line description>
//	---
//
//	# <Title>
//	## Core Rules       (bullet form compatible with promptpacker cards)
//	## Anti-Pattern
//	## Preferred Behavior
//	## Example
func RenderSkillMarkdownWithSlug(candidate LessonCandidate, slug string) string {
	description := skillDescription(candidate)
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "name: %s\n", yamlScalar(slug))
	fmt.Fprintf(&sb, "description: %s\n", yamlScalar(description))
	sb.WriteString("---\n\n")

	fmt.Fprintf(&sb, "# %s\n\n", singleLine(candidate.Title))
	fmt.Fprintf(&sb, "FlowPilot lesson promoted from `workflow_lesson_candidates` (CP-23 Phase 3, Task-336). Detected %d time(s) across %d run(s); exported only after human approval.\n\n",
		candidate.RepeatCount, len(candidate.SupportingRunIDs))

	sb.WriteString("## Core Rules\n\n")
	if rule := firstSentence(candidate.PreferredBehavior); rule != "" {
		fmt.Fprintf(&sb, "- %s\n", rule)
	} else {
		sb.WriteString("- Follow the preferred behavior below.\n")
	}

	sb.WriteString("\n## Anti-Pattern\n\n")
	if anti := strings.TrimSpace(candidate.AntiPattern); anti != "" {
		sb.WriteString(singleLine(anti) + "\n")
	} else {
		sb.WriteString(singleLine(candidate.TriggerPattern) + "\n")
	}

	sb.WriteString("\n## Preferred Behavior\n\n")
	if behavior := strings.TrimSpace(candidate.PreferredBehavior); behavior != "" {
		sb.WriteString(behavior + "\n")
	} else {
		sb.WriteString("Follow the drift correction guidance for this pattern.\n")
	}

	sb.WriteString("\n## Example\n\n")
	fmt.Fprintf(&sb, "- Trigger: %s\n", singleLine(candidate.TriggerPattern))
	fmt.Fprintf(&sb, "- Wrong: %s\n", firstSentence(candidate.AntiPattern))
	fmt.Fprintf(&sb, "- Right: %s\n", firstSentence(candidate.PreferredBehavior))

	return sb.String()
}

// skillDescription builds the deterministic one-line frontmatter description.
func skillDescription(candidate LessonCandidate) string {
	summary := firstSentence(candidate.AntiPattern)
	if summary == "" {
		summary = firstSentence(candidate.TriggerPattern)
	}
	if summary == "" {
		summary = singleLine(candidate.Title)
	}
	return fmt.Sprintf("FlowPilot lesson learned from %d repeated drift incident(s) — %s", candidate.RepeatCount, summary)
}

// CompactRuleCard renders the CP-23 T-3 compact rule card (3-5 lines) for a
// candidate — the form Task-334's BudgetPacker injects into prompts to save
// tokens. Library-only until the BudgetPacker wiring lands (consistent with
// Task-334's CompactSkillCard): no runner path calls it in Task-336.
// Deterministic: same candidate → same card.
func CompactRuleCard(candidate LessonCandidate) string {
	var lines []string
	if rule := firstSentence(candidate.PreferredBehavior); rule != "" {
		lines = append(lines, "- Rule: "+rule)
	}
	if anti := firstSentence(candidate.AntiPattern); anti != "" {
		lines = append(lines, "- Avoid: "+anti)
	}
	if trigger := singleLine(candidate.TriggerPattern); trigger != "" {
		lines = append(lines, "- Trigger: "+trigger)
	}
	if slug := Slugify(candidate.Title); slug != "" && strings.TrimSpace(candidate.Title) != "" {
		lines = append(lines, "- Skill: "+slug)
	}
	return strings.Join(lines, "\n")
}

// Slugify converts a title into a filesystem-safe skill slug: lower-case,
// non-alphanumeric runs collapsed to "-", trimmed, capped at maxSlugLen.
// Empty input yields "lesson".
func Slugify(title string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) && r < 128 || unicode.IsDigit(r):
			sb.WriteRune(r)
		default:
			sb.WriteByte('-')
		}
	}
	slug := sb.String()
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if len(slug) > maxSlugLen {
		slug = strings.TrimRight(slug[:maxSlugLen], "-")
	}
	if slug == "" {
		slug = "lesson"
	}
	return slug
}

// protectedCoreSkillSlugs are the safe-fix-contract core skills a promoted
// lesson must never shadow (Task-336 Constraint / DOD item 10).
var protectedCoreSkillSlugs = map[string]bool{
	"safe-fix-contract":   true,
	"additive-tests-only": true,
}

// forbiddenContractPhrases is the pragmatic contradiction heuristic of
// Task-336 DOD item 10: a candidate whose own text directs the AI to weaken
// the safe-fix-contract / additive-tests-only guarantees (edit or delete
// existing tests, skip or comment out tests, bypass human approval) is
// flagged as conflicting. The check is deliberately conservative — phrases
// that merely mention the topics are also flagged, because a silent
// contradiction is fatal while an extra human confirmation is cheap. Detection
// is case-insensitive over Title + TriggerPattern + AntiPattern +
// PreferredBehavior.
var forbiddenContractPhrases = []string{
	"edit existing tests",
	"editing existing tests",
	"edit pre-existing tests",
	"editing pre-existing tests",
	"modify existing tests",
	"modifying existing tests",
	"overwrite existing tests",
	"overwriting existing tests",
	"change existing tests",
	"delete old tests",
	"deleting old tests",
	"delete failing tests",
	"deleting failing tests",
	"remove old tests",
	"removing old tests",
	"skip old tests",
	"skipping old tests",
	"skip failing tests",
	"skipping failing tests",
	"comment out tests",
	"commenting out tests",
	"auto-approve",
	"auto approve",
	"approve automatically",
	"without human approval",
	"without user approval",
	"no human approval",
}

// DetectCoreSkillConflict reports whether the candidate collides with the
// safe-fix-contract core skills, returning a human-readable reason and true
// when conflicting (Task-336 DOD item 10). Two checks, both deterministic:
//
//  1. Name collision: Slugify(Title) equals a protected core skill name
//     ("safe-fix-contract", "additive-tests-only") — the promoted skill would
//     shadow the operator safety pack.
//  2. Content contradiction: the candidate text contains a forbidden
//     directive phrase (see forbiddenContractPhrases).
func DetectCoreSkillConflict(candidate LessonCandidate) (string, bool) {
	if slug := Slugify(candidate.Title); protectedCoreSkillSlugs[slug] {
		return fmt.Sprintf("skill name %q collides with the protected core skill", slug), true
	}
	text := strings.ToLower(singleLine(strings.Join([]string{
		candidate.Title, candidate.TriggerPattern, candidate.AntiPattern, candidate.PreferredBehavior,
	}, " ")))
	for _, phrase := range forbiddenContractPhrases {
		if strings.Contains(text, phrase) {
			return fmt.Sprintf("content contradicts safe-fix-contract/additive-tests-only: contains %q", phrase), true
		}
	}
	return "", false
}

// singleLine flattens text to one YAML/plain line: newlines → spaces, runs of
// whitespace collapsed, trimmed.
func singleLine(s string) string {
	flat := strings.ReplaceAll(s, "\r", " ")
	flat = strings.ReplaceAll(flat, "\n", " ")
	return strings.Join(strings.Fields(flat), " ")
}

// firstSentence returns the first sentence of s (up to the first ". " or a
// trailing "."), flattened to one line and capped at 200 runes.
func firstSentence(s string) string {
	sentence := singleLine(s)
	if sentence == "" {
		return ""
	}
	if idx := strings.Index(sentence, ". "); idx >= 0 {
		sentence = sentence[:idx+1]
	}
	return truncateRunes(sentence, 200)
}

// yamlScalar renders s as a safe single-line YAML scalar: quoting it in
// double quotes whenever it could be misparsed (colons, hashes, leading
// indicators), escaping backslashes and quotes. Deterministic.
func yamlScalar(s string) string {
	s = singleLine(s)
	if s == "" {
		return `""`
	}
	needsQuote := strings.ContainsAny(s, ":#\"'") || strings.HasPrefix(s, "-") ||
		strings.HasPrefix(s, "?") || strings.HasPrefix(s, "!") || strings.HasPrefix(s, "&") ||
		strings.HasPrefix(s, "*") || strings.HasPrefix(s, "[") || strings.HasPrefix(s, "{") ||
		strings.HasPrefix(s, ">") || strings.HasPrefix(s, "|") || strings.HasPrefix(s, "%") ||
		strings.HasPrefix(s, "@") || strings.Contains(s, " #")
	if !needsQuote {
		return s
	}
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
