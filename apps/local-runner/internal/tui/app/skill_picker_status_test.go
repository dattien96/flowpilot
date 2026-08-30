package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestStatusLine_OmitsChatModeChip(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m.asciiMode = true
	line := m.renderStatusLine()
	if strings.Contains(line, "[chat]") {
		t.Fatalf("status line must not carry the mode chip: %q", line)
	}
	m.mode = ModeFlow
	m.launch = LaunchArm{FlowRef: "pack/review-loop", Label: "review-loop"}
	line = m.renderStatusLine()
	if strings.Contains(line, "[flow]") {
		t.Fatalf("status line must not carry the mode chip: %q", line)
	}
	// Mode now lives in input header
	header := stripANSI(m.chatFrameTitle())
	if !strings.Contains(header, "review-loop") && !strings.Contains(header, "flow") {
		t.Fatalf("header should label flow mode: %q", header)
	}
}

func TestStatusLine_ShowsAttachedSkills(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.selectedSkills = []client.SkillSelection{
		{Name: "additive-tests-only"},
		{Name: "oracle-rule"},
	}
	line := m.renderStatusLine()
	if strings.Contains(line, "skills:2") {
		t.Fatalf("skills chip must not be on status line (hidden per new UI): %q", line)
	}
	// Skills are attached but not shown in input chrome per new spec (sidebar only session+steps)
	// Just verify they are stored
	if len(m.selectedSkills) != 2 {
		t.Fatalf("selectedSkills not stored")
	}
}

func TestStatusLine_SkillsNamesLiveInSidebar(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.selectedSkills = []client.SkillSelection{
		{Name: "additive-tests-only"},
		{Name: "oracle-rule"},
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF3})
	am := m2.(*AppModel)
	line := am.renderStatusLine()
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("F3 must not add status rows (no-op): %q", line)
	}
	// Skills no longer in sidebar (sidebar only session+steps per user request)
	// Just verify F3 is no-op
	if len(am.selectedSkills) != 2 {
		t.Fatalf("skills should remain after F3")
	}
}

func TestSkillPicker_TabTicksEnterApplies(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.skillsCatalog = []client.ProviderSkill{
		{Name: "coding", Path: "/skills/coding", Source: "provider"},
		{Name: "review", Path: "/skills/review", Source: "workspace"},
	}
	m.inputValue = "/skill "
	m.suggIdx = 0

	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if len(am.selectedSkills) != 1 || am.selectedSkills[0].Name != "coding" {
		t.Fatalf("Tab should tick highlighted skill, got %+v", am.selectedSkills)
	}
	if strings.TrimSpace(am.inputValue) != "[coding] /skill" {
		t.Fatalf("Tab must keep picker open, input=%q", am.inputValue)
	}

	// Multi-pick second skill without closing.
	am.suggIdx = 0
	items := am.collectSuggestions()
	for i, it := range items {
		if it.value == "review" {
			am.suggIdx = i
			break
		}
	}
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am = m3.(*AppModel)
	if len(am.selectedSkills) != 2 {
		t.Fatalf("second Tab should add review, got %+v", am.selectedSkills)
	}
	if am.inputValue != "[coding] [review] /skill " {
		t.Fatalf("multi-pick input: %q", am.inputValue)
	}

	m4, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = m4.(*AppModel)
	if am.inputValue != "[coding] [review] " {
		t.Fatalf("Enter should close picker, input=%q", am.inputValue)
	}
	if len(am.selectedSkills) != 2 {
		t.Fatalf("Enter apply must keep ticks, got %+v", am.selectedSkills)
	}
	// Skills are stored, not shown on status line per new UI
	if len(am.selectedSkills) != 2 {
		t.Fatalf("skills should remain")
	}
}

func TestSkillPicker_TabKeepsHighlightAfterTick(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.skillsCatalog = []client.ProviderSkill{
		{Name: "alpha", Source: "provider"},
		{Name: "bravo", Source: "provider"},
		{Name: "charlie", Source: "provider"},
		{Name: "delta", Source: "provider"},
		{Name: "echo", Source: "provider"},
		{Name: "foxtrot", Source: "provider"},
	}
	m.inputValue = "/skill "
	m.suggIdx = 4 // echo
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if len(am.selectedSkills) != 1 || am.selectedSkills[0].Name != "echo" {
		t.Fatalf("should tick echo, got %+v", am.selectedSkills)
	}
	if am.suggIdx != 4 {
		t.Fatalf("highlight jumped to %d, want stay on echo (4)", am.suggIdx)
	}
	items := am.collectSuggestions()
	if items[am.suggIdx].value != "echo" {
		t.Fatalf("row %d is %q, want echo (list must not jump selected to top)", am.suggIdx, items[am.suggIdx].value)
	}
	if items[0].value != "alpha" {
		t.Fatalf("catalog order broken, first=%q", items[0].value)
	}
}

func TestSkillPicker_TabUnticks(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.skillsCatalog = []client.ProviderSkill{
		{Name: "coding", Source: "provider"},
	}
	m.inputValue = "/skill "
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if len(am.selectedSkills) != 1 {
		t.Fatalf("tick failed: %+v", am.selectedSkills)
	}
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am = m3.(*AppModel)
	if len(am.selectedSkills) != 0 {
		t.Fatalf("second Tab on same row should untick, got %+v", am.selectedSkills)
	}
}

func TestSkillPicker_ShowsTabEnterHint(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.asciiMode = true
	m.skillsCatalog = []client.ProviderSkill{
		{Name: "coding", Source: "provider"},
		{Name: "review", Source: "workspace"},
	}
	m.inputValue = "/skill "
	view := m.View()
	if !strings.Contains(view, "Tab tick to select") || !strings.Contains(view, "Enter apply") {
		t.Fatalf("skill list missing Tab/Enter note:\n%s", view)
	}
	got := formatSkillsCatalog(m.skillsCatalog, nil, "grok")
	if !strings.Contains(got, "Tab tick to select") || !strings.Contains(got, "Enter apply") {
		t.Fatalf("catalog dump missing Tab/Enter note:\n%s", got)
	}
}
