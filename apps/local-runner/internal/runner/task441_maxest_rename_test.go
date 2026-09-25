package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Task-441 (CP-86 P-2): the runner's profile-budget read path
// (flowNodeProfileBudgetFor) must resolve the RENAMED field — the value is
// an estimated prompt-length cap, not provider usage. Uses the real builtin
// task-harness pack so a schema/field divergence fails here, not in prod.

func TestTask441_ProfileBudget_UsesRenamedField(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: workingmode.Vibe, Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	pack, perr := agentpack.LoadBuiltinPack()
	if perr != nil {
		t.Fatalf("LoadBuiltinPack: %v", perr)
	}
	var flow *agentpack.FlowDefinition
	for i := range pack.Flows {
		if pack.Flows[i].ID == "task-harness" {
			flow = &pack.Flows[i]
			break
		}
	}
	if flow == nil {
		t.Fatal("task-harness flow missing from builtin pack")
	}
	profile, ok := flow.ContextProfiles["reviewer"]
	if !ok {
		t.Fatal("task-harness reviewer profile missing from builtin pack")
	}
	if profile.MaxEstPromptTokens != 12000 {
		t.Fatalf("reviewer MaxEstPromptTokens = %d, want 12000 (builtin task-harness reviewer)", profile.MaxEstPromptTokens)
	}

	svc.mu.Lock()
	parentRs := svc.runs[parent.RunID]
	parentRs.activeFlowNodes = flow.Nodes
	parentRs.chatFlowRef = workingmode.PackPrefix + "task-harness"
	svc.mu.Unlock()

	if got := svc.flowNodeProfileBudgetFor(&interactiveRun{id: "run-c", parentRunID: parent.RunID, stepID: "reviewer"}); got != 12000 {
		t.Fatalf("flowNodeProfileBudgetFor = %d, want 12000", got)
	}
}
