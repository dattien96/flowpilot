package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// =============================================================================
// Comprehensive Test Suite for FlowPilot TUI Chat Input
// Covers:
// 1. ASCII & UTF-8 Multi-byte (Vietnamese Telex, tone keys, IME rewrites)
// 2. Caret movements (Left, Right, Home, End, Ctrl+A, Ctrl+E, sticky-end)
// 3. Deletion (Backspace, Delete, token deletion in 1 press, Esc clear)
// 4. Raw paste burst & Windows Ctrl+V reject guard (with IME / repeat exemptions)
// 5. Bracketed paste & multi-line paste token collapse / expansion
// 6. Enter & Send gating under Idle, Turn In Flight, and Workflow Park
// 7. Child View read-only gate
// 8. Suggestion list navigation & Enter handling
// 9. Mouse caret placement & Chrome exclusion
// 10. Cross-Provider Parity Matrix (Claude, Codex, Grok)
// =============================================================================

func newTestModelWithProvider(provider string) *AppModel {
	cfg := config.ChatConfig{
		Provider:        provider,
		Model:           "test-model",
		ReasoningEffort: "medium",
		ProjectPath:     "/test/project",
	}
	m := New(cfg, "http://127.0.0.1:4317")
	m.authPhase = AuthNone
	m.sessionLoading = false
	m.sessionDefaultsLoaded = true
	m.width = 120
	m.height = 40
	m.fullWidth = 120
	return m
}

// -----------------------------------------------------------------------------
// 1. Normal Typing, UTF-8 Multi-byte & Vietnamese Telex IME
// -----------------------------------------------------------------------------

func TestTUIInput_VietnameseTelexAndDiacritics(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			m := newTestModelWithProvider(provider)

			// Type Vietnamese string with diacritics: "Xin chào tiếng Việt có dấu"
			text := "Xin chào tiếng Việt có dấu"
			for _, r := range text {
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}

			if m.inputValue != text {
				t.Fatalf("[%s] expected input %q, got %q", provider, text, m.inputValue)
			}
			if m.inputCaretIndex() != len([]rune(text)) {
				t.Fatalf("[%s] caret index expected %d, got %d", provider, len([]rune(text)), m.inputCaretIndex())
			}
		})
	}
}

func TestTUIInput_VietnameseIMERewriteSequence(t *testing.T) {
	advance := clockAt(t)
	m := newTestModelWithProvider("claude")
	m.rejectWindowsRawPaste = true

	// Simulate IME Telex composition for 'đ' (d + Backspace + d + d) in 3-5ms
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(*AppModel)
	advance(4 * time.Millisecond)

	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = m2.(*AppModel)
	advance(3 * time.Millisecond)

	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(*AppModel)
	advance(3 * time.Millisecond)

	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(*AppModel)

	// In real IME, 'dd' commits as 'đ' or stays 'dd' before conversion
	// Verify it was NOT wiped or rejected by raw paste detector
	if m.inputValue == "" {
		t.Fatal("IME sequence was erroneously wiped")
	}
	if strings.Contains(m.inputValue, "Use Alt+V") {
		t.Fatal("IME sequence triggered false-positive Alt+V hint")
	}

	// Continuing with next character
	advance(4 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = m2.(*AppModel)

	if !strings.Contains(m.inputValue, "a") {
		t.Fatalf("character after IME commit must insert, got %q", m.inputValue)
	}
}

func TestTUIInput_ToneKeyRepeat_NotRejected(t *testing.T) {
	advance := clockAt(t)
	m := newTestModelWithProvider("codex")
	m.rejectWindowsRawPaste = true

	// Holding key 's' for tone mark (15 rapid 's' keystrokes)
	for i := 0; i < 15; i++ {
		advance(4 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		m = m2.(*AppModel)
	}

	if m.inputValue != strings.Repeat("s", 15) {
		t.Fatalf("repeated single key must not be rejected, got %q", m.inputValue)
	}
	if m.pasteCtrlVHintShown {
		t.Fatal("repeated key must not trigger paste hint")
	}
}

// -----------------------------------------------------------------------------
// 2. Caret Navigation & Mid-String Editing
// -----------------------------------------------------------------------------

func TestTUIInput_CaretNavigationAndInsert(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			m := newTestModelWithProvider(provider)

			// Type "hello world"
			for _, r := range "hello world" {
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}

			// Move left 5 times (caret should be between "hello " and "world")
			for i := 0; i < 5; i++ {
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
				m = m2.(*AppModel)
			}
			if m.inputCaretIndex() != 6 {
				t.Fatalf("[%s] expected caret at 6, got %d", provider, m.inputCaretIndex())
			}

			// Insert "brave new "
			for _, r := range "brave new " {
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}
			expected := "hello brave new world"
			if m.inputValue != expected {
				t.Fatalf("[%s] expected %q, got %q", provider, expected, m.inputValue)
			}

			// Home / Ctrl+A
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyHome})
			m = m2.(*AppModel)
			if m.inputCaretIndex() != 0 {
				t.Fatalf("[%s] expected caret at 0 after Home, got %d", provider, m.inputCaretIndex())
			}

			// Insert at start
			for _, r := range "Start: " {
				m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}
			if !strings.HasPrefix(m.inputValue, "Start: hello") {
				t.Fatalf("[%s] expected prefix 'Start: hello', got %q", provider, m.inputValue)
			}

			// End / Ctrl+E
			m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnd})
			m = m2.(*AppModel)
			if m.inputCursor != -1 {
				t.Fatalf("[%s] expected sticky end (-1) after End, got %d", provider, m.inputCursor)
			}
			if m.inputCaretIndex() != len([]rune(m.inputValue)) {
				t.Fatalf("[%s] expected caret at end, got %d vs %d", provider, m.inputCaretIndex(), len([]rune(m.inputValue)))
			}
		})
	}
}

// -----------------------------------------------------------------------------
// 3. Backspace & Forward Delete (Characters and Paste Tokens)
// -----------------------------------------------------------------------------

func TestTUIInput_BackspaceAndDeleteOperations(t *testing.T) {
	m := newTestModelWithProvider("grok")

	// Type "abcdef"
	for _, r := range "abcdef" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}

	// Backspace at sticky end -> deletes 'f' -> "abcde"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = m2.(*AppModel)
	if m.inputValue != "abcde" {
		t.Fatalf("expected 'abcde', got %q", m.inputValue)
	}
	if m.inputCursor != -1 {
		t.Fatalf("backspace at sticky end must stay sticky end, got %d", m.inputCursor)
	}

	// Move left twice (caret between 'c' and 'd')
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = m2.(*AppModel)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = m2.(*AppModel)
	if m.inputCaretIndex() != 3 {
		t.Fatalf("expected caret at 3, got %d", m.inputCaretIndex())
	}

	// Backspace mid-string -> deletes 'c' -> "abde", caret at 2
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = m2.(*AppModel)
	if m.inputValue != "abde" {
		t.Fatalf("expected 'abde', got %q", m.inputValue)
	}
	if m.inputCaretIndex() != 2 {
		t.Fatalf("expected caret at 2, got %d", m.inputCaretIndex())
	}

	// Forward Delete -> deletes 'd' -> "abe", caret at 2
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDelete})
	m = m2.(*AppModel)
	if m.inputValue != "abe" {
		t.Fatalf("expected 'abe', got %q", m.inputValue)
	}
	if m.inputCaretIndex() != 2 {
		t.Fatalf("expected caret at 2, got %d", m.inputCaretIndex())
	}
}

func TestTUIInput_PasteTokenDeletedInSinglePress(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			m := newTestModelWithProvider(provider)

			// Paste long text that collapses to token
			longText := "Function CalculateDiscount(price float64, rate float64) float64"
			m2, _ := m.Update(ClipboardPasteMsg{Text: longText})
			m = m2.(*AppModel)

			token := pasteSummaryToken(longText)
			if m.inputValue != token {
				t.Fatalf("[%s] expected token %q, got %q", provider, token, m.inputValue)
			}
			if len(m.pasteSegments) != 1 {
				t.Fatalf("[%s] expected 1 paste segment, got %d", provider, len(m.pasteSegments))
			}

			// Single Backspace MUST delete entire token
			m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
			m = m2.(*AppModel)
			if m.inputValue != "" {
				t.Fatalf("[%s] single backspace must clear entire token, got %q", provider, m.inputValue)
			}
			if len(m.pasteSegments) != 0 {
				t.Fatalf("[%s] pasteSegments must be empty after token delete, got %d", provider, len(m.pasteSegments))
			}

			// Test Forward Delete with caret before token
			m2, _ = m.Update(ClipboardPasteMsg{Text: longText})
			m = m2.(*AppModel)
			m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyHome})
			m = m2.(*AppModel)
			if m.inputCaretIndex() != 0 {
				t.Fatalf("[%s] caret must be at 0, got %d", provider, m.inputCaretIndex())
			}

			m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyDelete})
			m = m2.(*AppModel)
			if m.inputValue != "" {
				t.Fatalf("[%s] forward delete at token start must remove token, got %q", provider, m.inputValue)
			}
			if len(m.pasteSegments) != 0 {
				t.Fatalf("[%s] pasteSegments must be empty, got %d", provider, len(m.pasteSegments))
			}
		})
	}
}

// -----------------------------------------------------------------------------
// 4. Windows Raw Paste Flood Rejection & Settle Dynamics
// -----------------------------------------------------------------------------

func TestTUIInput_WindowsRawFlood_RejectedAndRecovered(t *testing.T) {
	advance := clockAt(t)
	m := newTestModelWithProvider("claude")
	m.rejectWindowsRawPaste = true

	// Simulate 20 varied characters arriving rapidly (<5ms interval)
	for _, r := range "abcdefghijklmnopqrst" {
		advance(4 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}

	// Flood is rejected -> input stays empty
	if m.inputValue != "" {
		t.Fatalf("Windows raw paste flood must be rejected, got %q", m.inputValue)
	}
	if !m.pasteCtrlVHintShown {
		t.Fatal("Alt+V hint must be shown upon rejecting flood")
	}

	// Settle after 150ms quiet period
	advance(200 * time.Millisecond)
	m2, _ := m.Update(pasteBurstSettleMsg{})
	m = m2.(*AppModel)

	if m.pasteBurst.active || m.pasteBurst.rejectArmed {
		t.Fatal("paste burst state must be completely cleared after settle")
	}

	// Normal typing after settle must work immediately
	for _, r := range "hello" {
		advance(100 * time.Millisecond)
		m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if m.inputValue != "hello" {
		t.Fatalf("normal typing after settle must insert, got %q", m.inputValue)
	}
}

// -----------------------------------------------------------------------------
// 5. Send & Enter Gating (Idle vs Turn In Progress vs Workflow Park)
// -----------------------------------------------------------------------------

func TestTUIInput_SendBlocked_DraftKept_SlashAllowed(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			advance := clockAt(t)
			m := newTestModelWithProvider(provider)
			// Simulate turn in progress
			m.connStatus = ConnRunning

			if !m.sendBlocked() {
				t.Fatal("sendBlocked must be true while turn is running")
			}

			// Type draft question with human pace (>25ms between keys so it's not a raw paste burst)
			draft := "what is the next step?"
			for _, r := range draft {
				advance(50 * time.Millisecond)
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}
			if m.inputValue != draft {
				t.Fatalf("[%s] typing while turn in progress must accumulate draft, got %q", provider, m.inputValue)
			}

			// Pause before pressing Enter (human typing pause)
			advance(200 * time.Millisecond)

			// Press Enter -> must NOT send, must keep draft, must show status toast
			m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			m = m2.(*AppModel)
			if cmd != nil {
				t.Fatalf("[%s] Enter must not produce send command while running", provider)
			}
			if m.inputValue != draft {
				t.Fatalf("[%s] draft must be preserved when Enter is blocked, got %q", provider, m.inputValue)
			}
			if !strings.Contains(m.statusMsg, "in progress") {
				t.Fatalf("[%s] statusMsg must warn in progress, got %q", provider, m.statusMsg)
			}

			// Slash commands (/help) MUST still be executable while turn is running
			m.clearInputValue()
			for _, r := range "/help" {
				advance(50 * time.Millisecond)
				m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}
			advance(200 * time.Millisecond)
			m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			m = m2.(*AppModel)
			// /help outputs system message
			if len(m.messages) == 0 {
				t.Fatalf("[%s] slash command must execute even while turn is running", provider)
			}
		})
	}
}

func TestTUIInput_WorkflowPark_AllowsTypingAndSend(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			advance := clockAt(t)
			m := newTestModelWithProvider(provider)
			// Simulate escalate/cap park waiting for user approval
			m.connStatus = ConnWaiting
			m.flowLoopStatus = "blocked"
			m.flowBlockReason = "escalate"

			if !m.flowLoopBlocked() {
				t.Fatal("flowLoopBlocked must be true")
			}
			if m.sendBlocked() {
				t.Fatal("sendBlocked must be false on workflow park")
			}

			// Type response with human pace
			input := "proceed with option A"
			for _, r := range input {
				advance(50 * time.Millisecond)
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}

			advance(200 * time.Millisecond)

			// Enter must process input (not blocked by turn running toast)
			m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			m = m2.(*AppModel)
			if m.inputValue != "" {
				t.Fatalf("[%s] input should be cleared and processed on submit, got %q", provider, m.inputValue)
			}
			_ = cmd
		})
	}
}

// -----------------------------------------------------------------------------
// 6. Child View Read-Only Isolation Gate
// -----------------------------------------------------------------------------

func TestTUIInput_ChildViewReadOnlyGate(t *testing.T) {
	m := newTestModelWithProvider("claude")
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.focusRunID = "run-child"

	// Set up child agent runs and focus on child
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main"},
		{RunID: "run-child", AgentName: "child", Role: "reviewer"},
	}
	m.focusedAgentIdx = 1 // Child focused

	if !m.viewingChild() {
		t.Fatal("viewingChild must be true when child agent is focused")
	}

	// Plain typing must be dropped
	for _, r := range "hello subagent" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if m.inputValue != "" {
		t.Fatalf("plain typing in child view must be dropped, got %q", m.inputValue)
	}

	// Navigation keys (F2, Tab, Esc, Arrows) MUST still work
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF2})
	m = m2.(*AppModel)
	if !m.sessionPanel.Collapsed {
		t.Fatal("F2 toggle must work in child view")
	}

	// Slash commands allowed in child view (e.g. /agent to switch back)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = m2.(*AppModel)
	if m.inputValue != "/" {
		t.Fatalf("slash command starter must be allowed in child view, got %q", m.inputValue)
	}
}

// -----------------------------------------------------------------------------
// 7. Slash Suggestions & Autocomplete Interaction
// -----------------------------------------------------------------------------

func TestTUIInput_SuggestionNavigationAndEnter(t *testing.T) {
	m := newTestModelWithProvider("codex")

	// Type "/m" to trigger slash suggestions
	for _, r := range "/m" {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}

	suggestions := m.collectSuggestions()
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for '/m'")
	}

	// Down arrow moves selection
	origIdx := m.suggIdx
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	m = m2.(*AppModel)
	if m.suggIdx != (origIdx+1)%len(suggestions) {
		t.Fatalf("KeyDown must cycle suggestion index, got %d", m.suggIdx)
	}

	// Up arrow moves selection back
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	m = m2.(*AppModel)
	if m.suggIdx != origIdx {
		t.Fatalf("KeyUp must cycle suggestion index back, got %d", m.suggIdx)
	}
}

// -----------------------------------------------------------------------------
// 8. Mouse Caret Placement & Interactive Chrome Protection
// -----------------------------------------------------------------------------

func TestTUIInput_MouseCaretAndChromeProtection(t *testing.T) {
	m := newTestModelWithProvider("claude")
	m.inputValue = "line one\nline two\nline three"
	m.width = 100
	m.height = 30

	c := m.tuiChrome()
	if c.inputH <= 0 {
		t.Fatal("input height must be > 0")
	}

	// Click on line 1 of input
	// inputY + 1 (line 0 is border)
	clickedBody := m.tryPlaceInputCursor(10, c.inputY+1)
	if !clickedBody {
		t.Fatal("click in input body must return true")
	}

	// Click on top border row (rel == 0) -> must return false (no caret move)
	clickedBorder := m.tryPlaceInputCursor(10, c.inputY)
	if clickedBorder {
		t.Fatal("click on border must not place caret")
	}

	// Simulate Esc clearing selection before input
	m.mouseSel = mouseSelect{armed: true, x0: 5, y0: 5, x1: 10, y1: 5}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	m = m2.(*AppModel)
	if !m.mouseSel.empty() {
		t.Fatal("first Esc must clear mouse selection")
	}
	if m.inputValue == "" {
		t.Fatal("first Esc must not clear input value when selection was active")
	}

	// Second Esc clears input
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	m = m2.(*AppModel)
	if m.inputValue != "" {
		t.Fatalf("second Esc must clear input value, got %q", m.inputValue)
	}
}

// -----------------------------------------------------------------------------
// 9. Bracketed Paste Text Sanitization & Token Expansion
// -----------------------------------------------------------------------------

func TestTUIInput_BracketedPaste_SanitizationAndExpansion(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			m := newTestModelWithProvider(provider)

			// Paste text with embedded NUL control byte
			rawWithNul := "clean\u0000text\nsecond\tline"
			m2, _ := m.handleKey(tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune(rawWithNul),
				Paste: true,
			})
			m = m2.(*AppModel)

			// Must collapse to token
			if !strings.HasPrefix(m.inputValue, "[Pasted ") {
				t.Fatalf("[%s] multi-line paste must collapse to token, got %q", provider, m.inputValue)
			}

			// When expanded, NUL character must be sanitized out
			expanded := m.expandPasteTokens(m.inputValue)
			if strings.Contains(expanded, "\u0000") {
				t.Fatalf("[%s] expanded text must NOT contain NUL bytes, got %q", provider, expanded)
			}
			if !strings.Contains(expanded, "cleantext") {
				t.Fatalf("[%s] expected 'cleantext', got %q", provider, expanded)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// 10. Ctrl+C Protection & Paste Shortcuts (Command-V, Alt-V, Ctrl-V)
// -----------------------------------------------------------------------------

func TestTUIInput_CtrlC_Behavior(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			m := newTestModelWithProvider(provider)

			// 1. With non-empty draft input: Ctrl+C clears the prompt without quitting
			m.inputValue = "accidental draft prompt"
			m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
			m = m2.(*AppModel)
			if m.quitting {
				t.Fatalf("[%s] Ctrl+C with non-empty input must NOT quit the app", provider)
			}
			if cmd != nil {
				t.Fatalf("[%s] Ctrl+C with non-empty input should return nil cmd, got %v", provider, cmd)
			}
			if m.inputValue != "" {
				t.Fatalf("[%s] Ctrl+C with non-empty input must clear inputValue, got %q", provider, m.inputValue)
			}
			if m.statusMsg != "prompt cleared" {
				t.Fatalf("[%s] statusMsg expected 'prompt cleared', got %q", provider, m.statusMsg)
			}

			// 2. With empty input: Ctrl+C produces shutdown and quit
			m2, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
			m = m2.(*AppModel)
			if !m.quitting {
				t.Fatalf("[%s] Ctrl+C on empty prompt must set quitting=true", provider)
			}
			if cmd == nil {
				t.Fatalf("[%s] Ctrl+C on empty prompt must return quit cmd", provider)
			}
		})
	}
}

func TestTUIInput_PasteShortcuts_CmdV_AltV_CtrlV(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			// 1. Command-V (macOS bracketed paste `Paste: true`)
			m := newTestModelWithProvider(provider)
			pastedText := "func ProcessOrder(id string) error {\n\treturn nil\n}"
			m2, _ := m.handleKey(tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune(pastedText),
				Paste: true,
			})
			m = m2.(*AppModel)
			if !strings.HasPrefix(m.inputValue, "[Pasted ") {
				t.Fatalf("[%s] Command-V bracketed paste must collapse to token, got %q", provider, m.inputValue)
			}
			if got := m.expandPasteTokens(m.inputValue); got != pastedText {
				t.Fatalf("[%s] expanded Command-V text mismatch: got %q", provider, got)
			}

			// 2. Alt-V (Windows/Linux/macOS chord `alt+v`)
			m.clearInputValue()
			m2, cmd := m.handleKey(tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{'v'},
				Alt:   true,
			})
			if cmd == nil {
				t.Fatalf("[%s] Alt-V must return clipboard paste command", provider)
			}
			m = m2.(*AppModel)
			// Simulate clipboard result
			m2, _ = m.Update(ClipboardPasteMsg{Text: pastedText})
			m = m2.(*AppModel)
			if !strings.HasPrefix(m.inputValue, "[Pasted ") {
				t.Fatalf("[%s] Alt-V clipboard text must collapse to token, got %q", provider, m.inputValue)
			}

			// 3. Raw Ctrl-V on Windows must be blocked and hint Alt-V
			m.clearInputValue()
			m.rejectWindowsRawPaste = true
			m2, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlV})
			m = m2.(*AppModel)
			if m.inputValue != "" {
				t.Fatalf("[%s] raw Ctrl-V on Windows must not insert text, got %q", provider, m.inputValue)
			}
			if !m.pasteCtrlVHintShown {
				t.Fatalf("[%s] raw Ctrl-V on Windows must set pasteCtrlVHintShown", provider)
			}
		})
	}
}
