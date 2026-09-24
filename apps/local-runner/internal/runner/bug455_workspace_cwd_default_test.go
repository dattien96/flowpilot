package runner

// BUG-455 (live run-94): a run created via the API with a bound project but
// no explicit cwd never resolves a workspace — rs.workspaceCwd stays "".
// Provider child sessions already fall back to Runner.workspace
// (sessions.go: req.WorkingDirectory "" → r.workspace) and worktreeRepoDir
// applies the same in.Cwd → runner.workspace default, so provider children
// write files in the project dir while every Go-inline consumer
// (command.validate's loadValidateCommand, captureGitHead,
// appendChangeContractIfAnyWithSecret, handoff, gate hooks) sees "".
//
// Live symptom: task-harness validate escalated skipped_no_command even after
// the operator configured .flowpilot/guard/test_baseline.json — cwd==""
// short-circuits loadValidateCommand before the baseline is ever read, and
// Continue re-runs validate into the same escalate: a dead park with no exit.
//
// Fix: default rs.workspaceCwd the same way worktreeRepoDir and the provider
// session path already do — in.Cwd wins, Runner.workspace is the default.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBug455_CreateRunDefaultsWorkspaceCwd asserts the create-time default:
// empty in.Cwd + runner workspace → rs.workspaceCwd is the runner workspace.
func TestBug455_CreateRunDefaultsWorkspaceCwd(t *testing.T) {
	svc, _ := newTestServer(t)
	ws := t.TempDir()
	svc.runner = &Runner{workspace: ws}

	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[h.RunID].workspaceCwd
	svc.mu.Unlock()
	if got != ws {
		t.Fatalf("workspaceCwd must default to runner workspace %q, got %q", ws, got)
	}
}

// TestBug455_ExplicitCwdStillWins pins the other half of the contract: a
// caller-supplied cwd is authoritative and is never overridden.
func TestBug455_ExplicitCwdStillWins(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.runner = &Runner{workspace: t.TempDir()}
	custom := t.TempDir()

	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui", Cwd: custom,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[h.RunID].workspaceCwd
	svc.mu.Unlock()
	if got != custom {
		t.Fatalf("explicit cwd must be kept, got %q want %q", got, custom)
	}
}

// TestBug455_ValidateFindsBaselineAfterCwdDefault is the end-to-end half of
// the repro: with the default applied, a project-bound run reaches
// command.validate and loadValidateCommand sees the workspace's real
// test_baseline.json instead of skipped_no_command-forever.
func TestBug455_ValidateFindsBaselineAfterCwdDefault(t *testing.T) {
	svc, _ := newTestServer(t)
	ws := t.TempDir()
	svc.runner = &Runner{workspace: ws}

	// Real baseline file in the workspace, exactly what the escalate message
	// told the operator to configure.
	guardDir := filepath.Join(ws, ".flowpilot", "guard")
	if err := os.MkdirAll(guardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseline := `{"captured_at":"2026-09-24T02:05:00Z","green_tests":["TestGcd"],"test_command":"go test ./...","suite_passed":true}`
	if err := os.WriteFile(filepath.Join(guardDir, "test_baseline.json"), []byte(baseline), 0o644); err != nil {
		t.Fatal(err)
	}

	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if cmd, _ := loadValidateCommand(svc.workspaceCwdFor(h.RunID)); cmd == "" {
		t.Fatal("loadValidateCommand must see the workspace baseline once workspaceCwd defaults")
	}
}

// TestBug455_ResumeHealsEmptyWorkspaceCwd covers runs persisted before the
// create-time default (run-94's durable record has no working_directory):
// reconstruct applies the same Runner.workspace fallback so a restarted run
// does not re-enter the skipped_no_command dead park.
func TestBug455_ResumeHealsEmptyWorkspaceCwd(t *testing.T) {
	svc, _ := newTestServer(t)
	ws := t.TempDir()
	svc.runner = &Runner{workspace: ws}

	rs, apiErr := svc.reconstructRunInternal(ProviderSessionState{
		RunID:       "run-heal",
		ProjectID:   "proj",
		ProviderKey: ProviderKeyCodex,
		Status:      RunStatusRunning,
		// WorkingDirectory intentionally empty — the pre-fix persisted shape.
	}, false)
	if apiErr != nil {
		t.Fatalf("reconstruct: %v", apiErr)
	}
	if rs.workspaceCwd != ws {
		t.Fatalf("resumed workspaceCwd must heal to runner workspace %q, got %q", ws, rs.workspaceCwd)
	}
}

// TestBug455_ResumeKeepsPersistedCwd pins that a real persisted directory is
// never overwritten by the default.
func TestBug455_ResumeKeepsPersistedCwd(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.runner = &Runner{workspace: t.TempDir()}
	persisted := t.TempDir()

	rs, apiErr := svc.reconstructRunInternal(ProviderSessionState{
		RunID:            "run-keep",
		ProjectID:        "proj",
		ProviderKey:      ProviderKeyCodex,
		Status:           RunStatusRunning,
		WorkingDirectory: persisted,
	}, false)
	if apiErr != nil {
		t.Fatalf("reconstruct: %v", apiErr)
	}
	if rs.workspaceCwd != persisted {
		t.Fatalf("persisted cwd must survive resume, got %q want %q", rs.workspaceCwd, persisted)
	}
}
