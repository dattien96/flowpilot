package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// youBoxLayoutRev stamps the You-box renderer revision into the /dumpview
// output so a stale binary is obvious before debugging layout (CA-606). Bump
// whenever youBox / the paint path changes.
const youBoxLayoutRev = "yb607"

// lastBoxBarCol returns the column of the rightmost box glyph (│ ┐ ┘ | +) in
// plain runes, or -1. Production copy of the test helper lastBarCol.
func lastBoxBarCol(rs []rune) int {
	for i := len(rs) - 1; i >= 0; i-- {
		switch rs[i] {
		case '│', '┐', '┘', '|', '+':
			return i
		}
	}
	return -1
}

// writeYouViewDump writes a diagnostic snapshot of the live View(): layout
// numbers, raw ANSI, plain text, and the You-box rows cut to chatW with their
// right-border column. The operator runs /dumpview after reproducing a broken
// prompt box and shares the file — no more guessing from screenshots.
func writeYouViewDump(m *AppModel, path string) error {
	var sb strings.Builder
	chatW := m.chatWidth()
	sb.WriteString(fmt.Sprintf("rev=%s\n", youBoxLayoutRev))
	sb.WriteString(fmt.Sprintf("pid=%d\n", os.Getpid()))
	sb.WriteString(fmt.Sprintf("width=%d height=%d\n", m.width, m.height))
	sb.WriteString(fmt.Sprintf("chatW=%d sideW=%d sidebar=%v\n", chatW, m.sideWidth(), m.useRightSidebar()))
	sb.WriteString(fmt.Sprintf("colorProfile=%s\n", lipgloss.ColorProfile().String()))
	view := m.View()
	sb.WriteString("--- raw ---\n")
	sb.WriteString(view)
	sb.WriteString("\n--- plain ---\n")
	sb.WriteString(stripANSI(view))
	sb.WriteString("\n--- you-box chat ---\n")
	for _, line := range strings.Split(stripANSI(view), "\n") {
		if len([]rune(line)) > chatW {
			line = string([]rune(line)[:chatW])
		}
		trim := strings.TrimSpace(line)
		if strings.Contains(trim, "You") || strings.HasPrefix(trim, "┌") || strings.HasPrefix(trim, "└") || strings.HasPrefix(trim, "│") || strings.Contains(line, "[copy]") {
			sb.WriteString(fmt.Sprintf("bar=%d |%s|\n", lastBoxBarCol([]rune(line)), line))
		}
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}