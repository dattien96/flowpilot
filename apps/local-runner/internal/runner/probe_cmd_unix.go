//go:build !windows

package runner

import (
	"os/exec"
)

// applyProbeSysProcAttr is a no-op on Unix: probes there do not attach to a
// shared console in the way Windows processes do, and stdin is already DevNull.
func applyProbeSysProcAttr(cmd *exec.Cmd) {
	_ = cmd
}
