package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findEngineStep(steps []EngineInitStepResult, name string) *EngineInitStepResult {
	for i := range steps {
		if steps[i].Step == name {
			return &steps[i]
		}
	}
	return nil
}

// BUG-416: engine init on a directory that is not a git work tree must report
// hook_install truthfully (skipped) and must not fabricate .git/hooks.
func TestBug416_InitOnNonGitDirSkipsHookInstall(t *testing.T) {
	dir := t.TempDir() // deliberately NOT a git repo

	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: t.TempDir()})

	_, initState := svc.runEngineInit("project-1", dir, "golang", "manual", "all")
	if initState == nil {
		t.Fatalf("initState = nil")
	}

	step := findEngineStep(initState.Steps, "hook_install")
	if step == nil {
		t.Fatalf("hook_install step missing from %+v", initState.Steps)
	}
	if step.Outcome != "skipped" {
		t.Fatalf("hook_install outcome = %q (detail=%q err=%q); want skipped on a non-git dir",
			step.Outcome, step.Detail, step.ErrorMessage)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Fatalf(".git fabricated in non-git dir (stat err=%v)", err)
	}
}

// BUG-416: changeledger_build must not report an artifact path that does not
// exist — a repo with no commits produces no feature_history.ndjson.
func TestBug416_ChangeledgerStepTruthfulOnEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init") // git repo, zero commits

	svc := NewInteractiveService()
	svc.AttachRunner(&Runner{workspace: t.TempDir()})

	_, initState := svc.runEngineInit("project-1", dir, "golang", "manual", "all")
	if initState == nil {
		t.Fatalf("initState = nil")
	}

	ledgerPath := filepath.Join(dir, ".flowpilot", "ledger", "feature_history.ndjson")
	_, statErr := os.Stat(ledgerPath)
	fileExists := statErr == nil

	step := findEngineStep(initState.Steps, "changeledger_build")
	if step == nil {
		t.Fatalf("changeledger_build step missing")
	}
	if !fileExists && step.Outcome == "ok" && strings.Contains(step.Detail, "feature_history.ndjson") {
		t.Fatalf("changeledger_build reported ok naming %q but the file does not exist (detail=%q)",
			ledgerPath, step.Detail)
	}
}
