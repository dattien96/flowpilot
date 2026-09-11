package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCA797_ForwardDonePredecessorsOfTaskSlicer(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var edges []agentpack.FlowEdge
	for _, def := range pack.Flows {
		if def.ID == vibeCpIngestFlowID {
			edges = def.Edges
			break
		}
	}
	if len(edges) == 0 {
		t.Fatal("missing vibe-cp-ingest")
	}
	got := map[string]bool{}
	for _, id := range flowForwardDonePredecessors(edges, vibeTaskSlicerNodeID) {
		got[id] = true
	}
	for _, want := range []string{"cp_reader", "cp_validator", vibeCpLockNodeID} {
		if !got[want] {
			t.Fatalf("missing predecessor %q in %v", want, got)
		}
	}
	if got[vibeTaskSlicerNodeID] {
		t.Fatal("start node must not be its own predecessor")
	}
}

func TestCA797_IngestJoinMarksSkippedCPPrefix(t *testing.T) {
	svc, runID := ca791ArmIngest(t)
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var nodes []agentpack.FlowNode
	var edges []agentpack.FlowEdge
	for _, def := range pack.Flows {
		if def.ID == vibeCpIngestFlowID {
			nodes, edges = def.Nodes, def.Edges
			break
		}
	}
	if len(nodes) == 0 || len(edges) == 0 {
		t.Fatal("missing vibe-cp-ingest")
	}
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.markForwardDonePredecessorsSkipped(context.Background(), runID, edges, vibeTaskSlicerNodeID)
	for _, id := range []string{"cp_reader", "cp_validator", vibeCpLockNodeID} {
		if got := flowStepStatus(t, svc, runID, id); got != StepStatusSkipped {
			t.Fatalf("%s=%q want SKIPPED", id, got)
		}
	}
	if got := flowStepStatus(t, svc, runID, vibeTaskSlicerNodeID); got != StepStatusPending {
		t.Fatalf("task_slicer=%q want PENDING", got)
	}
}

func TestCA797_SkipDoesNotClobberDone(t *testing.T) {
	svc, runID := ca791ArmIngest(t)
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var nodes []agentpack.FlowNode
	var edges []agentpack.FlowEdge
	for _, def := range pack.Flows {
		if def.ID == vibeCpIngestFlowID {
			nodes, edges = def.Nodes, def.Edges
			break
		}
	}
	if len(nodes) == 0 {
		t.Fatal("missing vibe-cp-ingest")
	}
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.setFlowStepStatus(context.Background(), runID, "cp_reader", StepStatusDone)
	svc.markForwardDonePredecessorsSkipped(context.Background(), runID, edges, vibeTaskSlicerNodeID)
	if got := flowStepStatus(t, svc, runID, "cp_reader"); got != StepStatusDone {
		t.Fatalf("cp_reader=%q want DONE", got)
	}
	if got := flowStepStatus(t, svc, runID, "cp_validator"); got != StepStatusSkipped {
		t.Fatalf("cp_validator=%q want SKIPPED", got)
	}
}


func TestCA797_EmptyStartDoesNotSkipPrefix(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	svc.mu.Unlock()
	svc.startResolvedFlow(context.Background(), parent.RunID, workingmode.PackPrefix+vibeCpIngestFlowID, "cp.md")
	if got := flowStepStatus(t, svc, parent.RunID, "cp_reader"); got == StepStatusSkipped {
		t.Fatal("user-start vibe-cp-ingest must not skip cp_reader")
	}
}
