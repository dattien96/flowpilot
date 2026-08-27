package app

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
)

var fileMentionRe = regexp.MustCompile(`(?i)(?:[A-Za-z]:[\\/])?(?:[\w.-]+[\\/])+[\w.-]+\.[A-Za-z0-9]+`)

type WorkspaceFilesMsg struct {
	Query string
	Paths []string
	Err   string
}

func activeAtFragment(input string, caret int) (query string, start int, ok bool) {
	runes := []rune(input)
	n := len(runes)
	if caret < 0 {
		caret = 0
	}
	if caret > n {
		caret = n
	}
	for i := caret - 1; i >= 0; i-- {
		if runes[i] == '@' {
			if i == 0 || unicode.IsSpace(runes[i-1]) {
				return string(runes[i+1 : caret]), i, true
			}
			return "", 0, false
		}
		if unicode.IsSpace(runes[i]) {
			return "", 0, false
		}
	}
	return "", 0, false
}

func isAgentAtMention(start int, query string, agents []client.AgentRunSummary) bool {
	if start != 0 {
		return false
	}
	if strings.ContainsAny(query, `/\.`) {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(query))
	if name == "" {
		return false
	}
	for _, a := range agents {
		if strings.ToLower(strings.TrimSpace(a.AgentName)) == name {
			return true
		}
	}
	return false
}

func replaceActiveAtWith(input string, caret int, path string) string {
	_, start, ok := activeAtFragment(input, caret)
	if !ok {
		return input
	}
	runes := []rune(input)
	if caret < 0 {
		caret = len(runes)
	}
	if caret > len(runes) {
		caret = len(runes)
	}
	return string(runes[:start]) + path + string(runes[caret:])
}

func filterFileMentionSuggestions(query string, paths []string) []suggestItem {
	if paths == nil {
		return []suggestItem{{value: "", detail: "loading files…", kind: "file"}}
	}
	out := make([]suggestItem, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, suggestItem{value: p, kind: "file"})
	}
	if len(out) == 0 {
		return []suggestItem{{value: "", detail: "(no matching files)", kind: "file"}}
	}
	return out
}

func (m *AppModel) projectCwd() string {
	cwd := strings.TrimSpace(m.cfg.ProjectPath)
	if m.project != nil && strings.TrimSpace(m.project.Path) != "" {
		cwd = m.project.Path
	}
	if strings.TrimSpace(m.projectPath) != "" {
		cwd = m.projectPath
	}
	return strings.TrimSpace(cwd)
}

func (m *AppModel) cmdLoadWorkspaceFiles(query string) tea.Cmd {
	cwd := m.projectCwd()
	if cwd == "" || m.client == nil {
		return func() tea.Msg {
			return WorkspaceFilesMsg{Query: query, Paths: []string{}}
		}
	}
	cl := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		paths, err := cl.ListWorkspaceFiles(ctx, cwd, query)
		if err != nil {
			return WorkspaceFilesMsg{Query: query, Err: err.Error()}
		}
		return WorkspaceFilesMsg{Query: query, Paths: paths}
	}
}

func (m *AppModel) cmdMaybePrefetchWorkspaceFiles() tea.Cmd {
	query, start, ok := activeAtFragment(m.inputValue, m.inputCaretIndex())
	if !ok || isAgentAtMention(start, query, m.agentRuns) {
		return nil
	}
	if m.workspaceFiles != nil && m.workspaceFilesQuery == query {
		return nil
	}
	return m.cmdLoadWorkspaceFiles(query)
}

type mentionSpan struct {
	start, end int
	kind       string
}

func collectMentionSpans(plain string, skillNames []string) []mentionSpan {
	var spans []mentionSpan
	for _, loc := range fileMentionRe.FindAllStringIndex(plain, -1) {
		raw := plain[loc[0]:loc[1]]
		if strings.HasPrefix(strings.ToLower(raw), "http://") || strings.HasPrefix(strings.ToLower(raw), "https://") {
			continue
		}
		if loc[0] >= 3 && plain[loc[0]-3:loc[0]] == "://" {
			continue
		}
		spans = append(spans, mentionSpan{start: loc[0], end: loc[1], kind: "file"})
	}
	for _, name := range skillNames {
		tok := skillPromptToken(name)
		if tok == "" {
			continue
		}
		from := 0
		for {
			i := strings.Index(plain[from:], tok)
			if i < 0 {
				break
			}
			start := from + i
			spans = append(spans, mentionSpan{start: start, end: start + len(tok), kind: "skill"})
			from = start + len(tok)
		}
	}
	if len(spans) < 2 {
		return spans
	}
	for i := 0; i < len(spans); i++ {
		for j := i + 1; j < len(spans); j++ {
			if spans[j].start < spans[i].start || (spans[j].start == spans[i].start && spans[j].end > spans[i].end) {
				spans[i], spans[j] = spans[j], spans[i]
			}
		}
	}
	out := spans[:0]
	cursor := 0
	for _, s := range spans {
		if s.start < cursor {
			continue
		}
		out = append(out, s)
		cursor = s.end
	}
	return out
}

func mentionStyle(kind string) lipgloss.Style {
	if kind == "file" {
		return styleMentionFile
	}
	return styleMention
}

func highlightMentions(plain string, skillNames []string, base lipgloss.Style) string {
	if plain == "" {
		return ""
	}
	return paintMentionRange(plain, 0, len(plain), collectMentionSpans(plain, skillNames), base)
}

func paintMentionRange(plain string, start, end int, spans []mentionSpan, base lipgloss.Style) string {
	if start >= end || start < 0 || end > len(plain) {
		return ""
	}
	var b strings.Builder
	cursor := start
	for _, s := range spans {
		if s.end <= start || s.start >= end {
			continue
		}
		a, z := s.start, s.end
		if a < start {
			a = start
		}
		if z > end {
			z = end
		}
		if a > cursor {
			b.WriteString(base.Render(plain[cursor:a]))
		}
		b.WriteString(mentionStyle(s.kind).Render(plain[a:z]))
		cursor = z
	}
	if cursor < end {
		b.WriteString(base.Render(plain[cursor:end]))
	}
	return b.String()
}

func paintWrappedMentions(plain string, skillNames []string, lines []string, base lipgloss.Style) []string {
	spans := collectMentionSpans(plain, skillNames)
	out := make([]string, len(lines))
	from := 0
	for i, line := range lines {
		idx := strings.Index(plain[from:], line)
		if idx < 0 {
			out[i] = base.Render(line)
			continue
		}
		start := from + idx
		end := start + len(line)
		out[i] = paintMentionRange(plain, start, end, spans, base)
		from = end
	}
	return out
}

func (m *AppModel) applyFileMention(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	m.inputValue = replaceActiveAtWith(m.inputValue, m.inputCaretIndex(), path)
	m.inputCursor = -1
	m.suggIdx = 0
	if m.mirrorReady() {
		m.syncTextareaValue()
	}
}

