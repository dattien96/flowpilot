//go:build windows

package app

import (
	"os/exec"
	"syscall"
)

const clipboardCreateNoWindow = 0x08000000

// applyClipboardSysProcAttr keeps PowerShell clipboard helpers from attaching
// to the TUI console on Windows. Without CREATE_NO_WINDOW the PS child
// inherits the conhost console and leaves it without KeyMsg/MouseMsg after
// Alt+V text paste (log pid 9288: 47s stall motionLive=true after
// ClipboardPasteMsg 192).
func applyClipboardSysProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= clipboardCreateNoWindow
}
