package app

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// A9.1 — package boundary: internal/tui must not import internal/runner.
func TestPackageBoundary_TuiDoesNotImportRunnerInternals(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), ".."))
	fset := token.NewFileSet()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(p, "internal/runner") {
				t.Errorf("%s imports forbidden %s", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk tui tree: %v", err)
	}
}

// A9.3 — slash surface matches registered commands.
func TestDocs_CommandAndSlashSurfaceMatchesRegisteredCommands(t *testing.T) {
	names := make(map[string]bool, len(knownSlashCommands))
	for _, sc := range knownSlashCommands {
		names[sc.name] = true
	}
	required := []string{
		"/help", "/clear", "/exit", "/yolo", "/agents", "/flow", "/chat",
		"/skill", "/image", "/provider", "/model", "/reasoning", "/new",
		"/step", "/status", "/resume", "/approve", "/deny",
	}
	for _, cmd := range required {
		if !names[cmd] {
			t.Errorf("knownSlashCommands missing %q", cmd)
		}
	}
}

// A9.4 — audit evidence file exists for cli-tui.
func TestAllTUITasksHaveCliTuiAuditEvidence(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", ".."))
	candidates, _ := filepath.Glob(filepath.Join(repoRoot, "change-audit", "CA-*-cli-tui*.md"))
	if len(candidates) == 0 {
		t.Fatal("no change-audit CA-*-cli-tui*.md entry found")
	}
}

// A9.5 — legacy console uses ASCII separators in statusline.
func TestLegacyConsoleStatusline_UsesASCIIFallback(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.statusMsg = "ok"
	line := m.renderStatusLine()
	if strings.Contains(line, "│") {
		t.Errorf("ascii statusline should not use unicode bar, got %q", line)
	}
	if !strings.Contains(line, "|") {
		t.Errorf("ascii statusline should use pipe separator, got %q", line)
	}
}

// A1.2 — message_delta appends streaming assistant text.
func TestMapEvent_MessageDeltaAppendsStreamingLine(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{Type: "message_delta", Text: "hel"}})
	m3, _ := m2.(*AppModel).Update(EventMsg{Ev: client.ProviderEvent{Type: "message_delta", Text: "lo"}})
	if len(m3.(*AppModel).messages) != 1 || m3.(*AppModel).messages[0].Content != "hello" {
		t.Fatalf("messages = %+v, want single assistant hello", m3.(*AppModel).messages)
	}
}

// A1.3 — turn_completed finalizes assistant message.
func TestMapEvent_TurnCompletedFinalizesAssistant(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{Type: "turn_completed", FinalMessage: "done"}})
	am := m2.(*AppModel)
	if len(am.messages) != 1 || am.messages[0].Content != "done" {
		t.Fatalf("messages = %+v", am.messages)
	}
}

func TestPickActiveSessionDefaults_UsesActiveAccountAndFirstModel(t *testing.T) {
	provider, model, label := pickActiveSessionDefaults("", "",
		[]client.ProviderAccountSummary{
			{ProviderKey: "claude", DisplayLabel: "alice@x", IsActive: false},
			{ProviderKey: "codex", DisplayLabel: "bob@x", IsActive: true},
		},
		[]client.Provider{
			{Key: "codex", Models: []client.ProviderModel{{Name: "gpt-5.4"}}},
			{Key: "claude", Models: []client.ProviderModel{{Name: "sonnet"}}},
		},
	)
	if provider != "codex" || model != "gpt-5.4" || label != "bob@x" {
		t.Fatalf("got provider=%q model=%q label=%q", provider, model, label)
	}
}

func TestPickActiveSessionDefaults_FlagOverridesAccount(t *testing.T) {
	provider, model, _ := pickActiveSessionDefaults("claude", "opus",
		[]client.ProviderAccountSummary{{ProviderKey: "codex", IsActive: true}},
		[]client.Provider{{Key: "claude", Models: []client.ProviderModel{{Name: "sonnet"}}}},
	)
	if provider != "claude" || model != "opus" {
		t.Fatalf("got provider=%q model=%q", provider, model)
	}
}

func TestStatusline_ShowsProviderAndModel(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = "codex"
	m.model = "gpt-5.4"
	m.asciiMode = true
	line := m.renderStatusLine()
	if !strings.Contains(line, "codex") || !strings.Contains(line, "gpt-5.4") {
		t.Fatalf("statusline missing provider/model: %q", line)
	}
}

// A2.1 — /yolo toggles session YOLO flag.
func TestSessionControls_YoloToggle(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/yolo")
	if !m2.(*AppModel).yolo {
		t.Fatal("expected yolo true after first toggle")
	}
	m3, _ := m2.(*AppModel).handleSlashCommand("/yolo")
	if m3.(*AppModel).yolo {
		t.Fatal("expected yolo false after second toggle")
	}
}

// A2.2 — provider change blocked after run started.
func TestSessionControls_ProviderLockedAfterRunStarted(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-1", ProviderKey: "codex"}
	m2, _ := m.handleSlashCommand("/provider claude")
	am := m2.(*AppModel)
	if am.provider != "codex" {
		t.Fatalf("provider changed to %q", am.provider)
	}
	if !strings.Contains(am.View(), "Cannot change provider") {
		t.Fatal("expected provider lock message")
	}
}

// A3.1 — skill list and attach via /skill.
func TestSkillState_ListAndAttach(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/skill my-skill")
	if len(m2.(*AppModel).selectedSkills) != 1 {
		t.Fatalf("selectedSkills = %+v", m2.(*AppModel).selectedSkills)
	}
	m3, _ := m2.(*AppModel).handleSlashCommand("/skill")
	if !strings.Contains(m3.(*AppModel).View(), "my-skill") {
		t.Fatal("skill list should mention attached skill")
	}
}

// A3.2 — /skill toggle removes skill.
func TestSkillState_Clear(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/skill x")
	m3, _ := m2.(*AppModel).handleSlashCommand("/skill x")
	if len(m3.(*AppModel).selectedSkills) != 0 {
		t.Fatalf("expected empty skills, got %+v", m3.(*AppModel).selectedSkills)
	}
}

// A3.4 — turn input builder includes pending skills (unit-level).
func TestTurnInput_IncludesPendingSkillsThenClearsAfterAccepted(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.selectedSkills = []client.SkillSelection{{Name: "s1"}}
	in := m.buildTurnInput("hi")
	if len(in.SelectedSkills) != 1 || in.SelectedSkills[0].Name != "s1" {
		t.Fatalf("SelectedSkills = %+v", in.SelectedSkills)
	}
	m.clearPendingTurnPayload()
	if len(m.selectedSkills) != 0 {
		t.Fatal("skills should clear after accepted turn payload")
	}
}

// A4.1 — builtin flow ref resolution helper.
func TestResolveFlowRef_BuiltinPack(t *testing.T) {
	ref, sub, err := resolveFlowRef([]client.BuiltinFlowOption{
		{FlowRef: "flowpilot-core-flow-pack/review-loop", Label: "review-loop"},
	}, nil, "review-loop")
	if err != nil || ref == "" || sub == "" {
		t.Fatalf("resolveFlowRef = %q %q err=%v", ref, sub, err)
	}
}

// A5.1 — usage line formatting.
func TestFormatUsageLine_ContextAndLastTurn(t *testing.T) {
	line := formatUsageLine(1200, 128000, 8000)
	if !strings.Contains(line, "1k") || !strings.Contains(line, "128k") {
		t.Fatalf("usage line = %q", line)
	}
}

// A6.1 — agents ordered with main first.
func TestProjectAgents_OrdersMainFirst(t *testing.T) {
	ordered := orderAgentsMainFirst([]client.AgentRunSummary{
		{AgentName: "child", Role: "reviewer"},
		{AgentName: "main", Role: "main"},
	})
	if ordered[0].AgentName != "main" {
		t.Fatalf("order = %+v", ordered)
	}
}

// A6.2 — child focus disables send.
func TestFocusAgent_DisablesSendOnChild(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.agentRuns = []client.AgentRunSummary{{AgentName: "main", Role: "main"}, {AgentName: "child", Role: "reviewer"}}
	m.agentsFocus = true
	m.focusedAgentIdx = 1
	if m.canSend() {
		t.Fatal("send should be disabled on child focus")
	}
}

// A6.4 — Tab wraps agent focus.
func TestCycleAgent_Wraps(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.agentRuns = []client.AgentRunSummary{{AgentName: "main", Role: "main"}, {AgentName: "child", Role: "reviewer"}}
	m.agentsFocus = true
	m.focusedAgentIdx = 1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if m2.(*AppModel).focusedAgentIdx != 0 {
		t.Fatalf("idx = %d want 0", m2.(*AppModel).focusedAgentIdx)
	}
}

// A7.1 — permission gate parsing.
func TestGateFromEvent_PermissionRequired(t *testing.T) {
	g := gateFromEvent(client.ProviderEvent{Type: "permission_required", ApprovalID: "ap-1"})
	if g == nil || g.Kind != "approval" {
		t.Fatalf("gate = %+v", g)
	}
}

// A8.1 — headless prints final message path (model quit on TurnDone in print mode).
func TestRunHeadless_PrintsFinalMessage(t *testing.T) {
	m := New(config.ChatConfig{Print: true}, "http://127.0.0.1:4317")
	m2, cmd := m.Update(TurnDoneMsg{FinalMsg: "hello"})
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if m2.(*AppModel).headlessOutput != "hello" {
		t.Fatalf("headlessOutput = %q", m2.(*AppModel).headlessOutput)
	}
}

// A8.4 — /new clears session state.
func TestNewCommand_ClearsAllSessionState(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "r1"}
	m.selectedSkills = []client.SkillSelection{{Name: "s"}}
	m2, _ := m.handleSlashCommand("/new")
	am := m2.(*AppModel)
	if am.runHandle != nil || len(am.selectedSkills) != 0 {
		t.Fatal(" /new should reset run and skills")
	}
}
