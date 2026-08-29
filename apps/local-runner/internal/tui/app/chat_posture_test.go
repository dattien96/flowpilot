package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Chat posture TUI (Task-xxx/CA-xxx): /mode, /mode-setup, and the Tab cycle
// route through the runner's GET/PUT /client/chat-posture and apply the active
// posture's profile to the session.

func chatPostureTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"code","profiles":{"scan":{"provider":"claude","model":"sonnet"},"plan":{},"code":{}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSlashMode_NoArgsCyclesPosture(t *testing.T) {
	srv := chatPostureTestServer(t)
	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"

	m2, cmd := m.handleSlashCommand("/mode")
	am := m2.(*AppModel)
	// CA-685: the /mode no-args cycle is plan → code → non → plan, so from
	// code the next posture is now "non" (was "plan" before the non posture
	// joined the cycle).
	if am.chatPosturePending != "apply:non" {
		t.Fatalf("pending = %q, want apply:non (code→non)", am.chatPosturePending)
	}
	if cmd == nil {
		t.Fatal("expected a load-chat-posture cmd")
	}
	// Executing the cmd must fetch the runner doc and deliver a chatPostureMsg.
	msg := cmd()
	if msg == nil {
		t.Fatal("expected chatPostureMsg")
	}
	cp := msg.(chatPostureMsg)
	if cp.Err != nil {
		t.Fatalf("load chat posture: %v", cp.Err)
	}
	if cp.Cfg.Active != "code" {
		t.Fatalf("loaded active = %q", cp.Cfg.Active)
	}
}

func TestSlashMode_NamedPostureAppliesProfile(t *testing.T) {
	srv := chatPostureTestServer(t)
	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	m.providers = []client.Provider{{
		Key: "claude",
		Models: []client.ProviderModel{
			{ID: "sonnet"},
			{ID: "opus"},
		},
	}}
	m.chatPosture = "code"

	m2, cmd := m.handleSlashCommand("/mode scan")
	am := m2.(*AppModel)
	if am.chatPosturePending != "apply:scan" {
		t.Fatalf("pending = %q, want apply:scan", am.chatPosturePending)
	}
	if cmd == nil {
		t.Fatal("expected a load-chat-posture cmd")
	}
	msg := cmd()
	cp := msg.(chatPostureMsg)
	if cp.Err != nil {
		t.Fatalf("load chat posture: %v", cp.Err)
	}
	// Apply the loaded profile (scan pins provider=claude, model=sonnet).
	am.chatPostureCfg = cp.Cfg
	am.chatPostureCmdFromPending(cp.Cfg)
	if am.chatPosture != "scan" {
		t.Fatalf("posture after apply = %q, want scan", am.chatPosture)
	}
	if am.provider != "claude" {
		t.Fatalf("provider after apply = %q, want claude", am.provider)
	}
	if am.model != "sonnet" {
		t.Fatalf("model after apply = %q, want sonnet", am.model)
	}
	// /mode apply must persist the new active posture back to the runner (SSOT)
	// so the Desktop and a later /new agree — mirrors the Desktop tab switch.
	if !am.chatPostureDirty {
		t.Fatal("apply must mark dirty (pending PUT of active)")
	}
	if am.chatPostureCfg.Active != "scan" {
		t.Fatalf("cfg active after apply = %q, want scan", am.chatPostureCfg.Active)
	}
}

func TestSlashMode_InvalidNameRejected(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m2, cmd := m.handleSlashCommand("/mode nonsense")
	am := m2.(*AppModel)
	if am.chatPosturePending != "" {
		t.Fatalf("pending = %q, want empty for invalid name", am.chatPosturePending)
	}
	if cmd != nil {
		t.Fatal("invalid /mode name must not dispatch a load cmd")
	}
}

func TestSlashMode_NotChatModeRejected(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m2, cmd := m.handleSlashCommand("/mode")
	am := m2.(*AppModel)
	if am.chatPosturePending != "" {
		t.Fatalf("pending = %q, want empty outside chat mode", am.chatPosturePending)
	}
	if cmd != nil {
		t.Fatal("/mode outside chat must not dispatch")
	}
}

func TestSlashModeSetup_EditProfileSaves(t *testing.T) {
	srv := chatPostureTestServer(t)
	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat

	m2, cmd := m.handleSlashCommand("/mode-setup plan provider claude")
	am := m2.(*AppModel)
	if am.chatPosturePending != "setup:plan:provider:claude" {
		t.Fatalf("pending = %q, want setup:plan:provider:claude", am.chatPosturePending)
	}
	if cmd == nil {
		t.Fatal("expected a load-chat-posture cmd")
	}
	msg := cmd()
	cp := msg.(chatPostureMsg)
	if cp.Err != nil {
		t.Fatalf("load chat posture: %v", cp.Err)
	}
	am.chatPostureCfg = cp.Cfg
	am.chatPostureCmdFromPending(cp.Cfg)
	if am.chatPostureDirty != true {
		t.Fatal("profile edit must mark dirty (pending save)")
	}
	if am.chatPostureCfg.Profiles["plan"].Provider != "claude" {
		t.Fatalf("plan provider = %q, want claude", am.chatPostureCfg.Profiles["plan"].Provider)
	}
}

func TestSlashModeSetup_ShowListsPostures(t *testing.T) {
	srv := chatPostureTestServer(t)
	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat

	m2, cmd := m.handleSlashCommand("/mode-setup")
	am := m2.(*AppModel)
	if am.chatPosturePending != "modal:" {
		t.Fatalf("pending = %q, want modal:", am.chatPosturePending)
	}
	if cmd == nil {
		t.Fatal("expected a load-chat-posture cmd")
	}
	msg := cmd()
	cp := msg.(chatPostureMsg)
	if cp.Err != nil {
		t.Fatalf("load chat posture: %v", cp.Err)
	}
	am.chatPostureCmdFromPending(cp.Cfg)
	if !am.modeSetupModalOpen {
		t.Fatalf("bare /mode-setup must open modal, got view:\n%s", am.View())
	}
}

func TestTabCycleOnEmptyInputAdvancesPosture(t *testing.T) {
	srv := chatPostureTestServer(t)
	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	m.sessionDefaultsLoaded = true
	m.chatPosture = "code"
	m.inputValue = ""

	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	// With no agent runs and no suggestions, Tab on empty input cycles posture.
	// CA-685: the cycle is plan → code → non → plan, so from code the next
	// posture is "non".
	if am.chatPosturePending != "apply:non" {
		t.Fatalf("pending after Tab = %q, want apply:non (code→non)", am.chatPosturePending)
	}
	if cmd == nil {
		t.Fatal("Tab cycle must dispatch a load cmd")
	}
}

func TestTabCycleFromScanGoesToPlan(t *testing.T) {
	srv := chatPostureTestServer(t)
	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	m.sessionDefaultsLoaded = true
	m.chatPosture = "scan"
	m.inputValue = ""

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	// Scan is not in tabPostureOrder; Tab should go to plan (not cycle back
	// to scan, which would be a no-op).
	if am.chatPosturePending != "apply:plan" {
		t.Fatalf("pending after Tab from scan = %q, want apply:plan", am.chatPosturePending)
	}
}

func TestStatuslineShowsPostureChip(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m.chatPosture = "scan"
	enableSidebarForTest(m)
	m.width, m.fullWidth, m.height = tuiSidebarMinWidth+10, tuiSidebarMinWidth+10, 30
	side := strings.Join(m.renderSidebarStatusSection(80), "\n")
	if !strings.Contains(side, "mode:") || !strings.Contains(side, "scan") {
		t.Fatalf("sidebar missing posture chip:\n%s", side)
	}
}

func TestNewChatReloadsPostureProfile(t *testing.T) {
	srv := chatPostureTestServer(t)
	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	m.sessionDefaultsLoaded = true
	m.chatPosture = "scan"
	m.provider = "codex"
	m.model = "o3"

	// /new should reload posture and apply the scan profile's pins.
	m2, cmd := m.handleSlashCommand("/new")
	am := m2.(*AppModel)
	if am.chatPosturePending != "apply:scan" {
		t.Fatalf("pending after /new = %q, want apply:scan", am.chatPosturePending)
	}
	if cmd == nil {
		t.Fatal("/new must dispatch a load cmd")
	}
	msg := cmd()
	cp := msg.(chatPostureMsg)
	if cp.Err != nil {
		t.Fatalf("load chat posture: %v", cp.Err)
	}
	am.chatPostureCfg = cp.Cfg
	am.chatPostureCmdFromPending(cp.Cfg)
	// The scan profile pins provider=claude, model=sonnet — /new must apply
	// them so the next turn uses the pinned selection.
	if am.provider != "claude" {
		t.Fatalf("provider after /new = %q, want claude (scan pin)", am.provider)
	}
	if am.model != "sonnet" {
		t.Fatalf("model after /new = %q, want sonnet (scan pin)", am.model)
	}
	// Active must be persisted back to runner so Desktop agrees.
	if !am.chatPostureDirty {
		t.Fatal("/new must mark dirty (pending PUT of active)")
	}
	if am.chatPostureCfg.Active != "scan" {
		t.Fatalf("cfg active after /new = %q, want scan", am.chatPostureCfg.Active)
	}
}

func TestGrokSyncFailedMsg_ReenablesFlag(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.postureGrokSyncSet = false
	m.postureGrokSync = false

	m2, cmd := m.Update(grokSyncFailedMsg{Yolo: true})
	am := m2.(*AppModel)
	if !am.postureGrokSyncSet {
		t.Fatal("grokSyncFailedMsg must re-enable postureGrokSyncSet")
	}
	if !am.postureGrokSync {
		t.Fatal("grokSyncFailedMsg must restore yolo value")
	}
	if cmd != nil {
		t.Fatal("grokSyncFailedMsg should not dispatch further cmds")
	}
}

func TestModeSetupPicker_PostureFieldValue(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{{Key: "claude"}, {Key: "codex"}}
	m.provider = "claude"
	// TAB picker for /mode-setup is now disabled (modal on Enter)
	m.inputValue = "/mode-setup "
	m.inputCursor = -1
	items := m.collectSuggestions()
	for _, it := range items {
		if it.kind == "mode-setup-posture" || it.kind == "mode-setup-field" || it.kind == "mode-setup-value" {
			t.Fatalf("TAB picker for /mode-setup must be disabled, got %+v", items)
		}
	}
	// Underlying filter still works for typed power-user path
	filtered := filterModeSetupSuggestions("/mode-setup ", m.providers, m.providerAccounts, m.provider, m.model, m.modeSetupDraft)
	found := map[string]bool{}
	for _, it := range filtered {
		found[it.value] = true
	}
	if !found["scan"] || !found["plan"] || !found["code"] {
		t.Fatalf("filterModeSetupSuggestions must still show scan/plan/code, got %+v", filtered)
	}
	m.inputValue = "/mode-setup scan "
	filtered = filterModeSetupSuggestions(m.inputValue, m.providers, m.providerAccounts, m.provider, m.model, m.modeSetupDraft)
	found = map[string]bool{}
	for _, it := range filtered {
		found[it.value] = true
	}
	if found["scan provider"] {
		t.Fatalf("field picker must not show provider (removed), got %+v", filtered)
	}
	if !found["scan model"] {
		t.Fatalf("field picker must show model, got %+v", filtered)
	}
	// Typing provider still works via value stage (not advertised in picker)
	m.inputValue = "/mode-setup scan provider "
	filtered = filterModeSetupSuggestions(m.inputValue, m.providers, m.providerAccounts, m.provider, m.model, m.modeSetupDraft)
	if len(filtered) == 0 {
		t.Fatal("provider value picker must show providers via filter")
	}
	hasClaude := false
	for _, it := range filtered {
		if it.value == "scan provider claude" {
			hasClaude = true
		}
	}
	if !hasClaude {
		t.Fatalf("provider picker must contain scan provider claude, got %+v", filtered)
	}
	// clear has no value stage
	m.inputValue = "/mode-setup scan clear"
	filtered = filterModeSetupSuggestions(m.inputValue, m.providers, m.providerAccounts, m.provider, m.model, m.modeSetupDraft)
	for _, it := range filtered {
		if it.kind == "mode-setup-value" {
			t.Fatalf("clear must not show value picker, got %+v", filtered)
		}
	}
}

func TestTabBeforeSessionDefaultsDoesNotCyclePosture(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m.chatPosture = "code"
	m.inputValue = ""
	// Before SessionDefaultsMsg, sessionDefaultsLoaded is false — Tab must not
	// trigger a posture GET (the treo report — chat just appeared, F2 lúc được lúc không).
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if am.chatPosturePending != "" {
		t.Fatalf("Tab before session defaults must not set posture pending, got %q", am.chatPosturePending)
	}
	if cmd != nil {
		t.Fatalf("Tab before session defaults must not dispatch a posture cmd")
	}
}

func TestTypingAfterSessionDefaultsGoesToInput(t *testing.T) {
	m := New(config.ChatConfig{ProjectPath: t.TempDir()}, "http://127.0.0.1:4317")
	m.sessionLoading = true
	m.sessionDefaultsLoaded = false
	next, _ := m.Update(SessionDefaultsMsg{
		Provider: "grok",
		Model:    "grok-4.5",
		Projects: []client.Project{{ID: "p1", Name: "proj", Path: m.cfg.ProjectPath}},
		Project:  &client.Project{ID: "p1", Name: "proj", Path: m.cfg.ProjectPath},
	})
	am := next.(*AppModel)
	if am.sessionLoading {
		t.Fatal("SessionDefaultsMsg must clear sessionLoading")
	}
	if !am.sessionDefaultsLoaded {
		t.Fatal("SessionDefaultsMsg must set sessionDefaultsLoaded")
	}
	// After the chat input is visible (spinner gone), a KeyRunes must go to
	// inputValue immediately — not swallowed even while history/skills fetches
	// are in flight (F2 lúc được lúc không).
	m2, _ := am.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	am2 := m2.(*AppModel)
	if am2.inputValue != "a" {
		t.Fatalf("KeyRunes after session defaults must go to inputValue, got %q", am2.inputValue)
	}
}