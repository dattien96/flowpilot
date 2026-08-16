package app

import (
	"hash/fnv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/charmbracelet/lipgloss"
)

const copyChip = " [copy]"

const (
	mdCacheCap       = 96
	mdFenceMaxLines  = 200
	mdFenceMaxBytes  = 32 * 1024
	mdFenceHeadLines = 80
	mdFenceTailLines = 40
)

var (
	styleMdH       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorAccent))
	styleMdCode    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorWarn))
	styleMdDim     = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim))
	styleMdBody    = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText))
	styleMdCodeBox = lipgloss.NewStyle().Foreground(lipgloss.Color(colorText)).Background(lipgloss.Color(colorCodeBg))
	// Fence border glyphs — dim on the same solid card bg (opencode-style, no
	// bright outline around the code block).
	styleMdCodeBar = lipgloss.NewStyle().Foreground(lipgloss.Color(colorTextDim)).Background(lipgloss.Color(colorCodeBg))
)

type mdCacheKey struct {
	width int
	ascii bool
	hash  uint64
}

var (
	mdMu         sync.Mutex
	mdCache      = map[mdCacheKey][]mdLine{}
	mdCacheOrder []mdCacheKey
	mdParseCount atomic.Uint64
)

type mdLine struct {
	Text     string
	CopyCode string
}

func markdownParseCount() uint64 { return mdParseCount.Load() }

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

func trimEmptyMDEdges(lines []mdLine) []mdLine {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(stripANSI(lines[start].Text)) == "" {
		start++
	}
	for end > start && strings.TrimSpace(stripANSI(lines[end-1].Text)) == "" {
		end--
	}
	if start >= end {
		return []mdLine{{Text: ""}}
	}
	return lines[start:end]
}

// renderMarkdown styles GFM for the TUI. Cache key is width + source hash.
func renderMarkdown(src string, width int) []string {
	return mdTexts(renderMarkdownRows(src, width, false))
}

func renderMarkdownMode(src string, width int, ascii bool) []string {
	return mdTexts(renderMarkdownRows(src, width, ascii))
}

func renderMarkdownRows(src string, width int, ascii bool) []mdLine {
	if width < 8 {
		width = 8
	}
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	key := mdCacheKey{width: width, ascii: ascii, hash: hashMDSource(src)}
	mdMu.Lock()
	if hit, ok := mdCache[key]; ok {
		mdMu.Unlock()
		return cloneMDRows(hit)
	}
	mdMu.Unlock()

	mdParseCount.Add(1)
	out := trimEmptyMDEdges(renderMarkdownFresh(src, width, ascii))
	mdMu.Lock()
	if _, ok := mdCache[key]; !ok {
		if len(mdCacheOrder) >= mdCacheCap {
			old := mdCacheOrder[0]
			mdCacheOrder = mdCacheOrder[1:]
			delete(mdCache, old)
		}
		mdCache[key] = cloneMDRows(out)
		mdCacheOrder = append(mdCacheOrder, key)
	}
	mdMu.Unlock()
	return out
}

func hashMDSource(src string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(src))
	return h.Sum64()
}

func cloneMDRows(in []mdLine) []mdLine {
	out := make([]mdLine, len(in))
	copy(out, in)
	return out
}

func mdTexts(in []mdLine) []string {
	out := make([]string, len(in))
	for i, line := range in {
		out[i] = line.Text
	}
	return out
}

func textsToMD(in []string) []mdLine {
	out := make([]mdLine, len(in))
	for i, s := range in {
		out[i] = mdLine{Text: s}
	}
	return out
}
