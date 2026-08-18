package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// SkillsListMsg carries GET /client/provider-skills results (Desktop ChatInput skill picker).
type SkillsListMsg struct {
	Skills []client.ProviderSkill
	Err    string
	Show   bool // dump list into chat after load
}

func skillSourceLabel(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "provider":
		return "Account"
	case "flowpilot", "workspace", "project":
		return "Project"
	case "user", "slash_picker":
		return "Selected"
	default:
		if strings.TrimSpace(source) == "" {
			return "—"
		}
		return source
	}
}

func selectedSkillSet(selected []client.SkillSelection) map[string]bool {
	out := make(map[string]bool, len(selected))
	for _, s := range selected {
		out[strings.ToLower(strings.TrimSpace(s.Name))] = true
	}
	return out
}

func findSkillInCatalog(catalog []client.ProviderSkill, name string) *client.ProviderSkill {
	want := strings.ToLower(strings.TrimSpace(name))
	for i := range catalog {
		if strings.ToLower(catalog[i].Name) == want {
			return &catalog[i]
		}
	}
	return nil
}

// formatSkillsCatalog mirrors Desktop skill picker rows: mark · /name · source · desc.
func formatSkillsCatalog(catalog []client.ProviderSkill, selected []client.SkillSelection, provider string) string {
	sel := selectedSkillSet(selected)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Skills · %d/%d selected", len(selected), len(catalog)))
	if p := strings.TrimSpace(provider); p != "" {
		sb.WriteString(" · provider " + p)
	}
	sb.WriteString("\n")
	sb.WriteString("Tab tick to select · multi-pick · Enter apply closes picker\n")

	seen := make(map[string]bool, len(catalog)+len(selected))
	// Selected-first (Desktop pickerSortSelection parity).
	for _, s := range selected {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		seen[key] = true
		src := skillSourceLabel(s.Source)
		desc := ""
		if sk := findSkillInCatalog(catalog, name); sk != nil {
			desc = strings.TrimSpace(sk.Description)
			src = skillSourceLabel(sk.Source)
			name = sk.Name
		}
		line := fmt.Sprintf("  [*] /%-24s %-8s", name, src)
		if desc != "" {
			line += "  " + truncateRunes(desc, 48)
		}
		sb.WriteString(line + "\n")
	}
	for _, sk := range catalog {
		key := strings.ToLower(strings.TrimSpace(sk.Name))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		mark := "[ ]"
		if sel[key] {
			mark = "[*]"
		}
		line := fmt.Sprintf("  %s /%-24s %-8s", mark, sk.Name, skillSourceLabel(sk.Source))
		if d := strings.TrimSpace(sk.Description); d != "" {
			line += "  " + truncateRunes(d, 48)
		}
		sb.WriteString(line + "\n")
	}
	if len(catalog) == 0 && len(selected) == 0 {
		sb.WriteString("  (no skills yet — wait for provider catalog, or check Desktop Skills)\n")
	} else if len(catalog) == 0 {
		sb.WriteString("  (catalog empty/loading — selected skills kept above)\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if max < 1 || len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func filterSkillSuggestions(input string, catalog []client.ProviderSkill, selected []client.SkillSelection) []suggestItem {
	ok, query := parseSlashArgPrefix(input, "/skill")
	if !ok {
		ok, query = parseSlashArgPrefix(input, "/s")
	}
	if !ok {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	sel := selectedSkillSet(selected)
	var out []suggestItem
	seen := make(map[string]bool)
	// Catalog order stays stable so Tab-tick does not jump the highlight to top.
	for _, sk := range catalog {
		name := strings.TrimSpace(sk.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		seen[key] = true
		if q != "" && !strings.Contains(key, q) && !strings.Contains(strings.ToLower(sk.Description), q) {
			continue
		}
		detail := "[ ] " + skillSourceLabel(sk.Source)
		if sel[key] {
			detail = "[*] " + skillSourceLabel(sk.Source)
		}
		if d := strings.TrimSpace(sk.Description); d != "" && !sel[key] {
			detail += " · " + truncateRunes(d, 32)
		}
		out = append(out, suggestItem{value: name, detail: detail, kind: "skill"})
	}
	for _, s := range selected {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(name), q) {
			continue
		}
		out = append(out, suggestItem{
			value:  name,
			detail: "[*] " + skillSourceLabel(s.Source),
			kind:   "skill",
		})
	}
	return out
}

func (m *AppModel) cmdLoadSkills(show bool) tea.Cmd {
	provider := strings.TrimSpace(m.provider)
	cwd := strings.TrimSpace(m.cfg.ProjectPath)
	if m.project != nil && strings.TrimSpace(m.project.Path) != "" {
		cwd = m.project.Path
	}
	if m.projectPath != "" {
		cwd = m.projectPath
	}
	cl := m.client
	return func() tea.Msg {
		// Bound so a slow/hung runner cannot freeze a slash completion fetch.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		skills, err := cl.ListSkills(ctx, provider, cwd)
		if err != nil {
			return SkillsListMsg{Err: err.Error(), Show: show}
		}
		return SkillsListMsg{Skills: skills, Show: show}
	}
}

func (m *AppModel) cmdMaybePrefetchSkills() tea.Cmd {
	line := m.slashSuggestLine()
	okSkill, _ := parseSlashArgPrefix(line, "/skill")
	okS, _ := parseSlashArgPrefix(line, "/s")
	bare := strings.EqualFold(strings.TrimSpace(line), "/skill") ||
		strings.EqualFold(strings.TrimSpace(line), "/s")
	if !okSkill && !okS && !bare {
		return nil
	}
	if len(m.skillsCatalog) > 0 {
		return nil
	}
	return m.cmdLoadSkills(false)
}

func (m *AppModel) toggleSkillByName(name string) {
	added, selName, src := m.toggleSkillByNameQuiet(name)
	if selName == "" {
		return
	}
	if added {
		m.addMessage("system", fmt.Sprintf("Skill added: %s (%s)", selName, skillSourceLabel(src)), "")
		return
	}
	m.addMessage("system", fmt.Sprintf("Skill removed: %s", selName), "")
}

// toggleSkillByNameQuiet ticks/unticks without dumping a chat line (picker Tab).
// Tab inserts/removes "[skill-name]" in the draft but keeps /skill open so the
// operator can multi-select. Enter (or Esc clear) closes the picker.
// Chip + SelectedSkills still drive injectSelectedSkills.
func (m *AppModel) toggleSkillByNameQuiet(name string) (added bool, selName, source string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, "", ""
	}
	for i, s := range m.selectedSkills {
		if strings.EqualFold(s.Name, name) {
			m.selectedSkills = append(m.selectedSkills[:i], m.selectedSkills[i+1:]...)
			m.inputValue = removeSkillPromptToken(m.inputValue, s.Name)
			m.inputCursor = -1
			return false, s.Name, s.Source
		}
	}
	sel := client.SkillSelection{Name: name, Source: "user"}
	if sk := findSkillInCatalog(m.skillsCatalog, name); sk != nil {
		sel.Name = sk.Name
		sel.Path = sk.Path
		sel.Source = sk.Source
		if sel.Source == "" {
			sel.Source = "slash_picker"
		}
	}
	m.selectedSkills = append(m.selectedSkills, sel)
	m.inputValue = insertSkillPromptToken(m.inputValue, m.inputCaretIndex(), sel.Name)
	m.inputCursor = -1
	return true, sel.Name, sel.Source
}

// skillPromptToken is the in-prompt mention form: [skill-name].
func skillPromptToken(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	return "[" + n + "]"
}

// styleInputBodyWithSkillTokens accents attached skill mentions like [coding]
// inside the chat input. Plain text stays styleInputFocus; tokens use styleStatusHi.
// selected names only — random [brackets] in the draft are not highlighted.
func styleInputBodyWithSkillTokens(plain string, selected []client.SkillSelection) string {
	return highlightMentions(plain, attachedSkillNames(selected), styleInputFocus)
}

func containsSkillPromptToken(input, token string) bool {
	if token == "" {
		return false
	}
	return strings.Contains(input, token)
}

// insertSkillPromptToken places "[name]" before the active /skill fragment and
// keeps the slash so multi-Tab pick stays open: "abc /skill " → "abc [name] /skill ".
// Bare "/skill " → "[name] /skill " (chip + mention, picker still open).
func insertSkillPromptToken(input string, caret int, name string) string {
	token := skillPromptToken(name)
	if token == "" || containsSkillPromptToken(input, token) {
		return input
	}
	_, start, ok := activeSlashLine(input, caret)
	if !ok {
		return input
	}
	runes := []rune(input)
	before := strings.TrimRight(string(runes[:start]), " \t")
	slash := string(runes[start:])
	// Normalize slash to bare command + space so the skill list stays open
	// even if the user had typed "/skill foo" as a filter query.
	slash = normalizeSkillSlashKeepOpen(slash)
	if before == "" {
		return token + " " + slash
	}
	return before + " " + token + " " + slash
}

// normalizeSkillSlashKeepOpen returns "/skill " or "/s " so filter stays on the
// skill picker after a Tab tick (drops any typed query after the command).
func normalizeSkillSlashKeepOpen(slash string) string {
	s := strings.TrimLeft(slash, " \t")
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "/skill"):
		return "/skill "
	case strings.HasPrefix(lower, "/s") && (len(lower) == 2 || lower[2] == ' ' || lower[2] == '\t'):
		return "/s "
	default:
		if s == "" {
			return "/skill "
		}
		// Keep original if it still looks like a skill slash line.
		return s
	}
}

// removeSkillPromptToken drops "[name]" mentions and collapses extra spaces.
func removeSkillPromptToken(input, name string) string {
	token := skillPromptToken(name)
	if token == "" || !containsSkillPromptToken(input, token) {
		return input
	}
	out := strings.ReplaceAll(input, " "+token+" ", " ")
	out = strings.ReplaceAll(out, " "+token, "")
	out = strings.ReplaceAll(out, token+" ", "")
	out = strings.ReplaceAll(out, token, "")
	for strings.Contains(out, "  ") {
		out = strings.ReplaceAll(out, "  ", " ")
	}
	return out
}

func attachedSkillNames(selected []client.SkillSelection) []string {
	names := make([]string, 0, len(selected))
	for _, s := range selected {
		n := strings.TrimSpace(s.Name)
		if n == "" {
			continue
		}
		names = append(names, n)
	}
	return names
}

func formatAttachedSkillsChip(n int, expanded, ascii bool) string {
	if n <= 0 {
		return ""
	}
	mark := "▸"
	if expanded {
		mark = "▾"
	}
	if ascii {
		mark = ">"
		if expanded {
			mark = "v"
		}
	}
	return fmt.Sprintf("skills:%d %s", n, mark)
}

func formatAttachedSkillsExpanded(names []string, width int) []string {
	if len(names) == 0 {
		return nil
	}
	if width < 12 {
		width = 12
	}
	body := strings.Join(names, ", ")
	wrapped := wrapText(body, width)
	const maxLines = 4
	if len(wrapped) > maxLines {
		wrapped = append(wrapped[:maxLines-1], fmt.Sprintf("… +%d more · F3", len(names)))
	}
	out := make([]string, 0, len(wrapped)+1)
	for _, line := range wrapped {
		out = append(out, "  "+line)
	}
	out = append(out, "  F3 collapse")
	return out
}

func (m *AppModel) retargetSkillSuggestion(name string) {
	items := m.collectSuggestions()
	for i, it := range items {
		if it.kind == "skill" && strings.EqualFold(it.value, name) {
			m.suggIdx = i
			return
		}
	}
}
