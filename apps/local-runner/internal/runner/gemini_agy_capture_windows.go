//go:build windows

package runner

import (
	"context"
	"os/exec"
)

// captureAgyPrint intentionally uses the ordinary buffered path on Windows.
//
// A previous ConPTY capture attempt avoided empty stdout for some AGY builds,
// but on the current Antigravity CLI it can terminate the AGY process with
// 0xc0000374 and take down the supervised runner. AGY exits cleanly through the
// normal process path; when it writes no pipe output, the caller recovers the
// assistant response from AGY's persisted conversation DB.
func captureAgyPrint(_ context.Context, cmd *exec.Cmd) (string, string, error) {
	return captureBufferedCommand(cmd)
}
