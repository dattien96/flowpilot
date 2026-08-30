package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Windows ConPTY mangles SS3 function-key sequences. Cursor (Chromium)
// sends F2 as "\x1bOQ" (vt100 SS3); ConPTY's VT->INPUT_RECORD translation
// drops the ESC and the record reader yields 'O' then 'Q' as two separate
// rune records — so F2/F4 land in the composer as "OQ"/"OS" text (tui.log
// pid 18400: KeyMsg runes "O","Q" then "O","S", then the burst guard
// rejected them). Windows Terminal sends proper VK_F2 records, which is why
// pid 19296 saw real KeyF2 messages.
//
// ss3FKeyState rebuilds the F-key from the 'O' + suffix pair within a tight
// window. A lone 'O' is held back until the suffix arrives, and flushed as
// text if the next key is not P/Q/R/S (or the window expires).
type ss3FKeyState struct {
	waiting bool
	at      time.Time
}

// ss3FKeyWindow bounds how long a bare 'O' is held. Real SS3 pairs arrive in
// the same ConPTY burst (a few ms apart); human typing is never this fast.
const ss3FKeyWindow = 80 * time.Millisecond

// ss3FKey processes one KeyMsg and reports how to continue:
//   - consumed=true, flush="", repl=KeyRunes: the msg was held (pending 'O')
//     or swallowed — the caller must insert nothing.
//   - consumed=true, flush!="", repl=KeyRunes: insert flush first, then
//     process the current msg as a normal key.
//   - consumed=true, flush="", repl=F-key: dispatch repl as the function key.
//   - consumed=false: pass the msg through untouched.
func (m *AppModel) ss3FKey(msg tea.KeyMsg) (repl tea.KeyMsg, flush string, consumed bool) {
	if msg.Type != tea.KeyRunes || msg.Paste || msg.Alt || len(msg.Runes) != 1 {
		if m.ss3.waiting {
			m.ss3.waiting = false
			return msg, "O", true
		}
		return msg, "", false
	}
	r := msg.Runes[0]
	if m.ss3.waiting {
		m.ss3.waiting = false
		if time.Since(m.ss3.at) <= ss3FKeyWindow {
			switch r {
			case 'P':
				return tea.KeyMsg{Type: tea.KeyF1}, "", true
			case 'Q':
				return tea.KeyMsg{Type: tea.KeyF2}, "", true
			case 'R':
				return tea.KeyMsg{Type: tea.KeyF3}, "", true
			case 'S':
				return tea.KeyMsg{Type: tea.KeyF4}, "", true
			}
		}
		// Not an SS3 suffix (or too late): flush the held 'O', then process
		// the current key normally.
		return msg, "O", true
	}
	if r == 'O' {
		m.ss3.waiting = true
		m.ss3.at = time.Now()
		return msg, "", true
	}
	return msg, "", false
}