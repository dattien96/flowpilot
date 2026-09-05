//go:build !windows

package flowgate

import (
	"os/exec"
	"syscall"
)

// setSuiteProcessGroup puts the suite command in its own process group so a
// cancel/timeout can kill the whole tree. BUG-354 (run-540927): CommandContext
// kills only the direct child, but cmd.Wait() blocks until the inherited
// stdout/stderr pipes close — a grandchild (test binary / sleep) holding the
// pipes keeps executeSuite stuck well past its 5-minute deadline and the
// post-turn gate never returns (implement pinned RUNNING).
func setSuiteProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killSuiteProcessGroup SIGKILLs the suite's whole process group (set up by
// setSuiteProcessGroup). Idempotent; safe when the process already exited.
func killSuiteProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
