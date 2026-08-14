package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestFormatFlowStepsBanner_MarksActive(t *testing.T) {
	got := formatFlowStepsBanner([]client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", Status: "DONE"},
		{StepID: "s2", NodeID: "reviewer", Status: "RUNNING"},
		{StepID: "s3", NodeID: "tester", Status: "PENDING"},
	})
	if !strings.Contains(got, "reviewer") || !strings.Contains(got, "In progress: reviewer") {
		t.Fatalf("banner=%q", got)
	}
	if !strings.Contains(got, ">  2. [RUNNING] reviewer") && !strings.Contains(got, "RUNNING") {
		t.Fatalf("missing running mark: %q", got)
	}
}

func TestStepsRuntimeMsg_AnnouncesActiveStep(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-1"}
	m.launch = LaunchArm{WorkflowID: "wf-1", Label: "grok-flow", Mode: ModeFlow}
	m2, _ := m.Update(StepsRuntimeMsg{
		RunID: "run-1",
		Steps: []client.WorkflowStepRuntime{
			{StepID: "a", NodeID: "coder", Status: "RUNNING"},
			{StepID: "b", NodeID: "reviewer", Status: "PENDING"},
		},
	})
	am := m2.(*AppModel)
	if am.flowStepsActive != "coder" {
		t.Fatalf("active=%q", am.flowStepsActive)
	}
	if !strings.Contains(am.View(), "coder") {
		t.Fatalf("view missing steps:\n%s", am.View())
	}
}

func TestTurnStreamOpened_StartsPolling(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	evCh := make(chan client.ProviderEvent)
	errCh := make(chan error, 1)
	close(evCh)
	close(errCh)
	m2, cmd := m.Update(turnStreamOpenedMsg{EvCh: evCh, ErrCh: errCh})
	am := m2.(*AppModel)
	if am.turnStream == nil {
		t.Fatal("expected turnStream state")
	}
	if cmd == nil {
		t.Fatal("expected poll cmd")
	}
	msg := cmd()
	if _, ok := msg.(turnStreamClosedMsg); !ok {
		t.Fatalf("msg=%T", msg)
	}
}

func TestClipboardPasteMsg_AttachesImage(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = "codex"
	att := &client.PromptAttachment{ID: "att-1", OriginalName: "clipboard.png", MimeType: "image/png", Width: 10, Height: 10}
	m2, _ := m.Update(ClipboardPasteMsg{Attachment: att})
	am := m2.(*AppModel)
	if len(am.pendingAttach) != 1 {
		t.Fatalf("pending=%d", len(am.pendingAttach))
	}
	if !strings.Contains(am.renderInputLine(), "1 img") {
		t.Fatalf("input missing attach chip: %q", am.renderInputLine())
	}
}
