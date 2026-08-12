package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
)

// sessionInfoPanel is a collapsible top-right overlay for bootstrap/session status
// (runner URL, project, active provider session) so it does not spam the chat transcript.
type sessionInfoPanel struct {
	Collapsed   bool
	RunnerURL   string
	ProjectPath string
	ProjectName string
	ProjectID   string
	Session     string // e.g. "codex · gpt-5.4 (acct-label)"
}

func (p sessionInfoPanel) hasContent() bool {
	return strings.TrimSpace(p.RunnerURL) != "" ||
		strings.TrimSpace(p.ProjectPath) != "" ||
		strings.TrimSpace(p.ProjectName) != "" ||
		strings.TrimSpace(p.Session) != ""
}

func (p sessionInfoPanel) lines() []string {
	var out []string
	if u := strings.TrimSpace(p.RunnerURL); u != "" {
		out = append(out, "Runner: "+u)
	}
	if path := strings.TrimSpace(p.ProjectPath); path != "" {
		out = append(out, "Path: "+path)
	}
	if name := strings.TrimSpace(p.ProjectName); name != "" {
		id := strings.TrimSpace(p.ProjectID)
		if id != "" {
			out = append(out, fmt.Sprintf("Project: %s (%s)", name, shortID(id)))
		} else {
			out = append(out, "Project: "+name)
		}
	}
	if s := strings.TrimSpace(p.Session); s != "" {
		out = append(out, "Session: "+s)
	}
	return out
}

func (m *AppModel) flowStepsPanelLines() []string {
	if len(m.flowSteps) == 0 {
		return nil
	}
	var out []string
	out = append(out, fmt.Sprintf("Steps %d:", len(m.flowSteps)))
	limit := len(m.flowSteps)
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		s := m.flowSteps[i]
		name := strings.TrimSpace(s.NodeID)
		if name == "" {
			name = strings.TrimSpace(s.StepType)
		}
		if name == "" {
			name = shortID(s.StepID)
		}
		st := strings.ToUpper(strings.TrimSpace(s.Status))
		prefix := " "
		lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
		switch st {
		case "RUNNING", "WAITING_USER_APPROVAL":
			prefix = ">"
			lineStyle = styleStepRunning
		case "DONE":
			prefix = "+"
			lineStyle = styleStepDone
		case "FAILED":
			prefix = "x"
			lineStyle = styleStepFailed
		}
		out = append(out, lineStyle.Render(fmt.Sprintf("%s%d.%s %s", prefix, i+1, st, name)))
		if st == "FAILED" {
			if note := strings.TrimSpace(s.RejectionNote); note != "" {
				out = append(out, lineStyle.Render("  "+note))
			}
		}
	}
	if len(m.flowSteps) > limit {
		out = append(out, fmt.Sprintf("… +%d more", len(m.flowSteps)-limit))
	}
	if m.flowStepsActive != "" {
		out = append(out, styleStepRunning.Render("Now: "+m.flowStepsActive))
	}
	return out
}

// bindActiveAccountForProvider sets account + accountLabel from the active
// connected account for the current provider only (never a stale other-provider
// account left over after /provider switch).
func (m *AppModel) bindActiveAccountForProvider() {
	key := strings.TrimSpace(m.provider)
	prevLabel := strings.TrimSpace(m.accountLabel)
	prevAccount := m.account
	m.account = nil
	m.accountLabel = ""
	if key == "" {
		return
	}
	var fallback *client.ProviderAccountSummary
	for i := range m.providerAccounts {
		a := &m.providerAccounts[i]
		if !strings.EqualFold(a.ProviderKey, key) {
			continue
		}
		if a.IsActive && strings.EqualFold(a.AuthStatus, "connected") {
			m.account = a
			m.accountLabel = strings.TrimSpace(a.DisplayLabel)
			return
		}
		if fallback == nil && a.IsActive {
			fallback = a
		}
		if fallback == nil && strings.EqualFold(a.AuthStatus, "connected") {
			fallback = a
		}
	}
	if fallback != nil {
		m.account = fallback
		m.accountLabel = strings.TrimSpace(fallback.DisplayLabel)
		return
	}
	// Keep prior binding only when it belongs to this provider (or accounts not loaded yet).
	if prevAccount != nil && strings.EqualFold(prevAccount.ProviderKey, key) {
		m.account = prevAccount
		m.accountLabel = strings.TrimSpace(prevAccount.DisplayLabel)
		if m.accountLabel == "" {
			m.accountLabel = prevLabel
		}
		return
	}
	if len(m.providerAccounts) == 0 && prevLabel != "" {
		m.accountLabel = prevLabel
	}
}

func (m *AppModel) activeProviderAccountLabel() string {
	key := strings.TrimSpace(m.provider)
	if m.account != nil && (key == "" || strings.EqualFold(m.account.ProviderKey, key)) {
		if label := strings.TrimSpace(m.account.DisplayLabel); label != "" {
			return label
		}
	}
	for _, a := range m.providerAccounts {
		if key != "" && !strings.EqualFold(a.ProviderKey, key) {
			continue
		}
		if !a.IsActive {
			continue
		}
		if label := strings.TrimSpace(a.DisplayLabel); label != "" {
			return label
		}
	}
	// Last resort: accountLabel when accounts are unknown, or it matches this provider.
	want := strings.TrimSpace(m.accountLabel)
	if want == "" {
		return ""
	}
	if key == "" || len(m.providerAccounts) == 0 {
		return want
	}
	for _, a := range m.providerAccounts {
		if strings.EqualFold(a.ProviderKey, key) && strings.TrimSpace(a.DisplayLabel) == want {
			return want
		}
	}
	return ""
}

func (m *AppModel) sessionDisplayLine() string {
	display := strings.TrimSpace(m.provider)
	if display == "" {
		return "provider not set — /provider"
	}
	if m.model != "" {
		display = fmt.Sprintf("%s · %s", display, m.model)
	} else {
		display = fmt.Sprintf("%s · (default model)", display)
	}
	if acc := m.activeProviderAccountLabel(); acc != "" {
		display = fmt.Sprintf("%s (%s)", display, acc)
	}
	return display
}

func (m *AppModel) refreshSessionPanel() {
	m.sessionPanel.RunnerURL = m.runnerURL
	m.sessionPanel.ProjectPath = m.cfg.ProjectPath
	if m.projectPath != "" {
		m.sessionPanel.ProjectPath = m.projectPath
	}
	if m.project != nil {
		m.sessionPanel.ProjectName = m.project.Name
		m.sessionPanel.ProjectID = m.project.ID
		if m.sessionPanel.ProjectPath == "" {
			m.sessionPanel.ProjectPath = m.project.Path
		}
	}
	m.sessionPanel.Session = m.sessionDisplayLine()
}

// renderSessionPanelOverlay returns right-aligned panel lines (collapsed chip or expanded box).
func (m *AppModel) renderSessionPanelOverlay() []string {
	if !m.sessionPanel.hasContent() {
		return nil
	}
	width := m.width
	if width < 20 {
		width = 80
	}
	// Match Desktop --text-dim / --accent / --bg-3 (styles.css :root).
	boxStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim)).BorderForeground(lipgloss.Color(colorAccent))
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))

	if m.sessionPanel.Collapsed {
		chip := "[info] F2 or /info"
		if m.asciiMode {
			chip = "[info] F2|/info"
		}
		return []string{rightAlignPlain(hintStyle.Render(chip), width)}
	}

	body := m.sessionPanel.lines()
	body = append(body, m.flowStepsPanelLines()...)
	if len(body) == 0 {
		return nil
	}
	maxInner := width * 2 / 5
	if maxInner < 28 {
		maxInner = 28
	}
	if maxInner > width-2 {
		maxInner = width - 2
	}
	var framed []string
	top := "┌─ session " + strings.Repeat("─", max(0, maxInner-11)) + "┐"
	if m.asciiMode {
		top = "+- session " + strings.Repeat("-", max(0, maxInner-11)) + "+"
	}
	framed = append(framed, top)
	for _, line := range body {
		// Width must ignore ANSI from step highlight styles.
		line = lipgloss.NewStyle().MaxWidth(maxInner - 2).Render(line)
		pad := maxInner - 2 - lipgloss.Width(line)
		if pad < 0 {
			pad = 0
		}
		inner := "│ " + line + strings.Repeat(" ", pad) + "│"
		if m.asciiMode {
			inner = "| " + line + strings.Repeat(" ", pad) + "|"
		}
		framed = append(framed, inner)
	}
	bot := "└─ F2 or /info " + strings.Repeat("─", max(0, maxInner-16)) + "┘"
	if m.asciiMode {
		bot = "+- F2 or /info " + strings.Repeat("-", max(0, maxInner-16)) + "+"
	}
	framed = append(framed, bot)

	out := make([]string, 0, len(framed))
	for _, line := range framed {
		out = append(out, rightAlignPlain(boxStyle.Render(line), width))
	}
	return out
}

func rightAlignPlain(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return strings.Repeat(" ", width-w) + s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// suggestionWindow returns an inclusive-exclusive [start,end) window of size limit
// that keeps selected index visible (so long pickers remain navigable).
func suggestionWindow(total, selected, limit int) (start, end int) {
	if total <= 0 {
		return 0, 0
	}
	if limit < 1 {
		limit = 1
	}
	if total <= limit {
		return 0, total
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= total {
		selected = total - 1
	}
	start = selected - limit/2
	if start < 0 {
		start = 0
	}
	end = start + limit
	if end > total {
		end = total
		start = end - limit
	}
	return start, end
}
