//go:build windows

package runner

import (
	"os/exec"
	"strconv"
)

// compilerGateCancel force-kills the whole command tree when the gate context
// expires. Recipes run through `cmd /c`, whose children (pnpm, tsc, sleep, ...)
// are separate processes: the default CommandContext kill only terminates
// cmd.exe, and the grandchildren keep the inherited output pipes open so
// cmd.Wait stalls until WaitDelay — the gate reported TimedOut but the process
// ran to completion. taskkill /T mirrors flowgate.killSuiteProcessGroup
// (BUG-354 run-540927 guard). Synchronous is safe: exec's watcher goroutine
// owns the call, and WaitDelay remains the backstop if taskkill stalls.
func compilerGateCancel(cmd *exec.Cmd) func() error {
	return func() error {
		if cmd == nil || cmd.Process == nil {
			return nil
		}
		return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}
}
