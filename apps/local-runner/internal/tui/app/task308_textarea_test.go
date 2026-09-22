package app

import (
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
	tea "github.com/charmbracelet/bubbletea"
)

// Task-308 additive tests — verify bubbles/textarea value mirror while
// inputValue/inputCursor stay live. These are NEW tests, not edits to old ones.
// Phase 1: textarea is value-only mirror (no caret poke, no typing via textarea).

func newTask308Model() *AppModel {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.provider = "codex"
	m.sessionLoading = false
	m.authPhase = AuthNone
	m.width = 120
	m.height = 40
	m.fullWidth = 120
	m.sessionDefaultsLoaded = true
	return m
}

// 1. UTF-8 multi-byte & diacritics, both one-rune-at-a-time and multi-rune
// KeyRunes (IME composition delivers whole clusters) — mirror must not drop runes.
func TestTUIInput_Bubbles_Utf8DiacriticsAndMultiRune(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			m := newTask308Model()
			m.provider = provider
			text := "Xin chào tiếng Việt có dấu"
			for _, r := range text {
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}
			if m.inputValue != text {
				t.Fatalf("[%s] inputValue expected %q got %q", provider, text, m.inputValue)
			}
			if m.textarea.Value() != text {
				t.Fatalf("[%s] textarea.Value expected %q got %q", provider, text, m.textarea.Value())
			}
			if m.inputCaretIndex() != len([]rune(text)) {
				t.Fatalf("[%s] caret expected %d got %d", provider, len([]rune(text)), m.inputCaretIndex())
			}
		})
	}
	// Multi-rune single KeyMsg (IME composition / word paste), not one rune each.
	m := newTask308Model()
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("tiếng Việt có dấu")})
	m = m2.(*AppModel)
	if m.inputValue != "tiếng Việt có dấu" {
		t.Fatalf("multi-rune inputValue mismatch: %q", m.inputValue)
	}
	if m.textarea.Value() != "tiếng Việt có dấu" {
		t.Fatalf("multi-rune textarea mismatch: %q", m.textarea.Value())
	}
}

// 2. Multi-line navigation & caret positioning — still via inputCursor helpers.
func TestTUIInput_Bubbles_CaretNavigationAndInsert(t *testing.T) {
	m := newTask308Model()
	for _, r := range "hello" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = m2.(*AppModel)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = m2.(*AppModel)
	if m.inputCaretIndex() != 3 {
		t.Fatalf("expected caret at 3, got %d (input=%q textarea=%q)", m.inputCaretIndex(), m.inputValue, m.textarea.Value())
	}
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	m = m2.(*AppModel)
	if m.inputValue != "helXlo" {
		t.Fatalf("expected helXlo, got %q textarea=%q", m.inputValue, m.textarea.Value())
	}
	if m.textarea.Value() != "helXlo" {
		t.Fatalf("textarea should mirror helXlo, got %q", m.textarea.Value())
	}
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyHome})
	m = m2.(*AppModel)
	if m.inputCaretIndex() != 0 {
		t.Fatalf("expected caret at 0 after Home, got %d", m.inputCaretIndex())
	}
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnd})
	m = m2.(*AppModel)
	if m.inputCaretIndex() != len([]rune(m.inputValue)) {
		t.Fatalf("expected caret at end, got %d", m.inputCaretIndex())
	}
}

// 3. Fast character stream — no heuristic drops when Windows reject off.
func TestTUIInput_Bubbles_FastTypingNoDrop(t *testing.T) {
	m := newTask308Model()
	text := "abcdefghij"
	for _, r := range text {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if m.inputValue != text {
		t.Fatalf("fast typing dropped chars, expected %q got %q textarea=%q", text, m.inputValue, m.textarea.Value())
	}
	if m.textarea.Value() != text {
		t.Fatalf("textarea fast typing mismatch, expected %q got %q", text, m.textarea.Value())
	}
}

// 4. Token collapsing & expansion — textarea mirrors token, Backspace clears token.
func TestTUIInput_Bubbles_PasteTokenCollapsingAndExpansion(t *testing.T) {
	m := newTask308Model()
	full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(full), Paste: true})
	am := m2.(*AppModel)
	if am.inputValue != "[Pasted 20 chars]" {
		t.Fatalf("composer must show token, got %q", am.inputValue)
	}
	if am.textarea.Value() != "[Pasted 20 chars]" {
		t.Fatalf("textarea must mirror token, got %q", am.textarea.Value())
	}
	if len(am.pasteSegments) != 1 || am.pasteSegments[0].text != full {
		t.Fatalf("pasteSegments broken: %+v", am.pasteSegments)
	}
	if got := am.expandPasteTokens(am.inputValue); got != full {
		t.Fatalf("expand must restore full, got %q", got)
	}
	m2, _ = am.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	am = m2.(*AppModel)
	if am.inputValue != "" {
		t.Fatalf("Backspace on token must clear, got %q", am.inputValue)
	}
	if am.textarea.Value() != "" {
		t.Fatalf("textarea after token delete must be empty, got %q", am.textarea.Value())
	}
}

// 5. Suggestion popup — textarea Value drives slash handling via mirror.
func TestTUIInput_Bubbles_SuggestionNavigationAndAccept(t *testing.T) {
	m := newTask308Model()
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = m2.(*AppModel)
	if len(m.collectSuggestions()) == 0 {
		t.Fatalf("slash should trigger suggestions, textarea=%q input=%q", m.textarea.Value(), m.inputValue)
	}
	if m.textarea.Value() != "/" {
		t.Fatalf("textarea should be /, got %q", m.textarea.Value())
	}
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = m2.(*AppModel)
	if m.inputValue != "/a" || m.textarea.Value() != "/a" {
		t.Fatalf("expected /a, got input=%q textarea=%q", m.inputValue, m.textarea.Value())
	}
}

// 6. Ctrl+C Clear vs Quit — textarea Reset must mirror clearInputValue.
func TestTUIInput_Bubbles_CtrlC_ClearVsQuit(t *testing.T) {
	m := newTask308Model()
	for _, r := range "hello" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if m.inputValue == "" {
		t.Fatal("setup: input should be hello")
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	am := m2.(*AppModel)
	if am.quitting {
		t.Fatal("Ctrl+C with text must not quit")
	}
	if am.inputValue != "" || am.textarea.Value() != "" {
		t.Fatalf("Ctrl+C must clear, got input=%q textarea=%q", am.inputValue, am.textarea.Value())
	}
	m2, _ = am.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	am = m2.(*AppModel)
	if !am.quitting {
		t.Fatal("Ctrl+C on empty must quit")
	}
}

// 7. Cross-provider parity — textarea wiring must not depend on provider.
func TestTUIInput_Bubbles_CrossProviderParity(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			m := newTask308Model()
			m.provider = provider
			for _, r := range "parity" {
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}
			if m.inputValue != "parity" || m.textarea.Value() != "parity" {
				t.Fatalf("[%s] parity mismatch input=%q textarea=%q", provider, m.inputValue, m.textarea.Value())
			}
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
			m = m2.(*AppModel)
			if m.inputValue != "" || m.textarea.Value() != "" {
				t.Fatalf("[%s] clear failed", provider)
			}
		})
	}
}

// 8. Shift+Enter inserts newline via handleKey (isPromptNewlineKey), not direct InsertString.
func TestTUIInput_Bubbles_ShiftEnterInsertsNewline(t *testing.T) {
	m := newTask308Model()
	for _, r := range "hello" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	// Alt+Enter is a prompt newline (see isPromptNewlineKey)
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m = m2.(*AppModel)
	if m.inputValue != "hello\n" {
		t.Fatalf("expected hello newline, got %q textarea=%q", m.inputValue, m.textarea.Value())
	}
	if m.textarea.Value() != "hello\n" {
		t.Fatalf("textarea should have newline, got %q", m.textarea.Value())
	}
	if m.textarea.LineCount() != 2 {
		t.Fatalf("textarea should have 2 lines, got %d value=%q", m.textarea.LineCount(), m.textarea.Value())
	}
}

// 9. Auth password must not leak into textarea mirror.
func TestTUIInput_Bubbles_AuthDoesNotLeakToTextarea(t *testing.T) {
	m := newTask308Model()
	m.authPhase = AuthPassword
	m.inputValue = "secret123"
	// sync should be blocked when auth != none
	m.syncTextareaValue()
	if m.textarea.Value() == "secret123" {
		t.Fatalf("password must not leak to textarea, got %q", m.textarea.Value())
	}
}

// 10. M1 fidelity: mirror must match what bubbles' sanitizer produces for
// tab/CRLF/control inputs, and the sync guard must not churn afterwards.
func TestTUIInput_Bubbles_MirrorNormalizesTabAndCRLF(t *testing.T) {
	m := newTask308Model()
	m.inputValue = "a\tb\r\nc\x00d"
	m.syncTextareaValue()
	// bubbles sanitizer maps \r AND \n to \n individually, so \r\n -> \n\n.
	want := "a    b\n\ncd"
	if got := m.textarea.Value(); got != want {
		t.Fatalf("mirror fidelity: want %q got %q (input=%q)", want, got, m.inputValue)
	}
	// Send must still use the original inputValue (tabs/CR preserved).
	if m.inputValue != "a\tb\r\nc\x00d" {
		t.Fatalf("live composer must not be rewritten, got %q", m.inputValue)
	}
	// Second sync must be a no-op (guard compares against normalized form).
	m.syncTextareaValue()
	if got := m.textarea.Value(); got != want {
		t.Fatalf("mirror changed after second sync: %q", got)
	}
}

// 11. M2: Windows raw-paste reject revert (app.go:2442-2449) must sync mirror.
func TestTUIInput_Bubbles_RejectRevertSyncsMirror(t *testing.T) {
	advance := clockAt(t)
	m := newTask308Model()
	m.rejectWindowsRawPaste = true
	// 8 varied rapid runes arm reject and wipe input to start (with sync).
	for _, r := range "Them ha" {
		advance(3 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	advance(3 * time.Millisecond)
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = m2.(*AppModel)
	if m.inputValue != "" {
		t.Fatalf("reject must wipe input, got %q", m.inputValue)
	}
	if m.textarea.Value() != "" {
		t.Fatalf("mirror must be wiped with input, got %q", m.textarea.Value())
	}
	// Settle-wipe branch (app.go:1399-1409): inject residue then settle.
	m.inputValue = "leaked residue"
	if !m.pasteBurst.rejectArmed {
		t.Fatal("rejectArmed must be true after 8 varied")
	}
	advance(200 * time.Millisecond)
	m2, _ = m.Update(pasteBurstSettleMsg{})
	m = m2.(*AppModel)
	if m.inputValue != "" {
		t.Fatalf("settle must wipe residue, got %q", m.inputValue)
	}
	if m.textarea.Value() != "" {
		t.Fatalf("mirror must sync after settle wipe, got %q", m.textarea.Value())
	}
}

// 12. M2: collapsePasteBurst (chat_paste.go:227/236) must sync mirror token.
func TestTUIInput_Bubbles_CollapseBurstSyncsMirror(t *testing.T) {
	advance := clockAt(t)
	m := newTask308Model()
	// 8+ varied rapid runes with reject OFF collapse to a paste token on settle.
	for _, r := range "abcdefghij" {
		advance(3 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if m.inputValue != "abcdefghij" {
		t.Fatalf("setup: input should be abcdefghij, got %q", m.inputValue)
	}
	advance(200 * time.Millisecond)
	m2, _ := m.Update(pasteBurstSettleMsg{})
	m = m2.(*AppModel)
	if !strings.HasPrefix(m.inputValue, "[Pasted ") {
		t.Fatalf("burst must collapse to token, got %q", m.inputValue)
	}
	if m.textarea.Value() != m.inputValue {
		t.Fatalf("mirror must equal token, input=%q mirror=%q", m.inputValue, m.textarea.Value())
	}
}

// 13-18. Phase 2: View-when-sticky-end (renderInputLine uses textarea.View when sticky-end with draft).
func TestTUIInput_Bubbles_View_WhenStickyEnd(t *testing.T) {
	m := newTask308Model()
	for _, r := range "hello" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	m.pasteBurst = pasteBurst{}
	if !m.useTextareaView() {
		t.Fatalf("sticky-end draft should use View (burst=%+v cursor=%d skills=%d)", m.pasteBurst, m.inputCursor, len(m.selectedSkills))
	}
	if m.textarea.Prompt != "" {
		t.Fatalf("View Prompt must be empty, got %q", m.textarea.Prompt)
	}
	// View-only side effect: SetHeight(LineCount) vs constructor 3.
	m.renderInputLine()
	if m.textarea.Height() != 1 {
		t.Fatalf("View should set Height to LineCount (1), got %d", m.textarea.Height())
	}
	view := m.renderInputLine()
	if !strings.Contains(view, "hello") {
		t.Fatalf("View must contain hello, got %q", view)
	}
	if strings.Contains(view, "┃") {
		t.Fatalf("View must not contain Prompt ┃, got %q", view)
	}
	// Deleting the View branch must break the Height assertion above — proves the path ran.
}

func TestTUIInput_Bubbles_View_EmptyShowsPlaceholder(t *testing.T) {
	m := newTask308Model()
	if m.inputValue != "" || m.textarea.Value() != "" {
		t.Fatalf("empty composer: input=%q mirror=%q", m.inputValue, m.textarea.Value())
	}
	if m.textarea.Placeholder != "Type a message, /command, or @file..." {
		t.Fatalf("placeholder mismatch: %q", m.textarea.Placeholder)
	}
	if m.useTextareaView() {
		t.Fatal("empty input must fallback to custom (no draft)")
	}
	view := m.renderInputLine()
	if !strings.Contains(strings.ToLower(view), "chat") {
		t.Fatalf("empty render must contain frame label, got %q", view)
	}
}

func TestTUIInput_Bubbles_View_FallsBackWhenCaretMid(t *testing.T) {
	m := newTask308Model()
	for _, r := range "hello" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	m.pasteBurst = pasteBurst{}
	// Establish sticky-end draft that would use View.
	if !m.useTextareaView() {
		t.Fatal("setup: sticky-end draft should use View")
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = m2.(*AppModel)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = m2.(*AppModel)
	if m.useTextareaView() {
		t.Fatal("mid-string caret should fallback to custom renderer")
	}
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	m = m2.(*AppModel)
	if m.inputValue != "helXlo" {
		t.Fatalf("mid insert failed, got %q", m.inputValue)
	}
}

func TestTUIInput_Bubbles_View_FallsBackOnSkillHighlight(t *testing.T) {
	m := newTask308Model()
	for _, r := range "hi" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	m.pasteBurst = pasteBurst{}
	if !m.useTextareaView() {
		t.Fatal("setup: hi should use View")
	}
	m.selectedSkills = []client.SkillSelection{{Name: "coding"}}
	if m.useTextareaView() {
		t.Fatal("skill highlight must fallback to custom renderer")
	}
}

func TestTUIInput_Bubbles_View_FallsBackOnAttachChip(t *testing.T) {
	m := newTask308Model()
	for _, r := range "hi" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	m.pasteBurst = pasteBurst{}
	if !m.useTextareaView() {
		t.Fatal("setup: hi should use View")
	}
	// Simulate image attach chip — must flip the predicate.
	m.pendingAttach = []client.PromptAttachment{{ID: "a1", OriginalName: "a.png"}}
	if m.useTextareaView() {
		t.Fatal("attach chip must fallback to custom renderer")
	}
}

func TestTUIInput_Bubbles_View_FallsBackOnLongLineWrap(t *testing.T) {
	// Exact boundary: innerW = safeTermWidth(chatWidth)-2. Width 40 -> 37.
	// 100 a's is huge overflow — kept as smoke, but ccc7eba innerW alignment
	// now requires exact-boundary lock below; reverting to chatWidth()-2 or >
	// would still pass the 100 case.
	m := newTask308Model()
	m.width = 40
	m.fullWidth = 40
	for _, r := range strings.Repeat("a", 100) {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	m.pasteBurst = pasteBurst{}
	if m.useTextareaView() {
		t.Fatal("long line that overflows inner width must fallback to keep caret visible")
	}
	// Exact boundary — guards ccc7eba s safeTermWidth and >=
	for _, tc := range []struct {
		name string
		len  int
		want bool
	}{
		{"innerW-1 stays on View", func() int {
			w := safeTermWidth(m.chatWidth())
			innerW := w - 2
			if innerW < 1 {
				innerW = 1
			}
			return innerW - 1
		}(), true},
		{"innerW falls back", func() int {
			w := safeTermWidth(m.chatWidth())
			innerW := w - 2
			if innerW < 1 {
				innerW = 1
			}
			return innerW
		}(), false},
	} {
		m2 := newTask308Model()
		m2.width = 40
		m2.fullWidth = 40
		for _, r := range strings.Repeat("a", tc.len) {
			mm, _ := m2.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m2 = mm.(*AppModel)
		}
		m2.pasteBurst = pasteBurst{}
		if got := m2.useTextareaView(); got != tc.want {
			w := safeTermWidth(m2.chatWidth())
			innerW := w - 2
			t.Fatalf("%s: len=%d innerW=%d chatWidth=%d want View=%v got %v", tc.name, tc.len, innerW, m2.chatWidth(), tc.want, got)
		}
	}
}

func TestTUIInput_Bubbles_View_BurstShowsPastingPlaceholder(t *testing.T) {
	m := newTask308Model()
	m.pasteBurst.active = true
	if m.useTextareaView() {
		t.Fatal("burst active must fallback")
	}
	view := m.renderInputLine()
	if !strings.Contains(view, "[Pasting…]") {
		t.Fatalf("burst render must show [Pasting…], got %q", view)
	}
}

func TestTUIInput_Bubbles_View_AuthDoesNotLeak(t *testing.T) {
	m := newTask308Model()
	m.authPhase = AuthPassword
	m.inputValue = "secret123"
	view := m.renderInputLine()
	if strings.Contains(view, "secret123") {
		t.Fatalf("auth View leaked password, got %q", view)
	}
	if !strings.Contains(view, "*") {
		t.Fatalf("auth View should mask password, got %q", view)
	}
}
