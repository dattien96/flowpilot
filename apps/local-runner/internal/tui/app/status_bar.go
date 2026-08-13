package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *AppModel) renderStatusLine() string {
	sep := " │ "
	if m.asciiMode {
		sep = " | "
	}
	w := m.width
	if w <= 0 {
		w = 80
	}
	w = safeTermWidth(w)

	line0 := m.renderStatusLine0(sep, w)
	if m.statusDetailsCollapsed {
		return line0
	}

	var lines []string
	lines = append(lines, line0)
	lines = append(lines, m.renderStatusModeLine(w))
	lines = append(lines, styleStatus.Render(fitStatusWidth(m.renderStatusModelLine(sep), w)))
	if m.statusSkillsExpanded {
		for _, extra := range formatAttachedSkillsExpanded(attachedSkillNames(m.selectedSkills), w) {
			lines = append(lines, styleStatus.Render(fitStatusWidth(extra, w)))
		}
	}
	if acc := m.renderStatusAccountLine(sep); acc != "" {
		lines = append(lines, styleStatus.Render(fitStatusWidth(acc, w)))
	}
	if usage := formatContextLimits(m.lastTokens, m.modelContextWin); usage != "" {
		lines = append(lines, styleStatus.Render(fitStatusWidth(usage, w)))
	}
	lines = append(lines, styleStatus.Render(fitStatusWidth(m.projectStatusLabel(), w)))
	return strings.Join(lines, "\n")
}

func (m *AppModel) statusFoldChip() string {
	if m.asciiMode {
		if m.statusDetailsCollapsed {
			return "> F4"
		}
		return "v F4"
	}
	if m.statusDetailsCollapsed {
		return "▸ F4"
	}
	return "▾ F4"
}

func (m *AppModel) statusReadyLabel() string {
	if m.sessionLoading {
		if m.statusMsg != "" {
			return m.statusMsg + " · loading…"
		}
		return "loading…"
	}
	if m.statusMsg != "" {
		return m.statusMsg
	}
	if m.connStatus == ConnIdle {
		return "ready"
	}
	return m.connStatus.String()
}

func (m *AppModel) renderStatusLine0(sep string, w int) string {
	var parts []string
	parts = append(parts, m.statusFoldChip())
	if m.turnIsActive() {
		parts = append(parts, styleError.Render("[stop]"))
	}
	statusStyle := styleStatus
	switch m.connStatus {
	case ConnRunning, ConnConnecting:
		statusStyle = styleStatusOK
	case ConnError:
		statusStyle = styleStatusErr
	}
	if m.sessionLoading {
		statusStyle = styleStatusOK
	}
	parts = append(parts, statusStyle.Render(m.statusReadyLabel()))
	if m.authNeedLogin {
		parts = append(parts, styleStatusErr.Render("SIGN-IN"))
	}
	if chip := m.formatAgentsChip(m.asciiMode); chip != "" {
		parts = append(parts, chip)
	}
	raw := strings.Join(parts, sep)
	if lipgloss.Width(raw) > w {
		raw = truncateVisual(raw, w)
	}
	return styleStatus.Render(raw)
}

func (m *AppModel) renderStatusModeLine(w int) string {
	mode := m.mode.String()
	chip := fmt.Sprintf("[%s]", mode)
	label := ""
	if m.mode == ModeFlow || m.mode == ModeStep {
		if s := m.launch.StatusLabel(); s != "" {
			if len([]rune(s)) > 24 {
				r := []rune(s)
				s = string(r[:21]) + "…"
			}
			label = s
		}
	}
	step := strings.TrimSpace(m.flowStepsActive)
	var body string
	switch {
	case label != "" && step != "":
		body = chip + " " + label + "  ▶ " + step
	case label != "":
		body = chip + " " + label
	case step != "":
		body = chip + "  ▶ " + step
	default:
		body = chip
	}
	body = fitStatusWidth(body, w)
	if m.mode == ModeFlow || m.mode == ModeStep {
		return stylePromptFocus.Render(body)
	}
	return styleStatus.Render(body)
}

func (m *AppModel) renderStatusModelLine(sep string) string {
	model := strings.TrimSpace(m.model)
	if model == "" {
		model = "—"
	}
	reasoning := strings.TrimSpace(m.reasoningEffort)
	if reasoning == "" {
		reasoning = "medium"
	}
	parts := []string{
		model,
		"reasoning: " + reasoning,
		m.yoloStatusLabel(),
	}
	if sk := formatAttachedSkillsChip(len(attachedSkillNames(m.selectedSkills)), m.statusSkillsExpanded, m.asciiMode); sk != "" {
		parts = append(parts, sk)
	}
	return strings.Join(parts, sep)
}

func (m *AppModel) renderStatusAccountLine(sep string) string {
	var parts []string
	if p := strings.TrimSpace(m.provider); p != "" {
		parts = append(parts, p)
	}
	if acc := m.activeProviderAccountLabel(); acc != "" {
		parts = append(parts, acc)
	}
	if lim := formatAccountLimits(m.account); lim != "" {
		parts = append(parts, lim)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, sep)
}
