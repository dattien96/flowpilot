package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
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
	// Model line already mixes dim labels + accent values — do not re-wrap in styleStatus.
	lines = append(lines, fitStatusWidth(m.renderStatusModelLine(sep), w))
	if m.statusSkillsExpanded {
		for _, extra := range formatAttachedSkillsExpanded(attachedSkillNames(m.selectedSkills), w) {
			lines = append(lines, styleStatus.Render(fitStatusWidth(extra, w)))
		}
	}
	if acc := m.renderStatusAccountLine(sep); acc != "" {
		lines = append(lines, fitStatusWidth(acc, w))
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
	// Pre-style segments so agent:<name> highlight survives. Open/back is on F2 panel.
	sepStyled := styleStatus.Render(sep)
	var parts []string
	parts = append(parts, styleStatus.Render(m.statusFoldChip()))
	if m.turnIsActive() {
		parts = append(parts, styleError.Render("[stop]"))
	}
	parts = append(parts, statusStyle.Render(m.statusReadyLabel()))
	if m.authNeedLogin {
		parts = append(parts, styleStatusErr.Render("SIGN-IN"))
	}
	if view := m.formatAgentViewStatus(); view != "" {
		parts = append(parts, view)
	}
	raw := strings.Join(parts, sepStyled)
	if lipgloss.Width(raw) > w {
		raw = truncateVisual(raw, w)
	}
	return raw
}

func (m *AppModel) renderStatusModeLine(w int) string {
	mode := m.mode.String()
	chip := styleStatus.Render(fmt.Sprintf("[%s]", mode))
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
	// Mode chip stays dim; flow name + active step use styleStatusFlow (pink),
	// not accent/styleStatusHi used by model/reasoning/YOLO.
	var body string
	switch {
	case label != "" && step != "":
		body = chip + " " + styleStatusFlow.Render(label) + styleStatus.Render("  ▶ ") + styleStatusFlow.Render(step)
	case label != "":
		body = chip + " " + styleStatusFlow.Render(label)
	case step != "":
		body = chip + styleStatus.Render("  ▶ ") + styleStatusFlow.Render(step)
	default:
		body = chip
	}
	return fitStatusWidth(body, w)
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
	// Highlight: full model · reasoning VALUE only · YOLO VALUE only · full skills:N chip.
	// Labels "reasoning: " / "YOLO:" stay dim.
	sepStyled := styleStatus.Render(sep)
	parts := []string{
		styleStatusHi.Render(model),
		styleStatus.Render("reasoning: ") + styleStatusHi.Render(reasoning),
		styleYoloStatus(m.yoloStatusLabel()),
	}
	if sk := formatAttachedSkillsChip(len(attachedSkillNames(m.selectedSkills)), m.statusSkillsExpanded, m.asciiMode); sk != "" {
		parts = append(parts, styleStatusHi.Render(sk))
	}
	return strings.Join(parts, sepStyled)
}

// styleYoloStatus dims the "YOLO:" label and accents ON / OFF / ON(auto).
func styleYoloStatus(label string) string {
	const prefix = "YOLO:"
	if strings.HasPrefix(label, prefix) {
		return styleStatus.Render(prefix) + styleStatusHi.Render(strings.TrimPrefix(label, prefix))
	}
	return styleStatusHi.Render(label)
}

func (m *AppModel) renderStatusAccountLine(sep string) string {
	var parts []string
	if p := strings.TrimSpace(m.provider); p != "" {
		parts = append(parts, styleStatus.Render(p))
	}
	if acc := m.activeProviderAccountLabel(); acc != "" {
		parts = append(parts, styleStatus.Render(acc))
	}
	if lim := formatAccountLimitsStyled(m.account); lim != "" {
		parts = append(parts, lim)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, styleStatus.Render(sep))
}

// formatAccountLimitsStyled matches formatAccountLimits plain text, but accents
// the full "7d:NN%" token (reset suffix stays dim).
func formatAccountLimitsStyled(acc *client.ProviderAccountSummary) string {
	if acc == nil {
		return ""
	}
	var parts []string
	if acc.Remaining5hPercent != nil {
		parts = append(parts, styleStatus.Render(formatQuotaChip("5h", *acc.Remaining5hPercent, acc.Remaining5hResetAt)))
	}
	if acc.Remaining7dPercent != nil {
		parts = append(parts, styleQuotaChip7d(*acc.Remaining7dPercent, acc.Remaining7dResetAt))
	}
	if len(parts) == 0 {
		for _, line := range acc.UsageDetailLines {
			label := strings.TrimSpace(line.Label)
			if label == "" {
				continue
			}
			reset := strings.TrimSpace(line.ResetAt)
			var resetPtr *string
			if reset != "" {
				resetPtr = &reset
			}
			// Highlight weekly/7d-style detail chips that look like 7d: or contain "7d".
			chip := formatQuotaChip(label, line.RemainingPercent, resetPtr)
			if strings.HasPrefix(strings.ToLower(label), "7d") || strings.Contains(strings.ToLower(label), "week") {
				parts = append(parts, styleQuotaChipHighlight(label, line.RemainingPercent, resetPtr))
			} else {
				parts = append(parts, styleStatus.Render(chip))
			}
		}
	}
	if len(parts) == 0 && acc.UsageSummary != nil && strings.TrimSpace(*acc.UsageSummary) != "" {
		return styleStatus.Render(strings.TrimSpace(*acc.UsageSummary))
	}
	return strings.Join(parts, " ")
}

func styleQuotaChip7d(pct int, resetAt *string) string {
	return styleQuotaChipHighlight("7d", pct, resetAt)
}

func styleQuotaChipHighlight(label string, pct int, resetAt *string) string {
	chip := fmt.Sprintf("%s:%d%%", label, pct)
	out := styleStatusHi.Render(chip)
	if resetAt == nil {
		return out
	}
	if when := formatQuotaResetAt(*resetAt); when != "" {
		return out + styleStatus.Render(" · resets "+when)
	}
	return out
}
