package app

import (
	"regexp"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func stripANSI(s string) string {
	return ansiCSI.ReplaceAllString(s, "")
}

type tuiChrome struct {
	panelLines     []string
	panelH         int
	bannerLines    int
	statusBlock    string
	statusH        int
	inputBlock     string
	inputH         int
	inputY         int
	sugg           []suggestItem
	suggLines      int
	messagesHeight int
	statusY        int
	chatSepH       int
}

func (m *AppModel) tuiChrome() tuiChrome {
	var c tuiChrome
	if m.authPhase == AuthNone && (!m.sessionLoading || strings.HasPrefix(m.inputValue, "/")) {
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
	c.panelLines = m.renderSessionPanelOverlay()
	c.panelH = len(c.panelLines)
	c.statusBlock = m.renderStatusLine()
	c.statusH = strings.Count(c.statusBlock, "\n") + 1
	c.inputBlock = m.renderInputLine()
	c.inputH = strings.Count(c.inputBlock, "\n") + 1
	// Blank line + rule always sit between the transcript and the status chrome.
	c.chatSepH = 2
	c.messagesHeight = m.height - c.statusH - c.inputH - c.suggLines - c.bannerLines - c.panelH - c.chatSepH
	if c.messagesHeight < 1 {
		c.messagesHeight = 1
	}
	c.statusY = c.panelH + c.messagesHeight + c.bannerLines + c.chatSepH
	c.inputY = c.statusY + c.statusH + c.suggLines
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
	// Drop Windows Terminal native selection (Shift+drag) by briefly leaving mouse mode.
	return tea.Sequence(tea.DisableMouse, tea.EnableMouseCellMotion)
}

func (m *AppModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.authPhase != AuthNone {
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp {
		m.scrollTranscript(3)
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		m.scrollTranscript(-3)
		return m, nil
	}
	if msg.Shift && isLeftMouse(msg) {
		m.mouseDrag = mouseDrag{}
		switch msg.Action {
		case tea.MouseActionPress:
			m.mouseSel = mouseSelect{armed: true, x0: msg.X, y0: msg.Y, x1: msg.X, y1: msg.Y}
		case tea.MouseActionMotion, tea.MouseActionRelease:
			if m.mouseSel.armed {
				m.mouseSel.x1 = msg.X
				m.mouseSel.y1 = msg.Y
			}
		}
		return m, nil
	}
	if isLeftMouse(msg) && !msg.Shift {
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
			return m, nil
		}
		// Click-release with no motion: same as before (clear leftover highlight).
		m.mouseSel = mouseSelect{}
		return m, pulse
	default:
		// Press, or Windows Type=MouseLeft with zero Action.
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

func (m *AppModel) dispatchMouseClick(x, y int) (tea.Model, tea.Cmd) {
	target := m.clickTargetAt(x, y)
	switch {
	case target == "session":
		m.sessionPanel.Collapsed = !m.sessionPanel.Collapsed
	case target == "skills":
		if len(attachedSkillNames(m.selectedSkills)) == 0 {
			return m, nil
		}
		m.statusSkillsExpanded = !m.statusSkillsExpanded
	case target == "status-details":
		m.statusDetailsCollapsed = !m.statusDetailsCollapsed
	case target == "stop":
		if m.turnIsActive() {
			return m, m.cmdStopTurn()
		}
	case target == "approve":
		if m.approval != nil {
			return m.submitPendingApproval("approve")
		}
		if m.question != nil {
			return m.submitQuestionAnswer("approve")
		}
	case target == "deny":
		if m.approval != nil {
			return m.submitPendingApproval("deny")
		}
		if m.question != nil {
			return m.submitQuestionAnswer("deny")
		}
	case target == "attach":
		return m, m.cmdClipboardPaste()
	case strings.HasPrefix(target, "qopt:"):
		idx, err := strconv.Atoi(strings.TrimPrefix(target, "qopt:"))
		if err == nil && m.question != nil && idx >= 0 && idx < len(m.question.Options) {
			return m.submitQuestionAnswer(questionOptionToken(m.question.Options[idx]))
		}
	case strings.HasPrefix(target, "copyfence:"):
		msgIdx, fenceIdx, ok := parseCopyFenceTarget(target)
		if ok {
			return m, m.cmdCopyFence(msgIdx, fenceIdx)
		}
	case strings.HasPrefix(target, "copy:"):
		idx, err := strconv.Atoi(strings.TrimPrefix(target, "copy:"))
		if err == nil {
			return m, m.cmdCopyMessage(idx)
		}
	case target == "load-earlier":
		return m, m.loadEarlierPrompts()
	}
	return m, nil
}

func (m *AppModel) clickTargetAt(x, y int) string {
	if x < 0 || y < 0 {
		return ""
	}
	c := m.tuiChrome()
	if hitSessionPanel(c, x, y) {
		return "session"
	}
	if hitSkillsChrome(c, x, y) {
		return "skills"
	}
	if hitStopChrome(c, x, y) {
		return "stop"
	}
	if hitStatusDetailsChrome(c, x, y) {
		return "status-details"
	}
	if t := hitApprovalChrome(c, x, y); t != "" {
		return t
	}
	if t := hitQuestionChrome(m, c, x, y); t != "" {
		return t
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
	return ""
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
	if y != c.statusY {
		return false
	}
	lines := strings.Split(c.statusBlock, "\n")
	if len(lines) == 0 {
		return false
	}
	return hitToken(stripANSI(lines[0]), "[stop]", x)
}

func hitApprovalChrome(c tuiChrome, x, y int) string {
	if c.inputH <= 0 || y < c.inputY || y >= c.inputY+c.inputH {
		return ""
	}
	lines := strings.Split(c.inputBlock, "\n")
	rel := y - c.inputY
	if rel < 0 || rel >= len(lines) {
		return ""
	}
	stripped := stripANSI(lines[rel])
	if hitToken(stripped, "Approve", x) || hitToken(stripped, "/approve", x) {
		return "approve"
	}
	if hitToken(stripped, "Deny", x) || hitToken(stripped, "/deny", x) {
		return "deny"
	}
	return ""
}

func hitQuestionChrome(m *AppModel, c tuiChrome, x, y int) string {
	if m.question == nil || c.inputH <= 0 || y < c.inputY || y >= c.inputY+c.inputH {
		return ""
	}
	lines := strings.Split(c.inputBlock, "\n")
	rel := y - c.inputY
	if rel < 0 || rel >= len(lines) {
		return ""
	}
	stripped := stripANSI(lines[rel])
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
	if hitToken(stripped, "[+img]", x) {
		return true
	}
	return strings.Contains(stripped, " img]") && hitToken(stripped, "[", x)
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
	y0, y1 := sel.y0, sel.y1
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	out := append([]string(nil), lines...)
	for i := range out {
		y := yOff + i
		if y < y0 || y > y1 {
			continue
		}
		out[i] = styleSelect.Render(stripANSI(out[i]))
	}
	return out
}
