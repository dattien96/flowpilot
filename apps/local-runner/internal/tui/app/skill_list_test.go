package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestFormatSkillsCatalog_DesktopParity(t *testing.T) {
	catalog := []client.ProviderSkill{
		{Name: "coding", Source: "provider", Description: "Write code carefully"},
		{Name: "review", Source: "workspace", Description: "Review diffs"},
	}
	selected := []client.SkillSelection{{Name: "coding", Source: "provider"}}
	got := formatSkillsCatalog(catalog, selected, "codex")
	if !strings.Contains(got, "1/2 selected") {
		t.Fatalf("header: %q", got)
	}
	if !strings.Contains(got, "[*] /coding") || !strings.Contains(got, "Account") {
		t.Fatalf("selected row missing: %q", got)
	}
	if !strings.Contains(got, "[ ] /review") || !strings.Contains(got, "Project") {
		t.Fatalf("catalog row missing: %q", got)
	}
}

func TestSkillSlash_ListsCatalogLikeDesktop(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = "codex"
	m.skillsCatalog = []client.ProviderSkill{
		{Name: "coding", Source: "provider", Description: "code"},
		{Name: "docs", Source: "flowpilot", Description: "docs"},
	}
	m.selectedSkills = []client.SkillSelection{{Name: "docs", Source: "flowpilot"}}
	m2, cmd := m.handleSlashCommand("/skill")
	am := m2.(*AppModel)
	view := am.View()
	if !strings.Contains(view, "/coding") || !strings.Contains(view, "/docs") {
		t.Fatalf("list missing skills:\n%s", view)
	}
	if !strings.Contains(view, "[*] /docs") {
		t.Fatalf("selected mark missing:\n%s", view)
	}
	// Catalog already warm — no forced reload show.
	_ = cmd
}

func TestFilterSkillSuggestions(t *testing.T) {
	catalog := []client.ProviderSkill{
		{Name: "coding", Source: "provider"},
		{Name: "review", Source: "workspace"},
	}
	items := filterSkillSuggestions("/skill co", catalog, nil)
	if len(items) != 1 || items[0].value != "coding" {
		t.Fatalf("items=%+v", items)
	}
}

func TestToggleSkill_UsesCatalogSourcePath(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.skillsCatalog = []client.ProviderSkill{
		{Name: "coding", Path: "/skills/coding", Source: "provider"},
	}
	m.toggleSkillByName("coding")
	if len(m.selectedSkills) != 1 || m.selectedSkills[0].Path != "/skills/coding" {
		t.Fatalf("selected=%+v", m.selectedSkills)
	}
	if m.selectedSkills[0].Source != "provider" {
		t.Fatalf("source=%q", m.selectedSkills[0].Source)
	}
}

func TestSkillsListMsg_ShowsOnRequest(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(SkillsListMsg{
		Show: true,
		Skills: []client.ProviderSkill{
			{Name: "alpha", Source: "provider"},
		},
	})
	am := m2.(*AppModel)
	if len(am.skillsCatalog) != 1 {
		t.Fatalf("catalog=%d", len(am.skillsCatalog))
	}
	if !strings.Contains(am.View(), "/alpha") {
		t.Fatalf("view missing skill:\n%s", am.View())
	}
}
