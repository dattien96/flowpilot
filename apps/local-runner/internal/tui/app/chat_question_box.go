package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// questionBox renders a question message as a solid grey card (colorBg3) with an amber border.
func questionBox(content string, width int, ascii bool, msgIdx int) []chatRow {
	if width < 10 {
		width = 10
	}
	innerW := width - 2
	if innerW < 4 {
		innerW = 4
	}

	tl, tr, bl, br, h, v := boxGlyphs(ascii)
	title := " Question "
	tw := lipgloss.Width(title)
	fill := innerW - tw
	if fill < 0 {
		fill = 0
		title = truncateVisual(title, innerW)
		tw = lipgloss.Width(title)
	}

	topBorder := styleQuestionBorder.Render(tl) +
		styleQuestionHeadBox.Render(title) +
		styleQuestionBorder.Render(strings.Repeat(h, fill)+tr)

	bottomBorder := styleQuestionBorder.Render(bl + strings.Repeat(h, innerW) + br)

	lines := strings.Split(content, "\n")
	var bodyRows []string

	textWidth := innerW - 2
	if textWidth < 4 {
		textWidth = 4
	}

	for lineIdx, rawLine := range lines {
		trimmed := strings.TrimRight(rawLine, "\r")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}

		line := trimmed
		if strings.HasPrefix(line, "[QUESTION]") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "[QUESTION]"))
		}

		wrapped := wrapText(line, textWidth)
		for _, wLine := range wrapped {
			styledLine := renderQuestionBoxLine(wLine)
			padded := " " + styledLine
			n := lipgloss.Width(stripANSI(padded))
			if n < innerW {
				padded += styleQuestionBoxBg.Render(strings.Repeat(" ", innerW-n))
			} else if n > innerW {
				padded = truncateVisual(padded, innerW)
			}
			rowText := styleQuestionBorder.Render(v) + padded + styleQuestionBorder.Render(v)
			bodyRows = append(bodyRows, rowText)
		}

		// If this was the question prompt and there are following option lines, add a subtle spacer
		if lineIdx == 0 && len(lines) > 1 && !strings.HasPrefix(strings.TrimSpace(lines[1]), "1)") {
			// Spacer row
			spacer := styleQuestionBoxBg.Render(strings.Repeat(" ", innerW))
			bodyRows = append(bodyRows, styleQuestionBorder.Render(v)+spacer+styleQuestionBorder.Render(v))
		}
	}

	out := make([]chatRow, 0, len(bodyRows)+2)
	out = append(out, chatRow{Text: topBorder, MsgIdx: msgIdx})
	for _, brText := range bodyRows {
		out = append(out, chatRow{Text: brText, MsgIdx: msgIdx})
	}
	out = append(out, chatRow{Text: bottomBorder, MsgIdx: msgIdx})
	return out
}

func renderQuestionBoxLine(line string) string {
	stripped := stripANSI(line)
	trimmedLead := strings.TrimLeft(stripped, " ")
	if i := strings.Index(trimmedLead, ") "); i > 0 && i <= 3 {
		leadSpaces := stripped[:len(stripped)-len(trimmedLead)]
		num := trimmedLead[:i+2]
		rest := trimmedLead[i+2:]
		label, desc := rest, ""
		if j := strings.Index(rest, " — "); j >= 0 {
			label, desc = rest[:j], rest[j+len(" — "):]
		}
		out := styleQuestionBodyBox.Render(leadSpaces) + styleQuestionHeadBox.Render(num) + " " + styleQuestionOptBox.Render(label)
		if desc != "" {
			out += styleQuestionDescBox.Render(" — " + desc)
		}
		return out
	}
	if strings.Contains(stripped, "[multi-select]") {
		return styleQuestionDescBox.Render(stripped)
	}
	return styleQuestionBodyBox.Render(stripped)
}

// answeredBox renders an answer confirmation as a solid grey card (colorBg3) with a green border.
func answeredBox(content string, width int, ascii bool, msgIdx int) []chatRow {
	if width < 10 {
		width = 10
	}
	innerW := width - 2
	if innerW < 4 {
		innerW = 4
	}

	tl, tr, bl, br, h, v := boxGlyphs(ascii)
	title := " Answered "
	tw := lipgloss.Width(title)
	fill := innerW - tw
	if fill < 0 {
		fill = 0
		title = truncateVisual(title, innerW)
		tw = lipgloss.Width(title)
	}

	topBorder := styleAnswerBorder.Render(tl) +
		styleAnswerHeadBox.Render(title) +
		styleAnswerBorder.Render(strings.Repeat(h, fill)+tr)

	bottomBorder := styleAnswerBorder.Render(bl + strings.Repeat(h, innerW) + br)

	ansText := strings.TrimSpace(content)
	if strings.HasPrefix(ansText, "Answered:") {
		ansText = strings.TrimSpace(strings.TrimPrefix(ansText, "Answered:"))
	}

	textWidth := innerW - 2
	if textWidth < 4 {
		textWidth = 4
	}

	wrapped := wrapText(ansText, textWidth)
	var bodyRows []string
	for _, wLine := range wrapped {
		styledLine := styleQuestionBodyBox.Render(wLine)
		padded := " " + styledLine
		n := lipgloss.Width(stripANSI(padded))
		if n < innerW {
			padded += styleQuestionBoxBg.Render(strings.Repeat(" ", innerW-n))
		} else if n > innerW {
			padded = truncateVisual(padded, innerW)
		}
		rowText := styleAnswerBorder.Render(v) + padded + styleAnswerBorder.Render(v)
		bodyRows = append(bodyRows, rowText)
	}

	out := make([]chatRow, 0, len(bodyRows)+2)
	out = append(out, chatRow{Text: topBorder, MsgIdx: msgIdx})
	for _, brText := range bodyRows {
		out = append(out, chatRow{Text: brText, MsgIdx: msgIdx})
	}
	out = append(out, chatRow{Text: bottomBorder, MsgIdx: msgIdx})
	return out
}
