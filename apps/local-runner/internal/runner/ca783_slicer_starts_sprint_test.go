package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCollectVibeSprintPlan_UsesSSWhenNoTasks(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "requirements", "05-System-Specs")
	if err := os.MkdirAll(spec, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SS-14-a.md", "SS-15-b.md", "FORMAT-REFERENCE-SS.md"} {
		if err := os.WriteFile(filepath.Join(spec, name), []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := collectVibeSprintPlan(dir)
	if len(got) != 2 {
		t.Fatalf("plan=%v want 2 SS files", got)
	}
}

func TestAdvanceHubDone_SprintSlicerStartsVibeSprint(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.vibeAwaitingLock = false
	rs.activeHubNodeID = vibeSprintSlicerNodeID
	rs.currentTurnID = "turn-slicer"
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: vibeSprintSlicerNodeID, Behavior: "hub.inline"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: vibeSprintSlicerNodeID, To: "done", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()

	res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done"})
	if !handled {
		t.Fatal("slicer --done--> done must be claimed so ingest does not settle")
	}
	if res.NextAction != "advancing" {
		t.Fatalf("NextAction=%q want advancing %+v", res.NextAction, res)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[parent.RunID]
	if rs.vibeSprintIndex < 1 {
		t.Fatalf("index=%d want started", rs.vibeSprintIndex)
	}
	found := false
	for _, n := range rs.activeFlowNodes {
		if n.ID == "preflight_contract_plan" || n.ID == "tdd" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want vibe-sprint nodes, got %+v", rs.activeFlowNodes)
	}
}
