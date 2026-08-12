package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestFilterSlashSuggestions_ShowsOnSlash(t *testing.T) {
	sugg := filterSlashSuggestions("/")
	if len(sugg) == 0 {
		t.Fatal("expected suggestions for '/'")
	}
	sugg = filterSlashSuggestions("/mo")
	if len(sugg) != 1 || sugg[0].name != "/model" {
		t.Fatalf("got %+v want /model", sugg)
	}
	if filterSlashSuggestions("/model gpt") != nil {
		t.Fatal("no suggestions once args are typed")
	}
}

func TestSlashTabCompletesCommand(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/pro"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if got := m2.(*AppModel).inputValue; got != "/provider " {
		t.Fatalf("inputValue=%q want /provider ", got)
	}
}

func TestProviderAndModelListAndSet(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "codex", Models: []client.ProviderModel{
			{ID: "gpt-5.4", Available: true},
			{ID: "o3", Available: true},
		}},
		{Key: "claude", Models: []client.ProviderModel{{ID: "sonnet", Available: true}}},
	}
	m.provider = "codex"
	m.model = "gpt-5.4"

	m2, _ := m.handleSlashCommand("/model")
	view := m2.(*AppModel).View()
	if !strings.Contains(view, "gpt-5.4") || !strings.Contains(view, "o3") {
		t.Fatalf("model list missing ids:\n%s", view)
	}

	m3, _ := m2.(*AppModel).handleSlashCommand("/model o3")
	if m3.(*AppModel).model != "o3" {
		t.Fatalf("model=%q", m3.(*AppModel).model)
	}

	m4, _ := m3.(*AppModel).handleSlashCommand("/provider")
	if !strings.Contains(m4.(*AppModel).View(), "claude") {
		t.Fatal("provider list should include claude")
	}
}

func TestPickActiveSessionDefaults_UsesModelID(t *testing.T) {
	provider, model, _ := pickActiveSessionDefaults("", "",
		[]client.ProviderAccountSummary{{ProviderKey: "codex", IsActive: true, DisplayLabel: "bob"}},
		[]client.Provider{{Key: "codex", Models: []client.ProviderModel{
			{ID: "gpt-5.4", Available: true},
		}}},
	)
	if provider != "codex" || model != "gpt-5.4" {
		t.Fatalf("got %q %q", provider, model)
	}
}

func TestMatchProjectByPath_CrossSlash(t *testing.T) {
	p := matchProjectByPath([]client.Project{
		{ID: "p1", Path: `D:\working\gate-sandbox`},
	}, "D:/working/gate-sandbox")
	if p == nil || p.ID != "p1" {
		t.Fatalf("match=%v", p)
	}
}

func TestMatchProjectByPath_BasenameFallback(t *testing.T) {
	p := matchProjectByPath([]client.Project{
		{ID: "p1", Path: `C:\Users\me\gate-sandbox`},
	}, `D:\working\gate-sandbox`)
	if p == nil || p.ID != "p1" {
		t.Fatalf("basename match=%v", p)
	}
}

func TestFormatAccountAndContextLimits(t *testing.T) {
	five, seven := 72, 40
	acc := &client.ProviderAccountSummary{Remaining5hPercent: &five, Remaining7dPercent: &seven}
	if got := formatAccountLimits(acc); got != "5h:72% 7d:40%" {
		t.Fatalf("account limits=%q", got)
	}
	win := int64(128000)
	usage := &client.TokenUsageSnapshot{
		ModelContextWindow: &win,
		Total:              &client.TokenUsageBreakdown{TotalTokens: 12000},
		Last:               &client.TokenUsageBreakdown{TotalTokens: 800},
	}
	got := formatContextLimits(usage, 0)
	if !strings.Contains(got, "left") || !strings.Contains(got, "12.0k") {
		t.Fatalf("context limits=%q", got)
	}
}

func TestFormatMissingProjectHelp_ListsCatalog(t *testing.T) {
	msg := formatMissingProjectHelp(`D:\working\gate-sandbox`, []client.Project{
		{ID: "abc", Name: "Gate", Path: `D:\other\gate`},
	})
	if !strings.Contains(msg, "project_id") || !strings.Contains(msg, "abc") {
		t.Fatalf("help=%q", msg)
	}
}

func TestWrapText_WrapsLongLines(t *testing.T) {
	lines := wrapText("hello world this is a long line", 12)
	if len(lines) < 2 {
		t.Fatalf("expected wrap, got %#v", lines)
	}
	for _, line := range lines {
		if len([]rune(line)) > 12 {
			t.Fatalf("line too long: %q", line)
		}
	}
}

func TestWrapText_PreservesNewlinesAndHardBreaks(t *testing.T) {
	lines := wrapText("abc\n" + strings.Repeat("x", 20), 10)
	if lines[0] != "abc" {
		t.Fatalf("first=%q", lines[0])
	}
	joined := strings.Join(lines[1:], "")
	if joined != strings.Repeat("x", 20) {
		t.Fatalf("hard-break lost chars: %q", joined)
	}
}

func TestRenderMessages_WrapsToWidth(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 20
	m.height = 40
	m.messages = []ChatMessage{{
		Role:    "assistant",
		Content: "This is a reasonably long assistant reply that must wrap across multiple terminal rows.",
	}}
	lines := m.renderMessages()
	if len(lines) < 3 {
		t.Fatalf("expected multiple wrapped lines, got %d: %#v", len(lines), lines)
	}
}

func TestFlowListMsg_ShowsBuiltinsWhenCatalogFails(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(FlowListMsg{
		Builtins:   []client.BuiltinFlowOption{{FlowRef: "pack/review-loop", Label: "review-loop"}},
		CatalogErr: "catalog_unavailable: supabase down",
	})
	view := m2.(*AppModel).View()
	if !strings.Contains(view, "pack/review-loop") {
		t.Fatalf("missing builtin in view:\n%s", view)
	}
	if !strings.Contains(view, "catalog unavailable") {
		t.Fatalf("missing catalog warning:\n%s", view)
	}
}

func TestProcessInput_StartsRunThenKeepsPendingPrompt(t *testing.T) {
	m := New(config.ChatConfig{ProjectPath: filepath.Join(t.TempDir(), "proj")}, "http://127.0.0.1:9")
	_ = m // project path need not exist for this unit path — only pending state
	// Use a path that Resolve isn't called for here.
	m.cfg.ProjectPath = t.TempDir()
	m2, cmd := m.processInput("hello world")
	am := m2.(*AppModel)
	if am.pendingPrompt != "hello world" {
		t.Fatalf("pendingPrompt=%q", am.pendingPrompt)
	}
	if cmd == nil {
		t.Fatal("expected start-run cmd")
	}
	// Simulate run started → should schedule send turn.
	am.pendingPrompt = "hello world"
	m3, cmd2 := am.Update(RunStartedMsg{Handle: client.RunHandle{
		RunID: "run-1", ProviderKey: "codex", StepID: "chat-run-1",
	}})
	if m3.(*AppModel).runHandle == nil || m3.(*AppModel).stepID != "chat-run-1" {
		t.Fatalf("run not armed: %+v", m3.(*AppModel).runHandle)
	}
	if cmd2 == nil {
		t.Fatal("expected send-turn cmd after RunStartedMsg")
	}
}

func TestTurnDoneMsg_RendersFinalAssistant(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(TurnDoneMsg{FinalMsg: "hi there"})
	am := m2.(*AppModel)
	if len(am.messages) == 0 || am.messages[len(am.messages)-1].Content != "hi there" {
		t.Fatalf("messages=%+v", am.messages)
	}
}

func TestAuthBanner_ShowsWhenNeedLogin(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.authNeedLogin = true
	m.width = 100
	m.height = 24
	view := m.View()
	if !strings.Contains(view, "SIGN IN REQUIRED") {
		t.Fatalf("missing auth banner:\n%s", view)
	}
	if !strings.Contains(m.renderStatusLine(), "SIGN-IN") {
		t.Fatalf("statusline missing SIGN-IN: %q", m.renderStatusLine())
	}
}

func TestLoginSlash_StartsEmailPhase(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/login")
	am := m2.(*AppModel)
	if am.authPhase != AuthEmail {
		t.Fatalf("authPhase=%v want AuthEmail", am.authPhase)
	}
	if !strings.Contains(am.renderInputLine(), "email") {
		t.Fatalf("input line=%q", am.renderInputLine())
	}
}

func TestLoginSlash_EmailThenPasswordMasked(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/login you@example.com")
	am := m2.(*AppModel)
	if am.authPhase != AuthPassword || am.authEmail != "you@example.com" {
		t.Fatalf("phase=%v email=%q", am.authPhase, am.authEmail)
	}
	am.inputValue = "secret"
	line := am.renderInputLine()
	if strings.Contains(line, "secret") || !strings.Contains(line, "*****") {
		t.Fatalf("password not masked: %q", line)
	}
}

func TestFilterFlowSuggestions_FiltersAsYouType(t *testing.T) {
	builtins := []client.BuiltinFlowOption{
		{FlowRef: "pack/review-loop", Label: "review-loop"},
		{FlowRef: "pack/fix-bug", Label: "fix-bug"},
	}
	workflows := []client.Workflow{
		{ID: "wf-android", Name: "Android ship", ProjectID: "p1"},
		{ID: "wf-ios", Name: "iOS ship", ProjectID: "p1"},
	}
	all := filterFlowSuggestions("/flow ", builtins, workflows, "p1")
	if len(all) != 4 {
		t.Fatalf("expected 4 flows, got %d: %+v", len(all), all)
	}
	filtered := filterFlowSuggestions("/flow rev", builtins, workflows, "p1")
	if len(filtered) != 1 || filtered[0].value != "pack/review-loop" {
		t.Fatalf("filtered=%+v", filtered)
	}
	if filterFlowSuggestions("/flow", builtins, workflows, "p1") != nil {
		t.Fatal("bare /flow should not open flow picker (command suggestions instead)")
	}
}

func TestFlowTabCompletesSelection(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.flowBuiltins = []client.BuiltinFlowOption{
		{FlowRef: "pack/review-loop", Label: "review-loop"},
		{FlowRef: "pack/fix-bug", Label: "fix-bug"},
	}
	m.inputValue = "/flow re"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if got := m2.(*AppModel).inputValue; got != "/flow pack/review-loop" {
		t.Fatalf("inputValue=%q", got)
	}
}

func TestFlowArrowSelectsThenTab(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.flowBuiltins = []client.BuiltinFlowOption{
		{FlowRef: "pack/a", Label: "a"},
		{FlowRef: "pack/b", Label: "b"},
	}
	m.inputValue = "/flow "
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	am := m2.(*AppModel)
	if am.suggIdx != 1 {
		t.Fatalf("suggIdx=%d", am.suggIdx)
	}
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if got := m3.(*AppModel).inputValue; got != "/flow pack/b" {
		t.Fatalf("inputValue=%q", got)
	}
}

func TestErrMsg_ClearsPendingStartAndSurfacesError(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.pendingPrompt = "hi"
	m.connStatus = ConnRunning
	m.statusMsg = "sending..."
	m2, _ := m.Update(ErrMsg{Err: fmt.Errorf("start run: timeout")})
	am := m2.(*AppModel)
	if am.pendingPrompt != "" {
		t.Fatal("pendingPrompt should clear on error")
	}
	if am.connStatus != ConnError {
		t.Fatalf("connStatus=%v", am.connStatus)
	}
	if !strings.Contains(am.View(), "start run: timeout") {
		t.Fatalf("error not shown:\n%s", am.View())
	}
}

func TestProcessInput_RequiresProviderAfterSessionDefaults(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = ""
	m.sessionDefaultsLoaded = true
	m2, cmd := m.processInput("hello")
	am := m2.(*AppModel)
	if cmd != nil {
		t.Fatal("should not start run without provider")
	}
	if am.runHandle != nil || am.pendingPrompt != "" {
		t.Fatal("should not arm a run without provider")
	}
	if !strings.Contains(am.View(), "No provider selected") {
		t.Fatalf("expected provider error:\n%s", am.View())
	}
}

func TestEnter_IgnoredWhileTurnInProgress_KeepsDraft(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.connStatus = ConnRunning
	m.runHandle = &client.RunHandle{RunID: "r1"}
	m.inputValue = "next question"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.inputValue != "next question" {
		t.Fatalf("draft should stay, got %q", am.inputValue)
	}
	if am.connStatus != ConnRunning {
		t.Fatalf("should stay in progress, got %v", am.connStatus)
	}
	// Typing still works while blocked.
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	if m3.(*AppModel).inputValue != "next question!" {
		t.Fatalf("typing should work while in progress, got %q", m3.(*AppModel).inputValue)
	}
}

func TestSessionLoading_DisablesChatUntilDefaults(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(ConnectedMsg{RunnerURL: "http://127.0.0.1:4317"})
	am := m2.(*AppModel)
	if !am.sessionLoading {
		t.Fatal("expected sessionLoading after ConnectedMsg")
	}
	view := am.View()
	if !strings.Contains(view, "FlowPilot") && !strings.Contains(view, "loading session") {
		t.Fatalf("missing FlowPilot loading UI:\n%s", view)
	}
	// Plain chat typing blocked; slash still allowed.
	am.inputValue = ""
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m3.(*AppModel).inputValue != "" {
		t.Fatalf("chat should be disabled while loading, got input=%q", m3.(*AppModel).inputValue)
	}
	m3b, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if m3b.(*AppModel).inputValue != "/" {
		t.Fatalf("slash should remain available while loading, got %q", m3b.(*AppModel).inputValue)
	}
	m4, _ := am.Update(SessionDefaultsMsg{Provider: "codex", Model: "o3"})
	ready := m4.(*AppModel)
	if ready.sessionLoading {
		t.Fatal("sessionLoading should clear after SessionDefaultsMsg")
	}
	if !strings.Contains(ready.View(), "Ready") {
		t.Fatalf("expected ready message:\n%s", ready.View())
	}
	ready.inputValue = ""
	m5, _ := ready.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m5.(*AppModel).inputValue != "h" {
		t.Fatalf("chat should unlock after load, input=%q", m5.(*AppModel).inputValue)
	}
}

func TestEnter_AcceptsHighlightedSlashSuggestion(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/"
	// Highlight /yolo (skip /help,/clear,/exit,/quit → index of /yolo in known list).
	items := m.collectSuggestions()
	yoloIdx := -1
	for i, it := range items {
		if it.value == "/yolo" {
			yoloIdx = i
			break
		}
	}
	if yoloIdx < 0 {
		t.Fatal("/yolo not in suggestions for '/'")
	}
	m.suggIdx = yoloIdx
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.inputValue != "" {
		t.Fatalf("input should clear after Enter accept, got %q", am.inputValue)
	}
	view := am.View()
	if !strings.Contains(view, "YOLO mode:") {
		t.Fatalf("Enter should run highlighted /yolo:\n%s", view)
	}
}

func TestEnter_AcceptsHighlightedFlowSuggestion(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.flowBuiltins = []client.BuiltinFlowOption{
		{FlowRef: "pack/review-loop", Label: "review-loop"},
		{FlowRef: "pack/fix-bug", Label: "fix-bug"},
	}
	m.inputValue = "/flow "
	m.suggIdx = 1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if !am.launch.IsBuiltin() || am.launch.FlowRef != "pack/fix-bug" {
		t.Fatalf("launch=%+v", am.launch)
	}
	if !strings.Contains(am.View(), "fix-bug") && !strings.Contains(am.View(), "pack/fix-bug") {
		t.Fatalf("expected flow armed message:\n%s", am.View())
	}
}

func TestEnter_BareSlashStillShowsSuggestionsPathViaHelpOnlyWhenEmpty(t *testing.T) {
	// With suggestions open, Enter must NOT fall through to "out of mode" /
	// unknown-command; it accepts the first row (/help).
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/"
	m.suggIdx = 0
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	view := m2.(*AppModel).View()
	if !strings.Contains(view, "Available commands:") && !strings.Contains(view, "/yolo") {
		t.Fatalf("expected /help from first suggestion:\n%s", view)
	}
}

func TestInputLine_ShowsFocusIndicator(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.cursorOn = true
	m.inputValue = "hello"
	line := m.renderInputLine()
	if !strings.Contains(line, "chat") {
		t.Fatalf("missing chat focus label: %q", line)
	}
	if !strings.Contains(line, "hello") {
		t.Fatalf("missing typed text: %q", line)
	}
	view := m.View()
	if !strings.Contains(view, "chat") {
		t.Fatalf("view missing input focus label:\n%s", view)
	}
}

func TestLoginResultMsg_ClearsNeedLoginAndShowsEmail(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.authNeedLogin = true
	m.authPhase = AuthPassword
	m2, cmd := m.Update(LoginResultMsg{
		Email:       "you@example.com",
		UserID:      "uid-1",
		PersistedTo: `C:\Users\x\AppData\Roaming\FlowPilot\supabase-auth-session.json`,
	})
	am := m2.(*AppModel)
	if am.authNeedLogin || am.authPhase != AuthNone {
		t.Fatalf("needLogin=%v phase=%v", am.authNeedLogin, am.authPhase)
	}
	if am.signedInEmail != "you@example.com" {
		t.Fatalf("signedInEmail=%q", am.signedInEmail)
	}
	view := am.View()
	if !strings.Contains(view, "Signed in as you@example.com") {
		t.Fatalf("missing signed-in message:\n%s", view)
	}
	if !strings.Contains(view, "you@example.com") {
		t.Fatalf("status/view missing email:\n%s", view)
	}
	if cmd == nil {
		t.Fatal("expected reload session defaults after login")
	}
}

func TestEscape_CancelsLoginWizard(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.authPhase = AuthEmail
	m.inputValue = "partial"
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	am := m2.(*AppModel)
	if am.authPhase != AuthNone || am.inputValue != "" {
		t.Fatalf("phase=%v input=%q", am.authPhase, am.inputValue)
	}
}

func TestKnownSlashCommands_IncludesLogin(t *testing.T) {
	found := false
	for _, sc := range knownSlashCommands {
		if sc.name == "/login" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("knownSlashCommands missing /login")
	}
}
