package flowgate

import (
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
