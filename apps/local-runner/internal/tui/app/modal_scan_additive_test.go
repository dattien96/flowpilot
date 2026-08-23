package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestScanSlashAppears(t *testing.T) {
	sugg := filterSlashSuggestions("/")
	found := false
	for _, sc := range sugg {
		if sc.name == "/scan" {
			found = true
		}
	}
	if !found {
		t.Fatalf("/scan must appear for '/', got %+v", sugg)
	}
	sugg = filterSlashSuggestions("/s")
	found = false
	for _, sc := range sugg {
		if sc.name == "/scan" {
			found = true
		}
	}
	if !found {
		t.Fatalf("/scan must appear for '/s', got %+v", sugg)
	}
	sugg = filterSlashSuggestions("/sc")
	found = false
	for _, sc := range sugg {
		if sc.name == "/scan" {
			found = true
		}
	}
	if !found {
		t.Fatalf("/scan must appear for '/sc', got %+v", sugg)
	}
	sugg = filterSlashSuggestions("/scan")
	found = false
	for _, sc := range sugg {
		if sc.name == "/scan" {
			found = true
		}
	}
	if !found {
		t.Fatalf("/scan must appear for '/scan', got %+v", sugg)
	}
	sugg = filterSlashSuggestions("/com")
	for _, sc := range sugg {
		if sc.name == "/scan" {
			t.Fatalf("/scan must not appear for '/com', got %+v", sugg)
		}
	}
}

func TestScanCommandSwitchesToScanPosture(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m2, cmd := m.handleSlashCommand("/scan")
	am := m2.(*AppModel)
	if am.chatPosturePending != "apply:scan" {
		t.Fatalf("pending = %q, want apply:scan", am.chatPosturePending)
	}
	if cmd == nil {
		t.Fatal("expected load cmd for /scan")
	}
}

func TestScanCommandOnlyInChat(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m2, cmd := m.handleSlashCommand("/scan")
	am := m2.(*AppModel)
	if am.chatPosturePending != "" {
		t.Fatalf("pending must be empty outside chat, got %q", am.chatPosturePending)
	}
	if cmd != nil {
		t.Fatal("must not dispatch load outside chat")
	}
	if len(am.messages) == 0 || !strings.Contains(am.messages[len(am.messages)-1].Content, "chat mode only") {
		t.Fatalf("must show chat-only error, got %+v", am.messages)
	}
}

func TestModePickerShowsPostures(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.inputValue = "/mode "
	m.inputCursor = -1
	items := m.collectSuggestions()
	found := map[string]bool{}
	for _, it := range items {
		if it.kind == "mode" {
			found[it.value] = true
		}
	}
	for _, want := range []string{"scan", "plan", "code"} {
		if !found[want] {
			t.Fatalf("/mode picker must show %q, got %+v", want, items)
		}
	}
	m.inputValue = "/mode s"
	m.inputCursor = -1
	items = m.collectSuggestions()
	found = map[string]bool{}
	for _, it := range items {
		if it.kind == "mode" {
			found[it.value] = true
		}
	}
	if !found["scan"] {
		t.Fatalf("filtered /mode s must show scan, got %+v", items)
	}
	if found["plan"] || found["code"] {
		t.Fatalf("filtered /mode s must not show plan/code, got %+v", items)
	}
}

func TestModePickerTabAccept(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m.inputValue = "/mode "
	m.inputCursor = -1
	items := m.collectSuggestions()
	idx := -1
	for i, it := range items {
		if it.kind == "mode" && it.value == "scan" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("must find scan in mode picker: %+v", items)
	}
	m.suggIdx = idx
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.chatPosturePending != "apply:scan" {
		t.Fatalf("pending after Enter on scan = %q, want apply:scan", am.chatPosturePending)
	}
}

func TestModeSetupBareOpensModal(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m.chatPosture = "plan"
	m.chatPostureCfg = client.ChatPostureConfig{Active: "plan", Profiles: map[string]client.ChatPostureProfile{
		"scan": {}, "plan": {Model: "opus"}, "code": {},
	}}
	m2, cmd := m.handleSlashCommand("/mode-setup")
	am := m2.(*AppModel)
	if am.chatPosturePending != "modal:" {
		t.Fatalf("pending = %q, want modal:", am.chatPosturePending)
	}
	if cmd == nil {
		t.Fatal("must return load cmd")
	}
	cfg := client.ChatPostureConfig{Active: "plan", Profiles: map[string]client.ChatPostureProfile{
		"scan": {}, "plan": {Model: "opus"}, "code": {},
	}}
	am.chatPosturePending = "modal:"
	am.chatPostureCmdFromPending(cfg)
	if !am.modeSetupModalOpen {
		t.Fatal("modal must be open after pending modal")
	}
	if am.modeSetupModalTab != "plan" {
		t.Fatalf("modal tab = %q, want plan (active)", am.modeSetupModalTab)
	}
	if am.modeSetupModalDraft == nil {
		t.Fatal("draft must exist")
	}
}

func TestModeSetupWithPostureOpensModalTab(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m2, _ := m.handleSlashCommand("/mode-setup scan")
	am := m2.(*AppModel)
	if am.chatPosturePending != "modal:scan" {
		t.Fatalf("pending = %q, want modal:scan", am.chatPosturePending)
	}
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{}}
	am.chatPostureCmdFromPending(cfg)
	if !am.modeSetupModalOpen || am.modeSetupModalTab != "scan" {
		t.Fatalf("modal tab = %q, want scan, open=%v", am.modeSetupModalTab, am.modeSetupModalOpen)
	}
}

func TestModalTabsSwitch(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{{Key: "claude"}, {Key: "codex"}}
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{}}
	m.openModeSetupModal(cfg, "scan")
	if m.modeSetupModalTab != "scan" {
		t.Fatalf("tab = %q", m.modeSetupModalTab)
	}
	m2, _ := m.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyRight})
	am := m2.(*AppModel)
	if am.modeSetupModalTab != "plan" {
		t.Fatalf("after Right tab = %q, want plan", am.modeSetupModalTab)
	}
	am2, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if am2.(*AppModel).modeSetupModalTab != "code" {
		t.Fatalf("after 3 tab = %q, want code", am2.(*AppModel).modeSetupModalTab)
	}
	am3, _ := am2.(*AppModel).handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyLeft})
	if am3.(*AppModel).modeSetupModalTab != "plan" {
		t.Fatalf("after Left tab = %q, want plan", am3.(*AppModel).modeSetupModalTab)
	}
	am4, _ := am3.(*AppModel).handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if am4.(*AppModel).modeSetupModalTab != "scan" {
		t.Fatalf("after 1 tab = %q, want scan", am4.(*AppModel).modeSetupModalTab)
	}
}

func TestModalReasoningGatedOnModel(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "claude", Models: []client.ProviderModel{{ID: "opus", SupportedReasoningEfforts: []string{"high", "medium", "low"}}, {ID: "sonnet"}}},
		{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}},
	}
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{"scan": {}}}
	m.openModeSetupModal(cfg, "scan")
	if m.isReasoningEnabled() {
		t.Fatal("reasoning must be disabled when model empty")
	}
	m.modeSetupModalFocus = modalFocusReasoning
	m2, _ := m.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.modeSetupModalPickerOpen {
		t.Fatal("reasoning picker must not open when model empty")
	}
	am.modeSetupModalFocus = modalFocusYolo
	am2, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !am2.(*AppModel).modeSetupModalPickerOpen || am2.(*AppModel).modeSetupModalPickerKind != "yolo" {
		t.Fatalf("yolo picker must open even without model, got %+v", am2)
	}
	am3, _ := am2.(*AppModel).handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEsc})
	am = am3.(*AppModel)
	am.modeSetupModalFocus = modalFocusModel
	am4, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = am4.(*AppModel)
	if !am.modeSetupModalPickerOpen || am.modeSetupModalPickerKind != "model" {
		t.Fatal("model picker must open")
	}
	opts := am.modalPickerOptions()
	idx := -1
	for i, o := range opts {
		if o == "opus" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("opus must be in options: %+v", opts)
	}
	am.modeSetupModalPickerIdx = idx
	am5, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = am5.(*AppModel)
	if am.modeSetupModalDraft.Profiles["scan"].Model != "opus" {
		t.Fatalf("model after pick = %q, want opus", am.modeSetupModalDraft.Profiles["scan"].Model)
	}
	if am.modeSetupModalDraft.Profiles["scan"].Provider != "claude" {
		t.Fatalf("provider inferred = %q, want claude", am.modeSetupModalDraft.Profiles["scan"].Provider)
	}
	if !am.isReasoningEnabled() {
		t.Fatal("reasoning must be enabled after model selected")
	}
	am.modeSetupModalFocus = modalFocusReasoning
	am6, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = am6.(*AppModel)
	if !am.modeSetupModalPickerOpen || am.modeSetupModalPickerKind != "reasoning" {
		t.Fatal("reasoning picker must open after model")
	}
	opts = am.modalPickerOptions()
	foundHigh := false
	for _, o := range opts {
		if o == "high" {
			foundHigh = true
		}
	}
	if !foundHigh {
		t.Fatalf("reasoning options must contain high after opus, got %+v", opts)
	}
}

func TestModalYoloIndependentAcrossTabs(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{}}
	m.openModeSetupModal(cfg, "scan")
	m.modeSetupModalFocus = modalFocusYolo
	m2, _ := m.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	for i, o := range am.modalPickerOptions() {
		if o == "on" {
			am.modeSetupModalPickerIdx = i
			break
		}
	}
	am3, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = am3.(*AppModel)
	if am.modeSetupModalDraft.Profiles["scan"].Yolo == nil || !*am.modeSetupModalDraft.Profiles["scan"].Yolo {
		t.Fatal("scan yolo must be on")
	}
	am4, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	am = am4.(*AppModel)
	if am.modeSetupModalTab != "plan" {
		t.Fatalf("tab = %q, want plan", am.modeSetupModalTab)
	}
	am.modeSetupModalFocus = modalFocusYolo
	am5, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = am5.(*AppModel)
	for i, o := range am.modalPickerOptions() {
		if o == "off" {
			am.modeSetupModalPickerIdx = i
			break
		}
	}
	am6, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = am6.(*AppModel)
	if am.modeSetupModalDraft.Profiles["plan"].Yolo == nil || *am.modeSetupModalDraft.Profiles["plan"].Yolo {
		t.Fatal("plan yolo must be off")
	}
	if am.modeSetupModalDraft.Profiles["scan"].Yolo == nil || !*am.modeSetupModalDraft.Profiles["scan"].Yolo {
		t.Fatal("scan yolo must remain on after plan edit")
	}
	if am.isReasoningEnabled() {
		t.Fatal("plan reasoning must still be disabled without model")
	}
}

func TestModalCrossProviderModelInference(t *testing.T) {
	cases := []struct {
		model    string
		wantProv string
	}{
		{"opus", "claude"},
		{"sonnet", "claude"},
		{"o3", "codex"},
		{"grok-4.5", "grok"},
	}
	for _, tc := range cases {
		m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
		m.providers = []client.Provider{
			{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}, {ID: "sonnet"}}},
			{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}},
			{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.5"}}},
		}
		cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{"scan": {}}}
		m.openModeSetupModal(cfg, "scan")
		m.modeSetupModalFocus = modalFocusModel
		m2, _ := m.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
		am := m2.(*AppModel)
		idx := -1
		for i, o := range am.modalPickerOptions() {
			if o == tc.model {
				idx = i
				break
			}
		}
		if idx < 0 {
			t.Fatalf("model %q not in picker: %+v", tc.model, am.modalPickerOptions())
		}
		am.modeSetupModalPickerIdx = idx
		am3, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
		am = am3.(*AppModel)
		if am.modeSetupModalDraft.Profiles["scan"].Provider != tc.wantProv {
			t.Fatalf("model %q provider = %q, want %q", tc.model, am.modeSetupModalDraft.Profiles["scan"].Provider, tc.wantProv)
		}
	}
}

func TestModalSaveAndCancel(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}}}}
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{"scan": {}}}
	m.openModeSetupModal(cfg, "scan")
	m.modeSetupModalFocus = modalFocusModel
	m2, _ := m.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	for i, o := range am.modalPickerOptions() {
		if o == "opus" {
			am.modeSetupModalPickerIdx = i
			break
		}
	}
	am3, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = am3.(*AppModel)
	am.modeSetupModalFocus = modalFocusSave
	_, cmd := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("save must return PUT cmd")
	}
	if am.modeSetupModalOpen {
		t.Fatal("modal must close after save")
	}
	if !am.chatPostureSaving {
		t.Fatal("must be saving after save")
	}
	m2b := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2b.openModeSetupModal(cfg, "scan")
	m2b.modeSetupModalFocus = modalFocusModel
	m2b2, _ := m2b.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	amB := m2b2.(*AppModel)
	for i, o := range amB.modalPickerOptions() {
		if o == "opus" {
			amB.modeSetupModalPickerIdx = i
			break
		}
	}
	amB3, _ := amB.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	amB = amB3.(*AppModel)
	_, cmd2 := amB.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd2 != nil {
		t.Fatal("cancel must not return cmd")
	}
	if amB.modeSetupModalOpen {
		t.Fatal("modal must close after cancel")
	}
	if amB.chatPostureSaving {
		t.Fatal("must not be saving after cancel")
	}
	if amB.modeSetupModalDraft != nil {
		t.Fatal("draft must be nil after cancel")
	}
}

func TestModalEnterSavesWhenOnSaveButton(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{}}
	m.openModeSetupModal(cfg, "code")
	m.modeSetupModalFocus = modalFocusSave
	_, cmd := m.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on Save must trigger save")
	}
}

func TestModalEscClosesPickerFirstThenModal(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}}}}
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{}}
	m.openModeSetupModal(cfg, "scan")
	m.modeSetupModalFocus = modalFocusModel
	m2, _ := m.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if !am.modeSetupModalPickerOpen {
		t.Fatal("picker must be open")
	}
	am3, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEsc})
	am = am3.(*AppModel)
	if am.modeSetupModalPickerOpen {
		t.Fatal("picker must close on Esc")
	}
	if !am.modeSetupModalOpen {
		t.Fatal("modal must stay open after picker Esc")
	}
	am4, _ := am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEsc})
	am = am4.(*AppModel)
	if am.modeSetupModalOpen {
		t.Fatal("modal must close on second Esc")
	}
}

func TestModalViewShowsTabsAndFocus(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{"scan": {Model: "opus"}}}
	m.openModeSetupModal(cfg, "scan")
	m.width, m.height = 80, 30
	view := m.View()
	for _, want := range []string{"SCAN", "PLAN", "CODE", "Model", "Reasoning", "YOLO", "Save", "Cancel"} {
		if !strings.Contains(strings.ToUpper(view), strings.ToUpper(want)) {
			if !strings.Contains(view, want) {
				t.Fatalf("modal view must contain %q:\n%s", want, view)
			}
		}
	}
}
