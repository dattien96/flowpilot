package flowgate

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// Task-330 (CP-47 P-1): unit tests for the deterministic DOD markdown parser.
// New file only — no pre-existing test is modified.

// Scenario: Tài liệu hợp lệ có đầy đủ checklist Definition of Done
// Input: Markdown chứa ## Definition of Done kèm 2 mục [x] và 1 mục [ ]
// Expect: Present=true, Total=3, Checked=2, len(OpenItems)=1
func TestParseDefinitionOfDone_ValidChecklist(t *testing.T) {
	content := `# Task-100: Sample

## Metadata

- Status: draft

## Definition of Done

- [x] Parser handles missing section
- [x] Parser counts checkboxes
- [ ] Gate fires on missing DOD

## Notes

- [ ] this checkbox is outside the DOD section and must not count
`
	got := ParseDefinitionOfDone(content)
	if !got.Present {
		t.Fatalf("Present = false, want true")
	}
	if got.Total != 3 {
		t.Fatalf("Total = %d, want 3 (checkboxes after the next heading must not count)", got.Total)
	}
	if got.Checked != 2 {
		t.Fatalf("Checked = %d, want 2", got.Checked)
	}
	if len(got.OpenItems) != 1 {
		t.Fatalf("len(OpenItems) = %d, want 1 (%v)", len(got.OpenItems), got.OpenItems)
	}
	if len(got.OpenItems) > 0 && got.OpenItems[0] != "Gate fires on missing DOD" {
		t.Fatalf("OpenItems[0] = %q, want %q", got.OpenItems[0], "Gate fires on missing DOD")
	}
}

// Scenario: Tiêu đề DoD viết tắt hoặc hoa thường khác nhau
// Input: Markdown chứa ## DoD với 1 mục [ ]
// Expect: Present=true, Total=1, Checked=0
func TestParseDefinitionOfDone_CaseInsensitiveHeading(t *testing.T) {
	content := "# Task-101: Sample\n\n## DoD\n\n- [ ] one open item\n"
	got := ParseDefinitionOfDone(content)
	if !got.Present {
		t.Fatalf("Present = false, want true for '## DoD' heading")
	}
	if got.Total != 1 {
		t.Fatalf("Total = %d, want 1", got.Total)
	}
	if got.Checked != 0 {
		t.Fatalf("Checked = %d, want 0", got.Checked)
	}
	// Lowercase and re-spaced variants of the long heading are accepted too.
	for _, heading := range []string{"## definition of done", "## Definition  OF  Done"} {
		variant := heading + "\n\n- [x] done item\n"
		v := ParseDefinitionOfDone(variant)
		if !v.Present || v.Total != 1 || v.Checked != 1 {
			t.Fatalf("heading %q: Present=%v Total=%d Checked=%d, want true/1/1", heading, v.Present, v.Total, v.Checked)
		}
	}
}

// Scenario: Section DOD chỉ có đoạn văn mô tả, không có checkbox nào
// Input: Markdown chứa ## Definition of Done nhưng bên dưới chỉ là text
// Expect: Total=0 (coi như không hợp lệ)
func TestParseDefinitionOfDone_NoCheckboxes(t *testing.T) {
	content := "# Task-102: Sample\n\n## Definition of Done\n\nAll work must be verified manually before completion.\n"
	got := ParseDefinitionOfDone(content)
	if got.Total != 0 {
		t.Fatalf("Total = %d, want 0 (prose-only section counts as missing)", got.Total)
	}
	if got.Present {
		t.Fatal("Present = true, want false when the section has 0 checkboxes")
	}
	if len(got.OpenItems) != 0 {
		t.Fatalf("OpenItems = %v, want empty", got.OpenItems)
	}
}

// Scenario: Checkbox nằm thụt đầu dòng (nested/indented)
// Input: Markdown chứa danh sách checklist thụt lề 2 hoặc 4 spaces
// Expect: Đếm chính xác cả các checkbox thụt dòng
func TestParseDefinitionOfDone_IndentedCheckboxes(t *testing.T) {
	content := "## Definition of Done\n\n- [ ] top level\n  - [x] indented two spaces\n    - [ ] indented four spaces\n"
	got := ParseDefinitionOfDone(content)
	if !got.Present {
		t.Fatal("Present = false, want true for indented checkboxes")
	}
	if got.Total != 3 {
		t.Fatalf("Total = %d, want 3 (indentation must not hide checkboxes)", got.Total)
	}
	if got.Checked != 1 {
		t.Fatalf("Checked = %d, want 1", got.Checked)
	}
	if len(got.OpenItems) != 2 {
		t.Fatalf("len(OpenItems) = %d, want 2 (%v)", len(got.OpenItems), got.OpenItems)
	}
}

// Scenario: Không có section DOD trong tài liệu
// Input: Markdown chỉ có ## Summary và ## Goal
// Expect: Present=false, Total=0
func TestParseDefinitionOfDone_MissingSection(t *testing.T) {
	content := "# Task-103: Sample\n\n## Summary\n\nSome text.\n\n## Goal\n\nOther text.\n"
	got := ParseDefinitionOfDone(content)
	if got.Present {
		t.Fatal("Present = true, want false when there is no DOD section")
	}
	if got.Total != 0 {
		t.Fatalf("Total = %d, want 0", got.Total)
	}
	if got.Checked != 0 {
		t.Fatalf("Checked = %d, want 0", got.Checked)
	}
	if len(got.OpenItems) != 0 {
		t.Fatalf("OpenItems = %v, want empty", got.OpenItems)
	}
}

// Scenario: Hardening — fenced code blocks never open or feed the DOD section
// Input: doc A quotes a fake "## Definition of Done" + checkbox inside ``` fences and has no real DOD;
//
//	doc B has a real DOD section containing a fenced block with a fake checkbox
//
// Expect: A: Present=false, Total=0 (no gaming vector). B: only the real checkbox counts.
func TestParseDefinitionOfDone_FencedBlocksIgnored(t *testing.T) {
	gamedDoc := "# Task\n\nExample:\n\n```md\n## Definition of Done\n- [x] fake\n```\n\nDone.\n"
	if got := ParseDefinitionOfDone(gamedDoc); got.Present || got.Total != 0 {
		t.Fatalf("fenced fake DOD must not count, got %+v", got)
	}
	realWithFencedFake := "## Definition of Done\n\n- [x] real criterion\n\n```md\n- [x] fake inside fence\n- [ ] another fake\n```\n\n## Notes\n"
	got := ParseDefinitionOfDone(realWithFencedFake)
	if got.Total != 1 || got.Checked != 1 || len(got.OpenItems) != 0 {
		t.Fatalf("fenced checkboxes must not count, got %+v", got)
	}
	unclosed := "## Definition of Done\n\n- [x] real\n\n```md\n- [ ] never counted\n"
	if got := ParseDefinitionOfDone(unclosed); got.Total != 1 || got.Checked != 1 {
		t.Fatalf("unclosed fence must not leak fake items, got %+v", got)
	}
}

// Scenario: Hardening — very long lines no longer truncate parsing
// Input: a 200KB single line before the DOD section, then a valid checklist
// Expect: Present=true, Total=2 (bufio.Scanner's 64KB limit previously cut this silently)
func TestParseDefinitionOfDone_LongLines_NoTruncation(t *testing.T) {
	huge := strings.Repeat("x", 200*1024)
	doc := "prose " + huge + "\n\n## Definition of Done\n\n- [x] a\n- [ ] b\n"
	got := ParseDefinitionOfDone(doc)
	if !got.Present || got.Total != 2 || got.Checked != 1 || len(got.OpenItems) != 1 {
		t.Fatalf("long line must not break parsing, got %+v", got)
	}
}

// Scenario: Round-2 blocking regression — the NUMBERED DOD heading form that
// every real Task-*/BUG-* document uses ("## 9. Definition of Done",
// "## 11. Definition of Done") must be recognized, not just the unnumbered
// form; non-DOD numbered headings and lookalike words stay rejected.
// Input: numbered DOD headings with checklists; "## 9. Definition of Doneness"
// Expect: numbered DODs counted (Present=true, correct totals); lookalikes not
func TestParseDefinitionOfDone_NumberedHeadingForms(t *testing.T) {
	taskDoc := "## 8. Completion Notes\n\n- done\n\n## 9. Definition of Done\n\n- [x] parser hardened\n- [ ] suite green\n"
	if got := ParseDefinitionOfDone(taskDoc); got.Total != 2 || got.Checked != 1 || len(got.OpenItems) != 1 {
		t.Fatalf("numbered Task heading must be recognized, got %+v", got)
	}
	bugDoc := "## 10. Regression Guard\n\n- n/a\n\n## 11. Definition of Done\n\n- [X] fixed\n"
	if got := ParseDefinitionOfDone(bugDoc); got.Total != 1 || got.Checked != 1 {
		t.Fatalf("numbered BUG heading must be recognized, got %+v", got)
	}
	if got := ParseDefinitionOfDone("## 9. Definition of Doneness\n\n- [x] lookalike\n"); got.Present {
		t.Fatalf("lookalike word after 'done' must not open a DOD section, got %+v", got)
	}
	if got := ParseDefinitionOfDone("## 9. Trigger\n\n- [x] not dod\n"); got.Present {
		t.Fatalf("non-DOD numbered heading must not count, got %+v", got)
	}
	if got := ParseDefinitionOfDone("## Definition of Done\n\n- [x] unnumbered still works\n"); got.Total != 1 {
		t.Fatalf("unnumbered form must keep working, got %+v", got)
	}
}

// Review round-2 acceptance (locked): the parser must recognize the DOD
// section in the repo's REAL Task-*/BUG-* documents, which use the NUMBERED
// heading form ("## 9. Definition of Done" / "## 11. Definition of Done").
// Every done/ document that carries a DOD heading (numbered or not) must be
// parsed as Present. Skips gracefully when the requirements tree is absent.
func TestParseDefinitionOfDone_RecognizesRealRepoDocs(t *testing.T) {
	dir := flowgateRepoRoot()
	if dir == "" {
		t.Skip("repository requirements/ tree not found above the package dir")
	}
	// Ground truth widened in round 3 (the round-2 ground truth was h2-biased
	// and structurally could not see the h3 DOD-heading population): any-level
	// ATX heading (h2/h3), multi-part numbering, and parenthesized variants.
	looseDODHeading := regexp.MustCompile(`(?im)^#{2,3}\s+(?:\d+(?:\.\d+)*[.)]?\s*)?definition of done\b|^#{2,3}\s+.*\(definition of done\)`)
	contractCheckbox := regexp.MustCompile(`^\s*-\s*\[([ xX])\]`)
	closeHeading := regexp.MustCompile(`^#{1,2}\s+`)
	checked, legacy := 0, 0
	var missed []string
	roots := []string{
		filepath.Join(dir, "requirements", "08-Task", "done"),
		filepath.Join(dir, "requirements", "09-BugFix", "done"),
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			base := filepath.Base(path)
			if !strings.HasPrefix(base, "Task-") && !strings.HasPrefix(base, "BUG-") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			lines := strings.Split(string(data), "\n")
			// Ground truth, independent of the parser: a DOD heading followed
			// (before the next h1/h2) by at least one SS-13 contract-form
			// checkbox. Legacy styles (prose bullets, backticked markers) are
			// counted separately — the gate NagGING them is by design (CP-47
			// R-1: run CP-48 autofix before enabling the gate).
			inDOD, hasCheckbox := false, false
			for _, line := range lines {
				if looseDODHeading.MatchString(line) {
					inDOD = true
					continue
				}
				if inDOD {
					if closeHeading.MatchString(line) {
						break
					}
					if contractCheckbox.MatchString(line) {
						hasCheckbox = true
						break
					}
				}
			}
			if !inDOD {
				return nil
			}
			if !hasCheckbox {
				legacy++
				return nil
			}
			checked++
			if !ParseDefinitionOfDone(string(data)).Present {
				missed = append(missed, base)
			}
			return nil
		})
	}
	if checked == 0 {
		t.Skip("no done/ Task-BUG docs with a contract-form DOD checklist found")
	}
	if len(missed) > 0 {
		t.Fatalf("parser missed contract-form DOD checklists in %d/%d real docs: %v", len(missed), checked, missed)
	}
	t.Logf("recognized %d contract-form DOD docs; %d legacy-style docs intentionally stay non-conforming", checked, legacy)
}

// flowgateRepoRoot walks up from this file looking for the repository root
// (identified by the requirements/05-System-Specs directory); "" when absent.
func flowgateRepoRoot() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return ""
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
	return ""
}

// Scenario: Round-3 blocking regression — h3-level DOD headings with
// multi-part numbering and parenthesized variants, used by 60+ real docs.
// Input: "### 6.1 Definition of Done (DOD)", "### 6.2 Definition of Done",
//
//	"## 6. Acceptance Check (Definition of Done)" with checklists
//
// Expect: all recognized (Present=true, checkboxes counted); non-DOD h3
//
//	headings stay rejected
func TestParseDefinitionOfDone_H3AndParenHeadingForms(t *testing.T) {
	h3Numbered := "### 6.1 Definition of Done (DOD)\n\n- [x] a\n- [ ] b\n\n### 6.3 Notes\n\n- plain\n"
	if got := ParseDefinitionOfDone(h3Numbered); got.Total != 2 || got.Checked != 1 {
		t.Fatalf("h3 multi-part DOD heading must be recognized, got %+v", got)
	}
	h3Plain := "### 6.2 Definition of Done\n\n- [x] only\n"
	if got := ParseDefinitionOfDone(h3Plain); got.Total != 1 || got.Checked != 1 {
		t.Fatalf("h3 plain DOD heading must be recognized, got %+v", got)
	}
	paren := "## 6. Acceptance Check (Definition of Done)\n\n- [x] gate\n- [ ] ui\n\n## 7. Next\n"
	if got := ParseDefinitionOfDone(paren); got.Total != 2 || got.Checked != 1 {
		t.Fatalf("parenthesized DOD heading must be recognized, got %+v", got)
	}
	if got := ParseDefinitionOfDone("### 6.3 Notes\n\n- [x] not a dod\n"); got.Present {
		t.Fatalf("non-DOD h3 heading must not open a DOD section, got %+v", got)
	}
}
