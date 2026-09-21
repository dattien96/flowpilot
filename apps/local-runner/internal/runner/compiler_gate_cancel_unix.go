//go:build !windows

package runner

import "os/exec"

// compilerGateCancel keeps the default kill semantics on unix: `sh -c` execs
// the recipe in-process, so killing the shell kills the worker — there is no
// surviving grandchild holding the output pipes open (unlike `cmd /c`).
func compilerGateCancel(cmd *exec.Cmd) func() error {
	return func() error {
		if cmd == nil || cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
}
