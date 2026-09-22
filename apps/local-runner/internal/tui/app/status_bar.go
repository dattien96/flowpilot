package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
)

func (m *AppModel) renderStatusLine() string {
	// Per user request: the separate status row is gone – the chat input frame
	// now owns the chrome (top-left: chat/flow · ready · agent, bottom-right:
	// Model · reasoning · YOLO). The only thing that still needs a dedicated
	// line is the transient flash toast (Copied, etc.).
	w := m.chatWidth()
	if w <= 0 {
		w = 80
	}
	w = safeTermWidth(w)
	if t := strings.TrimSpace(m.flashToast); t != "" {
		return fitStatusWidth(styleStatusOK.Render(t), w)
	}
	return ""
}

// renderProjectStatusLine is kept for callers that still want the project
// row; the main status line is single-row now (Task-311).
func (m *AppModel) renderProjectStatusLine(sep string, w int) string {
	body := styleStatus.Render(m.projectStatusLabel())
	if t := strings.TrimSpace(m.flashToast); t != "" {
		body = body + styleStatus.Render(sep) + styleStatusOK.Render(t)
	}
	return fitStatusWidth(body, w)
}

func (m *AppModel) statusReadyLabel() string {
	// Live work (chat turn or flow step): show the animated spinner + elapsed in
	// the status line (plain text so it picks up the status color) instead of
	// the static "thinking…" (CA-537). statusMsg itself stays untouched for
	// legacy tests.
	if m.workIsLive() {
		return thinkingLabelText(m.thinkingFrame, m.asciiMode)
	}
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
	// Pre-style segments so agent:<name> highlight survives. Open/back is on the sidebar.
	sepStyled := styleStatus.Render(sep)
	var parts []string
	// Pin remaining quota so the single always-visible line still shows 7d/5h like Desktop.
	// Compact without "· resets" suffix; the sidebar account row keeps full detail.
	if q := m.collapsedQuotaChip(); q != "" {
		parts = append(parts, q)
	}
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

// collapsedQuotaChip returns a compact quota chip for the always-visible F4 line.
// No "· resets" suffix — that stays on the expanded account row.
func (m *AppModel) collapsedQuotaChip() string {
	acc := m.account
	if acc == nil {
		return ""
	}
	var chips []string
	if acc.Remaining5hPercent != nil {
		chips = append(chips, styleStatus.Render(fmt.Sprintf("5h:%d%%", *acc.Remaining5hPercent)))
	}
	if acc.Remaining7dPercent != nil {
		chips = append(chips, styleStatusHi.Render(fmt.Sprintf("7d:%d%%", *acc.Remaining7dPercent)))
	}
	if len(chips) > 0 {
		return strings.Join(chips, " ")
	}
	for _, line := range acc.UsageDetailLines {
		if l := strings.TrimSpace(line.Label); l != "" {
			lower := strings.ToLower(l)
			if strings.HasPrefix(lower, "7d") || strings.Contains(lower, "weekly") || strings.Contains(lower, "week") {
				return styleStatusHi.Render(fmt.Sprintf("%s:%d%%", l, line.RemainingPercent))
			}
		}
	}
	// Fallback to first detail line (e.g. Team Credits) if weekly missing — but
	// never the Grok "Team <uuid>" / "Personal" UsageSummary (Desktop parity).
	for _, line := range acc.UsageDetailLines {
		l := strings.TrimSpace(line.Label)
		if l == "" {
			continue
		}
		// CA-684: informational stats lines carry no meter data — render the
		// label alone instead of a fabricated ":0%".
		if line.RemainingPercent <= 0 && strings.TrimSpace(line.ResetAt) == "" {
			return styleStatusHi.Render(l)
		}
		return styleStatusHi.Render(fmt.Sprintf("%s:%d%%", l, line.RemainingPercent))
	}
	return ""
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
	// Posture chip — dedicated hue per posture (not styleStatusHi accent).
	if posture := m.activePosture(); m.mode == ModeChat {
		parts = append(parts, styleStatus.Render("mode: ")+postureStyle(posture).Render(posture))
	}
	// CP-71 worktree chip — armed flag or live binding state.
	if badge := m.worktreeBadge(); badge != "" {
		parts = append(parts, styleStatus.Render("wt: ")+styleStatusHi.Render(strings.TrimPrefix(badge, "wt:")))
	}
	if sk := formatAttachedSkillsChip(len(attachedSkillNames(m.selectedSkills)), false, m.asciiMode); sk != "" {
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

// postureStyle returns the lipgloss style for a posture name (scan/plan/code).
func postureStyle(posture string) lipgloss.Style {
	switch posture {
	case "scan":
		return stylePostureScan
	case "plan":
		return stylePosturePlan
	default:
		return stylePostureCode
	}
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
	if len(parts) == 0 && acc.UsageSummary != nil {
		s := strings.TrimSpace(*acc.UsageSummary)
		// Grok sets UsageSummary to "Team <uuid>" / "Personal" as a team marker, not a quota.
		// Desktop never renders that as a quota chip (usageSummary.ts); don't pollute the
		// quota row with it. Keep real plan summaries (e.g. Claude "Pro").
		if s != "" && !strings.HasPrefix(s, "Team ") && s != "Personal" {
			return styleStatus.Render(s)
		}
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
