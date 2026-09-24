//go:build windows

package runnerboot

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

// assertSharedRunnerDetached verifies the Windows detach mechanism:
// DETACHED_PROCESS + CREATE_NEW_PROCESS_GROUP give the spawned runner no
// console of its own, so closing the TUI terminal delivers no CTRL_CLOSE_EVENT
// to a shared runner Desktop still leases (CP-81 Task-416 T-1).
func assertSharedRunnerDetached(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd.SysProcAttr == nil {
		t.Fatal("shared spawn must set SysProcAttr (detached)")
	}
	if cmd.SysProcAttr.CreationFlags&windows.DETACHED_PROCESS == 0 {
		t.Fatal("windows shared spawn must DETACHED_PROCESS-detach from the TUI console")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatal("windows shared spawn must CREATE_NEW_PROCESS_GROUP so console events cannot reach it")
	}
}
