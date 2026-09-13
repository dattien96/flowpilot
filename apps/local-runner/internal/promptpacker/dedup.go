package promptpacker

import (
	"sort"
	"strings"
)

// dedupWindowLines is the sliding-window size (in normalized lines) used to
// fingerprint content blocks (Task-334 T-3: sliding-window text fingerprints).
const dedupWindowLines = 3

// reasonDuplicateContent is the audit reason for text removed because the same
// block already appears in a higher-priority section.
const reasonDuplicateContent = "duplicate_content"

// dedupRemoval records tokens removed from one section by deduplication so
// PackPrompt can audit them.
type dedupRemoval struct {
	SectionTitle string
	Kind         SectionKind
	Tokens       int
	Reason       string
}

// DeduplicateContext removes duplicated text blocks between sections
// (Task-334 §11): a block whose sliding-window fingerprints all already appear
// in a different section is removed from the LOWER-priority copy (the higher
// Priority keeps its text; ties keep the earliest input copy). Sections come
// back in input order; a section emptied entirely by dedup is dropped from
// the result.
func DeduplicateContext(sections []PromptSection) []PromptSection {
	out, _ := deduplicateContextWithRemovals(sections)
	return out
}

// deduplicateContextWithRemovals is DeduplicateContext plus per-section
// removal records for the audit trail.
func deduplicateContextWithRemovals(sections []PromptSection) ([]PromptSection, []dedupRemoval) {
	if len(sections) == 0 {
		return nil, nil
	}
	// Higher priority (lower number) is processed first, stable on ties, so
	// the earlier / higher-priority copy is the one that keeps its text.
	order := make([]int, len(sections))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return sections[order[a]].Priority < sections[order[b]].Priority
	})

	// fingerprints maps each window fingerprint to the section indexes that
	// contain it (built incrementally as sections are processed).
	fingerprints := map[string]map[int]bool{}
	keptBlocks := make([][]string, len(sections))
	removedAny := make([]bool, len(sections))

	for _, idx := range order {
		sec := sections[idx]
		blocks := contentBlocks(sec.Content)
		kept := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if blockIsForeignDuplicate(block, idx, fingerprints) {
				removedAny[idx] = true
				continue
			}
			kept = append(kept, block)
			addFingerprints(block, idx, fingerprints)
		}
		keptBlocks[idx] = kept
	}

	out := make([]PromptSection, 0, len(sections))
	var removals []dedupRemoval
	for idx, sec := range sections {
		if len(keptBlocks[idx]) == 0 && removedAny[idx] {
			// Every block of this section duplicates a higher-priority copy.
			removals = append(removals, dedupRemoval{
				SectionTitle: sectionLabel(sec),
				Kind:         sec.Kind,
				Tokens:       EstimateTokens(sec.Content),
				Reason:       reasonDuplicateContent,
			})
			continue
		}
		content := sec.Content
		if removedAny[idx] {
			original := EstimateTokens(content)
			content = strings.Join(keptBlocks[idx], "\n\n")
			if d := original - EstimateTokens(content); d > 0 {
				removals = append(removals, dedupRemoval{
					SectionTitle: sectionLabel(sec),
					Kind:         sec.Kind,
					Tokens:       d,
					Reason:       reasonDuplicateContent,
				})
			}
		}
		out = append(out, PromptSection{
			Kind:     sec.Kind,
			Title:    sec.Title,
			Content:  content,
			Priority: sec.Priority,
		})
	}
	return out, removals
}

// blockIsForeignDuplicate reports whether every sliding-window fingerprint of
// block was already contributed by a DIFFERENT section (that section's copy
// wins). Fingerprints shared only within the same section never make a block
// a cross-section duplicate.
func blockIsForeignDuplicate(block string, sectionIdx int, fingerprints map[string]map[int]bool) bool {
	windows := blockFingerprints(block)
	if len(windows) == 0 {
		return false
	}
	for _, fp := range windows {
		foreign := false
		for origin := range fingerprints[fp] {
			if origin != sectionIdx {
				foreign = true
				break
			}
		}
		if !foreign {
			return false
		}
	}
	return true
}

// addFingerprints records a block's window fingerprints as contributed by
// sectionIdx.
func addFingerprints(block string, sectionIdx int, fingerprints map[string]map[int]bool) {
	for _, fp := range blockFingerprints(block) {
		if fingerprints[fp] == nil {
			fingerprints[fp] = map[int]bool{}
		}
		fingerprints[fp][sectionIdx] = true
	}
}

// blockFingerprints returns the sliding-window fingerprints of a block:
// dedupWindowLines consecutive normalized lines, step 1; a block shorter than
// the window yields one whole-block fingerprint.
func blockFingerprints(block string) []string {
	lines := strings.Split(block, "\n")
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		if n := normalizeText(line); n != "" {
			normalized = append(normalized, n)
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	if len(normalized) <= dedupWindowLines {
		return []string{strings.Join(normalized, "\n")}
	}
	fps := make([]string, 0, len(normalized)-dedupWindowLines+1)
	for i := 0; i+dedupWindowLines <= len(normalized); i++ {
		fps = append(fps, strings.Join(normalized[i:i+dedupWindowLines], "\n"))
	}
	return fps
}

// normalizeText canonicalizes a line for fingerprinting: lowercase, collapse
// all whitespace runs to single spaces, trim.
func normalizeText(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// contentBlocks splits section content into fence-aware blocks (paragraphs;
// each fenced code block stays atomic). Block boundaries are blank lines
// outside fences.
func contentBlocks(content string) []string {
	var blocks []string
	var cur []string
	inFence := false
	flush := func() {
		if len(cur) > 0 {
			blocks = append(blocks, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inFence {
				cur = append(cur, line)
				blocks = append(blocks, strings.Join(cur, "\n"))
				cur = nil
				inFence = false
				continue
			}
			flush()
			cur = []string{line}
			inFence = true
			continue
		}
		if inFence {
			cur = append(cur, line)
			continue
		}
		if trimmed == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return blocks
}
