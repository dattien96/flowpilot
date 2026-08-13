//go:build windows

package app

import "syscall"

const (
	vkShift   = 0x10
	vkControl = 0x11
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procGetKeyState = user32.NewProc("GetKeyState")
)

func init() {
	enterModifierHeld = windowsEnterModifiers
}

func windowsEnterModifiers() (ctrl, shift bool) {
	return virtKeyDown(vkControl), virtKeyDown(vkShift)
}

func virtKeyDown(vk uintptr) bool {
	r, _, _ := procGetKeyState.Call(vk)
	return r&0x8000 != 0
}
