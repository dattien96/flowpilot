package docscan

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// fixBlockKind classifies blocks for the auto-fixer.
type fixBlockKind int

const (
	blockPreamble fixBlockKind = iota // lines before the first heading
	blockMetadata
	blockAIV     // "## AI Quick View"
	blockSection // numbered "## N. Name"
	blockOther   // any other H1/H2 block
)

// fixBlock is an independent copy of one document block. body owns its own
// line slice so edits never alias the original content.
type fixBlock struct {
	kind    fixBlockKind
	heading string // full heading line ("" for preamble)
	body    []string
	number  int // for numbered sections
	rank    int // canonical rank for known sections, -1 otherwise
}

// sectionHeadingRe matches a numbered heading line for renumbering, keeping
// the original name text and spacing untouched.
var sectionHeadingRe = regexp.MustCompile(`^(\s*#{1,2}\s+)(\d+)([.)])([ \t]*)(.*)$`)

// AutoFixDocument applies deterministic, non-destructive codemods to bring a
// document closer to the SS-13 contract for the given phase:
//
//   - inserts a `## Metadata` skeleton (all required fields with safe
//     defaults) when the block is missing;
//   - inserts missing metadata fields with valid defaults (e.g.
//     "Feature Keys: None" on governing ss/sd/cp documents) without touching
//     any existing field;
//   - inserts the `## AI Quick View` skeleton (six sub-sections with TODO
//     markers) when missing, or appends missing sub-sections;
//   - reorders numbered sections into the phase's canonical order, renumbers
//     them 1..N, and adds missing canonical sections as empty skeletons with
//     a TODO marker.
//
// Body text inside sections is never deleted or rewritten; only heading lines
// of numbered sections are renumbered. If the document already conforms, the
// output is byte-for-byte identical to the input.
//
// AutoFixDocument returns an error for unsupported phase values and for
// binary / non-UTF-8 content. Empty content is returned unchanged.
func AutoFixDocument(content string, phase string) (string, error) {
	fixed, _, err := AutoFixDocumentDetailed(content, phase)
	return fixed, err
}

// AutoFixDocumentDetailed is AutoFixDocument plus an audit trail: the returned
// slice names each repair stage that was applied ("metadata-inserted",
// "metadata-fixed", "ai-quick-view-inserted", "ai-quick-view-fixed",
// "sections-rebuilt"). An empty slice means the document already conformed and
// the returned content is identical to the input. BUG-422: the standardize
// audit line `docscan_autofix_applied … changes=N` reports len(changes).
func AutoFixDocumentDetailed(content string, phase string) (string, []string, error) {
	ph, err := NormalizePhase(phase)
	if err != nil {
		return "", nil, err
	}
	if strings.ContainsRune(content, '\x00') || (len(content) > 0 && !utf8.ValidString(content)) {
		return "", nil, fmt.Errorf("docscan: content is not valid UTF-8 text; refusing to auto-fix possible binary content")
	}
	if strings.TrimSpace(content) == "" {
		return content, nil, nil
	}

	lines := strings.Split(content, "\n")
	doc := &parsedDoc{lines: lines, blocks: splitBlocks(lines)}
	fbs := classifyBlocks(doc, ph)
	changed := false
	var changes []string

	// --- Metadata block ---
	metaPos := -1
	for i, b := range fbs {
		if b.kind == blockMetadata {
			metaPos = i
			break
		}
	}
	if metaPos == -1 {
		nb := &fixBlock{kind: blockMetadata, heading: "## Metadata", body: metadataSkeleton(ph)}
		at := 0
		if len(fbs) > 0 && fbs[0].kind == blockPreamble {
			at = 1
		}
		fbs = insertBlock(fbs, at, nb)
		metaPos = at
		changed = true
		changes = append(changes, "metadata-inserted")
	} else if fixMetadataBody(fbs[metaPos], ph) {
		changed = true
		changes = append(changes, "metadata-fixed")
	}

	// --- AI Quick View block ---
	aivPos := -1
	for i, b := range fbs {
		if b.kind == blockAIV {
			aivPos = i
			break
		}
	}
	if aivPos == -1 {
		nb := &fixBlock{kind: blockAIV, heading: "## AI Quick View", body: aivSkeleton(aiQuickViewSubsections(ph))}
		fbs = insertBlock(fbs, metaPos+1, nb)
		changed = true
		changes = append(changes, "ai-quick-view-inserted")
	} else if fixAIVBody(fbs[aivPos], ph) {
		changed = true
		changes = append(changes, "ai-quick-view-fixed")
	}

	// --- Numbered sections ---
	regionStart := 0
	for i, b := range fbs {
		switch b.kind {
		case blockPreamble, blockMetadata, blockAIV:
			regionStart = i + 1
		}
	}
	preRegion := preRegionCanonicalRanks(fbs, regionStart)
	if sectionFixNeeded(fbs[regionStart:], ph, preRegion) {
		fbs = rebuildSectionRegion(fbs, regionStart, ph, preRegion)
		changed = true
		changes = append(changes, "sections-rebuilt")
	}

	if !changed {
		return content, nil, nil
	}

	out := make([]string, 0, len(lines)+16)
	for _, b := range fbs {
		if b.kind == blockPreamble {
			out = append(out, b.body...)
			continue
		}
		out = append(out, b.heading)
		out = append(out, b.body...)
	}
	return strings.Join(out, "\n"), changes, nil
}

// classifyBlocks converts parsedDoc blocks into independent fixBlocks.
func classifyBlocks(doc *parsedDoc, phase string) []*fixBlock {
	rank := map[string]int{}
	for i, name := range canonicalSections(phase) {
		rank[normalizeName(name)] = i
	}

	var out []*fixBlock
	metaSeen, aivSeen := false, false
	for _, b := range doc.blocks {
		fb := &fixBlock{rank: -1}
		if b.level == 0 {
			fb.kind = blockPreamble
			fb.body = append([]string{}, doc.lines[b.startIdx:b.endIdx]...)
			out = append(out, fb)
			continue
		}
		fb.heading = doc.lines[b.startIdx]
		fb.body = append([]string{}, doc.lines[b.startIdx+1:b.endIdx]...)
		norm := normalizeName(b.text)
		switch {
		case !metaSeen && norm == "metadata":
			fb.kind = blockMetadata
			metaSeen = true
		case !aivSeen && norm == "ai quick view":
			fb.kind = blockAIV
			aivSeen = true
		case b.level == 2 && b.isNumbered:
			fb.kind = blockSection
			fb.number = b.number
			if r, ok := rank[normalizeName(b.name)]; ok {
				fb.rank = r
			}
		default:
			fb.kind = blockOther
		}
		out = append(out, fb)
	}
	return out
}

// insertBlock returns a new slice with nb inserted at index at.
func insertBlock(fbs []*fixBlock, at int, nb *fixBlock) []*fixBlock {
	out := make([]*fixBlock, 0, len(fbs)+1)
	out = append(out, fbs[:at]...)
	out = append(out, nb)
	out = append(out, fbs[at:]...)
	return out
}

// metadataDefault returns the placeholder value synthesized for a missing
// metadata field.
func metadataDefault(phase string, field string) string {
	if normalizeName(field) == "phase" {
		return phaseMetadataValues[phase]
	}
	if v, ok := metadataFieldDefaults[normalizeName(field)]; ok {
		return v
	}
	return "TODO"
}

// metadataSkeleton builds the body of a full `## Metadata` block for a phase.
func metadataSkeleton(phase string) []string {
	body := []string{""}
	for _, field := range requiredMetadataFields(phase) {
		body = append(body, fmt.Sprintf("- %s: %s", field, metadataDefault(phase, field)))
	}
	if governingPhases[phase] {
		body = append(body, "- Feature Keys: None")
	}
	return append(body, "")
}

// aivSkeleton builds the body of a full `## AI Quick View` block.
func aivSkeleton(subsections []string) []string {
	body := []string{""}
	for _, sub := range subsections {
		body = append(body, "### "+sub, "", "- TODO", "")
	}
	return body
}

// fixMetadataBody inserts missing metadata fields (in canonical order) after
// the last existing field line. Existing lines are never modified. It reports
// whether anything changed.
func fixMetadataBody(fb *fixBlock, phase string) bool {
	present := map[string]bool{}
	lastFieldIdx := -1
	inFence := false
	for i, ln := range fb.body {
		trimmed := strings.TrimSpace(ln)
		if inFence {
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inFence = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = true
			continue
		}
		if m := metadataFieldRe.FindStringSubmatch(ln); m != nil {
			present[normalizeName(m[1])] = true
			lastFieldIdx = i
		}
	}

	want := append([]string{}, requiredMetadataFields(phase)...)
	if governingPhases[phase] {
		want = append(want, "Feature Keys")
	}
	var missing []string
	for _, field := range want {
		if !present[normalizeName(field)] {
			missing = append(missing, field)
		}
	}
	if len(missing) == 0 {
		return false
	}

	at := lastFieldIdx + 1
	if lastFieldIdx == -1 {
		at = 0
		for at < len(fb.body) && strings.TrimSpace(fb.body[at]) == "" {
			at++
		}
	}
	ins := make([]string, 0, len(missing))
	for _, field := range missing {
		ins = append(ins, fmt.Sprintf("- %s: %s", field, metadataDefault(phase, field)))
	}
	body := make([]string, 0, len(fb.body)+len(ins))
	body = append(body, fb.body[:at]...)
	body = append(body, ins...)
	body = append(body, fb.body[at:]...)
	fb.body = body
	return true
}

// fixAIVBody appends missing `### Sub-section` skeletons at the end of the AI
// Quick View block. It reports whether anything changed.
func fixAIVBody(fb *fixBlock, phase string) bool {
	present := map[string]bool{}
	eachTextLine(fb.body, func(ln string) {
		if m := subHeadingRe.FindStringSubmatch(ln); m != nil {
			present[normalizeName(m[1])] = true
		}
	})
	var missing []string
	for _, sub := range aiQuickViewSubsections(phase) {
		if !present[normalizeName(sub)] {
			missing = append(missing, sub)
		}
	}
	if len(missing) == 0 {
		return false
	}
	for _, sub := range missing {
		fb.body = appendToBody(fb.body, []string{"### " + sub, "", "- TODO"})
	}
	return true
}

// appendToBody appends newLines to a block body, keeping one blank separator
// line before the new content and preserving the body's original trailing
// blank lines at the very end.
func appendToBody(body []string, newLines []string) []string {
	core := body
	for len(core) > 0 && strings.TrimSpace(core[len(core)-1]) == "" {
		core = core[:len(core)-1]
	}
	trailing := len(body) - len(core)
	out := make([]string, 0, len(body)+len(newLines)+2)
	out = append(out, core...)
	if len(out) > 0 {
		out = append(out, "")
	}
	out = append(out, newLines...)
	for i := 0; i < trailing; i++ {
		out = append(out, "")
	}
	return out
}

// preRegionCanonicalRanks collects the canonical ranks of numbered section
// blocks that appear BEFORE the section region (e.g. a `## 1. Goal` heading
// placed before the Metadata block). Those sections already exist in the
// document, so the fixer must neither flag them as missing nor synthesize a
// duplicate skeleton for them (review hardening: an AutoFix on a
// scanner-CLEAN document stays byte-for-byte identical).
func preRegionCanonicalRanks(fbs []*fixBlock, regionStart int) map[int]bool {
	ranks := map[int]bool{}
	for _, b := range fbs[:regionStart] {
		if b.kind == blockSection && b.rank >= 0 {
			ranks[b.rank] = true
		}
	}
	return ranks
}

// sectionFixNeeded reports whether the section region requires a rebuild:
// numbers not strictly ascending, canonical names out of order, or canonical
// sections missing from BOTH the region and the pre-region area
// (preExisting — see preRegionCanonicalRanks).
func sectionFixNeeded(region []*fixBlock, phase string, preExisting map[int]bool) bool {
	rank := map[string]int{}
	for i, name := range canonicalSections(phase) {
		rank[normalizeName(name)] = i
	}
	prevNum, prevRank := 0, -1
	present := map[int]bool{}
	for _, b := range region {
		if b.kind != blockSection {
			continue
		}
		if b.number <= prevNum {
			return true
		}
		if b.rank >= 0 {
			if b.rank <= prevRank {
				return true
			}
			prevRank = b.rank
			present[b.rank] = true
		}
		prevNum = b.number
	}
	for i := range canonicalSections(phase) {
		if !present[i] && !preExisting[i] {
			return true
		}
	}
	return false
}

// rebuildSectionRegion reorders the numbered sections into the phase's
// canonical order, inserts skeletons for missing canonical sections, and
// renumbers known sections 1..N followed by unknown numbered sections N+1...
// Non-numbered blocks in the region are kept (in original relative order)
// after the numbered sections. No body text is removed. Canonical ranks in
// preExisting (already present before the region — see
// preRegionCanonicalRanks) get their number slot reserved but never a
// duplicate skeleton.
func rebuildSectionRegion(fbs []*fixBlock, regionStart int, phase string, preExisting map[int]bool) []*fixBlock {
	canonical := canonicalSections(phase)
	byRank := map[int]*fixBlock{}
	var unknowns, others []*fixBlock
	for _, b := range fbs[regionStart:] {
		if b.kind != blockSection {
			others = append(others, b)
			continue
		}
		if b.rank >= 0 && byRank[b.rank] == nil {
			byRank[b.rank] = b
			continue
		}
		// Unknown section names and duplicate canonical names keep their
		// content and are appended after the canonical ones.
		unknowns = append(unknowns, b)
	}

	region := make([]*fixBlock, 0, len(fbs)-regionStart+len(canonical))
	next := 1
	for i := range canonical {
		if b := byRank[i]; b != nil {
			b.heading = renumberHeading(b.heading, next)
			b.number = next
			region = append(region, b)
		} else if preExisting[i] {
			// Section already exists before the region — reserve the number
			// slot to keep canonical numbering consistent, but never emit a
			// duplicate skeleton.
		} else {
			region = append(region, &fixBlock{
				kind:    blockSection,
				heading: fmt.Sprintf("## %d. %s", next, canonical[i]),
				body:    []string{"", "- TODO", ""},
				rank:    i,
				number:  next,
			})
		}
		next++
	}
	for _, b := range unknowns {
		b.heading = renumberHeading(b.heading, next)
		b.number = next
		region = append(region, b)
		next++
	}
	region = append(region, others...)

	out := make([]*fixBlock, 0, len(fbs)-len(fbs[regionStart:])+len(region))
	out = append(out, fbs[:regionStart]...)
	out = append(out, region...)
	return out
}

// renumberHeading replaces only the number of a numbered heading line,
// preserving the original name text, separators and any trailing CR.
func renumberHeading(heading string, n int) string {
	if m := sectionHeadingRe.FindStringSubmatch(heading); m != nil {
		return m[1] + strconv.Itoa(n) + m[3] + m[4] + m[5]
	}
	return heading
}
