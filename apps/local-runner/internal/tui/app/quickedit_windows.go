//go:build windows

package app

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	enableProcessedInput       = 0x0001
	enableLineInput            = 0x0002
	enableEchoInput            = 0x0004
	enableMouseInput           = 0x0010
	enableQuickEditMode        = 0x0040
	enableExtendedFlags        = 0x0080
	enableVirtualTerminalInput = 0x0200
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode          = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode          = kernel32.NewProc("SetConsoleMode")
	procFlushConsoleInputBuffer = kernel32.NewProc("FlushConsoleInputBuffer")

	stdinModeToRestore uint32
	stdinModeSaved     bool
)

// disableConsoleQuickEdit keeps the old name for tests; it now puts stdin
// into raw VT mode as well (WithInput wrapper skips bubbletea MakeRaw).
func disableConsoleQuickEdit() {
	prepareWindowsConsoleInput()
}

func prepareWindowsConsoleInput() {
	h := os.Stdin.Fd()
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return
	}
	if !stdinModeSaved {
		stdinModeToRestore = mode
		stdinModeSaved = true
	}
	mode &^= enableEchoInput | enableLineInput | enableProcessedInput | enableQuickEditMode | enableMouseInput
	mode |= enableExtendedFlags | enableVirtualTerminalInput
	_, _, _ = procSetConsoleMode.Call(h, uintptr(mode))
	tuiLog("prepared Windows stdin VT mode fd=%d mode=0x%x", h, mode)
}

func restoreWindowsStdin() {
	if !stdinModeSaved {
		return
	}
	h := os.Stdin.Fd()
	_, _, _ = procSetConsoleMode.Call(h, uintptr(stdinModeToRestore))
	stdinModeSaved = false
}

func flushConsoleInputBuffer() {
	h := os.Stdin.Fd()
	r, _, _ := procFlushConsoleInputBuffer.Call(h)
	tuiLog("flushConsoleInputBuffer stdin fd=%d ok=%v", h, r != 0)
}
