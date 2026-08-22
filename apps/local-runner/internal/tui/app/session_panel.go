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
	RunID       string // current run id (e.g. "run-102521")
	ProjectPath string
	ProjectName string
	ProjectID   string
	Session     string // e.g. "codex · gpt-5.4 (acct-label)"
	DriveStatus string // live Drive sync/restore progress line (CA-551); "" when idle
	// DriveBadge is the sync marker of the currently open chat (CA-553): shown
	// as its own line right below the Run line so an open chat's Drive state is
	// always visible even when the history picker/dump rows are narrow.
	DriveBadge string
}

func (p sessionInfoPanel) hasContent() bool {
	return strings.TrimSpace(p.RunnerURL) != "" ||
		strings.TrimSpace(p.RunID) != "" ||
		strings.TrimSpace(p.ProjectPath) != "" ||
		strings.TrimSpace(p.ProjectName) != "" ||
		strings.TrimSpace(p.Session) != ""
}

func (p sessionInfoPanel) lines() []string {
	var out []string
	if u := strings.TrimSpace(p.RunnerURL); u != "" {
		out = append(out, "Runner: "+u)
	}
	if id := strings.TrimSpace(p.RunID); id != "" {
		out = append(out, "Run: "+shortID(id))
		if b := strings.TrimSpace(p.DriveBadge); b != "" {
			out = append(out, "Drive: "+b)
		}
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
	if ds := strings.TrimSpace(p.DriveStatus); ds != "" {
		out = append(out, ds)
	}
	return out
}

func (m *AppModel) flowStepsPanelLines() []string {
	if len(m.flowSteps) == 0 {
		return nil
	}
	var out []string
	// Steps are listed without a count header — the sidebar/overlay render a
	// "steps" section title (CA-542). Sub-agent [open] on the step row only;
	// the focused child gets no chip because [back] lives on the steps header.
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
		// OpenCode todo-list glyph: [✓] done, [•] in progress, [x] failed, [ ] pending.
		// Pending/empty must be [ ] — not ✓ — so the F2 list does not look done
		// before the step ever ran (rag-harness F1: validate/audit pending).
		glyph := " "
		lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
		var suffix string
		switch st {
		case "RUNNING":
			// CA-537: the RUNNING step shows the same animated spinner as the
			// status line (driven by thinkingFrame while work is live).
			glyph = thinkingSpinner(m.thinkingFrame, m.asciiMode)
			suffix = " " + st
			lineStyle = styleStepRunning
		case "WAITING_USER_APPROVAL":
			glyph = "•"
			suffix = " " + st
			lineStyle = styleStepRunning
		case "DONE":
			if m.asciiMode {
				glyph = "+"
			} else {
				glyph = "✓"
			}
			lineStyle = styleStepDone
		case "FAILED":
			glyph = "x"
			lineStyle = styleStepFailed
		case "SKIPPED", "CANCELLED", "CANCELED":
			glyph = "-"
		}
		line := fmt.Sprintf("[%s] %s%s", glyph, name, suffix)
		action := ""
		if child, ok := m.childRunForStep(s); ok {
			if m.viewingChild() && strings.EqualFold(strings.TrimSpace(child.RunID), strings.TrimSpace(m.focusRunID)) {
				// Focused child step is highlighted but shows no [back] chip —
				// back lives on the steps header (CA-542) so it never overlaps
				// the [open] column, making double-click on [open] idempotent.
				lineStyle = styleStatusHi
			} else {
				action = "  " + styleStepAgentAction.Render("[open]")
			}
		}
		out = append(out, lineStyle.Render(line)+action)
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

// stepsSectionTitle renders the "steps" section header for the sidebar and the
// overlay. When a child agent is focused it carries the [back] chip (CA-542) so
// back never overlaps the [open] column on step rows and double-click on [open]
// stays idempotent.
func (m *AppModel) stepsSectionTitle() string {
	title := styleGate.Render("steps")
	if m.viewingChild() {
		title = title + "  " + styleStepAgentAction.Render("[back]")
	}
	return title
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
	if lim := formatAccountLimits(m.account); lim != "" {
		display = fmt.Sprintf("%s · %s", display, lim)
	}
	return display
}

func (m *AppModel) refreshSessionPanel() {
	m.sessionPanel.RunnerURL = m.runnerURL
	m.sessionPanel.RunID = ""
	if m.runHandle != nil {
		m.sessionPanel.RunID = m.runHandle.RunID
	}
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
	m.sessionPanel.DriveBadge = m.openChatDriveBadge()
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

	m.sessionPanel.DriveStatus = m.driveIndicatorLine()
	m.sessionPanel.DriveBadge = m.openChatDriveBadge()
	body := m.sessionPanel.lines()
	if steps := m.flowStepsPanelLines(); len(steps) > 0 {
		body = append(body, m.stepsSectionTitle())
		body = append(body, steps...)
	}
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
		// Width must ignore ANSI from step highlight styles; keep any trailing
		// [open]/[back] chip visible when a long step row is squeezed (CA-528).
		line = truncateStepLine(line, maxInner-2)
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
	if w > width {
		return truncateVisual(s, width)
	}
	if w == width {
		return s
	}
	return strings.Repeat(" ", width-w) + s
}

// useRightSidebar reports whether F2 content should render as a full-height right
// sidebar column (OpenCode-style) instead of the legacy top-right overlay. It only
// engages on wide terminals (>=100 cols) with an expanded session panel.
func (m *AppModel) useRightSidebar() bool {
	return m.sessionPanel.hasContent() && m.terminalWidth() >= 100 && !m.sessionPanel.Collapsed
}

// terminalWidth returns the real terminal width (stable even mid-render).
func (m *AppModel) terminalWidth() int {
	if m.fullWidth > 0 {
		return m.fullWidth
	}
	return m.width
}

// sideWidth returns the right-sidebar column width (OpenCode default ~42, clamped).
func (m *AppModel) sideWidth() int {
	w := m.terminalWidth() * 2 / 5
	if w < 30 {
		w = 30
	}
	if w > 42 {
		w = 42
	}
	return w
}

// contentWidth is the chat-pane width. The -1 is the unused last terminal
// column (macOS autowrap), not a separator between panes.
func (m *AppModel) contentWidth() int {
	if !m.useRightSidebar() {
		return m.terminalWidth()
	}
	w := m.terminalWidth() - m.sideWidth() - 1
	if w < 20 {
		w = 20
	}
	return w
}

// chatWidth is the width every transcript/status/composer/attach renderer must
// use (CA-526). With the F2 right sidebar this is contentWidth(); otherwise the
// terminal width. Paint and hit-test paths must share it so clicks and
// drag-select align with what was drawn — the old View() width mutation made
// clickTargetAt run at a different wrap than the painted rows.
func (m *AppModel) chatWidth() int {
	w := m.contentWidth()
	if w < 1 {
		w = 80
	}
	return w
}

// renderRightSidebar returns the full-height right sidebar lines (OpenCode-style):
// session info header, then a todo-list of flow steps. h is the terminal height.
func (m *AppModel) renderRightSidebar(h int) []string {
	if !m.useRightSidebar() {
		return nil
	}
	w := m.sideWidth()
	var out []string
	out = append(out, styleGate.Render("session"))
	m.sessionPanel.DriveStatus = m.driveIndicatorLine()
	m.sessionPanel.DriveBadge = m.openChatDriveBadge()
	for _, line := range m.sessionPanel.lines() {
		out = append(out, styleSystem.Render(truncateVisual(line, w-2)))
	}
	out = append(out, "")
	out = append(out, m.stepsSectionTitle())
	steps := m.flowStepsPanelLines()
	for _, line := range steps {
		out = append(out, truncateStepLine(line, w-2))
	}
	if len(steps) == 0 {
		out = append(out, styleSystem.Render("(no steps)"))
	}
	out = append(out, "")
	out = append(out, styleLink.Render("[collapse]"))
	// Pad to full height so the sidebar is a solid right column.
	for len(out) < h {
		out = append(out, " ")
	}
	return out
}

// truncateStepLine squeezes a step row to width while keeping a trailing
// [open]/[back] action chip visible (CA-528). A plain end-truncation would cut
// the chip first, hiding the only way to open the child agent transcript.
func truncateStepLine(line string, width int) string {
	if width < 1 {
		return ""
	}
	if lipgloss.Width(line) <= width {
		return line
	}
	plain := stripANSI(line)
	chip := ""
	switch {
	case strings.HasSuffix(plain, "[open]"):
		chip = "[open]"
	case strings.HasSuffix(plain, "[back]"):
		chip = "[back]"
	}
	if chip == "" {
		return truncateVisual(line, width)
	}
	// Reserve the "  <chip>" suffix; squeeze the styled prefix into the rest.
	rest := strings.TrimSuffix(plain, chip)
	rest = strings.TrimRight(rest, " ")
	rest = truncateVisual(rest, max(0, width-2-lipgloss.Width(chip)))
	return rest + "  " + chip
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// padTo pads a (possibly ANSI-styled) string to width w with trailing spaces.
func padTo(s string, w int) string {
	cur := lipgloss.Width(stripANSI(s))
	if cur >= w {
		return s
	}
	return s + strings.Repeat(" ", w-cur)
}

// padLinesTo pads a slice of lines up to height h with empty lines.
func padLinesTo(lines []string, h int) []string {
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines
}

func (m *AppModel) renderSidebarPane(w, h int) string {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	lines := m.renderRightSidebar(h)
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, " ")
	}
	for i, line := range lines {
		lines[i] = paintRow(line, w, styleSidebar)
	}
	return strings.Join(lines, "\n")
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
