package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/workingmode"
)

func TestVibeCoderSpawnBlocked_RequiresSignaturesNotRepoTests(t *testing.T) {
	if !vibeCoderSpawnBlocked(workingmode.Vibe, "coder", false) {
		t.Fatal("vibe coder without tdd-signatures.md must block")
	}
	if vibeCoderSpawnBlocked(workingmode.Vibe, "coder", true) {
		t.Fatal("signatures present must allow spawn")
	}
	if vibeCoderSpawnBlocked(workingmode.Dev, "coder", false) {
		t.Fatal("dev coder must not use vibe tdd gate")
	}
}

func TestHasVibeTddSignatures_IgnoresPreexistingTests(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "foo_test.go"), []byte("package foo\nfunc TestFoo(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hasVibeTddSignatures(cwd) {
		t.Fatal("repo tests must not count as sprint tdd artifact")
	}
	sig := filepath.Join(cwd, filepath.FromSlash(vibeTddSignaturesRel))
	if err := os.MkdirAll(filepath.Dir(sig), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sig, []byte("# tdd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasVibeTddSignatures(cwd) {
		t.Fatal("sprint signatures file must count")
	}
}

func TestResumeVibeLock_RejectsInvalidCPWithoutWrite(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	rel := "requirements/07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md"
	abs := filepath.Join(cwd, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := "- Document ID: `CP-60`\n# CP-60\n"
	if err := os.WriteFile(abs, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-cp-ingest", Client: "tui", Cwd: cwd,
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workspaceCwd = cwd
	rs.vibeLockNodeID = vibeCpLockNodeID
	rs.vibeLockPath = rel
	svc.mu.Unlock()
	if _, ok := svc.resumeVibeLock(parent.RunID, "not a coding plan", AgentGraphSnapshot{}); !ok {
		t.Fatal("resume handled")
	}
	body, readErr := os.ReadFile(abs)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(body) != orig {
		t.Fatalf("invalid paste must not overwrite CP, got %q", body)
	}
}

func TestVibeSession_ReconstructRestoresPlanAndLock(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-cp-ingest", Client: "tui",
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.vibeTaskPlan = []string{"requirements/08-Task/todo/Task-321-Vibe-Cp-Driven-Entry.md"}
	rs.vibeLockedCP = "requirements/07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md"
	rs.vibeAwaitingLock = false
	rs.vibeSprintIndex = 2
	st := sessionStateOf(rs)
	svc.mu.Unlock()
	svc2 := NewInteractiveService()
	got, recErr := svc2.reconstructRun(st)
	if recErr != nil {
		t.Fatalf("reconstructRun: %v", recErr)
	}
	if got.workingMode != workingmode.Vibe || got.vibeSprintIndex != 2 {
		t.Fatalf("mode=%q idx=%d", got.workingMode, got.vibeSprintIndex)
	}
	if len(got.vibeTaskPlan) != 1 || got.vibeLockedCP == "" {
		t.Fatalf("plan=%v cp=%q", got.vibeTaskPlan, got.vibeLockedCP)
	}
}

func TestIsVibeTestSourcePath_TSAndGo(t *testing.T) {
	if !isVibeTestSourcePath("src/foo.test.ts") || !isVibeTestSourcePath("foo_test.go") {
		t.Fatal("ts and go tests must count")
	}
	if isVibeTestSourcePath("src/foo.ts") {
		t.Fatal("non-test ts must not count")
	}
}

func TestInjectVibeSSDrift_ReadsTypeScriptTests(t *testing.T) {
	cwd := t.TempDir()
	ss := filepath.Join(cwd, "SS-18.md")
	if err := os.WriteFile(ss, []byte("- AC-9 login\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := filepath.Join(cwd, "login.test.ts")
	if err := os.WriteFile(ts, []byte("test('AC-9 login', () => {})\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewInteractiveService()
	rs := &interactiveRun{workingMode: workingmode.Vibe, vibeLockedSS: ss, workspaceCwd: cwd}
	tr := flowgate.TurnResult{Tests: flowgate.TestOutcome{Ran: true}, WrittenPaths: []string{ts}}
	svc.injectVibeSSDrift(rs, &tr)
	if tr.RequirementDrift {
		t.Fatalf("AC-9 in ts test must not drift: %q", tr.RequirementDriftDetail)
	}
}

func TestInjectVibeSSDrift_TSMissingACStillDrifts(t *testing.T) {
	cwd := t.TempDir()
	ss := filepath.Join(cwd, "SS-18.md")
	if err := os.WriteFile(ss, []byte("- AC-9 login\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := filepath.Join(cwd, "login.test.ts")
	if err := os.WriteFile(ts, []byte("test('login', () => {})\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewInteractiveService()
	rs := &interactiveRun{workingMode: workingmode.Vibe, vibeLockedSS: ss, workspaceCwd: cwd}
	tr := flowgate.TurnResult{Tests: flowgate.TestOutcome{Ran: true}, WrittenPaths: []string{ts}}
	svc.injectVibeSSDrift(rs, &tr)
	if !tr.RequirementDrift || !strings.Contains(tr.RequirementDriftDetail, "AC-9") {
		t.Fatalf("want AC-9 drift, got %q", tr.RequirementDriftDetail)
	}
}
