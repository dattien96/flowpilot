package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestStripActiveSlashCommand_PreservesDraft(t *testing.T) {
	cases := []struct {
		in, want string
		caret    int
	}{
		{"/skill ", "", -1},
		{"/s coding", "", -1},
		{"hãy dùng skill a và b /skill ", "hãy dùng skill a và b ", -1},
		{"draft prompt /s ", "draft prompt ", -1},
		{"hello /skill review", "hello ", -1},
		{"abc [coding] /skill ", "abc [coding] ", -1},
		{"no slash here", "no slash here", -1},
		{"  /skill ", "", -1},
	}
	for _, tc := range cases {
		caret := tc.caret
		if caret < 0 {
			caret = len([]rune(tc.in))
		}
		got := stripActiveSlashCommand(tc.in, caret)
		if got != tc.want {
			t.Fatalf("strip(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

// Tab multi-pick keeps /skill open; Enter closes → "abc [a] [b] def".
func TestSkillPicker_InsertsBracketNamesThenContinuesDraft(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.skillsCatalog = []client.ProviderSkill{
		{Name: "coding", Path: "/skills/coding", Source: "provider"},
		{Name: "review", Path: "/skills/review", Source: "workspace"},
	}
	m.inputValue = "abc /skill "
	m.inputCursor = -1
	m.suggIdx = 0

	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if am.inputValue != "abc [coding] /skill " {
		t.Fatalf("Tab should insert [coding] and keep /skill, got %q", am.inputValue)
	}
	if len(am.selectedSkills) != 1 {
		t.Fatalf("chip tick: %+v", am.selectedSkills)
	}
	// Picker still open — multi-pick second skill without retyping /skill.
	items := am.collectSuggestions()
	if len(items) == 0 || items[0].kind != "skill" {
		t.Fatalf("picker must stay open after Tab, sugg=%+v", items)
	}
	for i, it := range items {
		if it.value == "review" {
			am.suggIdx = i
			break
		}
	}
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am = m3.(*AppModel)
	if am.inputValue != "abc [coding] [review] /skill " {
		t.Fatalf("second Tab: %q", am.inputValue)
	}
	m4, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = m4.(*AppModel)
	if am.inputValue != "abc [coding] [review] " {
		t.Fatalf("Enter closes picker: %q", am.inputValue)
	}
	am.insertInputAtCursor("def")
	if am.inputValue != "abc [coding] [review] def" {
		t.Fatalf("continued prompt: %q", am.inputValue)
	}
	// Skills now hidden per new UI (sidebar only session+steps, status line empty)
	if len(am.selectedSkills) != 2 {
		t.Fatalf("selectedSkills should remain 2, got %+v", am.selectedSkills)
	}
	in := am.buildTurnInput(am.inputValue)
	if in.Prompt != "abc [coding] [review] def" {
		t.Fatalf("Prompt=%q", in.Prompt)
	}
	if len(in.SelectedSkills) != 2 {
		t.Fatalf("SelectedSkills=%+v", in.SelectedSkills)
	}
}

func TestSkillPicker_UntickRemovesBracketToken(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.skillsCatalog = []client.ProviderSkill{
		{Name: "coding", Path: "/skills/coding", Source: "provider"},
	}
	m.inputValue = "abc /skill "
	m.suggIdx = 0
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if am.inputValue != "abc [coding] /skill " {
		t.Fatalf("expected token+slash: %q", am.inputValue)
	}
	// Same open picker — Tab again unticks.
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am = m3.(*AppModel)
	if strings.Contains(am.inputValue, "[coding]") {
		t.Fatalf("untick should remove token: %q", am.inputValue)
	}
	if len(am.selectedSkills) != 0 {
		t.Fatalf("untick chip: %+v", am.selectedSkills)
	}
	if !strings.Contains(am.inputValue, "/skill") {
		t.Fatalf("picker should stay open after untick: %q", am.inputValue)
	}
	if !strings.HasPrefix(strings.TrimSpace(am.inputValue), "abc") {
		t.Fatalf("draft should remain: %q", am.inputValue)
	}
}

// Bare /skill multi-Tab then Enter: tokens in prompt + chip; slash gone after Enter.
func TestSkillPicker_EnterBareSlashClearsInput(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.skillsCatalog = []client.ProviderSkill{
		{Name: "coding", Path: "/skills/coding", Source: "provider"},
	}
	m.inputValue = "/skill "
	m.suggIdx = 0
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if am.inputValue != "[coding] /skill " {
		t.Fatalf("bare Tab should keep picker: %q", am.inputValue)
	}
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = m3.(*AppModel)
	if am.inputValue != "[coding] " {
		t.Fatalf("Enter closes bare picker: %q", am.inputValue)
	}
	if len(am.selectedSkills) != 1 {
		t.Fatalf("skill should stay attached, got %+v", am.selectedSkills)
	}
}

// Tab on incomplete /skill command must complete the token without wiping draft.
func TestSkillPicker_TabCompletesCommandKeepsDraft(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.skillsCatalog = []client.ProviderSkill{
		{Name: "coding", Path: "/skills/coding", Source: "provider"},
	}
	// Exact user repro: type draft + /skill (no trailing space yet) then Tab.
	// Before CA-491, Tab applied kind=cmd and set inputValue="/skill ", dropping "abc".
	m.inputValue = "abc /skill"
	m.inputCursor = -1
	// Prefer the /skill command row if the list still shows cmd suggestions.
	items := m.collectSuggestions()
	if len(items) == 0 {
		t.Fatal("expected suggestions for abc /skill")
	}
	for i, it := range items {
		if it.kind == "cmd" && it.value == "/skill" {
			m.suggIdx = i
			break
		}
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if !strings.HasPrefix(am.inputValue, "abc ") {
		t.Fatalf("Tab wiped draft: input=%q", am.inputValue)
	}
	if !strings.Contains(am.inputValue, "/skill") {
		t.Fatalf("Tab should complete/open skill token: %q", am.inputValue)
	}

	// Skill row Tab: insert [name], keep /skill for multi-pick.
	am.inputValue = "abc /skill "
	am.inputCursor = -1
	am.suggIdx = 0
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am = m3.(*AppModel)
	if am.inputValue != "abc [coding] /skill " {
		t.Fatalf("skill Tab should leave abc [coding] /skill, got %q", am.inputValue)
	}
	if len(am.selectedSkills) != 1 || am.selectedSkills[0].Name != "coding" {
		t.Fatalf("tick failed: %+v", am.selectedSkills)
	}
	m4, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = m4.(*AppModel)
	if am.inputValue != "abc [coding] " {
		t.Fatalf("Enter closes picker: %q", am.inputValue)
	}
}

func TestReplaceActiveSlashWith(t *testing.T) {
	if got := replaceActiveSlashWith("abc /sk", 7, "/skill "); got != "abc /skill " {
		t.Fatalf("got %q", got)
	}
	if got := replaceActiveSlashWith("/pro", 4, "/provider "); got != "/provider " {
		t.Fatalf("bare got %q", got)
	}
	if got := replaceActiveSlashWith("notes /flow re", 14, "/flow review-loop"); got != "notes /flow review-loop" {
		t.Fatalf("nested got %q", got)
	}
}

// Mid-draft /s alias path (CA-489 word-boundary slash).
func TestSkillPicker_EnterPreservesDraft_SAlias(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.skillsCatalog = []client.ProviderSkill{
		{Name: "oracle-rule", Path: "/skills/oracle", Source: "flowpilot"},
	}
	m.inputValue = "please use oracle-rule /s "
	m.inputCursor = -1
	m.suggIdx = 0
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if am.inputValue != "please use oracle-rule [oracle-rule] /s " {
		t.Fatalf("got %q", am.inputValue)
	}
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = m3.(*AppModel)
	if am.inputValue != "please use oracle-rule [oracle-rule] " {
		t.Fatalf("Enter close: %q", am.inputValue)
	}
	if len(am.selectedSkills) != 1 || am.selectedSkills[0].Name != "oracle-rule" {
		t.Fatalf("selected=%+v", am.selectedSkills)
	}
}
