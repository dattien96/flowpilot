package app

import "strings"

const copyChip = " [copy]"

func trimEmptyEdges(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(stripANSI(lines[start])) == "" {
		start++
	}
	for end > start && strings.TrimSpace(stripANSI(lines[end-1])) == "" {
		end--
	}
	if start >= end {
		return []string{""}
	}
	return lines[start:end]
}

// renderMarkdown is the TUI MVP: wrap raw markdown, no Glamour/Goldmark preview.
func renderMarkdown(src string, width int) []string {
	if width < 8 {
		width = 8
	}
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	return trimEmptyEdges(wrapText(src, width))
}
