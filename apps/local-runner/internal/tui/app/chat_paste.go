package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Paste-summary constants (CA-560). Any paste that forms a text block — multi-
// line, or at least pasteCollapseMinRunes long — collapses to a one-line
// "[Pasted N chars]" placeholder in the composer instead of inserting the raw
// text (opencode-style). The full text is kept in pasteSegments and expanded
// when the prompt is submitted, so the timeline shows the real text while the
// composer stays a single line and never auto-submits partial lines. Single
// keystrokes (stuck-paste typing, tiny bits like " world") insert verbatim.
const (
	// pasteCollapseMinRunes is the shortest single-line paste that collapses.
	pasteCollapseMinRunes = 8
	// burstRuneGap is the inter-key interval under which consecutive runes are
	// considered one paste burst. Human typing is far slower than 25ms/key, so
	// this only ever matches real pastes.
	burstRuneGap = 25 * time.Millisecond
	// burstSettle is the quiet gap after which a paste burst is considered done.
	burstSettle = 150 * time.Millisecond
)

// pasteNow is the injectable clock for burst tests.
var pasteNow = time.Now

// pasteBurstSettleMsg fires ~burstSettle after the last raw-paste key so the
// burst region collapses to a token even when the user presses nothing after
// pasting (no need to wait for the next keystroke).
type pasteBurstSettleMsg struct{}

func cmdPasteBurstSettle() tea.Cmd {
	return tea.Tick(burstSettle, func(time.Time) tea.Msg { return pasteBurstSettleMsg{} })
}

func (m *AppModel) cmdPasteBurstSettleOnce() tea.Cmd {
	if m.pasteBurst.settlePending {
		return nil
	}
	m.pasteBurst.settlePending = true
	return cmdPasteBurstSettle()
}

// pasteSegment holds the full text behind one collapsed placeholder token.
type pasteSegment struct {
	token string // exact text present in inputValue
	text  string // full pasted text, expanded on submit
}

// pasteBurst tracks a raw (non-bracketed) paste delivered as a flood of
// individual key events (Windows conhost / per-char terminal delivery). While a
// burst is active, Enter is treated as a newline instead of a submit — that is
// what turned every pasted line into a separate auto-sent message. When the
// burst settles, the tracked region collapses to a paste token.
type pasteBurst struct {
	active        bool
	start         int // rune index in inputValue where the burst region begins
	buf           []rune
	chainLen      int   // consecutive rapid runes in the current chain
	chainStart    int   // inputValue rune length before the current chain began
	chainBuf      []rune
	lastRuneAt    time.Time
	rejectArmed   bool // CA-612: Windows reject stays armed across 25ms gaps until settle
	settlePending bool // at most one settle Tick in flight (prevents per-rune storm)
}

// pasteSummaryToken renders the visible placeholder for a collapsed paste — one
// generic line so the composer never shows the raw block.
func pasteSummaryToken(text string) string {
	return fmt.Sprintf("[Pasted %d chars]", len([]rune(text)))
}

// needsPasteSummary reports whether a pasted block should collapse to a token
// instead of being inserted verbatim: multi-line, or a block of at least
// pasteCollapseMinRunes chars. Single keystrokes and tiny snippets stay inline.
func needsPasteSummary(text string) bool {
	return len([]rune(text)) >= pasteCollapseMinRunes || strings.Contains(text, "\n")
}

// sanitizePasteText strips C0 control characters (NUL and friends) from
// externally pasted text, keeping only meaningful whitespace (\n \r \t).
// A prompt copied from a tool/terminal can carry a leading \u0000; when such a
// string is written to the Windows clipboard it is encoded to UTF-16 and the
// first code unit 0x0000 is read back as the string terminator — so the
// clipboard ends up EMPTY even though the app "copied" a full prompt
// (run-117747: prompt starting with \u0000 could not be copied or expanded).
func sanitizePasteText(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// isBurstNewlineKey reports whether the key is a raw newline delivery (Enter,
// Ctrl+J, or a lone '\n' rune) that a paste burst may swallow as a line break.
func isBurstNewlineKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyCtrlJ {
		return true
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == '\n' {
		return true
	}
	return msg.Type == tea.KeyEnter && !msg.Alt
}

// insertPasteSummary collapses a long paste into a placeholder token and keeps
// the full text for expansion on submit.
func (m *AppModel) insertPasteSummary(text string) {
	token := pasteSummaryToken(text)
	m.pasteSegments = append(m.pasteSegments, pasteSegment{token: token, text: text})
	m.insertInputAtCursor(token)
}

// expandPasteTokens replaces collapsed placeholders in input with their full
// pasted text. Applied right before submit so the timeline and prompt history
// receive the real prompt, not the placeholder.
func (m *AppModel) expandPasteTokens(input string) string {
	out := input
	for _, seg := range m.pasteSegments {
		out = strings.Replace(out, seg.token, seg.text, 1)
	}
	return out
}

// noteBurstRune advances burst tracking for one rune insertion (non-bracketed
// paste or fast typing). Must be called with the text actually inserted.
func (m *AppModel) noteBurstRune(s string) {
	b := &m.pasteBurst
	n := len([]rune(s))
	if n == 0 {
		return
	}
	// Slash commands are typed deliberately, never pasted. Never burst-guard an
	// input that is running a command (fast programmatic/typed keys would arm
	// the guard and swallow the Enter that runs the command).
	if strings.HasPrefix(m.inputValue, "/") {
		m.resetPasteBurst()
		return
	}
	now := pasteNow()
	if now.Sub(b.lastRuneAt) >= burstRuneGap {
		// New chain — this rune is the first of a fresh burst candidate.
		b.chainLen = n
		b.chainStart = len([]rune(m.inputValue))
		b.chainBuf = append(b.chainBuf[:0], []rune(s)...)
	} else {
		b.chainLen += n
		b.chainBuf = append(b.chainBuf, []rune(s)...)
	}
	b.lastRuneAt = now
	if !b.active && b.chainLen >= 2 {
		// Two rapid runes confirm a paste flood: arm the region.
		b.active = true
		b.start = b.chainStart
		b.buf = append(b.buf[:0], b.chainBuf...)
	} else if b.active {
		b.buf = append(b.buf, []rune(s)...)
	}
}

// handleBurstNewline converts a newline key into a paste-burst newline when it
// arrives inside a rapid rune flood (a raw non-bracketed paste). Returns true
// when swallowed; the caller must not treat it as a submit.
func (m *AppModel) handleBurstNewline(now time.Time) bool {
	b := &m.pasteBurst
	if strings.HasPrefix(m.inputValue, "/") || now.Sub(b.lastRuneAt) >= burstRuneGap || b.chainLen < 1 {
		// Slash command or not part of a rapid paste flood — a real Enter.
		return false
	}
	if !b.active {
		// First newline of a raw paste: arm the region from the pending chain.
		b.active = true
		b.start = b.chainStart
		b.buf = append(append([]rune{}, b.chainBuf...), '\n')
	} else {
		b.buf = append(b.buf, '\n')
	}
	b.lastRuneAt = now
	m.insertInputAtCursor("\n")
	return true
}

// collapsePasteBurst finishes an active burst: the tracked region is replaced
// by a placeholder when it is long enough, otherwise it stays verbatim. State
// is always reset so subsequent keys behave normally.
func (m *AppModel) collapsePasteBurst() {
	b := &m.pasteBurst
	if !b.active {
		m.resetPasteBurst()
		return
	}
	start := b.start
	raw := string(b.buf)
	m.resetPasteBurst()
	// Guard: only rewrite when the region still matches what we tracked. If the
	// caret moved or a "/" rewrite shifted the text mid-burst, keep it verbatim.
	end := start + len([]rune(raw))
	runes := []rune(m.inputValue)
	if start < 0 || start > len(runes) || end > len(runes) {
		return
	}
	if string(runes[start:end]) != raw {
		return
	}
	// NUL/control bytes can ride along in a raw paste; scrub them from the
	// inserted region so the eventual message copies cleanly (run-117747).
	text := sanitizePasteText(raw)
	if !needsPasteSummary(text) {
		// Below the collapse threshold: stay verbatim, but drop control junk.
		if text != raw {
			repl := append(append([]rune{}, runes[:start]...), []rune(text)...)
			repl = append(repl, runes[end:]...)
			m.inputValue = string(repl)
			m.setInputCaret(start + len([]rune(text)))
		}
		return
	}
	token := pasteSummaryToken(text)
	repl := append(append([]rune{}, runes[:start]...), []rune(token)...)
	repl = append(repl, runes[end:]...)
	m.inputValue = string(repl)
	m.setInputCaret(start + len([]rune(token)))
	m.pasteSegments = append(m.pasteSegments, pasteSegment{token: token, text: text})
}

func (m *AppModel) resetPasteBurst() {
	m.pasteBurst = pasteBurst{}
	m.pasteHijacked = false
}

// isSingleRepeatedRuneChain reports whether buf is a single rune repeated
// (e.g. "ssss"). Holding a tone key or IME repeat produces such a chain;
// a real WT Ctrl+V paste of varied text does not. Used to avoid
// false-positive Windows paste rejection for Vietnamese Telex (CA-611).
func isSingleRepeatedRuneChain(buf []rune) bool {
	if len(buf) < 2 {
		return false
	}
	first := buf[0]
	for _, r := range buf[1:] {
		if r != first {
			return false
		}
	}
	return true
}