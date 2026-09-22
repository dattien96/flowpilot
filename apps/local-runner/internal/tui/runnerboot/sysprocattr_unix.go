//go:build !windows

package runnerboot

import (
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	// Intentionally not Setsid-detached. SIGHUP on terminal close must reach
	// the spawned runner (CA-474).
	_ = cmd
}

// setSysProcAttrShared detaches the spawned runner into its own session
// (CP-81 Task-416 T-1): SIGHUP on TUI terminal close no longer reaches the
// shared runner, and TUI death never kills a runner Desktop still holds a
// lease on. Orphan prevention is now owned by lease TTL + idle grace.
func setSysProcAttrShared(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func assignRunnerJob(pid uint32) error {
	_ = pid
	return nil
}

func closeRunnerJob() {}
