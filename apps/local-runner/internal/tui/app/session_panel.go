package app

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
)

// sessionInfoPanel is a collapsible top-right overlay for bootstrap/session status
// (runner URL, project, active provider session) so it does not spam the chat transcript.
type sessionInfoPanel struct {
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

// flowStepsPanelLines is the legacy fixed-8-row view (kept for the overlay and
// the pre-CA-632 contract; the right sidebar uses the height-aware variant).
func (m *AppModel) flowStepsPanelLines() []string {
	return m.flowStepsPanelLinesMax(8)
}

// flowStepsPanelLinesMax renders up to maxRows step rows (each FAILED row may
// add one RejectionNote sub-line). A "… +N more" tail appears only when steps
// overflow maxRows — rag-harness has 9 nodes, so a 50-row sidebar now shows
// the full list including audit instead of truncating at 8 (run-142155).
func (m *AppModel) flowStepsPanelLinesMax(maxRows int) []string {
	if len(m.flowSteps) == 0 {
		return nil
	}
	if maxRows < 1 {
		maxRows = 1
	}
	var out []string
	// Steps are listed without a count header — the sidebar/overlay render a
	// "steps" section title (CA-542). Sub-agent [open] on the step row only;
	// the focused child gets no chip because [back] lives on the steps header.
	for i := 0; i < len(m.flowSteps) && i < maxRows; i++ {
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
				// Highlight the agent currently being viewed: teal marker +
				// styleStatusAgent (agent hue) so the selected step is obvious
				// without a clickable chip. No [open]/[back] — switching is
				// keyboard-only via /agents (user request).
				marker := "▸"
				if m.asciiMode {
					marker = ">"
				}
				line = fmt.Sprintf("[%s] %s %s%s", glyph, marker, name, suffix)
				lineStyle = styleStatusAgent
			}
		}
		out = append(out, lineStyle.Render(line)+action)
		if st == "FAILED" {
			if note := strings.TrimSpace(s.RejectionNote); note != "" {
				out = append(out, lineStyle.Render("  "+note))
			}
		}
	}
	if len(m.flowSteps) > maxRows {
		out = append(out, fmt.Sprintf("… +%d more", len(m.flowSteps)-maxRows))
	}
	if m.flowStepsActive != "" {
		out = append(out, styleStepRunning.Render("Now: "+m.flowStepsActive))
	}
	return out
}

// stepsSectionTitle renders the "steps" section header for the sidebar and the
// overlay. The focused child has no [back] chip (switching is keyboard-only via
// /agents + Esc, per user request) — it is highlighted in the step row instead.
func (m *AppModel) stepsSectionTitle() string {
	return styleGate.Render("steps")
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

// infoDump prints the session/status details as a chat message — the shared
// body of /info, /status and the F2 alias (Task-311: no panel toggle exists).
func (m *AppModel) infoDump() (tea.Model, tea.Cmd) {
	auth := "(not signed in)"
	if m.signedInEmail != "" {
		auth = m.signedInEmail
	} else if m.authNeedLogin {
		auth = "SIGN-IN REQUIRED — /login"
	}
	m.refreshSessionPanel()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Status: %s | Mode: %s | %s | Provider: %s | Model: %s | Auth: %s\n",
		m.connStatus, m.mode, m.yoloStatusLabel(), m.provider, m.model, auth))
	sb.WriteString("Active provider account: " + orDash(m.activeProviderAccountLabel()) + "\n")
	m.sessionPanel.DriveStatus = m.driveIndicatorLine()
	m.sessionPanel.DriveBadge = m.openChatDriveBadge()
	for _, line := range m.sessionPanel.lines() {
		sb.WriteString(line + "\n")
	}
	sb.WriteString("(sidebar follows terminal width · /info prints this dump · /open /agent for navigation)")
	m.addMessage("system", strings.TrimRight(sb.String(), "\n"), "")
	return m, nil
}

// renderSessionPanelOverlay is gone (Task-311): the session/steps/details
// content renders only as the reactive right sidebar column
// (renderRightSidebar). No overlay, no F2 toggle, no collapsed chip.

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
	return m.sessionPanel.hasContent() && m.terminalWidth() >= tuiSidebarMinWidth
}

// tuiSidebarMinWidth is the terminal width at which the reactive right sidebar
// appears (OpenCode-style): wide terminals get the session/steps/details
// column, narrow ones get chat only. There is no F2 toggle anymore — width is
// the only switch (Task-311).
const tuiSidebarMinWidth = 120

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
// session info header, flow steps, then the status-details section that used to be
// the F4-expanded status rows (mode / model / skills / account / context limits)
// (Task-311). h is the terminal height.
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
		plain := stripANSI(styleSystem.Render(line))
		for _, wl := range wrapText(plain, w-2) {
			if wl == "" {
				continue
			}
			out = append(out, styleSystem.Render(wl))
		}
	}
	out = append(out, "")
	out = append(out, m.stepsSectionTitle())
	// Height-aware step budget: fit as many steps as the sidebar rows allow
	// after the session header. Sidebar now only holds session + steps (no
	// status) per user request – status details live in the chat chrome.
	maxRows := h - len(out) - 2
	if maxRows < 8 {
		maxRows = 8
	}
	steps := m.flowStepsPanelLinesMax(maxRows)
	for _, line := range steps {
		out = append(out, truncateStepLine(line, w-2))
	}
	if len(steps) == 0 {
		out = append(out, styleSystem.Render("(no steps)"))
	}
	// Guide-F UX follow-up: live sub-agent rows (spawn_agent children, flow
	// reviewers) — the always-visible main-view counterpart of /agents, styled
	// like the steps section. Hidden entirely when the agent graph is empty.
	if len(m.agentRuns) > 0 {
		agentRows := h - len(out) - 2
		if agentRows > len(m.agentRuns)+1 {
			agentRows = len(m.agentRuns) + 1 // +1 section header
		}
		if agentRows >= 2 {
			out = append(out, "")
			for _, line := range m.agentRunsSectionLines(agentRows - 1) {
				out = append(out, truncateStepLine(line, w-2))
			}
		}
	}
	// Pad to full height so the sidebar is a solid right column.
	for len(out) < h {
		out = append(out, " ")
	}
	return out
}

// agentRowIsMain reports whether an agent-graph row is the hub/main run,
// which pins to the top of the sidebar agents section (BUG-336).
func agentRowIsMain(a client.AgentRunSummary) bool {
	return strings.EqualFold(strings.TrimSpace(a.Role), "main") ||
		strings.EqualFold(strings.TrimSpace(a.AgentName), "main")
}

// agentRunsSectionLines renders the sidebar "agents" section: one row per run
// in the current run's agent graph (spawn_agent children, flow reviewers),
// with the same status glyphs as the steps section. Header included; capped at
// maxRows data rows with a … +N more tail.
//
// Display order is DETERMINISTIC and stable across snapshot jitter (BUG-336,
// operator report: the section flickered and re-ordered continuously because
// graph events and hydrate polls deliver the same runs in varying orders):
// the main run pins to the top, the rest sort by CreatedAt (spawn time) with
// RunID as the tiebreak.
func (m *AppModel) agentRunsSectionLines(maxRows int) []string {
	if len(m.agentRuns) == 0 || maxRows < 1 {
		return nil
	}
	rows := make([]client.AgentRunSummary, len(m.agentRuns))
	copy(rows, m.agentRuns)
	sort.SliceStable(rows, func(i, j int) bool {
		mi, mj := agentRowIsMain(rows[i]), agentRowIsMain(rows[j])
		if mi != mj {
			return mi
		}
		if rows[i].CreatedAt != rows[j].CreatedAt {
			return rows[i].CreatedAt < rows[j].CreatedAt
		}
		return rows[i].RunID < rows[j].RunID
	})
	out := []string{styleGate.Render("agents")}
	for i, a := range rows {
		if i >= maxRows {
			out = append(out, fmt.Sprintf("… +%d more", len(rows)-maxRows))
			break
		}
		name := strings.TrimSpace(a.Label)
		if name == "" {
			name = strings.TrimSpace(a.AgentName)
		}
		if name == "" {
			name = shortID(a.RunID)
		}
		st := strings.ToUpper(strings.TrimSpace(a.Status))
		// Same glyph language as the steps rows: spinner while running,
		// ✓ done, x failed, - cancelled, blank pending/unknown.
		glyph := " "
		lineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
		suffix := ""
		switch st {
		case "RUNNING", "IN_PROGRESS":
			glyph = thinkingSpinner(m.thinkingFrame, m.asciiMode)
			suffix = " " + st
			lineStyle = styleStepRunning
		case "WAITING_USER_APPROVAL":
			glyph = "•"
			suffix = " " + st
			lineStyle = styleStepRunning
		case "COMPLETED", "DONE":
			if m.asciiMode {
				glyph = "+"
			} else {
				glyph = "✓"
			}
			lineStyle = styleStepDone
		case "FAILED", "ERROR":
			glyph = "x"
			lineStyle = styleStepFailed
		case "CANCELLED", "CANCELED", "SKIPPED":
			glyph = "-"
		}
		out = append(out, lineStyle.Render(fmt.Sprintf("[%s] %s%s", glyph, name, suffix)))
	}
	return out
}

// renderSidebarStatusSection is the former F4 status-details content, rendered
// as sidebar rows (Task-311): mode, model · reasoning · YOLO · posture, attached
// skills, active account + quota, context limits.
// Truncation hid the account email/quota (sidebar ~40 cols). Wrap instead so
// every token stays visible on a narrow sidebar.
func (m *AppModel) renderSidebarStatusSection(w int) []string {
	var out []string
	out = append(out, styleGate.Render("status"))
	appendWrapped := func(styled string) {
		if styled == "" {
			return
		}
		plain := stripANSI(styled)
		wrapped := wrapText(plain, w)
		for _, wl := range wrapped {
			if wl == "" {
				continue
			}
			out = append(out, styleSystem.Render(wl))
		}
	}
	// Mode line: render without the status-line fitStatusWidth truncation so
	// wrapText can show the full flow label instead of "…" on a 40-col sidebar.
	if line := m.renderStatusModeLine(200); line != "" {
		appendWrapped(line)
	}
	if line := m.renderStatusModelLine(" · "); line != "" {
		appendWrapped(line)
	}
	for _, extra := range formatAttachedSkillsExpanded(attachedSkillNames(m.selectedSkills), w) {
		appendWrapped(extra)
	}
	// Account line: provider | account | quota — wrap so email + quota never
	// get cut to "…" on a 40-col sidebar (user report: acc info missing).
	if acc := m.renderStatusAccountLine(" | "); acc != "" {
		appendWrapped(acc)
		// When quota wrapped onto its own line, also emit the full styled quota
		// detail on the next line so "7d:84% · resets Jan 5" is never lost.
		if m.account != nil {
			if q := strings.TrimSpace(formatAccountLimits(m.account)); q != "" && !strings.Contains(stripANSI(acc), stripANSI(q)) {
				appendWrapped(q)
			}
		}
	} else if p := strings.TrimSpace(m.provider); p != "" {
		// No account yet (catalog loading) — still show provider so sidebar never
		// looks empty; account appears once SessionDefaultsMsg binds it.
		appendWrapped(styleStatus.Render(p))
	}
	if usage := formatContextLimits(m.lastTokens, m.modelContextWin); usage != "" {
		appendWrapped(usage)
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
