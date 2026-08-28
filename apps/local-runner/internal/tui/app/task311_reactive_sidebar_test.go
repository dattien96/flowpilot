package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// enableSidebarForTest gives the model session-panel CONTENT (RunnerURL) so
// the reactive sidebar gate `hasContent && width >= tuiSidebarMinWidth` is
// driven purely by the width each test sets (Task-311).
func enableSidebarForTest(m *AppModel) {
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
}

// TestTask311_SidebarIsWidthReactive: no F2 toggle — width is the only switch.
func TestTask311_SidebarIsWidthReactive(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"

	m.width, m.fullWidth = tuiSidebarMinWidth-1, tuiSidebarMinWidth-1
	if m.useRightSidebar() {
		t.Fatal("narrow terminal must hide the sidebar")
	}
	if m.chatWidth() != m.terminalWidth() {
		t.Fatalf("narrow: chatWidth must equal terminal width, got %d vs %d", m.chatWidth(), m.terminalWidth())
	}

	m.width, m.fullWidth = tuiSidebarMinWidth+20, tuiSidebarMinWidth+20
	if !m.useRightSidebar() {
		t.Fatal("wide terminal must show the sidebar")
	}
	if m.chatWidth() != m.terminalWidth()-m.sideWidth()-1 {
		t.Fatalf("wide: chatWidth must shrink by sidebar, got %d", m.chatWidth())
	}
}

// TestTask311_F2PrintsInfoDump: F2 is the /info alias, it must not toggle any
// panel state (there is none anymore).
func TestTask311_F2PrintsInfoDump(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF2})
	am := got.(*AppModel)
	if len(am.messages) == 0 {
		t.Fatal("F2 must print the info dump as a chat message")
	}
	last := am.messages[len(am.messages)-1]
	if !strings.Contains(last.Content, "Status:") {
		t.Fatalf("F2 info dump must contain Status line, got %q", last.Content)
	}
	if !strings.Contains(last.Content, "Model:") {
		t.Fatalf("F2 info dump must contain Model line, got %q", last.Content)
	}
}

// TestTask311_F3F4AreNoOps: F3/F4 must not change any state.
func TestTask311_F3F4AreNoOps(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF3})
	if len(got.(*AppModel).messages) != 0 {
		t.Fatal("F3 must be a no-op")
	}
	got2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF4})
	am2 := got2.(*AppModel)
	if len(am2.messages) != 0 {
		t.Fatal("F4 must be a no-op")
	}
}

// TestTask311_MainStatusIsSingleLine: status row is now empty – the chrome
// lives in the input frame (top-left: chat/flow · ready · agent, bottom-right:
// Model · reasoning · YOLO). Only a flash toast uses the status line.
func TestTask311_MainStatusIsSingleLine(t *testing.T) {
	m := New(config.ChatConfig{Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.sessionLoading = false
	m.connStatus = ConnIdle
	m.statusMsg = ""
	m.project = &client.Project{Name: "Gate-sandbox"}
	m.projectBranch = "master"
	enableSidebarForTest(m)

	got := stripANSI(m.renderStatusLine())
	if strings.TrimSpace(got) != "" {
		t.Fatalf("status line must be empty (chrome moved to input frame), got %q", got)
	}
	// Ready and agent now live in the input frame header
	m.width, m.fullWidth = 80, 80
	header := stripANSI(m.chatFrameTitle())
	if !strings.Contains(header, "ready") {
		t.Fatalf("chat header must carry ready, got %q", header)
	}
	if !strings.Contains(m.inputFrameFooter(), "grok-4.6") {
		t.Fatalf("input footer must carry model, got %q", m.inputFrameFooter())
	}
}

// TestTask311_SidebarCarriesStatusDetails: sidebar now only holds session +
// steps (no status). Status details live in the input frame chrome.
func TestTask311_SidebarCarriesStatusDetails(t *testing.T) {
	m := New(config.ChatConfig{Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.sessionLoading = false
	m.connStatus = ConnIdle
	m.model = "grok-4.6"
	enableSidebarForTest(m)
	m.width, m.fullWidth = tuiSidebarMinWidth+20, tuiSidebarMinWidth+20

	side := m.renderRightSidebar(30)
	joined := strings.Join(side, "\n")
	if strings.Contains(joined, "status") {
		t.Fatalf("sidebar must NOT have a status section (moved to input frame):\n%s", joined)
	}
	if strings.Contains(stripANSI(joined), "grok-4.6") {
		t.Fatalf("sidebar must NOT carry the model line (now in input footer):\n%s", joined)
	}
	if !strings.Contains(joined, "session") {
		t.Fatalf("sidebar must keep the session section:\n%s", joined)
	}
	if !strings.Contains(joined, "steps") && !strings.Contains(joined, "(no steps)") {
		t.Fatalf("sidebar must keep the steps section:\n%s", joined)
	}
	// Input frame now owns the status chrome
	footer := m.inputFrameFooter()
	if !strings.Contains(footer, "grok-4.6") || !strings.Contains(footer, "reasoning:") || !strings.Contains(footer, "YOLO") {
		t.Fatalf("input footer must be Model · reasoning · YOLO, got %q", footer)
	}
	header := strings.ToLower(stripANSI(m.chatFrameTitle()))
	if !strings.Contains(header, "chat") && !strings.Contains(header, "flow") {
		t.Fatalf("input header must show chat/flow name, got %q", header)
	}
}

// TestTask311_NoOverlayInChatChrome: tuiChrome panelLines must be nil — the
// chat pane has no overlay/panel rows anymore.
func TestTask311_NoOverlayInChatChrome(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	enableSidebarForTest(m)
	m.width, m.fullWidth = tuiSidebarMinWidth+20, tuiSidebarMinWidth+20
	c := m.tuiChrome()
	if len(c.panelLines) != 0 {
		t.Fatalf("chat chrome must not carry overlay panel lines, got %d", len(c.panelLines))
	}
	if c.sideActive != true {
		t.Fatal("wide terminal must activate the sidebar in chrome")
	}
}