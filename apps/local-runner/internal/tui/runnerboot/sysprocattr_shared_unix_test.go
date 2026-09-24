//go:build !windows

package runnerboot

import (
	"os/exec"
	"testing"
)

// assertSharedRunnerDetached verifies the Unix detach mechanism: Setsid moves
// the spawned runner into its own session so TUI death (SIGHUP) cannot reach a
// shared runner that Desktop still leases (CP-81 Task-416 T-1).
func assertSharedRunnerDetached(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatal("unix shared spawn must Setsid-detach from the TUI session")
	}
}
