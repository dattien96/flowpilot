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

func TestPasteSummaryToken_OneGenericLine(t *testing.T) {
	if got := pasteSummaryToken("a\nb\nc"); got != "[Pasted 5 chars]" {
		t.Fatalf("got %q", got)
	}
	if got := pasteSummaryToken("single"); got != "[Pasted 6 chars]" {
		t.Fatalf("got %q", got)
	}
	if got := pasteSummaryToken(""); got != "[Pasted 0 chars]" {
		t.Fatalf("got %q", got)
	}
}

func TestNeedsPasteSummary_Thresholds(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"single keystroke", "h", false},
		{"tiny snippet", " world", false},
		{"short single line", "hello", false},
		{"block-length single line", "some text", true},
		{"multi-line always collapses", "a\nb", true},
		{"long single line", strings.Repeat("a", 200), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsPasteSummary(c.text); got != c.want {
				t.Fatalf("needsPasteSummary(%q) = %v, want %v", c.text, got, c.want)
			}
		})
	}
}

// CA-560: a multi-line bracketed paste (Windows Terminal Ctrl+V) must collapse
// to a one-line "[Pasted N chars]" token so the composer never shows a cut
// block and never loses the caret; the full text is stored for expansion.
func TestBracketedPaste_LongMultiLineCollapsesToToken(t *testing.T) {
	m := newPasteModel()
	full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")

	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(full), Paste: true})
	am := m2.(*AppModel)
	if am.inputValue != "[Pasted 20 chars]" {
		t.Fatalf("composer must show the paste token, got %q", am.inputValue)
	}
	if len(am.pasteSegments) != 1 || am.pasteSegments[0].text != full {
		t.Fatalf("full text must be stored, got %+v", am.pasteSegments)
	}
	if got := am.expandPasteTokens(am.inputValue); got != full {
		t.Fatalf("expand must restore full text, got %q", got)
	}
}

// Even a short multi-line paste collapses — the composer shows one token line,
// never raw multi-line text.
func TestBracketedPaste_ShortMultiLineCollapses(t *testing.T) {
	m := newPasteModel()
	m.inputValue = "hi "

	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("multi-line\npaste body"), Paste: true})
	am := m2.(*AppModel)
	if am.inputValue != "hi [Pasted 21 chars]" {
		t.Fatalf("multi-line paste must collapse, got %q", am.inputValue)
	}
	if len(am.pasteSegments) != 1 {
		t.Fatalf("one segment expected, got %+v", am.pasteSegments)
	}
}

// A block-length single-line paste collapses too.
func TestBracketedPaste_LongSingleLineCollapses(t *testing.T) {
	m := newPasteModel()
	full := strings.Repeat("a", 200)

	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(full), Paste: true})
	am := m2.(*AppModel)
	if am.inputValue != "[Pasted 200 chars]" {
		t.Fatalf("long single-line paste must collapse, got len=%d", len(am.inputValue))
	}
	if got := am.expandPasteTokens(am.inputValue); got != full {
		t.Fatalf("expand must restore, got len=%d", len(got))
	}
}

// Tiny single-line pastes (like " world") still insert verbatim.
func TestBracketedPaste_TinySnippetStaysVerbatim(t *testing.T) {
	m := newPasteModel()
	m.inputValue = "hello"

	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" world"), Paste: true})
	am := m2.(*AppModel)
	if am.inputValue != "hello world" {
		t.Fatalf("tiny snippet must insert verbatim, got %q", am.inputValue)
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
	if am.inputValue != "[Pasted 20 chars]" {
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
	want := "[Pasted 20 chars][Pasted 20 chars]"
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
	if m.inputValue != "[Pasted 20 chars]" {
		t.Fatalf("burst region must collapse to token, got %q", m.inputValue)
	}
	if len(m.pasteSegments) != 1 || m.pasteSegments[0].text != full {
		t.Fatalf("segment must hold full text, got %+v", m.pasteSegments)
	}
	if m.pasteBurst.active {
		t.Fatal("burst must reset after collapse")
	}
}

// The settle tick collapses an idle burst automatically — no keystroke needed.
func TestBurst_SettleTickCollapsesAutomatically(t *testing.T) {
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
		t.Fatalf("raw text must still be present pre-settle, got %q", m.inputValue)
	}

	advance(200 * time.Millisecond)
	m2, _ := m.Update(pasteBurstSettleMsg{})
	am := m2.(*AppModel)
	if am.inputValue != "[Pasted 20 chars]" {
		t.Fatalf("settle tick must collapse the burst, got %q", am.inputValue)
	}
}

// A burst Enter becomes a newline — no auto-submit — even for a short flood.
func TestBurst_EnterDuringShortBurstBecomesNewline(t *testing.T) {
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
}

// A short single-line burst stays verbatim (below the collapse threshold).
func TestBurst_ShortSingleLineBurstStaysVerbatim(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	for _, r := range "hi" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
		advance(5 * time.Millisecond)
	}
	if !m.pasteBurst.active {
		t.Fatal("burst should be armed")
	}
	m.collapsePasteBurst()
	if m.inputValue != "hi" {
		t.Fatalf("short single-line burst must stay verbatim, got %q", m.inputValue)
	}
	if len(m.pasteSegments) != 0 {
		t.Fatalf("no segment expected, got %+v", m.pasteSegments)
	}
}

// CA-560: the composer must not clamp to 6 lines. Clamping misaligned caret
// offsets once input grew past the window (cursor vanished, arrows/mouse
// stopped placing it). Pasting collapses to one token and manually typed
// multi-line prompts render fully.
func TestRenderInputLine_NoComposerClamp(t *testing.T) {
	m := newPasteModel()
	m.width = 100
	lines := []string{"l0", "l1", "l2", "l3", "l4", "l5", "l6", "l7"}
	m.inputValue = strings.Join(lines, "\n")
	m.inputCursor = -1

	out := stripANSI(m.renderInputLine())
	if !strings.Contains(out, "l0") {
		t.Fatalf("first line must render (no clamp):\n%s", out)
	}
	if !strings.Contains(out, "l7") {
		t.Fatalf("last line must render:\n%s", out)
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
