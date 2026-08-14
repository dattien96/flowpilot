package app

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/tui/prefs"
)

// isolateSessionFile points prefs.Paths at a per-test file and enables mode
// restore (package TestMain sets FLOWPILOT_TUI_SKIP_MODE_RESTORE=1).
func isolateSessionFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tui-session.json")
	t.Setenv("FLOWPILOT_TUI_SESSION_FILE", path)
	t.Setenv("FLOWPILOT_TUI_SKIP_MODE_RESTORE", "")
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	return path
}

func TestNew_RestoresFlowModeAndBuiltinFromPrefs(t *testing.T) {
	isolateSessionFile(t)
	if _, err := prefs.Save(prefs.Session{
		Provider:  "grok",
		Model:     "grok-4.5",
		Mode:      "flow",
		FlowRef:   "pack/fix-bug",
		FlowLabel: "Fix Bug",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	if m.mode != ModeFlow {
		t.Fatalf("mode=%v want ModeFlow", m.mode)
	}
	if m.launch.FlowRef != "pack/fix-bug" {
		t.Fatalf("FlowRef=%q", m.launch.FlowRef)
	}
	if m.launch.Label != "Fix Bug" {
		t.Fatalf("Label=%q", m.launch.Label)
	}
	if !m.firstTurnPending {
		t.Fatal("builtin restore should set firstTurnPending")
	}
	if m.launch.SubMode != "bug" || m.launch.ChangeType != "bugfix" {
		t.Fatalf("builtin extras SubMode=%q ChangeType=%q", m.launch.SubMode, m.launch.ChangeType)
	}
}

func TestNew_RestoresChatModeClearsLaunch(t *testing.T) {
	isolateSessionFile(t)
	if _, err := prefs.Save(prefs.Session{
		Provider: "grok",
		Mode:     "chat",
		FlowRef:  "stale", // Save normalizes chat → clear flow; write raw for Load path
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// After Save with mode=chat, flow is cleared on disk. New should be chat.
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	if m.mode != ModeChat {
		t.Fatalf("mode=%v want ModeChat", m.mode)
	}
	if m.launch.IsArmed() {
		t.Fatalf("launch should be empty: %+v", m.launch)
	}
}

func TestNew_RestoresCatalogWorkflowID(t *testing.T) {
	isolateSessionFile(t)
	if _, err := prefs.Save(prefs.Session{
		Mode:       "flow",
		WorkflowID: "wf-uuid-1",
		FlowLabel:  "Custom Flow",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	if m.mode != ModeFlow || m.launch.WorkflowID != "wf-uuid-1" {
		t.Fatalf("mode=%v launch=%+v", m.mode, m.launch)
	}
	if m.firstTurnPending {
		t.Fatal("catalog restore must not set firstTurnPending")
	}
}

func TestSlashFlow_PersistsModeAndFlow(t *testing.T) {
	path := isolateSessionFile(t)
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.flowBuiltins = []client.BuiltinFlowOption{
		{FlowRef: "pack/fix-bug", Label: "Fix Bug"},
	}
	m2, _ := m.handleSlashCommand("/flow pack/fix-bug")
	am := m2.(*AppModel)
	if am.mode != ModeFlow {
		t.Fatalf("mode=%v", am.mode)
	}
	got, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("Load: %v under %s", err, path)
	}
	if got.Mode != "flow" || got.FlowRef != "pack/fix-bug" {
		t.Fatalf("prefs=%+v", got)
	}
	if got.FlowLabel != "Fix Bug" {
		t.Fatalf("FlowLabel=%q", got.FlowLabel)
	}
}

func TestSlashChat_PersistsChatAndClearsFlow(t *testing.T) {
	isolateSessionFile(t)
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, FlowRef: "pack/x", Label: "X"}
	m2, _ := m.handleSlashCommand("/chat")
	am := m2.(*AppModel)
	if am.mode != ModeChat || am.launch.IsArmed() {
		t.Fatalf("mode=%v launch=%+v", am.mode, am.launch)
	}
	got, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Mode != "chat" || got.FlowRef != "" || got.WorkflowID != "" {
		t.Fatalf("prefs=%+v", got)
	}
}

func TestSlashYolo_PersistsChatPreference(t *testing.T) {
	isolateSessionFile(t)
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	if m.yolo {
		t.Fatal("start yolo false")
	}
	m2, _ := m.handleSlashCommand("/yolo")
	am := m2.(*AppModel)
	if !am.yolo {
		t.Fatal("yolo should be on")
	}
	got, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Yolo == nil || !*got.Yolo {
		t.Fatalf("prefs yolo=%v want true", got.Yolo)
	}
	// Restart restores chat YOLO (no --yolo flag).
	m3 := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	if !m3.yolo {
		t.Fatal("New should restore yolo=true from prefs")
	}
	// Toggle off and persist.
	m4, _ := m3.handleSlashCommand("/yolo")
	if m4.(*AppModel).yolo {
		t.Fatal("yolo should be off")
	}
	got, _, err = prefs.Load()
	if err != nil {
		t.Fatalf("Load off: %v", err)
	}
	if got.Yolo == nil || *got.Yolo {
		t.Fatalf("prefs yolo=%v want false", got.Yolo)
	}
}

func TestNew_YoloFlagOverridesPrefs(t *testing.T) {
	isolateSessionFile(t)
	off := false
	if _, err := prefs.Save(prefs.Session{Provider: "codex", Mode: "chat", Yolo: &off}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	m := New(config.ChatConfig{Provider: "codex", Yolo: true}, "http://127.0.0.1:4317")
	if !m.yolo {
		t.Fatal("--yolo flag must win over prefs false")
	}
}

func TestPersist_FlowModeKeepsChatYoloPreference(t *testing.T) {
	isolateSessionFile(t)
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.yolo = false
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf-1", Label: "grok-flow"}
	m.persistSessionPrefs()
	got, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Flow is auto-on at runtime; disk keeps chat preference (false), not forced true.
	if got.Yolo == nil || *got.Yolo {
		t.Fatalf("flow persist must not force yolo true on disk: %+v", got.Yolo)
	}
	if got.Mode != "flow" || got.WorkflowID != "wf-1" {
		t.Fatalf("prefs=%+v", got)
	}
}

func TestRefineLaunchFromCatalog_UpgradesPartialArm(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, FlowRef: "pack/fix-bug", Label: "pack/fix-bug"}
	m.flowBuiltins = []client.BuiltinFlowOption{
		{FlowRef: "pack/fix-bug", Label: "Fix Bug"},
	}
	m.refineLaunchFromCatalog()
	if m.launch.Label != "Fix Bug" {
		t.Fatalf("Label=%q want Fix Bug", m.launch.Label)
	}
	if m.launch.SubMode != "bug" {
		t.Fatalf("SubMode=%q", m.launch.SubMode)
	}
}

func TestFlowListMsg_SilentRefinesRestoredArm(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf-1", Label: "wf-1"}
	next, _ := m.Update(FlowListMsg{
		Silent: true,
		Workflows: []client.Workflow{
			{ID: "wf-1", Name: "My Catalog Flow", ProjectID: ""},
		},
	})
	am := next.(*AppModel)
	if am.launch.Label != "My Catalog Flow" {
		t.Fatalf("Label=%q after FlowListMsg", am.launch.Label)
	}
	if am.launch.WorkflowID != "wf-1" {
		t.Fatalf("WorkflowID=%q", am.launch.WorkflowID)
	}
}

// Ensure tea.Msg path still type-checks for Update.
var _ tea.Msg = FlowListMsg{}
