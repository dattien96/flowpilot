//go:build windows

package runnerboot

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// stillActive is the Windows exit code meaning "process is running".
const stillActive = 259

// TestRunnerBootJobKillOnClose proves the KILL_ON_JOB_CLOSE guarantee: when
// the owning process dies (or closes the job handle, which is what happens on
// any TUI death), the spawned runner is terminated by Windows — a wedged TUI
// can never orphan the Go runner on --port (BUG-328).
func TestRunnerBootJobKillOnClose(t *testing.T) {
	if os.Getenv("FLOWPILOT_JOB_CHILD") == "1" {
		// Child mode: stay alive until the job kills us.
		time.Sleep(120 * time.Second)
		return
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=TestRunnerBootJobKillOnClose")
	cmd.Env = append(os.Environ(), "FLOWPILOT_JOB_CHILD=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := uint32(cmd.Process.Pid)
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION, false, pid)
	if err != nil {
		t.Fatalf("open child: %v", err)
	}
	defer windows.CloseHandle(proc)

	waitExited := func(what string) bool {
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			var ec uint32
			if err := windows.GetExitCodeProcess(proc, &ec); err != nil {
				return true
			}
			if ec != stillActive {
				return true
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("%s: process survived job close", what)
		return false
	}

	if err := assignRunnerJob(pid); err != nil {
		t.Fatalf("assign job: %v", err)
	}
	// Closing the job handle is exactly what happens when the TUI process
	// exits with KILL_ON_JOB_CLOSE armed — the child must die.
	closeRunnerJob()
	waitExited("child after job close")
}