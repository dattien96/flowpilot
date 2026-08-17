package runner

import (
	"context"
	"os"
	"os/exec"
)

// newProbeCmd builds an *exec.Cmd for a provider-detection probe. Probes never
// read stdin and, on Windows, must not attach to the parent console (CA-535):
// a CLI probe that inherits the TUI's console input can freeze or swallow
// keystrokes while it runs, which makes the TUI appear locked for the probe's
// lifetime. Stdin is always wired to os.DevNull so a probe can never steal
// console input even when a platform-specific SysProcAttr is not available.
func newProbeCmd(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	if devNull, err := os.Open(os.DevNull); err == nil {
		cmd.Stdin = devNull
	}
	applyProbeSysProcAttr(cmd)
	return cmd
}
