package app

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// BUG-346: single-select question rendered TWO chips per option ("1)" + "Nam"),
// so highlightIdx (which mirrors actionRingItems: one qopt item per option)
// drifted — Tab idx 3 painted option 2's label ("2) An") but Enter chose option
// 4. The bar must render ONE chip per option and fill exactly that chip.
func TestQuestionBarUnifiedChip_HighlightFollowsOption(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	q := &QuestionState{ID: "q-1", RunID: "run-1", Prompt: "who", Options: []map[string]string{
		{"label": "Nam", "value": "nam"},
		{"label": "An", "value": "an"},
		{"label": "Binh", "value": "binh"},
		{"label": "Other", "value": "other"},
	}}
	bar := renderQuestionBar(styleInputStroke.Render("┃"), styleInputStroke.Render("│"), q, 200, 1)
	filled := filledChipText(bar)
	if !strings.Contains(filled, "2) An") {
		t.Fatalf("highlightIdx 1 must fill the whole option-2 chip (2) An), got %q", filled)
	}
	if strings.Contains(filled, "1)") {
		t.Fatalf("highlightIdx 1 must not fill option 1, got %q", filled)
	}
	// The bar must contain exactly one filled chip.
	if got := countFill(bar); got != 1 {
		t.Fatalf("exactly one chip must be filled, got %d", got)
	}
	// Sanity: plain text keeps every option visible.
	plain := stripANSI(bar)
	for _, want := range []string{"1) Nam", "2) An", "3) Binh", "4) Other"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("plain bar missing %q in %q", want, plain)
		}
	}
}

// Last option (Other) must fill at highlightIdx 3 — the same index the action
// ring uses for qopt:3 — so Enter on the highlighted chip answers option 4.
func TestQuestionBarUnifiedChip_LastOptionFill(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	q := &QuestionState{ID: "q-2", RunID: "run-1", Prompt: "who", Options: []map[string]string{
		{"label": "Nam", "value": "nam"},
		{"label": "An", "value": "an"},
		{"label": "Binh", "value": "binh"},
		{"label": "Other", "value": "other"},
	}}
	bar := renderQuestionBar(styleInputStroke.Render("┃"), styleInputStroke.Render("│"), q, 200, 3)
	if filled := filledChipText(bar); !strings.Contains(filled, "4) Other") {
		t.Fatalf("highlightIdx 3 must fill the 4th chip (4) Other), got %q", filled)
	}
}

// The squeeze path must keep one-chip-per-option rendering: fill survives long
// labels, the bar fits, and the hint stays.
func TestQuestionBarUnifiedChip_LongLabelsKeepFillAndFit(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	q := &QuestionState{ID: "q-3", RunID: "run-1", Prompt: "pick a filename", Options: []map[string]string{
		{"label": "hello-grok.txt (ghi đè lên file cũ nếu có)", "value": "a"},
		{"label": "hello-grok-abc-xyz-very-long-option-name.txt", "value": "b"},
		{"label": "grok-abc-secondary-backup-copy.txt", "value": "c"},
		{"label": "Nhập tên khác hoàn toàn mới", "value": "d"},
	}}
	for _, idx := range []int{0, 1, 2, 3} {
		bar := renderQuestionBar(styleInputStroke.Render("┃"), styleInputStroke.Render("│"), q, 100, idx)
		if lipgloss.Width(bar) > 100 {
			t.Fatalf("idx %d: question bar must fit width 100, got %d", idx, lipgloss.Width(bar))
		}
		if countFill(bar) != 1 {
			t.Fatalf("idx %d: exactly one chip must be filled, got %d", idx, countFill(bar))
		}
		if !strings.Contains(stripANSI(bar), "← → Enter") {
			t.Fatalf("idx %d: hint must survive squeezing", idx)
		}
	}
}

var fillSeq = regexp.MustCompile(`\x1b\[[0-9;]*48;5;62m([^\x1b]*)`)

// filledChipText returns the plain text inside the single filled (highlighted)
// chip, without padding.
func filledChipText(bar string) string {
	m := fillSeq.FindStringSubmatch(bar)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func countFill(bar string) int {
	return len(fillSeq.FindAllStringSubmatch(bar, -1))
}
