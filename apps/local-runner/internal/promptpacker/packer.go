// Package promptpacker implements CP-23 Phase 1 (Task-334): the Context
// Resolver + Budget Packer that packs prompt sections under a per-section and
// global token budget, deduplicates repeated text between sections, and
// records a prompt context audit for every pack.
//
// Provider-agnostic by construction: pure Go, heuristic token estimate
// (~4 bytes = 1 token), 0 LLM — identical behavior for every provider. The
// estimator counts BYTES, so Vietnamese/CJK text is overestimated ~1.5-2x:
// the conservative direction (packs prune earlier; the true token count never
// exceeds the budget) — review hardening note, Task-334 §8.
package promptpacker

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// SectionKind classifies a prompt section (Task-334 §11 Code Guide).
type SectionKind string

const (
	SectionSystemContract SectionKind = "system_contract"
	SectionCurrentTask    SectionKind = "current_task"
	SectionMandatoryDoc   SectionKind = "mandatory_doc"
	SectionMemorySummary  SectionKind = "memory_summary"
	SectionRawExcerpt     SectionKind = "raw_excerpt"
	SectionCompactSkills  SectionKind = "compact_skills"
)

// PromptSection is one named block of the assembled prompt.
type PromptSection struct {
	Kind     SectionKind `json:"kind"`
	Title    string      `json:"title"`
	Content  string      `json:"content"`
	Priority int         `json:"priority"` // 1 (Cao nhất) đến 6 (Thấp nhất)
}

// SectionBudget bounds the packed prompt. Per-kind caps <= 0 mean uncapped.
type SectionBudget struct {
	TotalMaxTokens   int `json:"total_max_tokens"`
	MaxExcerptTokens int `json:"max_excerpt_tokens"`
	MaxMemoryTokens  int `json:"max_memory_tokens"`
	MaxSkillTokens   int `json:"max_skill_tokens"`
}

// PackerOptions configures one packing pass (Task-334 DOD): the token budget
// today, with room for future packing knobs without changing PackPrompt's
// signature.
type PackerOptions struct {
	Budget SectionBudget `json:"budget"`
}

// DefaultPackerOptions returns the CP-23 Phase 1 default budget (Task-334
// T-1): 8,000 total with per-kind caps at the documented slice percentages —
// Memory Summary 30%, Raw Excerpts 20%, Compact Skills 10%. System contract /
// current task / mandatory docs carry no per-kind cap and are retained whole
// (CP-23 R-1).
func DefaultPackerOptions() PackerOptions {
	return PackerOptions{Budget: SectionBudget{
		TotalMaxTokens:   8000,
		MaxExcerptTokens: 1600,
		MaxMemoryTokens:  2400,
		MaxSkillTokens:   800,
	}}
}

// PromptAuditReport records what the packer selected, dropped and why
// (Task-334 §11). Warnings is an additive field beyond the Code Guide shape,
// required so mandatory sections retained whole over budget are still audited
// ("audit ghi cảnh báo vượt ngân sách", Task-334 §10 edge scenario).
type PromptAuditReport struct {
	SelectedTokens int            `json:"selected_tokens"`
	DroppedTokens  int            `json:"dropped_tokens"`
	DroppedItems   []string       `json:"dropped_items"`
	SectionUsage   map[string]int `json:"section_usage"`
	Warnings       []string       `json:"warnings,omitempty"`
}

// reasonExceededSectionBudget is the audit reason required by Task-334 §10 for
// anything pruned because it did not fit its section cap or the global budget.
const reasonExceededSectionBudget = "exceeded_section_budget"

// mandatoryKinds are retained whole even when individually over budget
// (CP-23 R-1: always keep System Contract + Current Task + mandatory context;
// only prune raw excerpts / compact skills / memory summaries).
var mandatoryKinds = map[SectionKind]bool{
	SectionSystemContract: true,
	SectionCurrentTask:    true,
	SectionMandatoryDoc:   true,
}

// sectionCaps maps the per-kind token caps configured on SectionBudget.
// Kinds absent from this map have no per-kind cap (only the global budget).
func sectionCaps(budget SectionBudget) map[SectionKind]int {
	return map[SectionKind]int{
		SectionRawExcerpt:    budget.MaxExcerptTokens,
		SectionMemorySummary: budget.MaxMemoryTokens,
		SectionCompactSkills: budget.MaxSkillTokens,
	}
}

// EstimateTokens is the heuristic token estimator required by CP-23
// Constraints: fast, ~4 bytes = 1 token (byte-based — conservative for
// Vietnamese/CJK text, see the package doc), 0 LLM.
func EstimateTokens(content string) int {
	n := len(content)
	if n == 0 {
		return 0
	}
	return (n + 3) / 4
}

// clipRunes clips s to at most maxBytes bytes without splitting a multi-byte
// UTF-8 rune (review hardening: a raw byte slice clip produced invalid UTF-8
// when a single line exceeded the whole cap). Budget accounting stays
// byte-based, so the result only ever shrinks.
func clipRunes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// PackPrompt assembles sections under the token budget (Task-334 §11):
//  1. deduplicate repeated content between sections (highest priority wins);
//  2. sort sections by Priority (1 = highest, stable on ties);
//  3. enforce per-kind caps (excerpt / memory / skill) by truncation;
//  4. enforce the global budget by dropping lowest-priority non-mandatory
//     sections first (CP-23 §2 allocation order 1..6);
//  5. record every drop/truncation in the audit report with the reason
//     "exceeded_section_budget", and warn when mandatory sections are
//     retained whole over budget;
//  6. assemble the packed prompt with clear section headers.
//
// Returns an error when budget.TotalMaxTokens <= 0.
func PackPrompt(sections []PromptSection, budget SectionBudget) (string, PromptAuditReport, error) {
	if budget.TotalMaxTokens <= 0 {
		return "", PromptAuditReport{}, errors.New("invalid budget: total must be positive")
	}
	report := PromptAuditReport{
		DroppedItems: []string{},
		SectionUsage: map[string]int{},
		Warnings:     []string{},
	}
	if len(sections) == 0 {
		return "", report, nil
	}

	// 1. Deduplicate (keeps the higher-priority copy; drops the lower).
	deduped, dedupRemovals := deduplicateContextWithRemovals(sections)
	for _, r := range dedupRemovals {
		report.DroppedTokens += r.Tokens
		report.DroppedItems = append(report.DroppedItems,
			fmt.Sprintf("%s (%d tokens): %s", r.SectionTitle, r.Tokens, r.Reason))
	}

	// 2. Sort by Priority ascending (1 highest); stable so equal priorities
	// keep input order (the earlier section is the higher-priority copy).
	sorted := make([]PromptSection, len(deduped))
	copy(sorted, deduped)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	// 3. Per-kind caps: truncate over-cap sections (line-boundary, never
	// exceeding the cap). Mandatory kinds have no per-kind cap.
	caps := sectionCaps(budget)
	kept := make([]PromptSection, 0, len(sorted))
	keptTokens := make([]int, 0, len(sorted))
	for _, sec := range sorted {
		tokens := EstimateTokens(sec.Content)
		if cap, ok := caps[sec.Kind]; ok && cap > 0 && tokens > cap {
			trimmed := truncateToTokens(sec.Content, cap)
			newTokens := EstimateTokens(trimmed)
			report.DroppedTokens += tokens - newTokens
			report.DroppedItems = append(report.DroppedItems, fmt.Sprintf(
				"%s (%d tokens): %s (truncated to %d tokens)",
				sectionLabel(sec), tokens-newTokens, reasonExceededSectionBudget, newTokens))
			sec.Content = trimmed
			tokens = newTokens
		}
		kept = append(kept, sec)
		keptTokens = append(keptTokens, tokens)
	}

	// 4. Global budget: drop the lowest-priority non-mandatory section first
	// (larger token count wins ties so each drop buys the most headroom).
	total := 0
	for _, t := range keptTokens {
		total += t
	}
	for total > budget.TotalMaxTokens {
		dropIdx := -1
		for i := range kept {
			if mandatoryKinds[kept[i].Kind] {
				continue
			}
			if dropIdx == -1 ||
				kept[i].Priority > kept[dropIdx].Priority ||
				(kept[i].Priority == kept[dropIdx].Priority && keptTokens[i] > keptTokens[dropIdx]) {
				dropIdx = i
			}
		}
		if dropIdx == -1 {
			// Only mandatory sections remain: retain them whole (CP-23 R-1)
			// and record the over-budget warning (Task-334 §10 edge scenario).
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"mandatory sections exceed total budget (%d > %d tokens); retained whole",
				total, budget.TotalMaxTokens))
			break
		}
		report.DroppedTokens += keptTokens[dropIdx]
		report.DroppedItems = append(report.DroppedItems, fmt.Sprintf(
			"%s (%d tokens): %s", sectionLabel(kept[dropIdx]), keptTokens[dropIdx], reasonExceededSectionBudget))
		total -= keptTokens[dropIdx]
		kept = append(kept[:dropIdx], kept[dropIdx+1:]...)
		keptTokens = append(keptTokens[:dropIdx], keptTokens[dropIdx+1:]...)
	}

	// 5. Audit: per-kind token usage and the final selected total.
	for i := range kept {
		report.SectionUsage[string(kept[i].Kind)] += keptTokens[i]
	}
	report.SelectedTokens = total

	// 6. Assemble.
	return assemblePrompt(kept), report, nil
}

// sectionLabel is the audit-friendly name of a section: its title when set,
// otherwise its kind.
func sectionLabel(sec PromptSection) string {
	if title := strings.TrimSpace(sec.Title); title != "" {
		return title
	}
	return string(sec.Kind)
}

// kindLabel is the human-readable header name for a section kind.
func kindLabel(kind SectionKind) string {
	switch kind {
	case SectionSystemContract:
		return "System Contract"
	case SectionCurrentTask:
		return "Current Task"
	case SectionMandatoryDoc:
		return "Mandatory Context"
	case SectionMemorySummary:
		return "Working Memory"
	case SectionRawExcerpt:
		return "Source Excerpt"
	case SectionCompactSkills:
		return "Skills"
	}
	return string(kind)
}

// sectionHeader renders the clear section header used in the packed prompt.
func sectionHeader(sec PromptSection) string {
	title := strings.TrimSpace(sec.Title)
	if title == "" {
		title = kindLabel(sec.Kind)
	}
	return "## " + title
}

// assemblePrompt joins packed sections in priority order with clear headers.
// Empty-content sections are skipped.
func assemblePrompt(sections []PromptSection) string {
	var sb strings.Builder
	for _, sec := range sections {
		content := strings.Trim(sec.Content, "\n")
		if strings.TrimSpace(content) == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(sectionHeader(sec))
		sb.WriteString("\n\n")
		sb.WriteString(content)
	}
	return sb.String()
}

// truncateToTokens cuts content to at most maxTokens tokens (~4 bytes/token),
// preferring a whole-line boundary. Never returns more than maxTokens tokens
// worth of content; empty when maxTokens <= 0.
func truncateToTokens(content string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}
	maxChars := maxTokens * 4
	if len(content) <= maxChars {
		return content
	}
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	used := 0
	for i, line := range lines {
		cost := len(line)
		if i > 0 {
			cost++ // newline
		}
		if used+cost > maxChars {
			break
		}
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(line)
		used += cost
	}
	out := sb.String()
	// A line longer than the whole cap does not fit the loop above; the
	// fallback below hard-clips (rune-safe — never splits UTF-8) so the result
	// never exceeds the token budget.
	if out == "" && content != "" {
		out = clipRunes(content, maxChars)
	}
	return out
}
