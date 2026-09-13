package flowgate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Task-330 (CP-47 P-2): gate tests for r-dod-present. New file only — no
// pre-existing test is modified.

const (
	// validDodDocTask100 is a Task document with a complete DOD checklist.
	validDodDocTask100 = `# Task-100: Sample

## Metadata

- Status: draft

## Definition of Done

- [x] Parser handles missing section
- [x] Parser counts checkboxes
- [ ] Gate fires on missing DOD
`
	// noDodDocTask100 is a Task document without any DOD section.
	noDodDocTask100 = `# Task-100: Sample

## Summary

This document has no Definition of Done section at all.
`
)

// writeDodWorkspaceDoc writes content at rel (slash-separated) under dir.
func writeDodWorkspaceDoc(t *testing.T, dir, rel, content string) {
	t.Helper()
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// findRDodPresentViolation returns the r-dod-present violation, or nil.
func findRDodPresentViolation(violations []Violation) *Violation {
	for i := range violations {
		if violations[i].Rule.ID == "r-dod-present" {
			return &violations[i]
		}
	}
	return nil
}

// Scenario: Ghi file Task có DOD hợp lệ -> Gate cho qua
// Input: TurnResult có WrittenPaths=["requirements/08-Task/todo/Task-100.md"], nội dung có DOD
// Expect: Không phát sinh violation r-dod-present
func TestRDodPresent_PassWhenValid(t *testing.T) {
	dir := t.TempDir()
	rel := "requirements/08-Task/todo/Task-100.md"
	writeDodWorkspaceDoc(t, dir, rel, validDodDocTask100)
	tr := TurnResult{
		WorkspaceCwd: dir,
		WrittenPaths: []string{rel},
	}
	if v := findRDodPresentViolation(Evaluate(tr, DefaultRules())); v != nil {
		t.Fatalf("r-dod-present must pass when the Task doc has a DOD checklist, got %+v", v)
	}
}

// Scenario: Ghi file Task thiếu DOD -> Gate reprompt
// Input: TurnResult có WrittenPaths=["requirements/08-Task/todo/Task-100.md"], nội dung không có DOD
// Expect: Phát sinh Violation rule "r-dod-present", Action="reprompt"
func TestRDodPresent_RepromptWhenMissing(t *testing.T) {
	dir := t.TempDir()
	rel := "requirements/08-Task/todo/Task-100.md"
	writeDodWorkspaceDoc(t, dir, rel, noDodDocTask100)
	tr := TurnResult{
		WorkspaceCwd: dir,
		WrittenPaths: []string{rel},
	}
	v := findRDodPresentViolation(Evaluate(tr, DefaultRules()))
	if v == nil {
		t.Fatal("expected r-dod-present violation when the Task doc has no DOD")
	}
	if v.Rule.Action != "reprompt" {
		t.Fatalf("Action = %q, want reprompt (present-gate never blocks)", v.Rule.Action)
	}
	if !strings.Contains(v.Detail, rel) {
		t.Fatalf("detail %q must name the offending doc", v.Detail)
	}
	if !strings.Contains(v.Detail, "Definition of Done") {
		t.Fatalf("detail %q must tell the AI to add a Definition of Done", v.Detail)
	}
}

// [Edge] Scenario: WrittenPaths chứa file không phải Task/BUG -> Gate bỏ qua
// Input: TurnResult có WrittenPaths=["requirements/05-System-Specs/SS-01.md"], nội dung thiếu DOD
// Expect: Không phát sinh violation r-dod-present (file không thuộc phạm vi)
func TestRDodPresent_IgnoresNonTaskBugFile(t *testing.T) {
	dir := t.TempDir()
	rel := "requirements/05-System-Specs/SS-01.md"
	writeDodWorkspaceDoc(t, dir, rel, noDodDocTask100)
	tr := TurnResult{
		WorkspaceCwd: dir,
		WrittenPaths: []string{rel},
	}
	if v := findRDodPresentViolation(Evaluate(tr, DefaultRules())); v != nil {
		t.Fatalf("r-dod-present must ignore non-Task/BUG docs, got %+v", v)
	}
}

// [Edge] Scenario: WrittenPaths rỗng -> Gate bỏ qua hoàn toàn
// Input: TurnResult có WrittenPaths=[] (chỉ sửa code)
// Expect: Không kích hoạt bất kỳ kiểm tra r-dod nào
func TestRDodPresent_EmptyWrittenPaths_Skips(t *testing.T) {
	dir := t.TempDir()
	tr := TurnResult{
		WorkspaceCwd: dir,
		WrittenPaths: nil,
	}
	if v := findRDodPresentViolation(Evaluate(tr, DefaultRules())); v != nil {
		t.Fatalf("r-dod-present must not fire when WrittenPaths is empty, got %+v", v)
	}
}

// [Error] Scenario: File không thể đọc được (quyền truy cập / bị xóa)
// Input: WrittenPaths=["requirements/08-Task/todo/Task-999.md"] nhưng file không tồn tại
// Expect: Gate xử lý graceful, log cảnh báo và không panic
func TestRDodPresent_FileReadError_GracefulDegradation(t *testing.T) {
	dir := t.TempDir() // deliberately does NOT contain Task-999.md
	rel := "requirements/08-Task/todo/Task-999.md"
	tr := TurnResult{
		WorkspaceCwd: dir,
		WrittenPaths: []string{rel},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("gate must not panic on unreadable file, got panic: %v", r)
		}
	}()
	if v := findRDodPresentViolation(Evaluate(tr, DefaultRules())); v != nil {
		t.Fatalf("unreadable doc must be skipped gracefully (fail-open reprompt gate), got %+v", v)
	}
}

// Task-330 fail-open contract: an empty WorkspaceCwd means the gate cannot
// read docs from disk, so it must skip instead of guessing from the path.
func TestRDodPresent_EmptyWorkspaceCwd_Skips(t *testing.T) {
	tr := TurnResult{
		WrittenPaths: []string{"requirements/08-Task/todo/Task-100.md"},
	}
	if v := findRDodPresentViolation(Evaluate(tr, DefaultRules())); v != nil {
		t.Fatalf("r-dod-present must skip when WorkspaceCwd is empty, got %+v", v)
	}
}

// Only incomplete Task/BUG docs are reported: a mixed turn (one doc with a
// checklist, one without) names exactly the incomplete one, and duplicates
// collapse into a single entry.
func TestMissingDodDocsReportsOnlyIncompleteDocs(t *testing.T) {
	dir := t.TempDir()
	okRel := "requirements/08-Task/todo/Task-100.md"
	badRel := "requirements/09-BugFix/todo/BUG-200.md"
	writeDodWorkspaceDoc(t, dir, okRel, validDodDocTask100)
	writeDodWorkspaceDoc(t, dir, badRel, noDodDocTask100)
	got := MissingDodDocs(dir, []string{okRel, badRel, badRel})
	if len(got) != 1 || got[0] != badRel {
		t.Fatalf("MissingDodDocs = %v, want [%s] (deduped, valid doc excluded)", got, badRel)
	}
}

// Task-330 §9: r-dod-present must be registered with the exact rule shape and
// join the tier-1 doc family (CP-47 §5 extends DocScopeRuleIDs).
func TestRDodPresentRegisteredInDefaultRules(t *testing.T) {
	found := false
	for _, r := range DefaultRules() {
		if r.ID == "r-dod-present" {
			found = true
			if r.Trigger != "task_or_bug_doc_missing_dod" {
				t.Fatalf("r-dod-present trigger = %q, want task_or_bug_doc_missing_dod", r.Trigger)
			}
			if r.RequiredOutput != "definition_of_done_section" {
				t.Fatalf("r-dod-present required_output = %q, want definition_of_done_section", r.RequiredOutput)
			}
			if r.Action != "reprompt" {
				t.Fatalf("r-dod-present action = %q, want reprompt", r.Action)
			}
			if r.Scope != "step" {
				t.Fatalf("r-dod-present scope = %q, want step", r.Scope)
			}
			if !r.Enabled {
				t.Fatal("r-dod-present must be enabled")
			}
		}
	}
	if !found {
		t.Fatal("DefaultRules must contain r-dod-present (Task-330)")
	}
	if !IsDocScopeRule("r-dod-present") {
		t.Fatal("r-dod-present must be in DocScopeRuleIDs (CP-47 §5 tier-1 doc family)")
	}
}

// Task-330 §9: MergeDefaultRules must auto-add r-dod-present to a legacy
// workspace flow-rules.json that predates the rule.
func TestMergeDefaultRulesAddsRDodPresent(t *testing.T) {
	legacy := []Rule{
		{ID: "r-ca", Scope: "step", Trigger: "code_changed", RequiredOutput: "change_audit_note", Action: "reprompt", Enabled: true},
	}
	// On-disk path: LoadRules merges defaults into the saved legacy file.
	dir := t.TempDir()
	if err := SaveRules(dir, legacy); err != nil {
		t.Fatalf("SaveRules: %v", err)
	}
	loaded, err := LoadRules(dir)
	if err != nil {
		t.Fatalf("LoadRules: %v", err)
	}
	if len(loaded) == 0 || loaded[0].ID != "r-ca" {
		t.Fatalf("loaded rules must keep stored order first, got %+v", loaded)
	}
	found := false
	for _, r := range loaded {
		if r.ID == "r-dod-present" {
			found = true
		}
	}
	if !found {
		t.Fatal("LoadRules must merge r-dod-present into a legacy flow-rules.json")
	}
	// Direct merge path: same guarantee, in-memory.
	merged := MergeDefaultRules(legacy)
	found = false
	for _, r := range merged {
		if r.ID == "r-dod-present" {
			found = true
		}
	}
	if !found {
		t.Fatal("MergeDefaultRules must append r-dod-present when missing")
	}
}
