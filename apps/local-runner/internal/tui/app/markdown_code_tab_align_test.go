package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// Tests for code block tab expansion and vertical stroke alignment (CA-609):
// In code blocks with tab indentation (e.g. Go code with \t and \t\t),
// tabs are expanded to 4 spaces so that visual width calculations match terminal
// column rendering, and the right vertical border (│) remains perfectly vertical
// across all lines.

func TestRenderMarkdown_CodeBlockTabIndentationBordersAligned(t *testing.T) {
	// Exact code from user screenshot with 1-tab and 2-tab indentations
	src := "```go\nvar ErrSubtrahendTooLarge = errors.New(\"subtrahend exceeds minuend\")\n\nfunc SubtractWithGuard(a, b int) (int, error) {\n\tif b > a {\n\t\treturn 0, ErrSubtrahendTooLarge\n\t}\n\treturn a - b, nil\n}\n```\n"

	widths := []int{60, 80, 100, 120}
	for _, w := range widths {
		for _, ascii := range []bool{false, true} {
			mdLines := renderMarkdownFresh(src, w, ascii)
			var boxLines []string
			for _, ml := range mdLines {
				plain := stripANSI(ml.Text)
				if strings.HasPrefix(plain, "┌") || strings.HasPrefix(plain, "│") || strings.HasPrefix(plain, "└") ||
					strings.HasPrefix(plain, "+") || strings.HasPrefix(plain, "|") {
					boxLines = append(boxLines, plain)
				}
			}

			if len(boxLines) == 0 {
				t.Fatalf("w=%d ascii=%v: expected code box lines, got none", w, ascii)
			}

			// All lines in the code box must have the exact same visual width
			expectedWidth := lipgloss.Width(boxLines[0])
			for i, line := range boxLines {
				actualWidth := lipgloss.Width(line)
				if actualWidth != expectedWidth {
					t.Fatalf("w=%d ascii=%v: line %d visual width %d != expected width %d:\n%s\n--- all box lines ---\n%s",
						w, ascii, i, actualWidth, expectedWidth, line, strings.Join(boxLines, "\n"))
				}

				// Check right border glyph alignment
				if !ascii {
					if i == 0 {
						if !strings.HasSuffix(line, "┐") {
							t.Fatalf("top line missing ┐: %q", line)
						}
					} else if i == len(boxLines)-1 {
						if !strings.HasSuffix(line, "┘") {
							t.Fatalf("bottom line missing ┘: %q", line)
						}
					} else {
						if !strings.HasSuffix(line, "│") {
							t.Fatalf("content line %d missing │: %q", i, line)
						}
					}
				} else {
					if !strings.HasSuffix(line, "+") && !strings.HasSuffix(line, "|") {
						t.Fatalf("ascii line %d missing right border: %q", i, line)
					}
				}
			}

			// Ensure content is preserved and tabs are expanded cleanly
			joined := strings.Join(boxLines, "\n")
			for _, need := range []string{
				"var ErrSubtrahendTooLarge",
				"func SubtractWithGuard",
				"if b > a {",
				"return 0, ErrSubtrahendTooLarge",
				"return a - b, nil",
			} {
				if !strings.Contains(joined, need) {
					t.Fatalf("w=%d ascii=%v: missing content %q in box:\n%s", w, ascii, need, joined)
				}
			}
		}
	}
}

func TestExpandTabs_TabStops(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		tabW     int
		expected string
	}{
		{
			name:     "leading single tab",
			input:    "\thello",
			tabW:     4,
			expected: "    hello",
		},
		{
			name:     "leading double tab",
			input:    "\t\thello",
			tabW:     4,
			expected: "        hello",
		},
		{
			name:     "mid-line tab to next stop",
			input:    "a\tb",
			tabW:     4,
			expected: "a   b",
		},
		{
			name:     "three chars tab to stop 4",
			input:    "abc\td",
			tabW:     4,
			expected: "abc d",
		},
		{
			name:     "four chars tab to stop 8",
			input:    "abcd\te",
			tabW:     4,
			expected: "abcd    e",
		},
		{
			name:     "no tabs unchanged",
			input:    "plain text 123",
			tabW:     4,
			expected: "plain text 123",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := expandTabs(tc.input, tc.tabW)
			if got != tc.expected {
				t.Fatalf("expandTabs(%q, %d) = %q, want %q", tc.input, tc.tabW, got, tc.expected)
			}
		})
	}
}
