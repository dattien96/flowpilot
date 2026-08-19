package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// clockAt swaps pasteNow for a manually advanced clock and returns an advance
// func so burst-timing tests are deterministic. Restored on cleanup.
func clockAt(t *testing.T) func(time.Duration) {
	t.Helper()
	old := pasteNow
	now := time.Unix(0, 0)
	pasteNow = func() time.Time { return now }
	t.Cleanup(func() { pasteNow = old })
	return func(d time.Duration) { now = now.Add(d) }
}

func newPasteModel() *AppModel {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.provider = "codex"
	m.sessionLoading = false
	m.authPhase = AuthNone
	return m
}

func TestPasteSummaryToken_CountsLinesAndChars(t *testing.T) {
	if got := pasteSummaryToken("a\nb\nc"); got != "[Pasted 3 lines · 5 chars]" {
		t.Fatalf("got %q", got)
	}
	if got := pasteSummaryToken("trailing\nline\n"); got != "[Pasted 2 lines · 14 chars]" {
		t.Fatalf("trailing newline must not inflate line count, got %q", got)
	}
	if got := pasteSummaryToken("single"); got != "[Pasted 1 lines · 6 chars]" {
		t.Fatalf("got %q", got)
	}
}

func TestNeedsPasteSummary_Thresholds(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"short single line", "hello world", false},
		{"short multi line", "multi-line\npaste body", false},
		{"5 lines fit composer", strings.Repeat("x\n", 4) + "x", false},
		{"7 lines overflow", strings.Repeat("line\n", 6) + "last", true},
		{"long single line stays verbatim", strings.Repeat("a", 200), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsPasteSummary(c.text); got != c.want {
				t.Fatalf("needsPasteSummary(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

// CA-5xx: a long multi-line bracketed paste (Windows Terminal Ctrl+V) must
// collapse to a "[Pasted N lines · C chars]" token so the composer never shows
// a cut block; the full text is stored for expansion on submit.
func TestBracketedPaste_LongMultiLineCollapsesToToken(t *testing.T) {
	m := newPasteModel()
	full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")

	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(full), Paste: true})
	am := m2.(*AppModel)
	if am.inputValue != "[Pasted 7 lines · 20 chars]" {
		t.Fatalf("composer must show the paste token, got %q", am.inputValue)
	}
	if len(am.pasteSegments) != 1 || am.pasteSegments[0].text != full {
		t.Fatalf("full text must be stored, got %+v", am.pasteSegments)
	}
	if got := am.expandPasteTokens(am.inputValue); got != full {
		t.Fatalf("expand must restore full text, got %q", got)
	}
}

// Short multi-line pastes still insert verbatim (CA-541 contract: the composer
// handles a couple of lines fine — only long blocks are collapsed).
func TestBracketedPaste_ShortMultiLineStaysVerbatim(t *testing.T) {
	m := newPasteModel()
	m.inputValue = "hi "

	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("multi-line\npaste body"), Paste: true})
	am := m2.(*AppModel)
	if am.inputValue != "hi multi-line\npaste body" {
		t.Fatalf("short multi-line paste must insert directly, got %q", am.inputValue)
	}
	if len(am.pasteSegments) != 0 {
		t.Fatalf("no segment expected for short paste, got %+v", am.pasteSegments)
	}
}

// Single-line pastes (paths, URLs, short code) always insert verbatim — only
// multi-line blocks overflow the 6-line composer and collapse.
func TestBracketedPaste_LongSingleLineStaysVerbatim(t *testing.T) {
	m := newPasteModel()
	full := strings.Repeat("a", 200)

	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(full), Paste: true})
	am := m2.(*AppModel)
	if am.inputValue != full {
		t.Fatalf("single-line paste must insert verbatim, got len=%d", len(am.inputValue))
	}
	if len(am.pasteSegments) != 0 {
		t.Fatalf("no segment expected, got %+v", am.pasteSegments)
	}
}

// ClipboardPasteMsg.Text (explicit Ctrl+V / Alt+V fallback) follows the same
// paste-summary rule.
func TestClipboardPasteMsg_LongMultiLineCollapses(t *testing.T) {
	m := newPasteModel()
	full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")

	m2, _ := m.Update(ClipboardPasteMsg{Text: full})
	am := m2.(*AppModel)
	if am.inputValue != "[Pasted 7 lines · 20 chars]" {
		t.Fatalf("got %q", am.inputValue)
	}
}

// Pressing Enter after a collapsed paste submits the FULL pasted text — the
// timeline and prompt history see the real prompt, not the token.
func TestPasteSummary_EnterSendsFullExpandedText(t *testing.T) {
	m := newPasteModel()
	full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")

	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(full), Paste: true})
	am := m2.(*AppModel)
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	sent := m3.(*AppModel)
	if sent.inputValue != "" {
		t.Fatalf("input must clear after send, got %q", sent.inputValue)
	}
	if len(sent.messages) == 0 || sent.messages[0].Content != full {
		t.Fatalf("timeline must show the full pasted text, first msg=%+v", sent.messages)
	}
}

func TestPasteSummary_ClearInputValueDropsSegments(t *testing.T) {
	m := newPasteModel()
	full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(full), Paste: true})
	am := m2.(*AppModel)
	if len(am.pasteSegments) == 0 {
		t.Fatal("expected segment after collapse")
	}
	am.clearInputValue()
	if len(am.pasteSegments) != 0 {
		t.Fatalf("segments must reset on clear, got %+v", am.pasteSegments)
	}
	if am.pasteBurst.active {
		t.Fatal("burst must reset on clear")
	}
}

func TestPasteSummary_TwoCollapsesExpandInOrder(t *testing.T) {
	m := newPasteModel()
	f1 := strings.Join([]string{"A1", "A2", "A3", "A4", "A5", "A6", "A7"}, "\n")
	f2 := strings.Join([]string{"B1", "B2", "B3", "B4", "B5", "B6", "B7"}, "\n")

	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(f1), Paste: true})
	m3, _ := m2.(*AppModel).handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(f2), Paste: true})
	am := m3.(*AppModel)
	want := "[Pasted 7 lines · 20 chars][Pasted 7 lines · 20 chars]"
	if am.inputValue != want {
		t.Fatalf("got %q", am.inputValue)
	}
	if got := am.expandPasteTokens(am.inputValue); got != f1+f2 {
		t.Fatalf("both segments must expand in order, got %q", got)
	}
}

// Raw (non-bracketed) paste arrives as a flood of key events where every line
// break is a plain Enter. Each Enter must become a newline — never a submit.
func TestBurst_RawMultiLinePasteEntersBecomeNewlines(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")
	lines := strings.Split(full, "\n")

	for i, line := range lines {
		for _, r := range line {
			advance(5 * time.Millisecond)
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = m2.(*AppModel)
		}
		if i < len(lines)-1 {
			advance(5 * time.Millisecond)
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			m = m2.(*AppModel)
		}
	}

	if m.inputValue != full {
		t.Fatalf("paste must stay in the composer verbatim, got %q", m.inputValue)
	}
	if len(m.messages) != 0 {
		t.Fatalf("raw multi-line paste must not auto-submit, got %d messages", len(m.messages))
	}
	if !m.pasteBurst.active {
		t.Fatal("burst should be active after the flood")
	}
	if got := string(m.pasteBurst.buf); got != full {
		t.Fatalf("burst buf=%q want %q", got, full)
	}
}

// Once the raw-paste flood settles, the tracked region collapses to a token
// exactly like the bracketed-paste path.
func TestBurst_CollapseReplacesRegionWithToken(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")
	lines := strings.Split(full, "\n")

	for i, line := range lines {
		for _, r := range line {
			advance(5 * time.Millisecond)
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = m2.(*AppModel)
		}
		if i < len(lines)-1 {
			advance(5 * time.Millisecond)
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			m = m2.(*AppModel)
		}
	}

	m.collapsePasteBurst()
	if m.inputValue != "[Pasted 7 lines · 20 chars]" {
		t.Fatalf("burst region must collapse to token, got %q", m.inputValue)
	}
	if len(m.pasteSegments) != 1 || m.pasteSegments[0].text != full {
		t.Fatalf("segment must hold full text, got %+v", m.pasteSegments)
	}
	if m.pasteBurst.active {
		t.Fatal("burst must reset after collapse")
	}
}

// A short raw paste (below the collapse threshold) is left verbatim but its
// Enters are still swallowed — no auto-submit — and the text survives.
func TestBurst_ShortRawPasteStaysVerbatimNoSubmit(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = m2.(*AppModel)
	advance(5 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = m2.(*AppModel)
	advance(5 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(*AppModel)

	if m.inputValue != "hi\n" {
		t.Fatalf("burst Enter must become a newline, got %q", m.inputValue)
	}
	if len(m.messages) != 0 {
		t.Fatalf("must not auto-submit, got %d messages", len(m.messages))
	}
	m.collapsePasteBurst()
	if m.inputValue != "hi\n" {
		t.Fatalf("short burst must stay verbatim, got %q", m.inputValue)
	}
}

// Normal typing (gaps far above burstRuneGap) must never arm the burst guard:
// Enter submits as usual.
func TestBurst_SlowTypingEnterSubmitsNormally(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	for _, r := range "hi" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
		advance(300 * time.Millisecond)
	}
	if m.pasteBurst.active {
		t.Fatal("slow typing must not arm the burst guard")
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	sent := m2.(*AppModel)
	if len(sent.messages) == 0 || sent.messages[0].Content != "hi" {
		t.Fatalf("normal Enter must submit, got %+v", sent.messages)
	}
}

// After a burst settles, a real Enter collapses the region and submits the
// full prompt — the user presses Enter once and the whole paste is sent.
func TestBurst_EnterAfterSettleCollapsesAndSubmits(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")
	lines := strings.Split(full, "\n")
	for i, line := range lines {
		for _, r := range line {
			advance(5 * time.Millisecond)
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = m2.(*AppModel)
		}
		if i < len(lines)-1 {
			advance(5 * time.Millisecond)
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			m = m2.(*AppModel)
		}
	}

	advance(200 * time.Millisecond)
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	sent := m2.(*AppModel)
	if len(sent.messages) == 0 || sent.messages[0].Content != full {
		t.Fatalf("Enter after settle must submit the full pasted text, first=%+v", sent.messages)
	}
}