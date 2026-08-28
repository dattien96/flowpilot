package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Task-311: the main status is ONE always-visible line; mode/model/account/
// context rows moved into the reactive sidebar.

func TestStatusBar_SingleLineAndSidebarDetails(t *testing.T) {
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

	// Per user request: status line is empty (only toast), chrome is in input frame
	got := stripANSI(m.renderStatusLine())
	if strings.TrimSpace(got) != "" {
		t.Fatalf("status line must be empty (chrome moved to input frame), got %q", got)
	}
	header := stripANSI(m.chatFrameTitle())
	if !strings.Contains(strings.ToLower(header), "chat") || !strings.Contains(strings.ToLower(header), "ready") {
		t.Fatalf("input header must show Chat · ready, got %q", header)
	}
	footer := m.inputFrameFooter()
	if !strings.Contains(footer, "grok-4.6") || !strings.Contains(footer, "reasoning: medium") || !strings.Contains(footer, "YOLO:OFF") {
		t.Fatalf("input footer must be Model · reasoning · YOLO, got %q", footer)
	}
	if strings.Contains(footer, "7d:99%") || strings.Contains(footer, "trashname") {
		t.Fatalf("footer must NOT contain quota/account (user request), got %q", footer)
	}

	enableSidebarForTest(m)
	m.width, m.fullWidth = tuiSidebarMinWidth+10, tuiSidebarMinWidth+10
	side := strings.Join(m.renderRightSidebar(30), "\n")
	// Sidebar now only session + steps (no status)
	if strings.Contains(side, "grok-4.6") || strings.Contains(side, "YOLO") || strings.Contains(side, "trashname") || strings.Contains(side, "ctx") {
		t.Fatalf("sidebar must NOT contain status details (moved to input frame):\n%s", side)
	}
	if !strings.Contains(side, "session") {
		t.Fatalf("sidebar must keep session:\n%s", side)
	}
	if !strings.Contains(side, "steps") && !strings.Contains(side, "(no steps)") {
		t.Fatalf("sidebar must keep steps:\n%s", side)
	}
}

func TestStatusBar_FlowModeHighlightsAndKeepsYoloAuto(t *testing.T) {
	m := New(config.ChatConfig{Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.yolo = false
	m.mode = ModeFlow
	m.launch = LaunchArm{Label: "flow-claude-2-review", Mode: ModeFlow, WorkflowID: "wf"}
	// Flow name and YOLO now live in input frame chrome, not sidebar
	header := stripANSI(m.chatFrameTitle())
	if !strings.Contains(header, "flow-claude-2-review") {
		t.Fatalf("header missing flow name: %s", header)
	}
	footer := m.inputFrameFooter()
	if !strings.Contains(footer, "YOLO:ON(auto)") {
		t.Fatalf("footer must still show YOLO auto-on: %s", footer)
	}
}

func TestStatusBar_F4IsNoOpAndLineStaysSingle(t *testing.T) {
	m := New(config.ChatConfig{Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.sessionLoading = false
	m.connStatus = ConnIdle
	m.statusMsg = ""
	m.project = &client.Project{Name: "Gate-sandbox"}
	m.projectBranch = "master"
	before := stripANSI(m.renderStatusLine())
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF4})
	am := m2.(*AppModel)
	after := stripANSI(am.renderStatusLine())
	if after != before {
		t.Fatalf("F4 must be a no-op: before=%q after=%q", before, after)
	}
	if strings.Contains(after, "\n") {
		t.Fatalf("status must stay a single line:\n%s", after)
	}
	// Ready now lives in input header, status line is empty (only toast)
	if strings.Contains(after, "ready") {
		t.Fatalf("status line must be empty (ready moved to input header), got %q", after)
	}
	header := stripANSI(am.chatFrameTitle())
	if !strings.Contains(header, "ready") {
		t.Fatalf("header must contain ready: %q", header)
	}
}

func TestStatusBar_StatusDetailsClickIsNoOp(t *testing.T) {
	m := New(config.ChatConfig{Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionLoading = false
	m2, _ := m.activateClickTarget("status-details")
	if len(m2.(*AppModel).messages) != 0 {
		t.Fatal("status-details click must be a no-op")
	}
}