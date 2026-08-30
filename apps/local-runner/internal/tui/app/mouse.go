package app

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var ansiCSI = regexp.MustCompile(`\x1b\[[0-9:;?]*[ -/]*[@-~]|\x1b\][^\x07]*\x07|\x1b\(B`)

func stripANSI(s string) string {
	return ansiCSI.ReplaceAllString(s, "")
}

type tuiChrome struct {
	panelLines        []string
	panelH            int
	sideActive        bool
	sideW             int
	sideX             int
	sideLines         []string
	bannerLines       int
	statusBlock       string
	statusH           int
	bottomNoticeBlock string
	bottomNoticeH     int
	modalBlock        string
	modalH            int
	inputBlock        string
	inputH            int
	inputY            int
	attachPanelBlock  string
	attachPanelH      int
	attachPanelY      int
	sugg              []suggestItem
	suggLines         int
	messagesHeight    int
	statusY           int
	chatSepH          int
}

func (m *AppModel) tuiChrome() tuiChrome {
	var c tuiChrome
	if m.authPhase == AuthNone && (!m.sessionLoading || strings.HasPrefix(strings.TrimSpace(m.slashSuggestLine()), "/")) {
		c.sugg = m.collectSuggestions()
	}
	if len(c.sugg) > 0 {
		limit := suggestionVisibleLimit(c.sugg)
		if len(c.sugg) < limit {
			limit = len(c.sugg)
		}
		c.suggLines = limit + 2
	}
	if m.sessionLoading {
		c.bannerLines = loadingBannerHeight
	} else if m.authNeedLogin && m.authPhase == AuthNone {
		c.bannerLines = 1
	}
	c.panelLines = nil // Task-311: no overlay — sidebar is the only panel surface
	c.panelH = len(c.panelLines)
	if m.useRightSidebar() {
		c.sideActive = true
		c.sideW = m.sideWidth()
		c.sideX = m.chatWidth()
		c.sideLines = m.renderRightSidebar(m.height)
		c.panelLines = nil
		c.panelH = 0
	}
	c.statusBlock = m.renderStatusLine()
	if strings.TrimSpace(c.statusBlock) == "" {
		c.statusBlock = ""
		c.statusH = 0
	} else {
		c.statusH = strings.Count(c.statusBlock, "\n") + 1
	}
	w := m.chatWidth()
	if w <= 0 {
		w = 80
	}
	// Bottom notice (Copied / input stalled) lives below the composer frame,
	// outside it — one small line at the bottom of the chat pane. The watchdog
	// warning is moved here so top-left chrome stays clean.
	c.bottomNoticeBlock = m.renderBottomNotice(w)
	if strings.TrimSpace(c.bottomNoticeBlock) == "" {
		c.bottomNoticeBlock = ""
		c.bottomNoticeH = 0
	} else {
		c.bottomNoticeH = strings.Count(c.bottomNoticeBlock, "\n") + 1
	}
	c.attachPanelBlock = m.renderAttachPanel(w)
	if c.attachPanelBlock != "" {
		c.attachPanelH = strings.Count(c.attachPanelBlock, "\n") + 1
	}
	// BUG-332: while the /mode-setup modal is open it renders between the
	// transcript and the attach panel (suggestions are suppressed). Its block
	// must be built ONCE here and its height MUST be part of the height
	// budget below — otherwise the frame overflows the terminal and the
	// modal's fields/buttons (and composer) get clipped off-screen.
	if m.modeSetupModalOpen {
		c.modalBlock = m.renderModeSetupModal(w)
		if c.modalBlock != "" {
			c.modalH = strings.Count(c.modalBlock, "\n") + 1
		}
	}
	c.inputBlock = m.renderInputLine()
	c.inputH = strings.Count(c.inputBlock, "\n") + 1
	// Blank line + rule always sit between the transcript and the status chrome.
	c.chatSepH = 2
	// statusH is kept for legacy callers but no longer occupies the main column
	// above the composer — the single visible bottom line is bottomNoticeH.
	c.messagesHeight = m.height - c.inputH - c.attachPanelH - c.suggLines - c.bannerLines - c.panelH - c.chatSepH - c.bottomNoticeH - c.modalH
	if c.messagesHeight < 1 {
		c.messagesHeight = 1
	}
	c.statusY = c.panelH + c.messagesHeight + c.bannerLines + c.chatSepH
	// With the modal open the suggestions block is NOT rendered — the modal
	// occupies that slot, so the mouse/cursor Y chain must step over modalH.
	c.attachPanelY = c.statusY + c.suggLines
	if m.modeSetupModalOpen {
		c.attachPanelY = c.statusY + c.modalH
	}
	c.inputY = c.attachPanelY + c.attachPanelH
	return c
}

func isLeftMouse(msg tea.MouseMsg) bool {
	return msg.Button == tea.MouseButtonLeft || msg.Type == tea.MouseLeft
}

func isLeftMouseClick(msg tea.MouseMsg) bool {
	if !isLeftMouse(msg) {
		return false
	}
	return msg.Action == tea.MouseActionPress || msg.Action == tea.MouseActionRelease || msg.Type == tea.MouseLeft
}

func pulseMouseTracking() tea.Cmd {
	// No-op. Mouse is already enabled via tuiProgramOpts WithMouseCellMotion.
	// Pulsing DisableMouse→EnableMouseCellMotion on every click or paste settle
	// dropped the next KeyMsg on Windows and bricked input for minutes
	// (CA-610: log 18216 29 min, 18700 19 min). Motion is filtered by
	// tuiMsgFilter, so the queue cannot fill. Return a no-op Batch member so
	// existing tests that assert Batch shape keep passing; the no-op does
	// nothing. See app.go tuiMsgFilter.
	return func() tea.Msg { return nil }
}

func (m *AppModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.authPhase != AuthNone {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.scrollTranscript(3)
		return m, nil
	case tea.MouseButtonWheelDown:
		m.scrollTranscript(-3)
		return m, nil
	}
	// Shift: copy selection (drag OR single click on a line). SGR 1006 encodes
	// drag motion and release with Button=None, so do NOT gate on isLeftMouse —
	// Terminal.app only delivers press/release reliably, not mid-drag motion.
	if msg.Shift {
		m.mouseDrag = mouseDrag{}
		switch msg.Action {
		case tea.MouseActionPress:
			m.mouseSel = mouseSelect{armed: true, x0: msg.X, y0: msg.Y, x1: msg.X, y1: msg.Y}
		case tea.MouseActionMotion, tea.MouseActionRelease:
			if m.mouseSel.armed {
				m.mouseSel.x1 = msg.X
				m.mouseSel.y1 = msg.Y
			}
			// Terminal.app never delivers Cmd+C, so releasing the drag copies the
			// selection (CA-515). Ctrl+C stays as a fallback elsewhere.
			if msg.Action == tea.MouseActionRelease {
				// Shift+click (zero-area selection, no drag motion) copies the
				// whole clicked line — the reliable copy affordance on terminals
				// that do not forward drag motion (macOS Terminal.app).
				if m.mouseSel.x0 == m.mouseSel.x1 && m.mouseSel.y0 == m.mouseSel.y1 {
					m.mouseSel.x0 = 0
					m.mouseSel.x1 = 1 << 20
				}
				return m, m.autoCopySelectionOnDragEnd()
			}
		}
		return m, nil
	}
	// Plain drag: motion may arrive with Button=None mid-drag (SGR 1006), so
	// handle it here rather than only on isLeftMouse.
	if msg.Action == tea.MouseActionMotion {
		if m.mouseDrag.down {
			return m.handlePlainLeftMouse(msg)
		}
		return m, nil
	}
	if isLeftMouse(msg) {
		return m.handlePlainLeftMouse(msg)
	}
	return m, nil
}

func (m *AppModel) handlePlainLeftMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	pulse := pulseMouseTracking()
	switch msg.Action {
	case tea.MouseActionMotion:
		if !m.mouseDrag.down {
			return m, nil
		}
		if msg.X != m.mouseDrag.x0 || msg.Y != m.mouseDrag.y0 {
			m.mouseDrag.moved = true
			m.mouseSel = mouseSelect{
				armed: true,
				x0:    m.mouseDrag.x0, y0: m.mouseDrag.y0,
				x1: msg.X, y1: msg.Y,
			}
		}
		return m, nil
	case tea.MouseActionRelease:
		dragged := m.mouseDrag.moved
		m.mouseDrag = mouseDrag{}
		if dragged {
			// Drag-select just ended: copy on release (Terminal.app swallows
			// Cmd+C, so the drag release is the reliable copy affordance, CA-515).
			return m, m.autoCopySelectionOnDragEnd()
		}
		// Click-release with no motion: same as before (clear leftover highlight).
		m.mouseSel = mouseSelect{}
		return m, pulse
	default:
		// Press, or Windows Type=MouseLeft with zero Action.
		// Click in the prompt body places the caret (Win + macOS cell motion).
		if m.tryPlaceInputCursor(msg.X, msg.Y) {
			m.mouseDrag = mouseDrag{}
			m.mouseSel = mouseSelect{}
			return m, pulse
		}
		m.mouseDrag = mouseDrag{}
		if msg.Action == tea.MouseActionPress && m.clickTargetAt(msg.X, msg.Y) == "" {
			m.mouseDrag = mouseDrag{down: true, x0: msg.X, y0: msg.Y}
		}
		m.mouseSel = mouseSelect{}
		if msg.Action == tea.MouseActionRelease {
			return m, pulse
		}
		m2, cmd := m.dispatchMouseClick(msg.X, msg.Y)
		return m2, tea.Batch(pulse, cmd)
	}
}

// selectionPlainText returns the visible transcript text under the current
// drag highlight (character columns, same range applyMouseSelection paints).
func (m *AppModel) selectionPlainText() string {
	if m.mouseSel.empty() {
		return ""
	}
	c := m.tuiChrome()
	lines := m.renderMessages()
	if !m.selecting() {
		m.clampViewport(len(lines), c.messagesHeight)
	}
	vis := sliceViewport(lines, c.messagesHeight, m.viewport.offset)
	var parts []string
	for i, line := range vis {
		y := c.panelH + i
		x0, x1, ok := m.mouseSel.colsOnLine(y)
		if !ok {
			continue
		}
		seg := extractVisualColumns(stripANSI(line), x0, x1)
		parts = append(parts, seg)
	}
	return strings.Join(parts, "\n")
}

// autoCopySelectionOnDragEnd copies the armed drag selection when the mouse is
// released. Terminal.app swallows Cmd+C for native copy and never delivers it to
// the TUI, so releasing the drag is the reliable "copy selection" affordance
// there (CA-515). The selection stays armed so Ctrl+C remains a working
// fallback (CA-480 keeps the highlight after release); the next press clears it.
// The CopiedMsg handler shows the "Copied selection." toast (CA-511).
func (m *AppModel) autoCopySelectionOnDragEnd() tea.Cmd {
	if m.mouseSel.empty() {
		return nil
	}
	text := m.selectionPlainText()
	if strings.TrimSpace(text) == "" {
		m.statusMsg = "nothing to copy"
		return nil
	}
	return m.cmdCopyText(text, "selection")
}

func (m *AppModel) dispatchMouseClick(x, y int) (tea.Model, tea.Cmd) {
	target := m.clickTargetAt(x, y)
	tuiLog("mouse click x=%d y=%d target=%q", x, y, target)
	if target == "" {
		return m, nil
	}
	if strings.HasPrefix(target, "agent-open:") {
		tuiLog("mouse agent-open runID=%s", strings.TrimPrefix(target, "agent-open:"))
	}
	return m.activateClickTarget(target)
}

// parseAttentionKey splits "runID/turnID".
func parseAttentionKey(key string) (runID, turnID string, ok bool) {
	i := strings.IndexByte(key, '/')
	if i < 0 {
		return "", "", false
	}
	return key[:i], key[i+1:], true
}

// parseAttentionResolve splits "runID/turnID:action".
func parseAttentionResolve(rest string) (runID, turnID, action string, ok bool) {
	key, act, found := strings.Cut(rest, ":")
	if !found {
		return "", "", "", false
	}
	r, t, ok := parseAttentionKey(key)
	return r, t, act, ok
}

// parseAttentionRepair splits "runID:action".
func parseAttentionRepair(rest string) (runID, action string, ok bool) {
	r, a, found := strings.Cut(rest, ":")
	if !found {
		return "", "", false
	}
	return r, a, true
}

func (m *AppModel) clickTargetAt(x, y int) string {
	if x < 0 || y < 0 {
		return ""
	}
	c := m.tuiChrome()
	if t := m.hitSidebarChrome(c, x, y); t != "" {
		return t
	}
	if t := m.hitSessionAgentChrome(c, x, y); t != "" {
		return t
	}
	if hitSessionPanel(c, x, y) {
		return "session"
	}
	if hitSkillsChrome(c, x, y) {
		return "skills"
	}
	if hitStopChrome(c, x, y) {
		return "stop"
	}
	// Agent open/back is only on F2 session/steps panel (hitSessionAgentChrome above).
	if hitStatusDetailsChrome(c, x, y) {
		return "status-details"
	}
	if t := m.hitApprovalChrome(c, x, y); t != "" {
		return t
	}
	if t := m.hitBlockedChrome(c, x, y); t != "" {
		return t
	}
	if t := hitGateChrome(m, c, x, y); t != "" {
		return t
	}
	if t := hitQuestionChrome(m, c, x, y); t != "" {
		return t
	}
	if t := m.hitAttentionChip(x, y); t != "" {
		return t
	}
	if action, idx := hitAttachPanelAction(c, x, y); idx > 0 {
		switch action {
		case "rm":
			return fmt.Sprintf("attach-rm:%d", idx)
		case "open":
			return fmt.Sprintf("attach-open:%d", idx)
		}
	}
	if hitAttachChrome(c, x, y) {
		return "attach"
	}
	if t := hitLoadEarlierChrome(m, c, x, y); t != "" {
		return t
	}
	if t := hitCopyChrome(m, c, x, y); t != "" {
		return t
	}
	if t := hitUserPromptChrome(m, c, x, y); t != "" {
		return t
	}
	if t := hitToolGroupChrome(m, c, x, y); t != "" {
		return t
	}
	return ""
}

// hitSidebarChrome maps a click in the full-height right sidebar (OpenCode-style,
// CA-524) to a target: step [open]/[back], [collapse], or the session header
// toggle. Only active when the sidebar is rendered (wide + expanded).
func (m *AppModel) hitSidebarChrome(c tuiChrome, x, y int) string {
	if !c.sideActive || y < 0 || y >= len(c.sideLines) {
		return ""
	}
	if x < c.sideX {
		return ""
	}
	// Sidebar lines are placed at absolute x>=sideX; hit-test against the line
	// itself by offsetting x to line-local coordinates.
	lx := x - c.sideX
	stripped := stripANSI(c.sideLines[y])
	if hitToken(stripped, "[back]", lx) && m.viewingChild() {
		return "agent-back"
	}
	if hitToken(stripped, "[open]", lx) {
		if runID := m.openRunIDFromPanelLine(stripped); runID != "" {
			return "agent-open:" + runID
		}
	}
	if hitToken(stripped, "[collapse]", lx) {
		return "sidebar-collapse"
	}
	// Session header row ("session") and steps header ("steps") toggle the panel.
	if hitToken(stripped, "session", lx) || hitToken(stripped, "steps", lx) {
		return "session"
	}
	return ""
}

func (m *AppModel) hitSessionAgentChrome(c tuiChrome, x, y int) string {
	if c.panelH == 0 || y < 0 || y >= c.panelH || y >= len(c.panelLines) {
		return ""
	}
	stripped := stripANSI(c.panelLines[y])
	if hitToken(stripped, "[back]", x) && m.viewingChild() {
		return "agent-back"
	}
	if hitToken(stripped, "[open]", x) {
		if runID := m.openRunIDFromPanelLine(stripped); runID != "" {
			return "agent-open:" + runID
		}
	}
	return ""
}

func (m *AppModel) openRunIDFromPanelLine(stripped string) string {
	if !strings.Contains(stripped, "[open]") {
		return ""
	}
	limit := len(m.flowSteps)
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		s := m.flowSteps[i]
		child, ok := m.childRunForStep(s)
		if !ok {
			continue
		}
		name := stepDisplayName(s)
		if name != "" && stepRowMatchesName(stripped, name) {
			return child.RunID
		}
	}
	return ""
}

// stepRowMatchesName matches a step row against a step display name, tolerating
// the "…" ellipsis that truncateStepLine appends when a long name/status row is
// squeezed (CA-528). A meaningful-prefix match keeps a truncated [open] row
// clickable.
func stepRowMatchesName(stripped, name string) bool {
	if strings.Contains(stripped, name) {
		return true
	}
	for i := len(name); i > 0; i-- {
		if i*2 < len(name) {
			break
		}
		if strings.Contains(stripped, name[:i]+"…") {
			return true
		}
	}
	return false
}

func hitSessionPanel(c tuiChrome, x, y int) bool {
	if c.panelH == 0 || y < 0 || y >= c.panelH || y >= len(c.panelLines) {
		return false
	}
	line := c.panelLines[y]
	vis := lipgloss.Width(line)
	content := lipgloss.Width(strings.TrimLeft(stripANSI(line), " "))
	if content <= 0 {
		return false
	}
	x0 := vis - content
	if x0 < 0 {
		x0 = 0
	}
	return x >= x0 && x < vis
}

func hitSkillsChrome(c tuiChrome, x, y int) bool {
	if c.statusH <= 0 {
		return false
	}
	lines := strings.Split(c.statusBlock, "\n")
	if len(lines) == 0 {
		return false
	}
	rel := y - c.statusY
	if rel < 0 || rel >= len(lines) {
		return false
	}
	stripped := stripANSI(lines[rel])
	if strings.HasPrefix(stripped, "  ") && rel > 0 {
		return true
	}
	i := strings.Index(stripped, "skills:")
	if i < 0 {
		return false
	}
	start := lipgloss.Width(stripped[:i])
	rest := stripped[i:]
	endRel := lipgloss.Width(rest)
	for _, sep := range []string{" │ ", " | "} {
		if j := strings.Index(rest, sep); j >= 0 {
			n := lipgloss.Width(rest[:j])
			if n < endRel {
				endRel = n
			}
		}
	}
	end := start + endRel
	return x >= start && x < end
}

func hitStatusDetailsChrome(c tuiChrome, x, y int) bool {
	if y != c.statusY {
		return false
	}
	if hitStopChrome(c, x, y) {
		return false
	}
	lines := strings.Split(c.statusBlock, "\n")
	if len(lines) == 0 {
		return false
	}
	return lipgloss.Width(stripANSI(lines[0])) > 0
}

func hitToken(stripped string, token string, x int) bool {
	i := strings.Index(stripped, token)
	if i < 0 {
		return false
	}
	start := lipgloss.Width(stripped[:i])
	end := start + lipgloss.Width(token)
	return x >= start && x < end
}

func hitStopChrome(c tuiChrome, x, y int) bool {
	// Status line is now empty (only toast) – [stop] lives in the input frame
	// header (top border) as part of chatFrameTitle. Check both places.
	if c.statusH > 0 && y == c.statusY {
		lines := strings.Split(c.statusBlock, "\n")
		if len(lines) > 0 && hitToken(stripANSI(lines[0]), "[stop]", x) {
			return true
		}
	}
	if c.inputH > 0 && y >= c.inputY && y < c.inputY+c.inputH {
		lines := strings.Split(c.inputBlock, "\n")
		if len(lines) > 0 && hitToken(stripANSI(lines[0]), "[stop]", x) {
			return true
		}
		// Fallback: also check any input line for [stop] (in case header wraps)
		for _, l := range lines {
			if hitToken(stripANSI(l), "[stop]", x) {
				return true
			}
		}
	}
	return false
}

func (m *AppModel) hitApprovalChrome(c tuiChrome, x, y int) string {
	checkLine := func(stripped string) string {
		if hitToken(stripped, "Approve all", x) {
			return "approve-all"
		}
		if hitToken(stripped, "Deny all", x) {
			return "deny-all"
		}
		if hitToken(stripped, "Approve forever", x) {
			return "approve-forever"
		}
		if m.approval != nil {
			for _, d := range m.approval.Decisions {
				if hitToken(stripped, d.Label, x) {
					return "adec:" + d.Value
				}
			}
		}
		if hitToken(stripped, "[Approve]", x) || hitToken(stripped, "Approve", x) || hitToken(stripped, "/approve", x) {
			return "approve"
		}
		if hitToken(stripped, "[Deny]", x) || hitToken(stripped, "Deny", x) || hitToken(stripped, "/deny", x) {
			return "deny"
		}
		return ""
	}

	if c.messagesHeight > 0 && y >= c.panelH && y < c.panelH+c.messagesHeight {
		rows := sliceChatRows(m.chatRows(), c.messagesHeight, m.viewport.offset)
		rel := y - c.panelH
		if rel >= 0 && rel < len(rows) {
			if res := checkLine(stripANSI(rows[rel].Text)); res != "" {
				return res
			}
		}
	}
	if c.inputH > 0 && y >= c.inputY && y < c.inputY+c.inputH {
		lines := strings.Split(c.inputBlock, "\n")
		rel := y - c.inputY
		if rel >= 0 && rel < len(lines) {
			if res := checkLine(stripANSI(lines[rel])); res != "" {
				return res
			}
		}
	}
	return ""
}

// hitBlockedChrome maps a click in the awaiting-user action bar to "retry" / "stop" / "allow".
func (m *AppModel) hitBlockedChrome(c tuiChrome, x, y int) string {
	if !m.flowLoopBlocked() {
		return ""
	}
	checkLine := func(stripped string, x int) string {
		if hitToken(stripped, "[Retry]", x) {
			return "retry"
		}
		if hitToken(stripped, "[Allow]", x) {
			return "allow"
		}
		if hitToken(stripped, "[Stop]", x) {
			return "stop"
		}
		// Alias for old tests / muscle memory.
		if hitToken(stripped, "[Continue]", x) {
			return "retry"
		}
		return ""
	}
	if c.messagesHeight > 0 && y >= c.panelH && y < c.panelH+c.messagesHeight {
		rows := sliceChatRows(m.chatRows(), c.messagesHeight, m.viewport.offset)
		rel := y - c.panelH
		if rel >= 0 && rel < len(rows) {
			stripped := stripANSI(rows[rel].Text)
			if t := checkLine(stripped, x); t != "" {
				return t
			}
		}
	}
	if c.inputH > 0 && y >= c.inputY && y < c.inputY+c.inputH {
		lines := strings.Split(c.inputBlock, "\n")
		rel := y - c.inputY
		if rel >= 0 && rel < len(lines) {
			stripped := stripANSI(lines[rel])
			if t := checkLine(stripped, x); t != "" {
				return t
			}
		}
	}
	return ""
}

// hitGateChrome maps a click on a gate decision chip ("[Fix code]", "[Suggest
// req]", "[Custom]") to "gopt:<option>" (CA-650). Only active while a gate
// card is armed; mirrors hitQuestionChrome.
func hitGateChrome(m *AppModel, c tuiChrome, x, y int) string {
	if m.gate == nil || len(m.gate.Options) == 0 {
		return ""
	}
	checkLine := func(stripped string) string {
		for _, opt := range m.gate.Options {
			if hitToken(stripped, gateOptionChip(opt), x) {
				return "gopt:" + opt
			}
		}
		return ""
	}
	if c.messagesHeight > 0 && y >= c.panelH && y < c.panelH+c.messagesHeight {
		rows := sliceChatRows(m.chatRows(), c.messagesHeight, m.viewport.offset)
		rel := y - c.panelH
		if rel >= 0 && rel < len(rows) {
			if res := checkLine(stripANSI(rows[rel].Text)); res != "" {
				return res
			}
		}
	}
	if c.inputH > 0 && y >= c.inputY && y < c.inputY+c.inputH {
		lines := strings.Split(c.inputBlock, "\n")
		rel := y - c.inputY
		if rel >= 0 && rel < len(lines) {
			if res := checkLine(stripANSI(lines[rel])); res != "" {
				return res
			}
		}
	}
	return ""
}

func hitQuestionChrome(m *AppModel, c tuiChrome, x, y int) string {
	if m.question == nil {
		return ""
	}
	checkLine := func(stripped string) string {
		if m.question.MultiSelect {
			if hitToken(stripped, "[Submit]", x) {
				return "qsubmit"
			}
			for i := range m.question.Options {
				label := questionOptionLabel(m.question.Options[i])
				if label != "" && hitToken(stripped, label, x) {
					return "qtoggle:" + strconv.Itoa(i)
				}
			}
			return ""
		}
		if hitToken(stripped, "Approve", x) || hitToken(stripped, "/approve", x) {
			return "approve"
		}
		if hitToken(stripped, "Deny", x) || hitToken(stripped, "/deny", x) {
			return "deny"
		}
		for i := range m.question.Options {
			token := strconv.Itoa(i+1) + ")"
			if hitToken(stripped, token, x) {
				return "qopt:" + strconv.Itoa(i)
			}
			label := questionOptionLabel(m.question.Options[i])
			if label != "" && hitToken(stripped, label, x) {
				return "qopt:" + strconv.Itoa(i)
			}
		}
		return ""
	}

	if c.messagesHeight > 0 && y >= c.panelH && y < c.panelH+c.messagesHeight {
		rows := sliceChatRows(m.chatRows(), c.messagesHeight, m.viewport.offset)
		rel := y - c.panelH
		if rel >= 0 && rel < len(rows) {
			if res := checkLine(stripANSI(rows[rel].Text)); res != "" {
				return res
			}
		}
	}
	if c.inputH > 0 && y >= c.inputY && y < c.inputY+c.inputH {
		lines := strings.Split(c.inputBlock, "\n")
		rel := y - c.inputY
		if rel >= 0 && rel < len(lines) {
			if res := checkLine(stripANSI(lines[rel])); res != "" {
				return res
			}
		}
	}
	return ""
}

func hitAttachChrome(c tuiChrome, x, y int) bool {
	if c.inputH <= 0 || y < c.inputY || y >= c.inputY+c.inputH {
		return false
	}
	lines := strings.Split(c.inputBlock, "\n")
	rel := y - c.inputY
	if rel < 0 || rel >= len(lines) {
		return false
	}
	stripped := stripANSI(lines[rel])
	// Full chip "[N img]" / legacy "[+img]" — not only the leading "[" (was a 1-cell hit).
	return hitAttachChipToken(stripped, x)
}

// hitAttachChipToken is true when x lands on (or within 1 col of) the pending
// image chip substring, e.g. "[2 img] " or "[+img]".
func hitAttachChipToken(stripped string, x int) bool {
	if hitTokenPadded(stripped, "[+img]", x, 1) {
		return true
	}
	// Find "[<digits> img]" — chip is always ASCII.
	start := strings.Index(stripped, "[")
	for start >= 0 {
		rest := stripped[start:]
		endRel := strings.Index(rest, " img]")
		if endRel < 0 {
			break
		}
		// Verify digits between [ and " img]"
		mid := rest[1:endRel]
		okDigits := len(mid) > 0
		for _, r := range mid {
			if r < '0' || r > '9' {
				okDigits = false
				break
			}
		}
		if okDigits {
			token := rest[:endRel+len(" img]")]
			if hitTokenPadded(stripped, token, x, 1) {
				return true
			}
		}
		next := strings.Index(stripped[start+1:], "[")
		if next < 0 {
			break
		}
		start = start + 1 + next
	}
	return false
}

// hitTokenPadded is hitToken with ±pad visual columns for easier mouse hits.
func hitTokenPadded(stripped, token string, x, pad int) bool {
	i := strings.Index(stripped, token)
	if i < 0 {
		return false
	}
	if pad < 0 {
		pad = 0
	}
	start := lipgloss.Width(stripped[:i]) - pad
	if start < 0 {
		start = 0
	}
	end := lipgloss.Width(stripped[:i]) + lipgloss.Width(token) + pad
	return x >= start && x < end
}

func hitLoadEarlierChrome(m *AppModel, c tuiChrome, x, y int) string {
	rows := sliceChatRows(m.chatRows(), c.messagesHeight, m.viewport.offset)
	rel := y - c.panelH
	if rel < 0 || rel >= len(rows) {
		return ""
	}
	if !rows[rel].LoadEarlier {
		return ""
	}
	stripped := stripANSI(rows[rel].Text)
	if strings.Contains(stripped, "Load earlier prompts") {
		return "load-earlier"
	}
	return ""
}

func hitCopyChrome(m *AppModel, c tuiChrome, x, y int) string {
	rows := sliceChatRows(m.chatRows(), c.messagesHeight, m.viewport.offset)
	rel := y - c.panelH
	if rel < 0 || rel >= len(rows) {
		return ""
	}
	if !rows[rel].Copy {
		return ""
	}
	stripped := stripANSI(rows[rel].Text)
	if !hitToken(stripped, "[copy]", x) {
		return ""
	}
	if rows[rel].CopyText != "" {
		return "copyfence:" + strconv.Itoa(rows[rel].MsgIdx) + ":" + strconv.Itoa(rows[rel].FenceIdx)
	}
	return "copy:" + strconv.Itoa(rows[rel].MsgIdx)
}

// hitUserPromptChrome maps a click on any row of a user prompt bubble that
// exceeds the 4-line clamp (CA-559/CA-607) to a "user-prompt-expand:<content>"
// toggle target. The whole box is clickable (every youBox row carries the key);
// the [copy] chip is hit-tested first (hitCopyChrome), so clicking the chip
// copies instead of toggling.
func hitUserPromptChrome(m *AppModel, c tuiChrome, x, y int) string {
	rows := sliceChatRows(m.chatRows(), c.messagesHeight, m.viewport.offset)
	rel := y - c.panelH
	if rel < 0 || rel >= len(rows) {
		return ""
	}
	if rows[rel].PromptExpandKey == "" {
		return ""
	}
	return "user-prompt-expand:" + rows[rel].PromptExpandKey
}

// hitToolGroupChrome maps a click on a collapsed/expanded multi-tool summary row
// (CA-525) to a "tool-group:<key>" toggle target. Individual → tool lines inside
// an expanded group carry no key, so only the summary row toggles.
func hitToolGroupChrome(m *AppModel, c tuiChrome, x, y int) string {
	rows := sliceChatRows(m.chatRows(), c.messagesHeight, m.viewport.offset)
	rel := y - c.panelH
	if rel < 0 || rel >= len(rows) {
		return ""
	}
	if rows[rel].ToolGroupKey == "" {
		return ""
	}
	return "tool-group:" + rows[rel].ToolGroupKey
}

func parseCopyFenceTarget(target string) (msgIdx, fenceIdx int, ok bool) {
	rest := strings.TrimPrefix(target, "copyfence:")
	parts := strings.Split(rest, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	var err error
	msgIdx, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	fenceIdx, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return msgIdx, fenceIdx, true
}

func applyMouseSelection(lines []string, sel mouseSelect, yOff int) []string {
	if sel.empty() || len(lines) == 0 {
		return lines
	}
	out := append([]string(nil), lines...)
	for i := range out {
		y := yOff + i
		x0, x1, ok := sel.colsOnLine(y)
		if !ok {
			continue
		}
		out[i] = highlightVisualColumns(stripANSI(out[i]), x0, x1)
	}
	return out
}

// colsOnLine returns inclusive display-column bounds for absolute row y.
// Multi-line ranges select from x0→EOL on the first row, full middle rows,
// and BOL→x1 on the last row (standard text selection).
func (s mouseSelect) colsOnLine(y int) (x0, x1 int, ok bool) {
	if s.empty() {
		return 0, 0, false
	}
	y0, y1 := s.y0, s.y1
	xa, xb := s.x0, s.x1
	if y0 > y1 || (y0 == y1 && xa > xb) {
		y0, y1 = y1, y0
		xa, xb = xb, xa
	}
	if y < y0 || y > y1 {
		return 0, 0, false
	}
	const toEOL = 1 << 20
	if y0 == y1 {
		return xa, xb, true
	}
	if y == y0 {
		return xa, toEOL, true
	}
	if y == y1 {
		return 0, xb, true
	}
	return 0, toEOL, true
}

// highlightVisualColumns reverse-styles the inclusive cell range [x0, x1]
// on a plain (ANSI-stripped) line. Wide runes count by lipgloss width.
func highlightVisualColumns(plain string, x0, x1 int) string {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if x1 < 0 {
		return plain
	}
	if x0 < 0 {
		x0 = 0
	}
	var b, mid strings.Builder
	flush := func() {
		if mid.Len() == 0 {
			return
		}
		b.WriteString(styleSelect.Render(mid.String()))
		mid.Reset()
	}
	col := 0
	for _, r := range plain {
		w := lipgloss.Width(string(r))
		if w < 1 {
			w = 1
		}
		// Rune covers cells [col, col+w). Selected if it overlaps [x0, x1].
		sel := col <= x1 && col+w-1 >= x0
		if sel {
			mid.WriteRune(r)
		} else {
			flush()
			b.WriteRune(r)
		}
		col += w
	}
	flush()
	return b.String()
}

// extractVisualColumns returns the plain substring covering cells [x0, x1].
func extractVisualColumns(plain string, x0, x1 int) string {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if x1 < 0 {
		return ""
	}
	if x0 < 0 {
		x0 = 0
	}
	var b strings.Builder
	col := 0
	for _, r := range plain {
		w := lipgloss.Width(string(r))
		if w < 1 {
			w = 1
		}
		if col <= x1 && col+w-1 >= x0 {
			b.WriteRune(r)
		}
		col += w
		if col > x1 {
			break
		}
	}
	return b.String()
}
