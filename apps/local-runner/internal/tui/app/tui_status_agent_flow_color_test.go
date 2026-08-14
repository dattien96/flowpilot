package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestStatusLine_AgentAndFlowValuesUseDistinctColors(t *testing.T) {
	forceStatusColorProfile(t)
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m.width = 100
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: "grok-flow"}
	m.flowStepsActive = "my-reviewer"
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "completed"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-1", AgentName: "main", Role: "main", Status: "completed"},
	}

	agentChip := m.formatAgentViewStatus()
	if !strings.Contains(agentChip, styleStatusAgent.Render("main")) {
		t.Fatalf("agent value missing teal style: %q", agentChip)
	}
	if strings.Contains(agentChip, styleStatusHi.Render("main")) {
		t.Fatalf("agent value must not use accent styleStatusHi: %q", agentChip)
	}
	if strings.Contains(agentChip, styleStatusFlow.Render("main")) {
		t.Fatalf("agent value must not use flow pink: %q", agentChip)
	}

	modeLine := m.renderStatusModeLine(100)
	if !strings.Contains(modeLine, styleStatusFlow.Render("grok-flow")) {
		t.Fatalf("flow name missing pink style: %q", modeLine)
	}
	if !strings.Contains(modeLine, styleStatusFlow.Render("my-reviewer")) {
		t.Fatalf("active step missing pink style: %q", modeLine)
	}
	// Whole line must not be solid accent (old stylePromptFocus wrap).
	if modeLine == stylePromptFocus.Render(stripANSI(modeLine)) {
		t.Fatalf("mode line must not be fully stylePromptFocus: %q", modeLine)
	}
	// Model line still uses accent for model — different from agent/flow.
	modelLine := m.renderStatusModelLine(" | ")
	if !strings.Contains(modelLine, styleStatusHi.Render("grok-4.5")) {
		t.Fatalf("model still accent: %q", modelLine)
	}
}
