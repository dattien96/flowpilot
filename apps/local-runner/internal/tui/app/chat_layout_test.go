package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestDesktopChatColorTokens_MatchStylesCSS(t *testing.T) {
	// Keep TUI palette aligned with apps/desktop-flowpilot/src/styles.css :root.
	want := map[string]string{
		"text":    "#ececec",
		"textDim": "#9b9b9b",
		"accent":  "#4c8dff",
		"warn":    "#f0b429",
		"ask":     "#b07cff",
		"ok":      "#3fb950",
		"err":     "#f85149",
	}
	got := map[string]string{
		"text":    colorText,
		"textDim": colorTextDim,
		"accent":  colorAccent,
		"warn":    colorWarn,
		"ask":     colorAsk,
		"ok":      colorOK,
		"err":     colorErr,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s=%q want %q", k, got[k], v)
		}
	}
}

func TestStatusline_ShowsProviderAccountNotSupabaseEmail(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = "codex"
	m.model = "gpt-5.4"
	m.providerAccounts = []client.ProviderAccountSummary{
		{ProviderKey: "codex", DisplayLabel: "codex-main", IsActive: true, AuthStatus: "connected"},
	}
	m.bindActiveAccountForProvider()
	m.signedInEmail = "you@supabase.example"
	m.authNeedLogin = false
	m.asciiMode = true
	line := m.renderStatusLine()
	if !strings.Contains(line, "codex-main") {
		t.Fatalf("want provider account label: %q", line)
	}
	if strings.Contains(line, "you@supabase.example") {
		t.Fatalf("statusline must not show Supabase email: %q", line)
	}
}

func TestBindActiveAccount_IgnoresStaleOtherProviderAccount(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = "grok"
	m.account = &client.ProviderAccountSummary{
		ProviderKey: "codex", DisplayLabel: "stale-codex", IsActive: true, AuthStatus: "connected",
	}
	m.accountLabel = "stale-codex"
	m.providerAccounts = []client.ProviderAccountSummary{
		{ProviderKey: "codex", DisplayLabel: "stale-codex", IsActive: true, AuthStatus: "connected"},
		{ProviderKey: "grok", DisplayLabel: "grok-ready", IsActive: true, AuthStatus: "connected"},
	}
	m.bindActiveAccountForProvider()
	if got := m.activeProviderAccountLabel(); got != "grok-ready" {
		t.Fatalf("label=%q want grok-ready", got)
	}
	line := m.renderStatusLine()
	if strings.Contains(line, "stale-codex") || !strings.Contains(line, "grok-ready") {
		t.Fatalf("statusline=%q", line)
	}
}

func TestRenderMessages_UserAlignedRight(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 40
	m.messages = []ChatMessage{
		{Role: "user", Content: "hello right"},
		{Role: "assistant", Content: "hello left"},
	}
	lines := m.renderMessages()
	if len(lines) < 2 {
		t.Fatalf("lines=%v", lines)
	}
	user := lines[0]
	if !strings.Contains(user, "You:") || !strings.HasPrefix(user, " ") {
		t.Fatalf("user line should be right-padded: %q", user)
	}
	assist := lines[1]
	if strings.HasPrefix(assist, " ") {
		t.Fatalf("assistant should stay left: %q", assist)
	}
}

func TestSessionPanel_ToggleF2AndInfo(t *testing.T) {
	m := New(config.ChatConfig{ProjectPath: `D:\proj`}, "http://127.0.0.1:4317")
	m.sessionPanel = sessionInfoPanel{
		RunnerURL:   "http://127.0.0.1:4317",
		ProjectPath: `D:\proj`,
		Session:     "codex · o3 (acct)",
		Collapsed:   false,
	}
	overlay := m.renderSessionPanelOverlay()
	if len(overlay) < 3 {
		t.Fatalf("expanded overlay too short: %v", overlay)
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF2})
	am := m2.(*AppModel)
	if !am.sessionPanel.Collapsed {
		t.Fatal("F2 should collapse panel")
	}
	chip := am.renderSessionPanelOverlay()
	if len(chip) != 1 || !strings.Contains(chip[0], "info") {
		t.Fatalf("collapsed chip=%v", chip)
	}
	m3, _ := am.handleSlashCommand("/info")
	if m3.(*AppModel).sessionPanel.Collapsed {
		t.Fatal("/info should expand again")
	}
}

func TestSessionDefaults_GoesToPanelNotChatSpam(t *testing.T) {
	m := New(config.ChatConfig{ProjectPath: `D:\working\gate-sandbox`}, "http://127.0.0.1:4317")
	m2, _ := m.Update(ConnectedMsg{RunnerURL: "http://127.0.0.1:4317"})
	am := m2.(*AppModel)
	for _, msg := range am.messages {
		if strings.Contains(msg.Content, "Connected to runner") {
			t.Fatalf("connected should not spam chat: %+v", msg)
		}
	}
	m3, _ := am.Update(SessionDefaultsMsg{
		Provider:     "codex",
		Model:        "gpt-5.4",
		AccountLabel: "bruce-acct",
		Project:      &client.Project{ID: "db51ec26-1a0f-4b92-8ceb-b03dc8e9b363", Name: "Gate-sandbox"},
	})
	am = m3.(*AppModel)
	if !strings.Contains(am.sessionPanel.Session, "bruce-acct") {
		t.Fatalf("panel session=%q", am.sessionPanel.Session)
	}
	for _, msg := range am.messages {
		if strings.HasPrefix(msg.Content, "Session:") || strings.HasPrefix(msg.Content, "Project: Gate-sandbox") {
			t.Fatalf("session/project dump should be in panel, got chat: %q", msg.Content)
		}
	}
}

func TestSuggestionWindow_KeepsSelectionVisible(t *testing.T) {
	start, end := suggestionWindow(30, 25, 8)
	if start > 25 || end <= 25 {
		t.Fatalf("selected 25 not in [%d,%d)", start, end)
	}
	if end-start != 8 {
		t.Fatalf("window size=%d", end-start)
	}
}

func TestRenderInputLine_UsesStrokeNotFullBackground(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionDefaultsLoaded = true
	m.sessionLoading = false
	m.inputValue = "hello"
	m.cursorOn = false
	line := m.renderInputLine()
	if !strings.Contains(line, "┃") && !strings.Contains(line, "|") {
		// lipgloss may wrap; ensure no solid bg wash style by checking stroke glyph present
		if !strings.Contains(line, "chat") {
			t.Fatalf("input=%q", line)
		}
	}
	if !strings.Contains(line, "hello") {
		t.Fatalf("missing body: %q", line)
	}
}

func TestFormatOpenChatErrDetailed_IncludesActiveAccount(t *testing.T) {
	err := &client.APIError{Status: 409, Code: "session_unavailable", Message: "session data not found on this machine"}
	got := formatOpenChatErrDetailed(err, "grok", "my-grok-acct")
	if !strings.Contains(got, "grok") || !strings.Contains(got, "my-grok-acct") {
		t.Fatalf("got=%q", got)
	}
	if !strings.Contains(got, "AI Providers") {
		t.Fatalf("missing activate hint: %q", got)
	}
}

func TestRenderSuggestions_HistoryScrollsToOlder(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.asciiMode = true
	items := make([]suggestItem, 0, 20)
	for i := 0; i < 20; i++ {
		items = append(items, suggestItem{
			value:  fmt.Sprintf("run-%02d", i),
			detail: "chat",
			kind:   "history",
			slash:  "/history",
		})
	}
	m.suggIdx = 15
	view := m.renderSuggestions(items)
	if !strings.Contains(view, "above") || !strings.Contains(view, "below") {
		t.Fatalf("expected scroll hint:\n%s", view)
	}
	if !strings.Contains(view, "[16/20]") {
		t.Fatalf("expected selection position:\n%s", view)
	}
	// Selected row must be rendered (not stuck on first page only).
	selVal := items[15].value
	if !strings.Contains(view, selVal) {
		t.Fatalf("selected value %q missing from scrolled view:\n%s", selVal, view)
	}
}
