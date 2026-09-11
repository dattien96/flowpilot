package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)
func ca791ArmIngest(t *testing.T) (*InteractiveService, string) {
	t.Helper()
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
	svc.mu.Unlock()
	return svc, parent.RunID
}

func TestCA791_CpWriterJoinsAtTaskSlicer(t *testing.T) {
	svc, runID := ca791ArmIngest(t)
	svc.onVibeCpNodeDone(runID, vibeCpWriterNodeID)

	svc.mu.Lock()
	rs := svc.runs[runID]
	if workingmode.BareFlowID(rs.chatFlowRef) != vibeCpIngestFlowID {
		svc.mu.Unlock()
		t.Fatalf("chatFlowRef=%q want vibe-cp-ingest", rs.chatFlowRef)
	}
	if rs.vibeAwaitingLock {
		svc.mu.Unlock()
		t.Fatal("ingest must skip cp_lock after writing CP")
	}
	hasSlicer, hasReader := false, false
	for _, n := range rs.activeFlowNodes {
		if n.ID == vibeTaskSlicerNodeID {
			hasSlicer = true
		}
		if n.ID == "cp_reader" {
			hasReader = true
		}
	}
	svc.mu.Unlock()
	if !hasSlicer || !hasReader {
		t.Fatalf("want overlayed vibe-cp-ingest graph (slicer=%v reader=%v)", hasSlicer, hasReader)
	}
	if countChildrenWithLabel(svc, runID, "cp_reader") != 0 {
		t.Fatal("ingest must not spawn cp_reader")
	}
}

func TestCA791_CpWriterIdempotentOnCpIngestKeepsLock(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-cp-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.vibeAwaitingLock = true
	svc.mu.Unlock()

	before := countChildrenWithLabel(svc, parent.RunID, vibeTaskSlicerNodeID)
	svc.onVibeCpNodeDone(parent.RunID, vibeCpWriterNodeID)

	svc.mu.Lock()
	rs = svc.runs[parent.RunID]
	ref := rs.chatFlowRef
	awaiting := rs.vibeAwaitingLock
	svc.mu.Unlock()
	if workingmode.BareFlowID(ref) != vibeCpIngestFlowID {
		t.Fatalf("chatFlowRef=%q", ref)
	}
	if !awaiting {
		t.Fatal("user-start vibe-cp-ingest must still await cp_lock")
	}
	if got := countChildrenWithLabel(svc, parent.RunID, vibeTaskSlicerNodeID); got != before {
		t.Fatalf("slicer children=%d want %d (no jump)", got, before)
	}
}

func TestCA791_MissingCPFileStillJoinsSlicer(t *testing.T) {
	svc, runID := ca791ArmIngest(t)
	svc.mu.Lock()
	svc.runs[runID].workspaceCwd = t.TempDir()
	svc.mu.Unlock()

	svc.onVibeCpNodeDone(runID, vibeCpWriterNodeID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if workingmode.BareFlowID(rs.chatFlowRef) != vibeCpIngestFlowID {
		t.Fatalf("chatFlowRef=%q", rs.chatFlowRef)
	}
	if rs.vibeAwaitingLock {
		t.Fatal("missing CP file must not repark cp_lock")
	}
}

func TestCA791_SlicerAfterIngestJoinStartsSprint(t *testing.T) {
	svc, runID := ca791ArmIngest(t)
	dir := t.TempDir()
	todo := filepath.Join(dir, "requirements", "08-Task", "todo")
	if err := os.MkdirAll(todo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(todo, "Task-1-x.md"), []byte("# T\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[runID].workspaceCwd = dir
	svc.mu.Unlock()

	svc.onVibeCpNodeDone(runID, vibeCpWriterNodeID)
	svc.onVibeCpNodeDone(runID, vibeTaskSlicerNodeID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs.vibeSprintIndex < 1 {
		t.Fatalf("index=%d want sprint started", rs.vibeSprintIndex)
	}
	if rs.vibeAwaitingLock {
		t.Fatal("sprint start must not wait cp_lock after ingest join")
	}
}

func TestCA791_StartFromMissingNodeKeepsPriorGraph(t *testing.T) {
	svc, runID := ca791ArmIngest(t)
	svc.mu.Lock()
	svc.runs[runID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "ingest_reader", Behavior: "agent.delegate"},
		{ID: vibeCpWriterNodeID, Behavior: "agent.delegate"},
	}
	svc.mu.Unlock()

	svc.startResolvedFlowFromNode(context.Background(), runID, workingmode.PackPrefix+vibeCpIngestFlowID, "x", "no-such-node")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if len(rs.activeFlowNodes) != 2 || rs.activeFlowNodes[0].ID != "ingest_reader" {
		t.Fatalf("missing start node mutated graph: %+v", rs.activeFlowNodes)
	}
	if workingmode.BareFlowID(rs.chatFlowRef) == vibeCpIngestFlowID {
		t.Fatal("missing start must not retarget chatFlowRef")
	}
}
