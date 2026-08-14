package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
)

// pendingTempRoot holds materialized pending images so detach can unlink files
// immediately (Desktop chip × parity). Separate from Grok turn-time .tmp/images.
func pendingTempRoot() string {
	return filepath.Join(os.TempDir(), "flowpilot-tui-pending")
}

// materializePendingAttachment writes attachment bytes under the pending temp
// dir and returns the absolute path (best-effort; empty path on failure).
func materializePendingAttachment(att client.PromptAttachment) string {
	raw, err := decodeAttachmentData(att.Data)
	if err != nil || len(raw) == 0 {
		return ""
	}
	if err := os.MkdirAll(pendingTempRoot(), 0o700); err != nil {
		return ""
	}
	ext := ".png"
	if strings.Contains(strings.ToLower(att.MimeType), "jpeg") || strings.Contains(strings.ToLower(att.MimeType), "jpg") {
		ext = ".jpg"
	}
	base := strings.TrimSpace(att.OriginalName)
	if base == "" {
		base = "image" + ext
	} else if filepath.Ext(base) == "" {
		base += ext
	}
	// Unique file per attachment id so two clipboard.png do not collide.
	safeID := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, att.ID)
	if safeID == "" {
		safeID = "att"
	}
	path := filepath.Join(pendingTempRoot(), safeID+"_"+filepath.Base(base))
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func (m *AppModel) trackPendingLocalPath(att client.PromptAttachment) {
	if m.pendingLocalPaths == nil {
		m.pendingLocalPaths = map[string]string{}
	}
	if p := materializePendingAttachment(att); p != "" {
		m.pendingLocalPaths[att.ID] = p
	}
}

// removePendingAttachment removes the 1-based index from pendingAttach and
// deletes its local temp file when present.
func (m *AppModel) removePendingAttachment(index1 int) (name string, ok bool) {
	if index1 < 1 || index1 > len(m.pendingAttach) {
		return "", false
	}
	att := m.pendingAttach[index1-1]
	name = att.OriginalName
	if name == "" {
		name = fmt.Sprintf("image-%d", index1)
	}
	m.unlinkPendingLocal(att.ID)
	m.pendingAttach = append(m.pendingAttach[:index1-1], m.pendingAttach[index1:]...)
	if len(m.pendingAttach) == 0 {
		m.attachPanelOpen = false
	}
	return name, true
}

func (m *AppModel) clearPendingAttachments() int {
	n := len(m.pendingAttach)
	for _, att := range m.pendingAttach {
		m.unlinkPendingLocal(att.ID)
	}
	m.pendingAttach = nil
	m.pendingLocalPaths = nil
	m.attachPanelOpen = false
	return n
}

func (m *AppModel) unlinkPendingLocal(id string) {
	if m.pendingLocalPaths == nil {
		return
	}
	if p := m.pendingLocalPaths[id]; p != "" {
		_ = os.Remove(p)
		delete(m.pendingLocalPaths, id)
	}
}

func (m *AppModel) appendPendingAttachment(att client.PromptAttachment) {
	m.pendingAttach = append(m.pendingAttach, att)
	m.trackPendingLocalPath(att)
}

func (m *AppModel) openAttachPanel() {
	if len(m.pendingAttach) == 0 {
		m.attachPanelOpen = false
		return
	}
	m.attachPanelOpen = true
}

func (m *AppModel) closeAttachPanel() {
	m.attachPanelOpen = false
}

// renderAttachPanel is a modal-style list above the prompt (Desktop attachment chips).
func (m *AppModel) renderAttachPanel(width int) string {
	if !m.attachPanelOpen || len(m.pendingAttach) == 0 {
		return ""
	}
	if width < 20 {
		width = 20
	}
	var sb strings.Builder
	// Avoid raw "[x]" / "[open]" in the title — those tokens are hit-tested on rows.
	title := fmt.Sprintf(" pending images (%d/6) · open or X · Esc close · Alt+V paste ", len(m.pendingAttach))
	if m.asciiMode {
		sb.WriteString(styleSystem.Render("+" + strings.Repeat("-", width-2) + "+"))
		sb.WriteByte('\n')
		sb.WriteString(styleSystem.Render("|"))
		sb.WriteString(stylePromptFocus.Render(truncateVisual(title, width-2)))
		sb.WriteString(styleSystem.Render("|"))
		sb.WriteByte('\n')
	} else {
		sb.WriteString(styleSystem.Render("╭" + strings.Repeat("─", width-2) + "╮"))
		sb.WriteByte('\n')
		sb.WriteString(styleSystem.Render("│"))
		sb.WriteString(stylePromptFocus.Render(truncateVisual(title, width-2)))
		sb.WriteString(styleSystem.Render("│"))
		sb.WriteByte('\n')
	}
	for i, a := range m.pendingAttach {
		name := a.OriginalName
		if name == "" {
			name = fmt.Sprintf("image-%d", i+1)
		}
		meta := fmt.Sprintf("%s  %dx%d  %s", a.MimeType, a.Width, a.Height, humanBytes(a.SizeBytes))
		row := fmt.Sprintf(" %d. %s  %s  ", i+1, name, meta)
		// Trailing [open] [x] — open preview / remove (two ways to open: panel + /image open picker).
		actions := "[open] [x] "
		bodyW := width - 2 - lipgloss.Width(actions)
		if bodyW < 8 {
			bodyW = 8
		}
		line := truncateVisual(row, bodyW)
		pad := bodyW - lipgloss.Width(line)
		if pad < 0 {
			pad = 0
		}
		if m.asciiMode {
			sb.WriteString(styleSystem.Render("|"))
		} else {
			sb.WriteString(styleSystem.Render("│"))
		}
		sb.WriteString(styleInputFocus.Render(line + strings.Repeat(" ", pad)))
		sb.WriteString(styleLink.Render("[open]"))
		sb.WriteString(" ")
		sb.WriteString(styleLink.Render("[x]"))
		sb.WriteString(" ")
		if m.asciiMode {
			sb.WriteString(styleSystem.Render("|"))
		} else {
			sb.WriteString(styleSystem.Render("│"))
		}
		sb.WriteByte('\n')
	}
	if m.asciiMode {
		sb.WriteString(styleSystem.Render("+" + strings.Repeat("-", width-2) + "+"))
	} else {
		sb.WriteString(styleSystem.Render("╰" + strings.Repeat("─", width-2) + "╯"))
	}
	return sb.String()
}

func humanBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%dKB", n/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
}

// hitAttachPanelAction returns ("open"|"rm", 1-based index) for a data-row click.
// [open] / name body → open preview; [x] → remove. Title/border → ("", 0).
func hitAttachPanelAction(c tuiChrome, x, y int) (action string, idx int) {
	if c.attachPanelH <= 0 || y < c.attachPanelY || y >= c.attachPanelY+c.attachPanelH {
		return "", 0
	}
	lines := strings.Split(c.attachPanelBlock, "\n")
	rel := y - c.attachPanelY
	if rel < 2 || rel >= len(lines) {
		return "", 0
	}
	if rel == len(lines)-1 {
		return "", 0
	}
	idx = rel - 1 // rel=2 → image #1
	if idx < 1 {
		return "", 0
	}
	stripped := stripANSI(lines[rel])
	if strings.TrimSpace(strings.Trim(stripped, "|│╭╮╰╯-+─")) == "" {
		return "", 0
	}
	if x < 1 {
		return "", 0
	}
	// Prefer explicit action chips (padded for easy hits).
	if hitTokenPadded(stripped, "[x]", x, 2) {
		return "rm", idx
	}
	if hitTokenPadded(stripped, "[open]", x, 2) {
		return "open", idx
	}
	// Rest of the row body → open preview (name/meta click).
	vis := lipgloss.Width(stripped)
	if x >= 1 && x < vis {
		return "open", idx
	}
	return "", 0
}

// hitAttachPanelRemove kept for tests — remove chip only.
func hitAttachPanelRemove(c tuiChrome, x, y int) int {
	action, idx := hitAttachPanelAction(c, x, y)
	if action == "rm" {
		return idx
	}
	return 0
}
