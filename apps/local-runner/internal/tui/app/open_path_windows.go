//go:build windows

package app

import (
	"syscall"
	"unsafe"
)

var (
	shell32             = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
)

// openPath opens the file with the OS default handler without spawning
// cmd.exe. Using ShellExecuteW avoids the conhost race where
// `cmd /c start` attaches to the TUI console and a window-activate on
// Photos steals focus, leaving the terminal without KeyMsg/MouseMsg
// (log pid 12736: 49s stall motionLive=true after attach-open).
func openPath(path string) error {
	op, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(op)),
		uintptr(unsafe.Pointer(file)),
		0,
		0,
		1, // SW_SHOWNORMAL
	)
	if ret <= 32 {
		return syscall.Errno(ret)
	}
	return nil
}
