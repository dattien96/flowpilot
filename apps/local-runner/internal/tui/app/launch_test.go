package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestResolveFlowRef_WorkflowCatalog(t *testing.T) {
	arm, err := resolveFlowLaunch(nil, []client.Workflow{
		{ID: "9ecadf22-0a0e-4963-a45e-22812f0f9700", Name: "Ship Android", ProjectID: "p1"},
	}, "p1", "9ecadf22-0a0e-4963-a45e-22812f0f9700")
	if err != nil {
		t.Fatal(err)
	}
	if !arm.IsCatalogWorkflow() || arm.IsBuiltin() || arm.SubMode != "" || arm.FlowRef != "" {
		t.Fatalf("catalog arm wrongly treated as builtin: %+v", arm)
	}
	if arm.WorkflowID != "9ecadf22-0a0e-4963-a45e-22812f0f9700" || arm.Label != "Ship Android" {
		t.Fatalf("arm=%+v", arm)
	}
}

func TestResolveFlowRef_MissingHintsDesktopSettings(t *testing.T) {
	_, err := resolveFlowLaunch(nil, nil, "p1", "missing")
	if err == nil || !strings.Contains(err.Error(), "Desktop → Settings") {
		t.Fatalf("err=%v", err)
	}
}

func TestLaunchArm_FirstTurnSendsSubModeAndFlowRef(t *testing.T) {
	arm := builtinArm(client.BuiltinFlowOption{FlowRef: "pack/review-loop", Label: "review-loop"})
	sub, fr, ct, _, ok := arm.FirstTurnExtras()
	if !ok || sub != "bug" || fr != "pack/review-loop" || ct != "bugfix" {
		t.Fatalf("extras sub=%q fr=%q ct=%q ok=%v", sub, fr, ct, ok)
	}
	catalog := catalogArm(client.Workflow{ID: "wf-1", Name: "W"})
	if _, _, _, _, ok := catalog.FirstTurnExtras(); ok {
		t.Fatal("catalog must not expose first-turn flowRef extras")
	}
}

func TestLaunchArm_FirstTurnSendsChangeTypeBugfix(t *testing.T) {
	arm, err := resolveFlowLaunch([]client.BuiltinFlowOption{
		{FlowRef: "flowpilot-core-flow-pack/review-loop", Label: "review-loop"},
	}, nil, "", "review-loop")
	if err != nil {
		t.Fatal(err)
	}
	if arm.ChangeType != "bugfix" || arm.SubMode != "bug" {
		t.Fatalf("arm=%+v", arm)
	}
}

func TestResolveFlowRef_SubModeCarriedFromMatchedOption(t *testing.T) {
	ref, sub, err := resolveFlowRef([]client.BuiltinFlowOption{
		{FlowRef: "pack/fix-bug", Label: "fix-bug"},
	}, nil, "pack/fix-bug")
	if err != nil || ref != "pack/fix-bug" || sub != "bug" {
		t.Fatalf("ref=%q sub=%q err=%v", ref, sub, err)
	}
}

func TestLaunchArm_ToStartRunInput_CatalogUsesWorkflowID(t *testing.T) {
	arm := catalogArm(client.Workflow{ID: "wf-uuid", Name: "Ship"})
	in := arm.ToStartRunInput("proj", "codex", "o3", "high", "/tmp/p", true)
	if in.ChatMode != "" || in.WorkflowID != "wf-uuid" {
		t.Fatalf("start input=%+v", in)
	}
	builtin := builtinArm(client.BuiltinFlowOption{FlowRef: "pack/x", Label: "x"})
	in2 := builtin.ToStartRunInput("proj", "codex", "o3", "", "/tmp/p", false)
	if in2.ChatMode != "normal_chat" || in2.WorkflowID != "" {
		t.Fatalf("builtin start=%+v", in2)
	}
}

func TestFlowSlash_CatalogDoesNotForceSubModeBug(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.flowWorkflows = []client.Workflow{
		{ID: "9ecadf22-0a0e-4963-a45e-22812f0f9700", Name: "Ship Android", ProjectID: "p1"},
	}
	m.project = &client.Project{ID: "p1", Name: "p"}
	m2, _ := m.handleSlashCommand("/flow 9ecadf22-0a0e-4963-a45e-22812f0f9700")
	am := m2.(*AppModel)
	if am.launch.SubMode != "" || am.firstTurnPending {
		t.Fatalf("launch=%+v firstTurnPending=%v", am.launch, am.firstTurnPending)
	}
	view := am.View()
	if strings.Contains(view, "subMode: bug") || strings.Contains(view, "subMode=bug") {
		t.Fatalf("catalog arm must not advertise subMode bug:\n%s", view)
	}
	if !strings.Contains(view, "Ship Android") {
		t.Fatalf("expected workflow name in message:\n%s", view)
	}
	if !strings.Contains(strings.Join(am.renderSidebarStatusSection(80), "\n"), "Ship Android") {
		t.Fatalf("sidebar missing flow name")
	}
}

func TestLaunchArm_RejectsChangeAfterRunStarted(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-1"}
	m.flowBuiltins = []client.BuiltinFlowOption{{FlowRef: "pack/x", Label: "x"}}
	m2, _ := m.handleSlashCommand("/flow pack/x")
	view := m2.(*AppModel).View()
	if !strings.Contains(view, "/new") {
		t.Fatalf("expected reject after run started:\n%s", view)
	}
}

func TestCmdSendTurn_CatalogOmitsFlowRef(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.launch = catalogArm(client.Workflow{ID: "wf-1", Name: "W"})
	m.firstTurnPending = false
	m.runHandle = &client.RunHandle{RunID: "r1", StepID: "s1"}
	in := m.buildTurnInput("go")
	if in.FlowRef != "" || in.SubMode != "" || in.ChangeType != "" {
		t.Fatalf("turn must not carry builtin extras for catalog: %+v", in)
	}
}

func TestCmdSendTurn_BuiltinFirstTurnOnly(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.launch = builtinArm(client.BuiltinFlowOption{FlowRef: "pack/review-loop", Label: "review-loop"})
	m.firstTurnPending = true
	in := m.buildTurnInput("go")
	if in.FlowRef != "pack/review-loop" || in.SubMode != "bug" || in.ChangeType != "bugfix" {
		t.Fatalf("first turn extras=%+v", in)
	}
	// Simulate consume (cmdSendTurn clears the flag; buildTurnInput does not).
	m.firstTurnPending = false
	in2 := m.buildTurnInput("follow-up")
	if in2.FlowRef != "" || in2.SubMode != "" {
		t.Fatalf("follow-up must omit extras: %+v", in2)
	}
}
