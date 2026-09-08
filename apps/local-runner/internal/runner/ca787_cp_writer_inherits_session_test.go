package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestResolveFlowNodeModel_CpWriterIgnoresDocWriterRole(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf"] = []Step{{
		ID:    "flow-agent-delegate-doc-writer",
		Name:  "Flow: Doc Writer",
		Model: "gpt-5.4",
	}}
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini",
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	node := agentpack.FlowNode{ID: "cp_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md"}
	if got := svc.resolveFlowNodeModel(context.Background(), parent.RunID, node); got != "" {
		t.Fatalf("resolveFlowNodeModel(cp_writer)=%q want empty inherit", got)
	}
	pk, model := svc.resolveFlowNodeProviderModel(context.Background(), parent.RunID, node)
	if pk != "codex" || model != "gpt-5.4-mini" {
		t.Fatalf("posture=%s/%s want codex/gpt-5.4-mini (session), not gpt-5.4 role", pk, model)
	}
}

func TestResolveFlowNodeModel_CpWriterIgnoresUnscopedNodeIDHit(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf"] = []Step{{
		ID:     "some_other_cp_writer_row",
		NodeID: "cp_writer",
		Model:  "gpt-5.4",
	}}
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini",
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	node := agentpack.FlowNode{ID: "cp_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md"}
	if got := svc.resolveFlowNodeModel(context.Background(), parent.RunID, node); got != "" {
		t.Fatalf("unique node_id hit must not pin cp_writer: %q", got)
	}
}

func TestResolveFlowNodeModel_CpWriterHonorsFlowScopedSetting(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf"] = []Step{
		{ID: "flow-agent-delegate-doc-writer", Name: "Flow: Doc Writer", Model: "gpt-5.4"},
		{ID: "flowpilot_core_flow_pack_vibe_ingest_cp_writer", NodeID: "cp_writer", Model: "claude-sonnet-4-5"},
	}
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini",
		WorkingMode: "vibe", FlowRef: "flowpilot-core-flow-pack/vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	node := agentpack.FlowNode{ID: "cp_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md"}
	got := svc.resolveFlowNodeModel(context.Background(), parent.RunID, node)
	if got != "claude-sonnet-4-5" {
		t.Fatalf("got %q want flow-scoped settings model", got)
	}
}

func TestVibeInheritsSessionModel_TaskSlicer(t *testing.T) {
	if !vibeInheritsSessionModel("cp_writer") || !vibeInheritsSessionModel("task_slicer") {
		t.Fatal("cp_writer and task_slicer must inherit session by default")
	}
	if vibeInheritsSessionModel("plan_writer") {
		t.Fatal("harness plan_writer must keep role-row model")
	}
}
