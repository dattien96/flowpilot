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
	plain := stripANSI(s)
	r := []rune(plain)
	if len(r) <= width {
		return s
	}
	if width == 1 {
		return string(r[:1])
	}
	return string(r[:width-1]) + "…"
}

func padVisual(s string, width int) string {
	s = truncateVisual(s, width)
	n := lipgloss.Width(s)
	if n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

func strokeLine(inner string, boxW int, ascii bool) string {
	_, _, _, _, _, v := boxGlyphs(ascii)
	innerW := boxW - 2
	if innerW < 4 {
		innerW = 4
		boxW = innerW + 2
	}
	return v + padVisualANSI(inner, innerW) + v
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
	n := lipgloss.Width(s)
	if n > width {
		return truncateVisual(s, width)
	}
	if n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

func frameInput(lines []string, width int, title, footer string, ascii bool) string {
	if width < 10 {
		width = 10
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	tl, tr, bl, br, h, v := roundGlyphs(ascii)
	innerW := width - 2
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
	b.WriteString(tl + title + strings.Repeat(h, fill) + tr)
	for _, line := range lines {
		b.WriteByte('\n')
		b.WriteString(v + padVisualANSI(" "+line, innerW) + v)
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
		b.WriteString(bl + strings.Repeat(h, lead) + foot + br)
		return b.String()
	}
	b.WriteByte('\n')
	b.WriteString(bl + strings.Repeat(h, innerW) + br)
	return b.String()
}

func strokeChatRows(inner []chatRow, width int, user, ascii bool) []chatRow {
	if len(inner) == 0 || width < 10 {
		return inner
	}
	title := "You"
	boxW := hugBoxWidth(inner, title, width, user)
	out := make([]chatRow, 0, len(inner)+2)
	idx := inner[0].MsgIdx
	top := strokeTop(title, boxW, ascii)
	if user {
		top = rightAlignPlain(top, width)
	}
	out = append(out, chatRow{Text: top, MsgIdx: idx})
	for i, r := range inner {
		text := " " + r.Text
		copyOn := r.Copy && i == len(inner)-1
		innerW := boxW - 2
		if innerW < 4 {
			innerW = 4
		}
		if copyOn {
			chip := stripANSI(copyChip)
			room := innerW - lipgloss.Width(chip)
			if room < 4 {
				room = 4
			}
			text = padVisualANSI(text, room) + chip
		}
		line := strokeLine(text, boxW, ascii)
		if user {
			line = rightAlignPlain(line, width)
		}
		out = append(out, chatRow{Text: line, MsgIdx: r.MsgIdx, Copy: copyOn})
	}
	bot := strokeBottom(boxW, ascii)
	if user {
		bot = rightAlignPlain(bot, width)
	}
	out = append(out, chatRow{Text: bot, MsgIdx: inner[len(inner)-1].MsgIdx})
	return out
}

func hugBoxWidth(inner []chatRow, title string, maxW int, user bool) int {
	innerW := 4
	for _, r := range inner {
		w := lipgloss.Width(r.Text) + 1
		if r.Copy {
			w += lipgloss.Width(copyChip)
		}
		if w > innerW {
			innerW = w
		}
	}
	tlen := len([]rune(" " + strings.TrimSpace(title) + " "))
	if tlen > innerW {
		innerW = tlen
	}
	boxW := innerW + 2
	capW := maxW
	if user {
		capW = maxW * 7 / 10
		if capW < 16 {
			capW = maxW
		}
	}
	if boxW > capW {
		boxW = capW
	}
	if boxW > maxW {
		boxW = maxW
	}
	if boxW < 10 {
		boxW = 10
		if boxW > maxW {
			boxW = maxW
		}
	}
	return boxW
}
