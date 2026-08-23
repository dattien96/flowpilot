package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func boxGlyphs(ascii bool) (tl, tr, bl, br, h, v string) {
	if ascii {
		return "+", "+", "+", "+", "-", "|"
	}
	return "┌", "┐", "└", "┘", "─", "│"
}

func truncateVisual(s string, width int) string {
	if width < 1 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	plain := stripANSI(s)
	if width == 1 {
		for _, ch := range plain {
			if lipgloss.Width(string(ch)) <= 1 {
				return string(ch)
			}
			break
		}
		return "…"
	}
	target := width - lipgloss.Width("…")
	if target < 1 {
		return "…"
	}
	var b strings.Builder
	n := 0
	for _, ch := range plain {
		cw := lipgloss.Width(string(ch))
		if n+cw > target {
			break
		}
		b.WriteRune(ch)
		n += cw
	}
	return b.String() + "…"
}

// safeTermWidth leaves the last column empty so Windows Terminal does not
// wrap a full-width row (right-border `|` falling onto the next line).
func safeTermWidth(w int) int {
	if w <= 1 {
		return w
	}
	return w - 1
}

func padVisual(s string, width int) string {
	s = truncateVisual(s, width)
	n := lipgloss.Width(s)
	if n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

func strokeTop(title string, boxW int, ascii bool) string {
	return strokeTopChip(title, "", boxW, ascii)
}

func strokeTopChip(title, chip string, boxW int, ascii bool) string {
	tl, tr, _, _, h, _ := boxGlyphs(ascii)
	innerW := boxW - 2
	if innerW < 4 {
		innerW = 4
	}
	title = strings.TrimSpace(title)
	if title != "" {
		title = " " + title + " "
	}
	tw := lipgloss.Width(title)
	cw := lipgloss.Width(chip)
	if tw+cw > innerW {
		room := innerW - cw
		if room < 0 {
			chip = truncateVisual(chip, innerW)
			cw = lipgloss.Width(chip)
			room = innerW - cw
		}
		if room < 0 {
			room = 0
		}
		if tw > room {
			title = truncateVisual(title, room)
			tw = lipgloss.Width(title)
		}
	}
	fill := innerW - tw - cw
	if fill < 0 {
		fill = 0
	}
	return tl + title + strings.Repeat(h, fill) + chip + tr
}

func strokeBottom(boxW int, ascii bool) string {
	_, _, bl, br, h, _ := boxGlyphs(ascii)
	innerW := boxW - 2
	if innerW < 4 {
		innerW = 4
	}
	return bl + strings.Repeat(h, innerW) + br
}

func roundGlyphs(ascii bool) (tl, tr, bl, br, h, v string) {
	if ascii {
		return "+", "+", "+", "+", "-", "|"
	}
	return "╭", "╮", "╰", "╯", "─", "│"
}

func padVisualANSI(s string, width int) string {
	n := len([]rune(stripANSI(s)))
	if n > width {
		return truncateVisual(s, width)
	}
	if n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// paintRow pads s to width and paints the whole row with the given background.
// The text keeps any inner background (e.g. code panels), and the trailing
// padding is emitted as its own styled segment so a lipgloss reset inside the
// styled text cannot leak the terminal background behind the row (CA-532).
// Measure visual width on stripANSI plain (rune count) so Ghostty's SGR colon
// handling (38;2;R;G;B) does not miscount and leave the narrow You box short
// (CA-593). Check again after Render so a styled SGR never makes raw wider.
func paintRow(s string, width int, st lipgloss.Style) string {
	if width < 1 {
		return st.Render(s)
	}
	if len([]rune(stripANSI(s))) > width {
		s = truncateVisual(s, width)
	}
	out := st.Render(s)
	n := len([]rune(stripANSI(out)))
	if n > width {
		return truncateVisual(out, width)
	} else if n < width {
		out += st.Render(strings.Repeat(" ", width-n))
	}
	return out
}

// isYouBoxRow reports whether a chat row is a You-box border/content line
// (CA-605). These rows must never be wrapped in styleCanvas.Render: Ghostty /
// cellbuf count truecolor SGR (38;2;R;G;B) differently from rune counts, which
// wrapped the full-width box mid-pane and hid prompt lines.
func isYouBoxRow(s string) bool {
	p := stripANSI(s)
	if p == "" {
		return false
	}
	switch []rune(p)[0] {
	case '┌', '│', '└', '+', '|':
		return true
	}
	return false
}

// padYouBoxRow pads a You-box row with the canvas background WITHOUT styling
// the box glyphs/text (CA-605). The canvas fill comes from cellbuf.Fill / the
// trailing pad segment, so the row never carries a leading SGR wrap.
func padYouBoxRow(s string, width int) string {
	if width < 1 {
		return s
	}
	n := len([]rune(stripANSI(s)))
	if n > width {
		return truncateVisual(s, width)
	}
	if n < width {
		return s + styleCanvas.Render(strings.Repeat(" ", width-n))
	}
	return s
}

func frameInput(lines []string, width int, title, footer string, ascii bool) string {
	if width < 1 {
		width = 1
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	tl, tr, bl, br, h, v := roundGlyphs(ascii)
	innerW := width - 2
	if innerW < 1 {
		innerW = 1
	}
	title = strings.TrimSpace(title)
	if title != "" {
		title = " " + title + " "
	}
	tRunes := []rune(title)
	if len(tRunes) > innerW {
		title = string(tRunes[:innerW])
		tRunes = []rune(title)
	}
	fill := innerW - len(tRunes)
	if fill < 0 {
		fill = 0
	}
	var b strings.Builder
	write := func(s string) {
		b.WriteString(truncateVisual(s, width))
	}
	write(tl + title + strings.Repeat(h, fill) + tr)
	for _, line := range lines {
		b.WriteByte('\n')
		write(v + padVisualANSI(" "+line, innerW) + v)
	}
	foot := strings.TrimSpace(footer)
	if foot != "" {
		foot = " " + foot + " "
		fRunes := []rune(foot)
		if len(fRunes) > innerW {
			foot = string(fRunes[:innerW])
			fRunes = []rune(foot)
		}
		lead := innerW - len(fRunes)
		if lead < 0 {
			lead = 0
		}
		b.WriteByte('\n')
		write(bl + strings.Repeat(h, lead) + foot + br)
		return b.String()
	}
	b.WriteByte('\n')
	write(bl + strings.Repeat(h, innerW) + br)
	return b.String()
}

// youBox frames a user prompt as a full chat-pane-width box (CA-602). Plain
// runes only — no styled spans, no ANSI-aware padding — so every row's right
// border sits at exactly width-1 on any terminal. Lines arrive pre-wrapped at
// the box inner text width and pre-clamped (CA-607: max 4 lines + "...." tail
// when collapsed). The [copy] chip rides its own last row (CA-604): content
// rows never share a row with the chip, so prompt text is never cut to make
// room for it. When truncatable, every row (borders included) carries
// PromptExpandKey so the whole box is a click-to-expand target.
func youBox(lines []string, width int, ascii bool, copyOn bool, msgIdx int, truncatable bool, expandKey string) []chatRow {
	if width < 10 {
		width = 10
	}
	innerW := width - 2
	if innerW < 4 {
		innerW = 4
	}
	row := func(s string, isCopy bool) chatRow {
		r := chatRow{Text: s, MsgIdx: msgIdx, Copy: isCopy}
		if truncatable {
			r.PromptExpandKey = expandKey
		}
		return r
	}
	body := func(line string) string {
		b := " " + line
		if len([]rune(b)) < innerW {
			b += strings.Repeat(" ", innerW-len([]rune(b)))
		} else if len([]rune(b)) > innerW {
			b = string([]rune(b)[:innerW])
		}
		return "│" + b + "│"
	}
	out := make([]chatRow, 0, len(lines)+3)
	out = append(out, row(strokeTop("You", width, ascii), false))
	for _, line := range lines {
		out = append(out, row(body(line), false))
	}
	if copyOn {
		chip := stripANSI(copyChip)
		b := strings.Repeat(" ", innerW-len([]rune(chip)))
		out = append(out, row("│"+b+chip+"│", true))
	}
	out = append(out, row(strokeBottom(width, ascii), false))
	return out
}
