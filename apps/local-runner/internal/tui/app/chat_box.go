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

func strokeLine(inner string, boxW int, ascii bool) string {
	_, _, _, _, _, v := boxGlyphs(ascii)
	innerW := boxW - 2
	if innerW < 1 {
		innerW = 1
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

func strokeChatRows(inner []chatRow, width int, user, ascii, alignRight bool) []chatRow {
	if len(inner) == 0 || width < 10 {
		return inner
	}
	title := "You"
	fullPane := user && !alignRight
	boxW := hugBoxWidth(inner, title, width, user, fullPane)
	innerW := boxW - 2
	if innerW < 4 {
		innerW = 4
	}
	// Re-wrap inner rows that are wider than the final innerW (hug may be
	// smaller than the width used for the initial wrap). Wrap, don't
	// truncate — the old pad-then-truncate produced the "mãi không xong"
	// where long prompts were cut mid-box (e.g. "|Path:" lost).
	wrappedInner := make([]chatRow, 0, len(inner)*2)
	for i, r := range inner {
		copyOn := r.Copy && i == len(inner)-1
		textW := innerW - 1 // leading " "
		if copyOn {
			textW -= len([]rune(stripANSI(copyChip)))
			if textW < 4 {
				textW = 4
			}
		}
		plain := stripANSI(r.Text)
		if len([]rune(plain)) <= textW {
			wrappedInner = append(wrappedInner, r)
			continue
		}
		parts := wrapText(plain, textW)
		for j, part := range parts {
			txt := part
			if user {
				// Preserve user prompt styling (and re-apply after split).
				txt = styleUser.Render(part)
			}
			nr := chatRow{Text: txt, MsgIdx: r.MsgIdx, PromptExpandKey: r.PromptExpandKey}
			// Only the last wrapped piece of the last inner carries the copy chip.
			if copyOn && j == len(parts)-1 {
				nr.Copy = true
			}
			wrappedInner = append(wrappedInner, nr)
		}
	}
	inner = wrappedInner
	// Recompute boxW after re-wrap (content may have grown taller but narrower).
	boxW = hugBoxWidth(inner, title, width, user, fullPane)
	innerW = boxW - 2
	if innerW < 4 {
		innerW = 4
	}

	alignW := width
	if user && alignRight {
		alignW = width - userBoxGutter
		if alignW < 10 {
			alignW = width
		}
	}
	out := make([]chatRow, 0, len(inner)+2)
	idx := inner[0].MsgIdx
	top := strokeTop(title, boxW, ascii)
	if user && alignRight {
		top = rightAlignPlain(top, alignW)
	}
	out = append(out, chatRow{Text: top, MsgIdx: idx, PromptExpandKey: inner[0].PromptExpandKey})
	for i, r := range inner {
		text := " " + r.Text
		copyOn := r.Copy && i == len(inner)-1
		if copyOn {
			chip := stripANSI(copyChip)
			room := innerW - len([]rune(chip))
			if room < 4 {
				room = 4
			}
			text = padVisualANSI(text, room) + chip
		}
		line := strokeLine(text, boxW, ascii)
		if user && alignRight {
			line = rightAlignPlain(line, alignW)
		}
		if fullPane {
			plainPart := stripANSI(r.Text)
			base := " " + plainPart
			if copyOn {
				chipPlain := stripANSI(copyChip)
				target := innerW - len([]rune(chipPlain))
				if len([]rune(base)) < target {
					base = base + strings.Repeat(" ", target-len([]rune(base)))
				} else if len([]rune(base)) > target {
					base = string([]rune(base)[:target])
				}
				base = base + chipPlain
			}
			if len([]rune(base)) < innerW {
				base = base + strings.Repeat(" ", innerW-len([]rune(base)))
			} else if len([]rune(base)) > innerW {
				base = string([]rune(base)[:innerW])
			}
			line = "│" + base + "│"
		}
		out = append(out, chatRow{Text: line, MsgIdx: r.MsgIdx, Copy: copyOn, PromptExpandKey: r.PromptExpandKey})
	}
	bot := strokeBottom(boxW, ascii)
	if user && alignRight {
		bot = rightAlignPlain(bot, alignW)
	}
	out = append(out, chatRow{Text: bot, MsgIdx: inner[len(inner)-1].MsgIdx, PromptExpandKey: inner[len(inner)-1].PromptExpandKey})
	return out
}

const userBoxGutter = 2

func hugBoxWidth(inner []chatRow, title string, maxW int, user bool, fullPane bool) int {
	if user && fullPane {
		return maxW
	}
	innerW := 4
	for _, r := range inner {
		w := len([]rune(stripANSI(r.Text))) + 1
		if r.Copy {
			w += len([]rune(stripANSI(copyChip)))
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
		effW := maxW - userBoxGutter
		if effW < 10 {
			effW = maxW
		}
		capW = effW * 7 / 10
		if capW < 16 {
			capW = effW
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
