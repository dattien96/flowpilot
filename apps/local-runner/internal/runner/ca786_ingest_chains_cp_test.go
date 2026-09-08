package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCollectLatestVibeCP_PicksTodoFile(t *testing.T) {
	dir := t.TempDir()
	todo := filepath.Join(dir, "requirements", "07-Coding-Plan", "todo")
	if err := os.MkdirAll(todo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(todo, "CP-60-x.md"), []byte("# CP\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := collectLatestVibeCP(dir)
	if got != "requirements/07-Coding-Plan/todo/CP-60-x.md" {
		t.Fatalf("got %q", got)
	}
}

func TestOnVibeCpWriterDone_StartsVibeCpIngest(t *testing.T) {
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

	svc.onVibeCpNodeDone(parent.RunID, vibeCpWriterNodeID)

	svc.mu.Lock()
	rs = svc.runs[parent.RunID]
	ref := rs.chatFlowRef
	awaiting := rs.vibeAwaitingLock
	nodes := append([]agentpack.FlowNode(nil), rs.activeFlowNodes...)
	svc.mu.Unlock()
	if workingmode.BareFlowID(ref) != vibeCpIngestFlowID {
		t.Fatalf("chatFlowRef=%q want vibe-cp-ingest", ref)
	}
	if awaiting {
		t.Fatal("ingest cp_writer must skip cp_lock")
	}
	found := false
	for _, n := range nodes {
		if n.ID == "cp_reader" || n.ID == vibeCpLockNodeID || n.ID == vibeTaskSlicerNodeID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want vibe-cp-ingest nodes, got %+v", nodes)
	}
	if countChildrenWithLabel(svc, parent.RunID, "cp_reader") != 0 {
		t.Fatal("ingest must not spawn cp_reader")
	}
}

func TestOnVibeCpWriterDone_IdempotentWhenAlreadyCpIngest(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-cp-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.onVibeCpNodeDone(parent.RunID, vibeCpWriterNodeID)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[parent.RunID]
	if workingmode.BareFlowID(rs.chatFlowRef) != vibeCpIngestFlowID {
		t.Fatalf("chatFlowRef=%q", rs.chatFlowRef)
	}
}
