package flowgate

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-1183: r-task fired on legs that merely REFERENCE a task they did not
// author — reviewer / spec-aligner / synthesis turns routinely mention
// "Task-039" in the final message while writing only prose. Requiring the
// task doc inside THIS turn's GitDiff is a false positive: the doc's presence
// in the workspace satisfies the rule's contract (the referenced task is
// documented), exactly like a doc authored in any earlier turn would.

func writeTaskDoc(t *testing.T, root, rel string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte("# Task doc\n"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
}

func bug1183Rule() Rule {
	return Rule{ID: "r-task", Scope: "step", Trigger: "task_referenced", RequiredOutput: "task_doc", Action: "reprompt", Enabled: true}
}

func TestBug1183_TaskRefSatisfiedByExistingDocOnDisk(t *testing.T) {
	dir := t.TempDir()
	writeTaskDoc(t, dir, "requirements/08-Task/todo/Task-039-vault-io.md")
	tr := TurnResult{
		FinalMessage: "Review complete for Task-039 — no code changes required.",
		GitDiff:      []ChangedFile{{Path: "app/src/main.kt", Status: "M"}},
		WorkspaceCwd: dir,
	}
	if v := checkRule(bug1183Rule(), tr); v != nil {
		t.Fatalf("existing Task-039 doc on disk must satisfy r-task for a non-writer leg, got %v", v.Detail)
	}
}

func TestBug1183_TaskRefMissingDocStillViolates(t *testing.T) {
	dir := t.TempDir() // no task docs anywhere
	tr := TurnResult{
		FinalMessage: "Review complete for Task-039 — no code changes required.",
		GitDiff:      []ChangedFile{{Path: "app/src/main.kt", Status: "M"}},
		WorkspaceCwd: dir,
	}
	if v := checkRule(bug1183Rule(), tr); v == nil {
		t.Fatal("referenced task with no doc anywhere must still violate r-task")
	}
}

func TestBug1183_TaskRefWrongIDStillViolates(t *testing.T) {
	dir := t.TempDir()
	writeTaskDoc(t, dir, "requirements/08-Task/done/Task-001-old.md")
	tr := TurnResult{
		FinalMessage: "Scaffolded per Task-039.",
		GitDiff:      []ChangedFile{{Path: "app/src/main.kt", Status: "M"}},
		WorkspaceCwd: dir,
	}
	if v := checkRule(bug1183Rule(), tr); v == nil {
		t.Fatal("only an unrelated Task-001 doc exists — the Task-039 reference must still violate")
	}
}

func TestBug1183_DeclaredTaskModeSatisfiedByExistingDoc(t *testing.T) {
	dir := t.TempDir()
	writeTaskDoc(t, dir, "requirements/08-Task/done/Task-114-selector.md")
	tr := TurnResult{
		ChangeType:  "task",
		SourceDocID: "Task-114",
		GitDiff:     []ChangedFile{{Path: "app/src/main.kt", Status: "M"}},
		WorkspaceCwd: dir,
	}
	if v := checkRule(bug1183Rule(), tr); v != nil {
		t.Fatalf("declared task mode with Task-114 doc already on disk must pass, got %v", v.Detail)
	}
}

func TestBug1183_TaskDocInHeldWrittenPaths(t *testing.T) {
	// BUG-288 shape: a remediation re-check carries pendingGateCodePaths as
	// WrittenPaths with an empty GitDiff — the task doc the flow authored must
	// still satisfy r-task, mirroring HasBugFixDocInPaths.
	tr := TurnResult{
		FinalMessage: "see Task-039",
		WrittenPaths: []string{"requirements/08-Task/todo/Task-039-vault-io.md"},
	}
	if v := checkRule(bug1183Rule(), tr); v != nil {
		t.Fatalf("held WrittenPaths task doc must satisfy r-task, got %v", v.Detail)
	}
}

func TestBug1183_EmptyWorkspaceKeepsPriorBehavior(t *testing.T) {
	// WorkspaceCwd empty → the disk check cannot run; nothing changes vs today.
	tr := TurnResult{
		FinalMessage: "mentions Task-039",
		GitDiff:      []ChangedFile{{Path: "app/src/main.kt", Status: "M"}},
	}
	if v := checkRule(bug1183Rule(), tr); v == nil {
		t.Fatal("empty WorkspaceCwd must keep the pre-BUG-1183 fail behavior")
	}
}
