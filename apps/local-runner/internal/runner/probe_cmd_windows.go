//go:build windows

package runner

import (
	"os/exec"
	"syscall"
)

// createNoWindow mirrors syscall's CREATE_NO_WINDOW (0x08000000), which is not
// exposed as a named constant in the stdlib syscall package.
const createNoWindow = 0x08000000

// applyProbeSysProcAttr keeps provider-detection probes from attaching to the
// TUI's console on Windows. CREATE_NO_WINDOW stops the probe from allocating
// a console of its own and inheriting the parent's console input, which is the
// mechanism behind the TUI input freeze (CA-535).
func applyProbeSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow,
	}
}
