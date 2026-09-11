package docscan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// validTaskDoc is a hand-written, fully conforming task document with sections
// 1-9 (the eight canonical task sections plus an extra "Definition of Done"
// section, which SS-13 permits as a non-canonical addition). It is an
// independent oracle: it does not reuse any production table.
const validTaskDoc = `# Task-900: Example Conforming Task

## Metadata

- Document ID: Task-900
- Title: Example conforming task
- Phase: task
- Status: draft
- Owner: FlowPilot
- Reviewers: Operator
- Created: 2026-09-11
- Last Updated: 2026-09-11
- Parent Documents: CP-48
- Child Documents: None
- Related Documents: SS-13
- Replaces: None
- Tags: docscan

## AI Quick View

### Summary

- Adds the docscan package.

### Current Ask

- Implement the scanner and autofixer.

### Key Decisions

- T-1 Pure Go offline parsing.

### Constraints

- No runner wiring in this task.

### Open Questions

- None.

### Source Refs

- CP-48, SS-13.

## 1. Goal

Build the doc conformance engine.

## 2. Parent Links

- coding plan: CP-48

## 3. Trigger

Docs drift from SS-13.

## 4. Exact Change

- T-1 Add docscan package.

## 5. Touched Areas

- files: apps/local-runner/internal/docscan

## 6. Acceptance Check

- go test ./internal/docscan/... passes.

## 7. Out of Scope

- Runner integration.

## 8. Completion Notes

- result: pending

## 9. Definition of Done

- All tests green.
`

// fixtureSpec configures a generated fixture document.
type fixtureSpec struct {
	phase           string
	id              string
	withMeta        bool
	omittedFields   map[string]bool // normalized field names
	withFeatureKeys bool
	withAIV         bool
	omittedSubs     map[string]bool // normalized sub-section names
	sections        []string        // canonical section names in appearance order
	omittedSections map[string]bool
	extraSections   []string
}

type fixtureOpt func(*fixtureSpec)

func withoutMeta() fixtureOpt     { return func(s *fixtureSpec) { s.withMeta = false } }
func withoutAIV() fixtureOpt      { return func(s *fixtureSpec) { s.withAIV = false } }
func omitFeatureKeys() fixtureOpt { return func(s *fixtureSpec) { s.withFeatureKeys = false } }
func withFeatureKeys() fixtureOpt { return func(s *fixtureSpec) { s.withFeatureKeys = true } }
func omitSection(n string) fixtureOpt {
	return func(s *fixtureSpec) { s.omittedSections[normalizeName(n)] = true }
}
func omitField(n string) fixtureOpt {
	return func(s *fixtureSpec) { s.omittedFields[normalizeName(n)] = true }
}
func omitAIVSub(n string) fixtureOpt {
	return func(s *fixtureSpec) { s.omittedSubs[normalizeName(n)] = true }
}

func withSectionOrder(names ...string) fixtureOpt {
	return func(s *fixtureSpec) { s.sections = names }
}

func withExtraSection(name string) fixtureOpt {
	return func(s *fixtureSpec) { s.extraSections = append(s.extraSections, name) }
}

func defaultFixtureID(phase string) string {
	switch phase {
	case PhaseSS:
		return "SS-90"
	case PhaseSD:
		return "SD-90"
	case PhaseCP:
		return "CP-90"
	case PhaseBugFix:
		return "BUG-900"
	default:
		return "Task-900"
	}
}

// buildFixture generates a fixture document for a phase. By default it is
// fully conforming (including "Feature Keys" on governing phases).
func buildFixture(phase string, opts ...fixtureOpt) string {
	spec := &fixtureSpec{
		phase:           phase,
		id:              defaultFixtureID(phase),
		withMeta:        true,
		withAIV:         true,
		omittedFields:   map[string]bool{},
		omittedSubs:     map[string]bool{},
		omittedSections: map[string]bool{},
		withFeatureKeys: governingPhases[phase],
		sections:        append([]string{}, canonicalSections(phase)...),
	}
	for _, opt := range opts {
		opt(spec)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s: Fixture\n\n", spec.id)

	if spec.withMeta {
		b.WriteString("## Metadata\n\n")
		for _, field := range requiredMetadataFields(phase) {
			if spec.omittedFields[normalizeName(field)] {
				continue
			}
			value := metadataDefault(phase, field)
			if normalizeName(field) == "document id" {
				value = spec.id
			}
			fmt.Fprintf(&b, "- %s: %s\n", field, value)
		}
		if governingPhases[phase] && spec.withFeatureKeys {
			b.WriteString("- Feature Keys: None\n")
		}
		b.WriteString("\n")
	}

	if spec.withAIV {
		b.WriteString("## AI Quick View\n\n")
		for _, sub := range aiQuickViewSubsections(phase) {
			if spec.omittedSubs[normalizeName(sub)] {
				continue
			}
			fmt.Fprintf(&b, "### %s\n\n- Fixture %s content.\n\n", sub, sub)
		}
	}

	rank := map[string]int{}
	for i, name := range canonicalSections(phase) {
		rank[name] = i
	}
	maxNum := 0
	for _, name := range spec.sections {
		if spec.omittedSections[normalizeName(name)] {
			continue
		}
		fmt.Fprintf(&b, "## %d. %s\n\n- Fixture content for %s.\n\n", rank[name]+1, name, name)
		if rank[name]+1 > maxNum {
			maxNum = rank[name] + 1
		}
	}
	for _, name := range spec.extraSections {
		maxNum++
		fmt.Fprintf(&b, "## %d. %s\n\n- Fixture extra content for %s.\n\n", maxNum, name, name)
	}
	return b.String()
}

// fixturePath returns a non-FORMAT-REFERENCE path for a phase so phase
// detection and the governing-doc Feature Keys rule both apply.
func fixturePath(phase string) string {
	switch phase {
	case PhaseSS:
		return "requirements/05-System-Specs/SS-90-Fixture.md"
	case PhaseSD:
		return "requirements/06-System-Tech-Design/SD-90-Fixture.md"
	case PhaseCP:
		return "requirements/07-Coding-Plan/CP-90-Fixture.md"
	case PhaseBugFix:
		return "requirements/09-BugFix/BUG-900-Fixture.md"
	default:
		return "requirements/08-Task/Task-900-Fixture.md"
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustScan(t *testing.T, filePath string, content string) []ScanIssue {
	t.Helper()
	issues, err := ScanDocument(filePath, content)
	if err != nil {
		t.Fatalf("ScanDocument(%q) returned error: %v", filePath, err)
	}
	return issues
}

func findRule(issues []ScanIssue, ruleID string) *ScanIssue {
	for i := range issues {
		if issues[i].RuleID == ruleID {
			return &issues[i]
		}
	}
	return nil
}

func assertNoIssues(t *testing.T, filePath string, issues []ScanIssue) {
	t.Helper()
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues for %s, got %d:\n%s", filePath, len(issues), formatIssues(issues))
	}
}

func formatIssues(issues []ScanIssue) string {
	var b strings.Builder
	for _, iss := range issues {
		fmt.Fprintf(&b, "  - line %d [%s/%s] can_fix=%v: %s\n", iss.Line, iss.Severity, iss.RuleID, iss.CanAutoFix, iss.Message)
	}
	return b.String()
}

// findRepoRoot walks up from this test file to the repository root by looking
// for the requirements/05-System-Specs directory.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 8; i++ {
		if info, err := os.Stat(filepath.Join(dir, "requirements", "05-System-Specs")); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repository root above %s", filepath.Dir(thisFile))
	return ""
}

// ---------------------------------------------------------------------------
// Task-332 §10 mandated tests
// ---------------------------------------------------------------------------

// Scenario: Quét tài liệu chuẩn 100%
// Input: Markdown Task hợp lệ có đầy đủ Metadata, AI Quick View, và section 1-9
// Expect: len(issues) == 0
func TestScanDocument_ValidConformingDoc(t *testing.T) {
	issues := mustScan(t, fixturePath(PhaseTask), validTaskDoc)
	assertNoIssues(t, fixturePath(PhaseTask), issues)
}

// Scenario: Quét tài liệu thiếu trường Feature Keys trong Metadata
// Input: Markdown hợp lệ nhưng thiếu dòng Feature Keys
// Expect: len(issues) == 1, RuleID="missing_feature_keys", CanAutoFix=true
func TestScanDocument_MissingFeatureKeys(t *testing.T) {
	path := fixturePath(PhaseSS)
	content := buildFixture(PhaseSS, omitFeatureKeys())

	// Control: the same document with Feature Keys conforms.
	assertNoIssues(t, path, mustScan(t, path, buildFixture(PhaseSS)))

	issues := mustScan(t, path, content)
	if len(issues) != 1 {
		t.Fatalf("expected exactly 1 issue, got %d:\n%s", len(issues), formatIssues(issues))
	}
	iss := issues[0]
	if iss.RuleID != "missing_feature_keys" {
		t.Errorf("RuleID = %q, want missing_feature_keys", iss.RuleID)
	}
	if iss.Severity != SeverityImportant {
		t.Errorf("Severity = %q, want important", iss.Severity)
	}
	if !iss.CanAutoFix {
		t.Error("CanAutoFix = false, want true")
	}
	if iss.FilePath != path {
		t.Errorf("FilePath = %q, want %q", iss.FilePath, path)
	}
	if iss.Line < 1 {
		t.Errorf("Line = %d, want >= 1", iss.Line)
	}
}

// Scenario: Quét tài liệu bị đảo lộn thứ tự các section
// Input: Markdown có ## 3. Trigger đứng trước ## 1. Goal
// Expect: len(issues) >= 1, RuleID="section_out_of_order", CanAutoFix=true
func TestScanDocument_SectionOutOfOrder(t *testing.T) {
	path := fixturePath(PhaseTask)
	content := buildFixture(PhaseTask, withSectionOrder(
		"Trigger", "Goal", "Parent Links", "Exact Change", "Touched Areas",
		"Acceptance Check", "Out of Scope", "Completion Notes",
	))
	if !strings.Contains(content, "## 3. Trigger") || !strings.Contains(content, "## 1. Goal") {
		t.Fatal("fixture must contain '## 3. Trigger' before '## 1. Goal'")
	}

	issues := mustScan(t, path, content)
	if len(issues) < 1 {
		t.Fatalf("expected at least 1 issue, got 0")
	}
	iss := findRule(issues, "section_out_of_order")
	if iss == nil {
		t.Fatalf("expected a section_out_of_order issue, got:\n%s", formatIssues(issues))
	}
	if iss.Severity != SeverityMinor {
		t.Errorf("Severity = %q, want minor", iss.Severity)
	}
	if !iss.CanAutoFix {
		t.Error("CanAutoFix = false, want true")
	}
}

// Scenario: AutoFix tự động bổ sung Feature Keys và sắp xếp section
// Input: Markdown lỗi từ testcase trên
// Expect: Kết quả sau AutoFix trả về markdown chuẩn, ScanDocument lại đạt 0 lỗi
func TestAutoFixDocument_FixesStructureAndMetadata(t *testing.T) {
	path := fixturePath(PhaseSS)
	content := buildFixture(PhaseSS, omitFeatureKeys(), withSectionOrder(
		"Goal", "Scope", "Problem", "Non-Goals", "User Stories or Primary Use Cases",
		"Acceptance Criteria", "Business Rules", "Edge Cases", "Dependencies",
		"Open Questions", "Definition of Done",
	))

	// Sanity: the input is actually broken.
	before := mustScan(t, path, content)
	if findRule(before, "missing_feature_keys") == nil || findRule(before, "section_out_of_order") == nil {
		t.Fatalf("fixture must contain both missing_feature_keys and section_out_of_order issues, got:\n%s", formatIssues(before))
	}

	fixed, err := AutoFixDocument(content, PhaseSS)
	if err != nil {
		t.Fatalf("AutoFixDocument error: %v", err)
	}
	if !strings.Contains(fixed, "- Feature Keys: None") {
		t.Error("AutoFix did not insert 'Feature Keys: None'")
	}
	if !strings.Contains(fixed, "- Status: draft") {
		t.Error("AutoFix changed an existing metadata field ('Status')")
	}
	// Section order repaired: Problem (2) must now precede Scope (3).
	if got := strings.Index(fixed, "## 2. Problem"); got < 0 {
		t.Error("fixed document is missing '## 2. Problem'")
	} else if scope := strings.Index(fixed, "## 3. Scope"); scope < 0 || scope < got {
		t.Errorf("'## 2. Problem' must precede '## 3. Scope' after AutoFix")
	}
	// Non-destructive: every section body survives.
	for _, name := range canonicalSections(PhaseSS) {
		if !strings.Contains(fixed, fmt.Sprintf("Fixture content for %s.", name)) {
			t.Errorf("body of section %q was lost by AutoFix", name)
		}
	}
	// Round-trip: the fixed document scans clean.
	assertNoIssues(t, path, mustScan(t, path, fixed))
}

// [Edge] Scenario: Quét tài liệu đã hoàn toàn chuẩn -> AutoFix không thay đổi gì
// Input: Markdown chuẩn 100% chạy qua AutoFix
// Expect: Output trả về giống byte-for-byte với input
func TestAutoFixDocument_AlreadyCompliant_NoChange(t *testing.T) {
	for _, phase := range allPhases {
		content := buildFixture(phase)
		out, err := AutoFixDocument(content, phase)
		if err != nil {
			t.Fatalf("phase %s: AutoFixDocument error: %v", phase, err)
		}
		if out != content {
			t.Fatalf("phase %s: AutoFix must not change an already-compliant document\n--- input ---\n%q\n--- output ---\n%q", phase, content, out)
		}
	}
	// The hand-written conforming task doc (with its extra section 9) is also
	// left byte-for-byte untouched.
	out, err := AutoFixDocument(validTaskDoc, PhaseTask)
	if err != nil {
		t.Fatalf("AutoFixDocument error: %v", err)
	}
	if out != validTaskDoc {
		t.Fatal("AutoFix must not change the already-compliant hand-written task document")
	}
}

// [Error] Scenario: Nội dung file rỗng hoàn toàn
// Input: content = ""
// Expect: Trả về ScanIssue Severity=Critical, RuleID="empty_document"
func TestScanDocument_EmptyContent_ReturnsCritical(t *testing.T) {
	for _, content := range []string{"", "   \n\t  \n"} {
		issues, err := ScanDocument(fixturePath(PhaseTask), content)
		if err != nil {
			t.Fatalf("ScanDocument(%q) error: %v", content, err)
		}
		if len(issues) != 1 {
			t.Fatalf("content %q: expected exactly 1 issue, got %d:\n%s", content, len(issues), formatIssues(issues))
		}
		iss := issues[0]
		if iss.RuleID != "empty_document" {
			t.Errorf("content %q: RuleID = %q, want empty_document", content, iss.RuleID)
		}
		if iss.Severity != SeverityCritical {
			t.Errorf("content %q: Severity = %q, want critical", content, iss.Severity)
		}
		if iss.CanAutoFix {
			t.Errorf("content %q: CanAutoFix = true, want false", content)
		}
		if iss.Line != 1 {
			t.Errorf("content %q: Line = %d, want 1", content, iss.Line)
		}
	}
}

// [Error] Scenario: File chứa binary / non-UTF8 content
// Input: Nội dung binary không phải markdown hợp lệ
// Expect: Trả về error rõ ràng, không corrupt dữ liệu
func TestAutoFixDocument_BinaryContent_ReturnsError(t *testing.T) {
	binary := "\x00\x01\x02\xff\xfe\xfdPK\u0003\u0004-not-markdown"
	out, err := AutoFixDocument(binary, PhaseTask)
	if err == nil {
		t.Fatal("expected an error for binary content, got nil")
	}
	if !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("error should mention UTF-8, got: %v", err)
	}
	if out != "" {
		t.Errorf("on error the result must be empty, got %q", out)
	}
	// Pure NUL bytes are binary even though they are valid UTF-8.
	if _, err := AutoFixDocument("hello\x00world", PhaseTask); err == nil {
		t.Error("expected an error for NUL-byte content, got nil")
	}
}

// [Error] Scenario: Phase không được hỗ trợ
// Input: phase = "unknown_phase"
// Expect: Trả về error rõ ràng "unsupported phase"
func TestAutoFixDocument_UnsupportedPhase_ReturnsError(t *testing.T) {
	out, err := AutoFixDocument("# Some doc\n\ncontent\n", "unknown_phase")
	if err == nil {
		t.Fatal("expected an error for unsupported phase, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported phase") {
		t.Errorf("error should contain 'unsupported phase', got: %v", err)
	}
	if out != "" {
		t.Errorf("on error the result must be empty, got %q", out)
	}
}

// ---------------------------------------------------------------------------
// Sync test against the real FORMAT-REFERENCE-*.md samples (acceptance:
// zero issues on every reference file).
// ---------------------------------------------------------------------------

func formatReferenceSamples() []struct {
	rel   string
	phase string
} {
	return []struct {
		rel   string
		phase string
	}{
		{"requirements/05-System-Specs/FORMAT-REFERENCE-SS.md", PhaseSS},
		{"requirements/06-System-Tech-Design/FORMAT-REFERENCE-SD.md", PhaseSD},
		{"requirements/07-Coding-Plan/FORMAT-REFERENCE-CP.md", PhaseCP},
		{"requirements/08-Task/FORMAT-REFERENCE-TASK.md", PhaseTask},
		{"requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md", PhaseBugFix},
	}
}

func TestScanDocument_FormatReferenceSamples_ZeroIssues(t *testing.T) {
	root := findRepoRoot(t)
	for _, sample := range formatReferenceSamples() {
		path := filepath.Join(root, sample.rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		issues := mustScan(t, path, string(data))
		assertNoIssues(t, path, issues)
	}
}

func TestAutoFixDocument_FormatReferenceSamples(t *testing.T) {
	root := findRepoRoot(t)
	for _, sample := range formatReferenceSamples() {
		path := filepath.Join(root, sample.rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		content := string(data)
		fixed, err := AutoFixDocument(content, sample.phase)
		if err != nil {
			t.Fatalf("AutoFixDocument(%s): %v", path, err)
		}
		switch {
		case sample.phase == PhaseTask || sample.phase == PhaseBugFix:
			// Task/BugFix guides conform as-is: autofix must be a no-op.
			if fixed != content {
				t.Fatalf("AutoFix changed the conforming reference %s byte-for-byte", path)
			}
		default:
			// SS/SD/CP guides omit the optional 'Feature Keys' field, and the
			// fixer (phase-only, no filename context) adds it. The result must
			// still scan clean when scanned as the reference path.
			if !strings.Contains(fixed, "- Feature Keys: None") {
				t.Fatalf("AutoFix(%s) did not add 'Feature Keys: None'", path)
			}
			assertNoIssues(t, path, mustScan(t, path, fixed))
		}
	}
}

// ---------------------------------------------------------------------------
// ScanDirectory: report counts and performance budget
// ---------------------------------------------------------------------------

func TestScanDirectory_Performance_120Files_Under500ms(t *testing.T) {
	dir := t.TempDir()
	dirNames := map[string]string{
		PhaseSS:     "05-System-Specs",
		PhaseSD:     "06-System-Tech-Design",
		PhaseCP:     "07-Coding-Plan",
		PhaseTask:   "08-Task",
		PhaseBugFix: "09-BugFix",
	}
	phases := []string{PhaseSS, PhaseSD, PhaseCP, PhaseTask, PhaseBugFix}
	const total = 120
	prefixes := map[string]string{
		PhaseSS: "SS", PhaseSD: "SD", PhaseCP: "CP", PhaseTask: "Task", PhaseBugFix: "BUG",
	}
	for i := 0; i < total; i++ {
		phase := phases[i%len(phases)]
		sub := ""
		if i%3 == 0 {
			sub = "todo"
		} else if i%3 == 1 {
			sub = "done"
		}
		target := filepath.Join(dir, dirNames[phase], sub)
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("%s-%03d-Perf.md", prefixes[phase], i)
		if err := os.WriteFile(filepath.Join(target, name), []byte(buildFixture(phase)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Now()
	report, err := ScanDirectory(dir)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ScanDirectory error: %v", err)
	}
	if report.TotalFilesScanned != total {
		t.Errorf("TotalFilesScanned = %d, want %d", report.TotalFilesScanned, total)
	}
	if report.ConformingFiles != total {
		t.Errorf("ConformingFiles = %d, want %d (issues: %s)", report.ConformingFiles, total, formatIssues(report.Issues))
	}
	if len(report.Issues) != 0 {
		t.Errorf("expected 0 issues on generated conforming docs, got:\n%s", formatIssues(report.Issues))
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("ScanDirectory took %v for %d files, budget is 500ms", elapsed, total)
	}
	t.Logf("scanned %d files in %v", total, elapsed)
}

func TestScanDirectory_ReportCounts(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("05-System-Specs/SS-01-Good.md", buildFixture(PhaseSS))
	write("08-Task/todo/Task-001-Good.md", buildFixture(PhaseTask))
	write("08-Task/Task-002-Broken.md", buildFixture(PhaseTask, withoutAIV()))
	write("README.md", "# Not a phase document\n")
	write("notes.txt", "not markdown")

	report, err := ScanDirectory(dir)
	if err != nil {
		t.Fatalf("ScanDirectory error: %v", err)
	}
	if report.TotalFilesScanned != 3 {
		t.Errorf("TotalFilesScanned = %d, want 3 (non-phase markdown must be skipped)", report.TotalFilesScanned)
	}
	if report.ConformingFiles != 2 {
		t.Errorf("ConformingFiles = %d, want 2", report.ConformingFiles)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d:\n%s", len(report.Issues), formatIssues(report.Issues))
	}
	if report.Issues[0].RuleID != ruleMissingAIQuickView {
		t.Errorf("RuleID = %q, want %q", report.Issues[0].RuleID, ruleMissingAIQuickView)
	}
}

// ---------------------------------------------------------------------------
// Rule coverage beyond the mandated scenarios
// ---------------------------------------------------------------------------

func TestScanDocument_MissingMetadataBlock(t *testing.T) {
	path := fixturePath(PhaseTask)
	content := buildFixture(PhaseTask, withoutMeta())
	issues := mustScan(t, path, content)
	if len(issues) != 1 {
		t.Fatalf("expected exactly 1 issue (block-level), got %d:\n%s", len(issues), formatIssues(issues))
	}
	iss := issues[0]
	if iss.RuleID != ruleMissingMetadataBlock {
		t.Errorf("RuleID = %q, want %q", iss.RuleID, ruleMissingMetadataBlock)
	}
	if iss.Severity != SeverityCritical {
		t.Errorf("Severity = %q, want critical", iss.Severity)
	}
	if !iss.CanAutoFix {
		t.Error("CanAutoFix = false, want true")
	}
}

func TestScanDocument_MissingAIQuickView(t *testing.T) {
	path := fixturePath(PhaseTask)
	content := buildFixture(PhaseTask, withoutAIV())
	issues := mustScan(t, path, content)
	if len(issues) != 1 {
		t.Fatalf("expected exactly 1 issue, got %d:\n%s", len(issues), formatIssues(issues))
	}
	iss := issues[0]
	if iss.RuleID != ruleMissingAIQuickView {
		t.Errorf("RuleID = %q, want %q", iss.RuleID, ruleMissingAIQuickView)
	}
	if iss.Severity != SeverityImportant {
		t.Errorf("Severity = %q, want important", iss.Severity)
	}
	if !iss.CanAutoFix {
		t.Error("CanAutoFix = false, want true")
	}
}

func TestScanDocument_MissingAIQuickViewSubsections(t *testing.T) {
	path := fixturePath(PhaseTask)
	content := buildFixture(PhaseTask, omitAIVSub("Open Questions"), omitAIVSub("Source Refs"))
	issues := mustScan(t, path, content)
	oq := findRule(issues, ruleMissingOpenQuestions)
	if oq == nil {
		t.Fatalf("expected missing_open_questions issue, got:\n%s", formatIssues(issues))
	}
	if oq.Severity != SeverityMinor {
		t.Errorf("missing_open_questions Severity = %q, want minor", oq.Severity)
	}
	sr := findRule(issues, ruleMissingAIVSubsection)
	if sr == nil {
		t.Fatalf("expected missing_ai_quick_view_subsection issue, got:\n%s", formatIssues(issues))
	}
	if sr.Severity != SeverityImportant {
		t.Errorf("missing_ai_quick_view_subsection Severity = %q, want important", sr.Severity)
	}
	if !strings.Contains(sr.Message, "Source Refs") {
		t.Errorf("subsection issue message should name the missing sub-section, got: %s", sr.Message)
	}
}

func TestScanDocument_MissingRequiredField(t *testing.T) {
	path := fixturePath(PhaseSS)
	content := buildFixture(PhaseSS, omitField("Status"), omitField("Owner"))
	issues := mustScan(t, path, content)
	if len(issues) != 2 {
		t.Fatalf("expected exactly 2 issues, got %d:\n%s", len(issues), formatIssues(issues))
	}
	for _, iss := range issues {
		if iss.RuleID != ruleMissingRequiredField {
			t.Errorf("RuleID = %q, want %q", iss.RuleID, ruleMissingRequiredField)
		}
		if iss.Severity != SeverityImportant {
			t.Errorf("Severity = %q, want important", iss.Severity)
		}
		if !iss.CanAutoFix {
			t.Error("CanAutoFix = false, want true")
		}
	}
	joined := issues[0].Message + issues[1].Message
	if !strings.Contains(joined, `"Status"`) || !strings.Contains(joined, `"Owner"`) {
		t.Errorf("messages should name the missing fields, got: %s", joined)
	}
}

func TestScanDocument_MissingRequiredSection(t *testing.T) {
	path := fixturePath(PhaseTask)
	content := buildFixture(PhaseTask, omitSection("Exact Change"))
	issues := mustScan(t, path, content)
	if len(issues) != 1 {
		t.Fatalf("expected exactly 1 issue, got %d:\n%s", len(issues), formatIssues(issues))
	}
	iss := issues[0]
	if iss.RuleID != ruleMissingRequiredSection {
		t.Errorf("RuleID = %q, want %q", iss.RuleID, ruleMissingRequiredSection)
	}
	if iss.Severity != SeverityImportant {
		t.Errorf("Severity = %q, want important", iss.Severity)
	}
	if !iss.CanAutoFix {
		t.Error("CanAutoFix = false, want true")
	}
	if !strings.Contains(iss.Message, "Exact Change") {
		t.Errorf("message should name the missing section, got: %s", iss.Message)
	}
}

func TestScanDocument_FeatureKeys_OnlyForGoverningPhases(t *testing.T) {
	// Task and BugFix documents never carry 'Feature Keys' and must scan clean.
	for _, phase := range []string{PhaseTask, PhaseBugFix} {
		issues := mustScan(t, fixturePath(phase), buildFixture(phase))
		assertNoIssues(t, fixturePath(phase), issues)
		if iss := findRule(issues, ruleMissingFeatureKeys); iss != nil {
			t.Errorf("phase %s: missing_feature_keys must never fire on execution docs", phase)
		}
	}
	// CP and SD governing docs without the field are flagged, like SS.
	for _, phase := range []string{PhaseSD, PhaseCP} {
		issues := mustScan(t, fixturePath(phase), buildFixture(phase, omitFeatureKeys()))
		if findRule(issues, ruleMissingFeatureKeys) == nil {
			t.Errorf("phase %s: expected missing_feature_keys issue, got:\n%s", phase, formatIssues(issues))
		}
	}
}

func TestScanDocument_UndetectablePhase_ReturnsError(t *testing.T) {
	issues, err := ScanDocument("docs/readme.md", "# Hello\n\nsome content\n")
	if err == nil {
		t.Fatalf("expected error for undetectable phase, got issues %v", issues)
	}
	if issues != nil {
		t.Errorf("issues should be nil on error, got %v", issues)
	}
}

// ---------------------------------------------------------------------------
// AutoFixDocument: further structural repairs
// ---------------------------------------------------------------------------

func TestAutoFixDocument_ReordersScrambledSections(t *testing.T) {
	path := fixturePath(PhaseTask)
	content := buildFixture(PhaseTask, withSectionOrder(
		"Trigger", "Goal", "Parent Links", "Exact Change", "Touched Areas",
		"Acceptance Check", "Out of Scope", "Completion Notes",
	))
	fixed, err := AutoFixDocument(content, PhaseTask)
	if err != nil {
		t.Fatalf("AutoFixDocument error: %v", err)
	}
	assertNoIssues(t, path, mustScan(t, path, fixed))

	numberedRe := regexp.MustCompile(`(?m)^## \d+\. .+$`)
	got := numberedRe.FindAllString(fixed, -1)
	want := []string{
		"## 1. Goal", "## 2. Parent Links", "## 3. Trigger", "## 4. Exact Change",
		"## 5. Touched Areas", "## 6. Acceptance Check", "## 7. Out of Scope",
		"## 8. Completion Notes",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d numbered headings, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("heading %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAutoFixDocument_InsertsMissingBlocksAndSections(t *testing.T) {
	path := fixturePath(PhaseTask)
	content := "# Task-901: Bare Document\n\nJust some prose that must survive.\n"

	fixed, err := AutoFixDocument(content, PhaseTask)
	if err != nil {
		t.Fatalf("AutoFixDocument error: %v", err)
	}
	if !strings.Contains(fixed, "## Metadata") {
		t.Error("fixed document must contain a '## Metadata' block")
	}
	if !strings.Contains(fixed, "## AI Quick View") {
		t.Error("fixed document must contain an '## AI Quick View' block")
	}
	for _, name := range canonicalSections(PhaseTask) {
		if !strings.Contains(fixed, "- TODO") {
			t.Error("missing-section skeletons must carry a TODO marker")
			break
		}
		if !strings.Contains(fixed, name) {
			t.Errorf("fixed document must contain skeleton for section %q", name)
		}
	}
	if !strings.Contains(fixed, "Just some prose that must survive.") {
		t.Error("AutoFix must preserve original body text")
	}
	assertNoIssues(t, path, mustScan(t, path, fixed))
}

func TestAutoFixDocument_PreservesLineEndingsAndBody(t *testing.T) {
	crlf := strings.ReplaceAll(buildFixture(PhaseTask, withSectionOrder(
		"Trigger", "Goal", "Parent Links", "Exact Change", "Touched Areas",
		"Acceptance Check", "Out of Scope", "Completion Notes",
	)), "\n", "\r\n")

	fixed, err := AutoFixDocument(crlf, PhaseTask)
	if err != nil {
		t.Fatalf("AutoFixDocument error: %v", err)
	}
	if !strings.Contains(fixed, "\r\n") {
		t.Error("CRLF line endings must be preserved")
	}
	for _, name := range canonicalSections(PhaseTask) {
		if !strings.Contains(fixed, fmt.Sprintf("Fixture content for %s.\r", name)) {
			t.Errorf("body line of section %q was lost or its CRLF ending altered", name)
		}
	}
	assertNoIssues(t, fixturePath(PhaseTask), mustScan(t, fixturePath(PhaseTask), fixed))
}

// ---------------------------------------------------------------------------
// Rules table and phase normalization
// ---------------------------------------------------------------------------

func TestDefaultConformanceRules(t *testing.T) {
	all := map[string]bool{}
	for _, p := range allPhases {
		all[p] = true
	}
	governing := map[string]bool{PhaseSS: true, PhaseSD: true, PhaseCP: true}

	phasesMatch := func(phases []string, want map[string]bool) bool {
		if len(phases) != len(want) {
			return false
		}
		for _, p := range phases {
			if !want[p] {
				return false
			}
		}
		return true
	}

	byID := map[string]ConformanceRule{}
	for _, r := range DefaultConformanceRules() {
		if _, dup := byID[r.ID]; dup {
			t.Errorf("duplicate rule id %q", r.ID)
		}
		byID[r.ID] = r
	}

	expected := map[string]struct {
		severity Severity
		phases   map[string]bool
		canFix   bool
	}{
		ruleMissingMetadataBlock: {SeverityCritical, all, true},
		ruleMissingAIQuickView:   {SeverityImportant, all, true},
		ruleMissingOpenQuestions: {SeverityMinor, all, true},
		ruleSectionOutOfOrder:    {SeverityMinor, all, true},
		ruleMissingFeatureKeys:   {SeverityImportant, governing, true},
		ruleMissingRequiredField: {SeverityImportant, all, true},
		ruleEmptyDocument:        {SeverityCritical, all, false},
	}
	for id, want := range expected {
		r, ok := byID[id]
		if !ok {
			t.Errorf("DefaultConformanceRules is missing mandated rule %q", id)
			continue
		}
		if r.Severity != want.severity {
			t.Errorf("rule %q severity = %q, want %q", id, r.Severity, want.severity)
		}
		if !phasesMatch(r.Phases, want.phases) {
			t.Errorf("rule %q phases = %v, want %v", id, r.Phases, want.phases)
		}
		if r.CanAutoFix != want.canFix {
			t.Errorf("rule %q CanAutoFix = %v, want %v", id, r.CanAutoFix, want.canFix)
		}
	}
}

func TestNormalizePhase(t *testing.T) {
	cases := map[string]string{
		"SS":          PhaseSS,
		"ss":          PhaseSS,
		"system_spec": PhaseSS,
		"SD":          PhaseSD,
		"Tech Design": PhaseSD,
		"cp":          PhaseCP,
		"coding_plan": PhaseCP,
		"Task":        PhaseTask,
		"BUGFIX":      PhaseBugFix,
		"bug_fix":     PhaseBugFix,
	}
	for input, want := range cases {
		got, err := NormalizePhase(input)
		if err != nil {
			t.Errorf("NormalizePhase(%q) error: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizePhase(%q) = %q, want %q", input, got, want)
		}
	}
	for _, bad := range []string{"unknown_phase", "", "docx"} {
		if _, err := NormalizePhase(bad); err == nil {
			t.Errorf("NormalizePhase(%q) expected error, got nil", bad)
		} else if !strings.Contains(err.Error(), "unsupported phase") {
			t.Errorf("NormalizePhase(%q) error should contain 'unsupported phase', got: %v", bad, err)
		}
	}
}
