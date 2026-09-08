package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/workingmode"
)

func TestCollectVibeTaskPlan_ReadsTodoTasks(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, "requirements", "08-Task", "todo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Task-321-Vibe-Cp-Driven-Entry.md"), []byte("# Task-321\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Task-323-Vibe-Sprint-V2-Parity.md"), []byte("# Task-323\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := collectVibeTaskPlan(cwd)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestVibeCpSlicer_UsesTaskFilesNotStub(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	dir := filepath.Join(cwd, "requirements", "08-Task", "todo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "Task-009-Sample.md")
	if err := os.WriteFile(path, []byte("# Task-009\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-cp-ingest", Client: "tui", Cwd: cwd,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].workspaceCwd = cwd
	svc.mu.Unlock()
	svc.onVibeCpNodeDone(parent.RunID, vibeCpLockNodeID)
	svc.onVibeCpNodeDone(parent.RunID, vibeTaskSlicerNodeID)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[parent.RunID]
	if rs == nil || len(rs.vibeTaskPlan) == 0 {
		t.Fatal("empty plan")
	}
	if rs.vibeTaskPlan[0] == "sprint-0" {
		t.Fatalf("stub plan %v", rs.vibeTaskPlan)
	}
}

func TestVibeLock_ParkAndResumeClearsAwaiting(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !svc.parkVibeLock(parent.RunID, vibeSSLockNodeID) {
		t.Fatal("park")
	}
	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.BlockReason != vibeLockBlockReason {
		t.Fatalf("loop=%+v", st)
	}
	if _, ok := svc.resumeVibeLock(parent.RunID, "", AgentGraphSnapshot{}); !ok {
		t.Fatal("resume")
	}
	if svc.vibeSprintStartBlocked(parent.RunID) {
		t.Fatal("lock must clear awaiting")
	}
}

func TestVibeCoderBlocked_TddWithoutTests(t *testing.T) {
	if !vibeCoderBlocked(workingmode.Vibe, "tdd", "coder", true, false) {
		t.Fatal("tdd→coder without tests must block")
	}
	if vibeCoderBlocked(workingmode.Vibe, "synthesis", "coder", true, false) {
		t.Fatal("code-loop re-entry must not require a new test file")
	}
	if vibeCoderBlocked(workingmode.Dev, "tdd", "coder", true, false) {
		t.Fatal("dev must not use vibe tdd guard")
	}
}

func TestInjectVibeSSDrift_SetsRequirementDrift(t *testing.T) {
	cwd := t.TempDir()
	ss := filepath.Join(cwd, "SS-18.md")
	if err := os.WriteFile(ss, []byte("- AC-9 missing mapping\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testPath := filepath.Join(cwd, "foo_test.go")
	if err := os.WriteFile(testPath, []byte("func TestFoo(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewInteractiveService()
	rs := &interactiveRun{workingMode: workingmode.Vibe, vibeLockedSS: ss, workspaceCwd: cwd}
	tr := flowgate.TurnResult{
		Tests:        flowgate.TestOutcome{Ran: true},
		WrittenPaths: []string{testPath},
	}
	svc.injectVibeSSDrift(rs, &tr)
	if !tr.RequirementDrift {
		t.Fatalf("want drift, detail=%q", tr.RequirementDriftDetail)
	}
}
