//go:build windows

package app

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	enableQuickEditMode = 0x0040
	enableExtendedFlags = 0x0080
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

// disableConsoleQuickEdit clears ENABLE_QUICK_EDIT_MODE on Windows console stdin.
// When QuickEdit is enabled, clicking or dragging on the terminal window enters
// Select mode (Mark mode), which suspends all console input and output until Esc
// or Enter is pressed. Disabling it ensures mouse clicks reach the TUI and never
// freeze the event loop.
func disableConsoleQuickEdit() {
	h := os.Stdin.Fd()
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	if r != 0 {
		mode &^= enableQuickEditMode
		mode |= enableExtendedFlags
		_, _, _ = procSetConsoleMode.Call(h, uintptr(mode))
		tuiLog("disabled QuickEdit mode on Windows console (stdin fd=%d)", h)
	}
}
