package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Status line only shows agent:main / agent:<name> — open/back is F2 only (CA-506/507).

func TestStatusLine_ViewMainAndChildOnly(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-98153"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-98153", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-98158", AgentName: "coder", Label: "grok-coder", Status: "completed"},
	}

	line0 := stripANSI(strings.Split(m.renderStatusLine0(" | ", 120), "\n")[0])
	if !strings.Contains(line0, "agent:main") {
		t.Fatalf("want agent:main on status: %q", line0)
	}
	if strings.Contains(line0, "agents:") || strings.Contains(line0, "[back]") {
		t.Fatalf("status must not list agents or [back]: %q", line0)
	}

	m.focusRunID = "run-98158"
	line1 := stripANSI(m.renderStatusLine0(" | ", 120))
	if !strings.Contains(line1, "agent:grok-coder") {
		t.Fatalf("want agent:grok-coder: %q", line1)
	}
	if strings.Contains(line1, "agent:main") {
		t.Fatalf("child view must not also show agent:main: %q", line1)
	}
	if strings.Contains(line1, "[back]") || strings.Contains(line1, "agents:") {
		t.Fatalf("status must not show agent chrome: %q", line1)
	}
	// Name uses dedicated agent color (not model accent). Needs TrueColor for ANSI.
	forceStatusColorProfile(t)
	raw := m.formatAgentViewStatus()
	if !strings.Contains(raw, styleStatusAgent.Render("grok-coder")) {
		t.Fatalf("name must use styleStatusAgent: %q", raw)
	}
	if strings.Contains(raw, styleStatusHi.Render("grok-coder")) {
		t.Fatalf("agent name must not reuse styleStatusHi accent: %q", raw)
	}
}

func TestStatusLine_NoAgentOpenClickTarget(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-98153"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-98153", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-98158", AgentName: "coder", Label: "grok-coder", Status: "completed"},
	}
	// Collapse F2 so only status could expose agent targets (it must not).
	m.sessionPanel.Collapsed = true
	_ = m.View()
	if _, _, ok := findClickTarget(m, "agent-open:run-98158"); ok {
		t.Fatal("status/collapsed chrome must not expose agent-open click")
	}
	if _, _, ok := findClickTarget(m, "agent-back"); ok {
		// Only when viewing child + F2 expanded with [back]
		t.Fatal("collapsed panel must not expose agent-back")
	}
}

func TestFormatAgentViewStatus_MainAndChild(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-98158", AgentName: "coder", Label: "grok-coder", Status: "completed"},
	}
	if got := stripANSI(m.formatAgentViewStatus()); got != "agent:main" {
		t.Fatalf("main=%q", got)
	}
	m.focusRunID = "run-98158"
	if got := stripANSI(m.formatAgentViewStatus()); got != "agent:grok-coder" {
		t.Fatalf("child=%q", got)
	}
}
