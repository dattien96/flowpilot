//go:build !windows

package app

// disableConsoleQuickEdit is a no-op on non-Windows platforms.
func disableConsoleQuickEdit() {}

func flushConsoleInputBuffer() {}

func restoreWindowsStdin() {}
