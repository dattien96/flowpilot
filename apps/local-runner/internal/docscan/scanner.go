// Package docscan implements the offline, deterministic conformance scanner
// and non-destructive auto-fixer for FlowPilot phase documents defined by
// SS-13 ("AI-Followable Document Contract") and the FORMAT-REFERENCE-*.md
// samples (CP-48 P-1 / P-2, Task-332).
//
// The scanner is pure Go markdown structure parsing: it never calls an LLM,
// consumes 0 tokens, and scans 100+ markdown files in well under 500ms.
package docscan

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// mdBlock is one H1/H2-delimited block of the document. The preamble (lines
// before the first heading) is modeled as a level-0 block.
type mdBlock struct {
	startIdx   int    // 0-based index of the heading line (0 for preamble)
	endIdx     int    // exclusive 0-based end index
	level      int    // 0 preamble, 1 H1, 2 H2
	text       string // heading text without the leading '#'s ("" for preamble)
	line       int    // 1-based line number of the heading (0 for preamble)
	isNumbered bool   // level-2 heading of the form "N. Name"
	number     int
	name       string
}

// parsedDoc is the result of splitting a document into blocks. The blocks
// partition lines exactly, so reassembling them in order is lossless.
type parsedDoc struct {
	lines  []string
	blocks []*mdBlock
}

var (
	headingLineRe   = regexp.MustCompile(`^(#{1,2}) (.+)$`)
	numberedTextRe  = regexp.MustCompile(`^(\d+)[.)][ \t]*(.*)$`)
	metadataFieldRe = regexp.MustCompile(`^\s*-\s*([^:]+):`)
	subHeadingRe    = regexp.MustCompile(`^\s*###\s+(.+?)\s*$`)
)

// phaseDirAliases maps requirements/ directory names to phases.
var phaseDirAliases = map[string]string{
	"05-system-specs":       PhaseSS,
	"06-system-tech-design": PhaseSD,
	"07-coding-plan":        PhaseCP,
	"08-task":               PhaseTask,
	"09-bugfix":             PhaseBugFix,
}

// phaseFilePrefixes maps filename prefixes to phases (checked case-insensitively).
var phaseFilePrefixes = []struct {
	prefix string
	phase  string
}{
	{"ss-", PhaseSS},
	{"sd-", PhaseSD},
	{"cp-", PhaseCP},
	{"task-", PhaseTask},
	{"bug-", PhaseBugFix},
}

// DetectPhase infers the document phase from a file path. Directory names win
// (e.g. "05-System-Specs" -> "ss"), then filename prefixes (e.g. "Task-332-..."
// -> "task"). It returns false when the phase cannot be determined.
func DetectPhase(filePath string) (string, bool) {
	slash := filepath.ToSlash(filePath)
	parts := strings.Split(slash, "/")
	for _, dir := range parts[:len(parts)-1] {
		if phase, ok := phaseDirAliases[strings.ToLower(strings.TrimSpace(dir))]; ok {
			return phase, true
		}
	}
	base := strings.ToLower(parts[len(parts)-1])
	for _, p := range phaseFilePrefixes {
		if strings.HasPrefix(base, p.prefix) {
			return p.phase, true
		}
	}
	return "", false
}

// isFormatReferencePath reports whether the file is a FORMAT-REFERENCE-*.md
// writing guide. SS-13 §11: "These files are not business truth. They are
// writing guides", so they are exempt from the governing-doc-only
// `Feature Keys` rule (they omit the optional field by design).
func isFormatReferencePath(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasPrefix(strings.ToLower(base), "format-reference-")
}

// ScanDocument analyzes one markdown document against the SS-13 contract for
// its phase and returns the list of violations.
//
// The phase is detected from filePath (directory name or filename prefix). An
// error is returned when the phase cannot be detected or when the content is
// not valid UTF-8 text. An empty or whitespace-only document yields a single
// Critical "empty_document" issue instead of an error.
func ScanDocument(filePath string, content string) ([]ScanIssue, error) {
	phase, ok := DetectPhase(filePath)
	if !ok {
		return nil, fmt.Errorf("docscan: could not detect document phase from path %q (expected a phase directory like 05-System-Specs or a filename prefix like SS-, SD-, CP-, Task-, BUG-)", filePath)
	}
	return scanWithPhase(filePath, content, phase)
}

// scanWithPhase runs all checks for a document with a known phase.
func scanWithPhase(filePath string, content string, phase string) ([]ScanIssue, error) {
	if strings.TrimSpace(content) == "" {
		return []ScanIssue{{
			FilePath:   filePath,
			Line:       1,
			Severity:   SeverityCritical,
			RuleID:     ruleEmptyDocument,
			Message:    "document is empty (no content to validate)",
			CanAutoFix: false,
		}}, nil
	}
	if !utf8.ValidString(content) {
		return nil, fmt.Errorf("docscan: %s is not valid UTF-8 text; refusing to scan possible binary content", filePath)
	}

	lines := strings.Split(content, "\n")
	doc := &parsedDoc{lines: lines, blocks: splitBlocks(lines)}

	issues := []ScanIssue{}
	issues = append(issues, checkMetadataBlock(filePath, doc, phase)...)
	issues = append(issues, checkAIQuickView(filePath, doc, phase)...)
	issues = append(issues, checkNumberedSections(filePath, doc, phase, len(lines))...)
	return issues, nil
}

// ScanDirectory walks dirPath recursively, scans every governed phase markdown
// file and returns the aggregated report. Files outside the five governed
// phases (undetectable phase) and non-markdown files are skipped. The walk is
// lexical, so the report order is deterministic.
func ScanDirectory(dirPath string) (*ScanReport, error) {
	report := &ScanReport{Issues: []ScanIssue{}}
	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		phase, ok := DetectPhase(path)
		if !ok {
			return nil // not a governed phase document
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("docscan: reading %s: %w", path, err)
		}
		issues, err := scanWithPhase(path, string(data), phase)
		if err != nil {
			return fmt.Errorf("docscan: scanning %s: %w", path, err)
		}
		report.TotalFilesScanned++
		if len(issues) == 0 {
			report.ConformingFiles++
		}
		report.Issues = append(report.Issues, issues...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

// splitBlocks partitions lines into H1/H2 blocks, ignoring heading-looking
// lines inside fenced code blocks.
func splitBlocks(lines []string) []*mdBlock {
	type headingAt struct {
		idx   int
		level int
		text  string
	}
	var headings []headingAt
	inFence := false
	for i, ln := range lines {
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
		if m := headingLineRe.FindStringSubmatch(ln); m != nil {
			headings = append(headings, headingAt{idx: i, level: len(m[1]), text: strings.TrimRight(m[2], " \t\r")})
		}
	}

	var blocks []*mdBlock
	first := len(lines)
	if len(headings) > 0 {
		first = headings[0].idx
	}
	if first > 0 {
		blocks = append(blocks, &mdBlock{startIdx: 0, endIdx: first, level: 0})
	}
	for i, h := range headings {
		end := len(lines)
		if i+1 < len(headings) {
			end = headings[i+1].idx
		}
		b := &mdBlock{startIdx: h.idx, endIdx: end, level: h.level, text: h.text, line: h.idx + 1}
		if h.level == 2 {
			if m := numberedTextRe.FindStringSubmatch(h.text); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil {
					b.isNumbered = true
					b.number = n
					b.name = strings.TrimSpace(m[2])
				}
			}
		}
		blocks = append(blocks, b)
	}
	return blocks
}

// findNamedBlock returns the first level-1/2 block whose heading matches the
// given normalized name (e.g. "metadata", "ai quick view").
func findNamedBlock(doc *parsedDoc, normalized string) *mdBlock {
	for _, b := range doc.blocks {
		if b.level > 0 && b.level <= 2 && normalizeName(b.text) == normalized {
			return b
		}
	}
	return nil
}

// bodyOf returns the lines of a block after its heading line.
func bodyOf(doc *parsedDoc, b *mdBlock) []string {
	return doc.lines[b.startIdx+1 : b.endIdx]
}

// eachTextLine iterates body lines that are outside fenced code blocks.
func eachTextLine(body []string, fn func(line string)) {
	inFence := false
	for _, ln := range body {
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
		fn(ln)
	}
}

// metadataFieldNames returns the normalized field names present in a metadata
// block body.
func metadataFieldNames(body []string) map[string]bool {
	fields := map[string]bool{}
	eachTextLine(body, func(ln string) {
		if m := metadataFieldRe.FindStringSubmatch(ln); m != nil {
			fields[normalizeName(m[1])] = true
		}
	})
	return fields
}

// checkMetadataBlock validates the `## Metadata` block and its fields.
func checkMetadataBlock(filePath string, doc *parsedDoc, phase string) []ScanIssue {
	issues := []ScanIssue{}
	meta := findNamedBlock(doc, "metadata")
	if meta == nil {
		// Emit only the block-level Critical issue; per-field issues would be
		// noise when the whole block is absent.
		issues = append(issues, ScanIssue{
			FilePath:   filePath,
			Line:       1,
			Severity:   SeverityCritical,
			RuleID:     ruleMissingMetadataBlock,
			Message:    "missing the required '## Metadata' block (SS-13 §5.1)",
			CanAutoFix: true,
		})
		return issues
	}

	present := metadataFieldNames(bodyOf(doc, meta))
	for _, field := range requiredMetadataFields(phase) {
		if !present[normalizeName(field)] {
			issues = append(issues, ScanIssue{
				FilePath:   filePath,
				Line:       meta.line,
				Severity:   SeverityImportant,
				RuleID:     ruleMissingRequiredField,
				Message:    fmt.Sprintf("metadata block is missing required field %q (SS-13 §5.1)", field),
				CanAutoFix: true,
			})
		}
	}
	if governingPhases[phase] && !present["feature keys"] && !isFormatReferencePath(filePath) {
		issues = append(issues, ScanIssue{
			FilePath:   filePath,
			Line:       meta.line,
			Severity:   SeverityImportant,
			RuleID:     ruleMissingFeatureKeys,
			Message:    "governing SS/SD/CP document is missing the 'Feature Keys' metadata field (BUG-280, SS-13 §5.1)",
			CanAutoFix: true,
		})
	}
	return issues
}

// checkAIQuickView validates the `## AI Quick View` block and its sub-sections.
func checkAIQuickView(filePath string, doc *parsedDoc, phase string) []ScanIssue {
	issues := []ScanIssue{}
	aiv := findNamedBlock(doc, "ai quick view")
	if aiv == nil {
		expectedLine := 1
		if meta := findNamedBlock(doc, "metadata"); meta != nil {
			expectedLine = meta.endIdx + 1 // first line after the metadata block
		}
		issues = append(issues, ScanIssue{
			FilePath:   filePath,
			Line:       expectedLine,
			Severity:   SeverityImportant,
			RuleID:     ruleMissingAIQuickView,
			Message:    "missing the required '## AI Quick View' block (SS-13 §5.2)",
			CanAutoFix: true,
		})
		return issues
	}

	present := map[string]bool{}
	eachTextLine(bodyOf(doc, aiv), func(ln string) {
		if m := subHeadingRe.FindStringSubmatch(ln); m != nil {
			present[normalizeName(m[1])] = true
		}
	})
	for _, sub := range aiQuickViewSubsections(phase) {
		if present[normalizeName(sub)] {
			continue
		}
		if normalizeName(sub) == "open questions" {
			issues = append(issues, ScanIssue{
				FilePath:   filePath,
				Line:       aiv.line,
				Severity:   SeverityMinor,
				RuleID:     ruleMissingOpenQuestions,
				Message:    "'## AI Quick View' is missing the 'Open Questions' sub-section (SS-13 §5.2)",
				CanAutoFix: true,
			})
			continue
		}
		issues = append(issues, ScanIssue{
			FilePath:   filePath,
			Line:       aiv.line,
			Severity:   SeverityImportant,
			RuleID:     ruleMissingAIVSubsection,
			Message:    fmt.Sprintf("'## AI Quick View' is missing the %q sub-section (SS-13 §5.2)", sub),
			CanAutoFix: true,
		})
	}
	return issues
}

// checkNumberedSections validates the ordering and completeness of the
// numbered `## N.` sections for the phase.
func checkNumberedSections(filePath string, doc *parsedDoc, phase string, totalLines int) []ScanIssue {
	issues := []ScanIssue{}
	rank := map[string]int{}
	for i, name := range canonicalSections(phase) {
		rank[normalizeName(name)] = i
	}

	var numbered []*mdBlock
	for _, b := range doc.blocks {
		if b.level == 2 && b.isNumbered {
			numbered = append(numbered, b)
		}
	}

	// Order check: numbers must strictly ascend and recognized canonical
	// names must appear in canonical order. One issue is emitted per
	// offending section heading.
	prevNum, prevRank := 0, -1
	for _, b := range numbered {
		r, known := rank[normalizeName(b.name)]
		violation := false
		if b.number <= prevNum {
			violation = true
		}
		if known && r <= prevRank {
			violation = true
		}
		if violation {
			issues = append(issues, ScanIssue{
				FilePath:   filePath,
				Line:       b.line,
				Severity:   SeverityMinor,
				RuleID:     ruleSectionOutOfOrder,
				Message:    fmt.Sprintf("section %q is out of the canonical order for %q documents (per FORMAT-REFERENCE)", strings.TrimRight(b.text, "\r"), phase),
				CanAutoFix: true,
			})
		}
		prevNum = b.number
		if known {
			prevRank = r
		}
	}

	// Completeness check: every canonical section must be present. Extra
	// non-canonical sections are allowed (SS-13 §5.3 "minimum sections").
	// Legacy Vietnamese-titled headings intentionally do not match the
	// canonical English names (see canonicalSectionsByPhase) — the resulting
	// missing_required_section issue is a deferral flag for Task-333, and
	// AutoFix must never translate or drop the legacy heading.
	present := map[int]bool{}
	for _, b := range numbered {
		if r, ok := rank[normalizeName(b.name)]; ok {
			present[r] = true
		}
	}
	for i, name := range canonicalSections(phase) {
		if !present[i] {
			issues = append(issues, ScanIssue{
				FilePath:   filePath,
				Line:       totalLines,
				Severity:   SeverityImportant,
				RuleID:     ruleMissingRequiredSection,
				Message:    fmt.Sprintf("missing required section %q (expected as \"## %d. %s\")", name, i+1, name),
				CanAutoFix: true,
			})
		}
	}
	return issues
}
