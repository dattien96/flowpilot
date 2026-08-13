package app

import (
	"regexp"
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
	sugg           []suggestItem
	suggLines      int
	messagesHeight int
	statusY        int
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
	c.messagesHeight = m.height - c.statusH - 2 - c.suggLines - c.bannerLines - c.panelH
	if c.messagesHeight < 1 {
		c.messagesHeight = 1
	}
	c.statusY = c.panelH + c.messagesHeight + c.bannerLines
	return c
}

func (m *AppModel) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.authPhase != AuthNone {
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	switch m.clickTargetAt(msg.X, msg.Y) {
	case "session":
		m.sessionPanel.Collapsed = !m.sessionPanel.Collapsed
	case "skills":
		if len(attachedSkillNames(m.selectedSkills)) == 0 {
			return m, nil
		}
		m.statusSkillsExpanded = !m.statusSkillsExpanded
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
	stripped := stripANSI(lines[0])
	i := strings.Index(stripped, "skills:")
	if i < 0 {
		return false
	}
	if rel == 0 {
		start := lipgloss.Width(stripped[:i])
		rest := stripped[i:]
		endRel := len([]rune(rest))
		for _, sep := range []string{" │ ", " | "} {
			if j := strings.Index(rest, sep); j >= 0 {
				n := len([]rune(rest[:j]))
				if n < endRel {
					endRel = n
				}
			}
		}
		end := start + endRel
		return x >= start && x < end
	}
	// Expanded name rows sit between line1 and the project row (last line).
	return rel > 0 && rel < len(lines)-1
}
