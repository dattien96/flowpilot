//go:build !windows

package runnerboot

import (
	"os/exec"
)

func setSysProcAttr(cmd *exec.Cmd) {
	// Intentionally not Setsid-detached. SIGHUP on terminal close must reach
	// the spawned runner (CA-474).
	_ = cmd
}

func assignRunnerJob(pid uint32) error {
	_ = pid
	return nil
}

func closeRunnerJob() {}
