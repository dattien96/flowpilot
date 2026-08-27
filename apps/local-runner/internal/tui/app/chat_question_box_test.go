package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestQuestionBox_RendersSolidGreyCardWithAmberBorder(t *testing.T) {
	prompt := "[QUESTION] Which file should callerSum live in?\n  1) calc.go — keep here\n  2) user.go"
	width := 60
	rows := questionBox(prompt, width, false, 0)

	if len(rows) < 4 {
		t.Fatalf("question box should have at least 4 rows, got %d", len(rows))
	}

	// 1. Top border has " Question " and amber border
	top := rows[0].Text
	if !strings.Contains(top, "Question") {
		t.Fatalf("top border must contain title 'Question': %q", top)
	}
	if !strings.Contains(top, "┌") || !strings.Contains(top, "┐") {
		t.Fatalf("top border must contain box glyphs: %q", top)
	}

	// 2. Body lines contain prompt text and options
	joined := ""
	for _, r := range rows {
		joined += r.Text + "\n"
	}
	if !strings.Contains(joined, "Which file should callerSum live in?") {
		t.Fatalf("body must contain prompt text:\n%s", joined)
	}
	if !strings.Contains(joined, "calc.go") || !strings.Contains(joined, "keep here") {
		t.Fatalf("body must contain option 1:\n%s", joined)
	}
	if !strings.Contains(joined, "user.go") {
		t.Fatalf("body must contain option 2:\n%s", joined)
	}

	// 3. Bottom border
	bottom := rows[len(rows)-1].Text
	if !strings.Contains(bottom, "└") || !strings.Contains(bottom, "┘") {
		t.Fatalf("bottom border must contain box glyphs: %q", bottom)
	}

	// 4. Width check for all rows
	for i, r := range rows {
		w := lipgloss.Width(stripANSI(r.Text))
		if w != width {
			t.Fatalf("row %d width = %d, want %d: %q", i, w, width, r.Text)
		}
	}
}

func TestAnsweredBox_RendersSolidGreyCardWithGreenBorder(t *testing.T) {
	ans := "Answered: calc.go (Recommended)"
	width := 50
	rows := answeredBox(ans, width, false, 0)

	if len(rows) < 3 {
		t.Fatalf("answered box should have at least 3 rows, got %d", len(rows))
	}

	// 1. Top border has " Answered "
	top := rows[0].Text
	if !strings.Contains(top, "Answered") {
		t.Fatalf("top border must contain title 'Answered': %q", top)
	}
	if !strings.Contains(top, "┌") || !strings.Contains(top, "┐") {
		t.Fatalf("top border must contain box glyphs: %q", top)
	}

	// 2. Body contains choice text
	joined := ""
	for _, r := range rows {
		joined += r.Text + "\n"
	}
	if !strings.Contains(joined, "calc.go (Recommended)") {
		t.Fatalf("body must contain answer text:\n%s", joined)
	}

	// 3. Bottom border
	bottom := rows[len(rows)-1].Text
	if !strings.Contains(bottom, "└") || !strings.Contains(bottom, "┘") {
		t.Fatalf("bottom border must contain box glyphs: %q", bottom)
	}

	// 4. Width check
	for i, r := range rows {
		w := lipgloss.Width(stripANSI(r.Text))
		if w != width {
			t.Fatalf("row %d width = %d, want %d: %q", i, w, width, r.Text)
		}
	}
}

func TestTimelineRows_BoxesQuestionAndAnswerMessages(t *testing.T) {
	m := &AppModel{
		width: 100,
		messages: []ChatMessage{
			{Role: "system", Content: "[QUESTION] Pick one\n  1) A\n  2) B", FormatHint: "question"},
			{Role: "system", Content: "Answered: A", FormatHint: "question"},
		},
	}
	rows := m.buildChatRows()
	joined := ""
	for _, r := range rows {
		joined += r.Text + "\n"
	}

	if !strings.Contains(joined, "Question") {
		t.Fatalf("timeline rows must contain boxed Question:\n%s", joined)
	}
	if !strings.Contains(joined, "Answered") {
		t.Fatalf("timeline rows must contain boxed Answered:\n%s", joined)
	}
}
