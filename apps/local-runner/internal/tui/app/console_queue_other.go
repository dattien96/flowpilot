//go:build !windows

package app

func pendingConsoleInputEvents() int {
	return -1
}