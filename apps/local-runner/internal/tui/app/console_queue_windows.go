//go:build windows

package app

import "github.com/erikgeiser/coninput"

// pendingConsoleInputEvents reports how many INPUT_RECORDs are waiting in the
// console input queue right now (-1 when it cannot be queried). The watchdog
// stamps it into the stall fingerprint so a wedge is attributable: records
// piling up = the app reader stopped consuming (app-side); zero records = the
// host stopped delivering keys (Cursor/ConPTY-side).
func pendingConsoleInputEvents() int {
	h, err := coninput.NewStdinHandle()
	if err != nil {
		return -1
	}
	n, err := coninput.GetNumberOfConsoleInputEvents(h)
	if err != nil {
		return -1
	}
	return int(n)
}
