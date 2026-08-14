//go:build windows

package runnerboot

import (
	"os/exec"
)

func setSysProcAttr(cmd *exec.Cmd) {
	// Intentionally not detached. Closing the TUI terminal must take the
	// spawned runner with it (CA-474).
	_ = cmd
}
