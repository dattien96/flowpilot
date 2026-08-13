package runnerboot

import (
	"os/exec"
	"testing"
)

// CA-474: spawned runner must stay in the TUI process group so closing the
// terminal (or /exit) can stop it. Additive test — do not edit runnerboot_test.go.
func TestSetSysProcAttrDoesNotDetach(t *testing.T) {
	cmd := exec.Command("true")
	setSysProcAttr(cmd)
	if cmd.SysProcAttr != nil {
		t.Fatalf("SysProcAttr = %#v, want nil (must not CREATE_NEW_PROCESS_GROUP / Setsid)", cmd.SysProcAttr)
	}
}
