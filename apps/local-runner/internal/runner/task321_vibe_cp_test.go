package runner

import (
	"testing"

	"flowpilot-runner/internal/workingmode"
)

func TestDetect_CodingPlanCPPath(t *testing.T) {
	ok := workingmode.IsCodingPlanCPPath("requirements/07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md")
	if !ok {
		t.Fatal("expected CP path")
	}
	if workingmode.IsCodingPlanCPPath("requirements/05-System-Specs/SS-18-Vibe-Working-Mode.md") {
		t.Fatal("SS path must not match")
	}
	if workingmode.IsCodingPlanCPPath("README.md") {
		t.Fatal("random md must not match")
	}
}

func TestDetect_CPDocumentID(t *testing.T) {
	if !workingmode.HasCPDocumentID("- Document ID: `CP-60`\n") {
		t.Fatal("frontmatter Document ID")
	}
	if workingmode.HasCPDocumentID("- Document ID: `SS-18`\n") {
		t.Fatal("SS id must not match")
	}
}

func TestDetect_RejectNonCP(t *testing.T) {
	if err := workingmode.RejectNonCP("README.md", ""); err == nil {
		t.Fatal("want reject")
	}
	if err := workingmode.RejectNonCP("requirements/07-Coding-Plan/todo/CP-1.md", ""); err != nil {
		t.Fatalf("path-only CP: %v", err)
	}
	if err := workingmode.RejectNonCP("requirements/07-Coding-Plan/todo/CP-1.md", "Document ID: SS-18"); err == nil {
		t.Fatal("want reject mismatched id")
	}
}

func TestVibeSprint_LockBlocksStart(t *testing.T) {
	d := decideNextVibeSprint(true, []string{"t1"}, 0, 8)
	if !d.Locked || d.Start {
		t.Fatalf("%+v", d)
	}
}

func TestVibeSprint_SequentialIndex(t *testing.T) {
	plan := []string{"a", "b"}
	d1 := decideNextVibeSprint(false, plan, 0, 8)
	if !d1.Start || d1.Task != "a" {
		t.Fatalf("first %+v", d1)
	}
	d2 := decideNextVibeSprint(false, plan, 1, 8)
	if !d2.Start || d2.Task != "b" {
		t.Fatalf("second %+v", d2)
	}
	d3 := decideNextVibeSprint(false, plan, 2, 8)
	if !d3.Done || d3.Start {
		t.Fatalf("done %+v", d3)
	}
}

func TestVibeSprint_BudgetStops(t *testing.T) {
	d := decideNextVibeSprint(false, []string{"a", "b", "c"}, 8, 8)
	if !d.Budget || d.Start {
		t.Fatalf("%+v", d)
	}
}

func vibeCpRun(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-cp-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	return svc, parent.RunID
}

func TestVibeSprint_RestartReplaysIndex(t *testing.T) {
	svc, runID := vibeCpRun(t)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.vibeTaskPlan = []string{"t0", "t1", "t2"}
	rs.vibeSprintIndex = 1
	rs.vibeSprintBudget = 8
	rs.vibeAwaitingLock = false
	d := svc.takeNextVibeSprintLocked(rs)
	idx := rs.vibeSprintIndex
	svc.mu.Unlock()
	if !d.Start || d.Task != "t1" {
		t.Fatalf("replay %+v", d)
	}
	if idx != 2 {
		t.Fatalf("index=%d, want 2", idx)
	}
}

func TestVibeSprint_StartBlockedFlag(t *testing.T) {
	svc, runID := vibeCpRun(t)
	if !svc.vibeSprintStartBlocked(runID) {
		t.Fatal("cp ingest must await lock at start")
	}
	svc.mu.Lock()
	svc.runs[runID].vibeAwaitingLock = false
	svc.mu.Unlock()
	if svc.vibeSprintStartBlocked(runID) {
		t.Fatal("cleared lock must allow sprint")
	}
}
