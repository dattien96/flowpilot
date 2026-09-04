//go:build windows

package flowgate

import (
	"os/exec"
	"strconv"
)

// setSuiteProcessGroup is a no-op on Windows; killSuiteProcessGroup uses
// taskkill /T to take the tree down instead.
func setSuiteProcessGroup(cmd *exec.Cmd) {}

// killSuiteProcessGroup force-kills the suite command tree (BUG-354 run-540927
// guard). taskkill /T covers children that inherited the output pipes and
// would otherwise keep cmd.Wait blocked past the suite deadline. Fire-and-
// forget (review F4): a slow/hung taskkill must never block executeSuite past
// its deadline+grace the way a stuck suite would — the 2s grace in
// executeSuite owns the bound.
func killSuiteProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	pid := cmd.Process.Pid
	go func() {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	}()
}
