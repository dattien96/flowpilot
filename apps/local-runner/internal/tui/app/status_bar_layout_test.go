package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestStatusBar_ExpandedRowOrder(t *testing.T) {
	seven := 99
	reset := "2026-08-20T06:46:00Z"
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.sessionLoading = false
	m.connStatus = ConnIdle
	m.statusMsg = ""
	m.mode = ModeChat
	m.yolo = false
	m.accountLabel = "trashname899@gmail.com"
	m.account = &client.ProviderAccountSummary{
		ProviderKey:        "grok",
		DisplayLabel:       "trashname899@gmail.com",
		IsActive:           true,
		Remaining7dPercent: &seven,
		Remaining7dResetAt: &reset,
	}
	m.projectPath = `D:\working\gate-sandbox`
	m.projectBranch = "master"
	m.project = &client.Project{Name: "Gate-sandbox", Path: `D:\working\gate-sandbox`}
	win := int64(500000)
	m.modelContextWin = 500000
	m.lastTokens = &client.TokenUsageSnapshot{ModelContextWindow: &win}

	got := stripANSI(m.renderStatusLine())
	lines := strings.Split(got, "\n")
	if len(lines) < 6 {
		t.Fatalf("want 6 status rows, got %d:\n%s", len(lines), got)
	}
	if !strings.Contains(lines[0], "ready") || !strings.Contains(lines[0], "F4") {
		t.Fatalf("line0 status/fold: %q", lines[0])
	}
	if !strings.Contains(lines[1], "[chat]") {
		t.Fatalf("line1 mode: %q", lines[1])
	}
	if strings.Contains(lines[1], "grok-4.6") {
		t.Fatalf("model must not sit on mode row: %q", lines[1])
	}
	if !strings.Contains(lines[2], "grok-4.6") || !strings.Contains(lines[2], "reasoning: medium") || !strings.Contains(lines[2], "YOLO:OFF") {
		t.Fatalf("line2 model/reasoning/yolo: %q", lines[2])
	}
	if strings.Contains(lines[1], "reasoning:") {
		t.Fatalf("reasoning must sit on the model row, not mode: %q", lines[1])
	}
	if !strings.Contains(lines[3], "trashname899@gmail.com") || !strings.Contains(lines[3], "7d:99%") || !strings.Contains(lines[3], "resets") {
		t.Fatalf("line3 account/quota: %q", lines[3])
	}
	if !strings.Contains(lines[4], "ctx") {
		t.Fatalf("line4 context: %q", lines[4])
	}
	if !strings.Contains(lines[5], "Gate-sandbox") || !strings.Contains(lines[5], "master") {
		t.Fatalf("line5 project/branch: %q", lines[5])
	}
}

func TestStatusBar_FlowModeHighlightsAndKeepsYoloAuto(t *testing.T) {
	m := New(config.ChatConfig{Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.yolo = false
	m.mode = ModeFlow
	m.launch = LaunchArm{Label: "flow-claude-2-review", Mode: ModeFlow, WorkflowID: "wf"}
	got := stripANSI(m.renderStatusLine())
	if !strings.Contains(got, "[flow]") || !strings.Contains(got, "flow-claude-2-review") {
		t.Fatalf("missing flow mode: %s", got)
	}
	if !strings.Contains(got, "YOLO:ON(auto)") {
		t.Fatalf("flow must still show YOLO auto-on: %s", got)
	}
	raw := m.renderStatusLine()
	if !strings.Contains(raw, "flow-claude-2-review") {
		t.Fatalf("flow name missing from styled line")
	}
}

func TestStatusBar_F4CollapsesDetailsKeepsLine0(t *testing.T) {
	m := New(config.ChatConfig{Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.sessionLoading = false
	m.connStatus = ConnIdle
	m.statusMsg = ""
	m.project = &client.Project{Name: "Gate-sandbox"}
	m.projectBranch = "master"
	full := stripANSI(m.renderStatusLine())
	if strings.Count(full, "\n") < 2 {
		t.Fatalf("expanded should have detail rows:\n%s", full)
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF4})
	am := m2.(*AppModel)
	if !am.statusDetailsCollapsed {
		t.Fatal("F4 should collapse details")
	}
	got := stripANSI(am.renderStatusLine())
	if strings.Contains(got, "\n") {
		t.Fatalf("collapsed must keep only line 0:\n%s", got)
	}
	if !strings.Contains(got, "ready") || !strings.Contains(got, "F4") {
		t.Fatalf("line0 missing: %q", got)
	}
	if strings.Contains(got, "[chat]") || strings.Contains(got, "Gate-sandbox") || strings.Contains(got, "YOLO") {
		t.Fatalf("details leaked while collapsed: %q", got)
	}
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyF4})
	if m3.(*AppModel).statusDetailsCollapsed {
		t.Fatal("F4 again should expand")
	}
}

func TestStatusBar_ClickLine0TogglesDetails(t *testing.T) {
	m := New(config.ChatConfig{Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionLoading = false
	m.sessionPanel.Collapsed = true
	x, y, ok := findClickTarget(m, "status-details")
	if !ok {
		t.Fatal("expected clickable status row")
	}
	m2, _ := m.Update(clickLeft(x, y))
	if !m2.(*AppModel).statusDetailsCollapsed {
		t.Fatalf("click at %d,%d should collapse status details", x, y)
	}
}
