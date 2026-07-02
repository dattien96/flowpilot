//go:build !windows

package runner

import (
	"context"
	"os/exec"
)

// captureAgyPrint on non-Windows platforms runs agy normally: it writes its
// --print response to stdout, so the buffered capture path works as-is.
func captureAgyPrint(_ context.Context, cmd *exec.Cmd) (string, string, error) {
	return captureBufferedCommand(cmd)
}
