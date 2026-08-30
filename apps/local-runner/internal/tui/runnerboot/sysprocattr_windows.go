//go:build windows

package runnerboot

import (
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// runnerJobHandle is the Windows Job Object that owns every runner this TUI
// spawns. The handle is created once and kept open for the TUI's lifetime;
// with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE set, Windows terminates ALL job
// members the moment this process dies — force-kill, crash, or window close —
// so a wedged TUI (BUG-328: terminal stops delivering keys) can never orphan
// the Go runner listening on --port.
var runnerJobHandle windows.Handle

func setSysProcAttr(cmd *exec.Cmd) {
	// The runner stays attached to the TUI's console (closing the terminal
	// sends CTRL_CLOSE_EVENT to both), and assignRunnerJob adds the hard
	// guarantee: KILL_ON_JOB_CLOSE when this process dies (CA-474/CA-445).
	_ = cmd
}

// assignRunnerJob places the runner process (pid) into the kill-on-close job.
// Failures are non-fatal (the runner may already be in a parent job, e.g.
// launched from an IDE); callers log and continue.
func assignRunnerJob(pid uint32) error {
	if pid == 0 {
		return fmt.Errorf("runner pid is 0")
	}
	if runnerJobHandle == 0 {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return fmt.Errorf("create job object: %w", err)
		}
		var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
			_ = windows.CloseHandle(h)
			return fmt.Errorf("set kill-on-close: %w", err)
		}
		runnerJobHandle = h
	}

	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_INFORMATION, false, pid)
	if err != nil {
		return fmt.Errorf("open runner process %d: %w", pid, err)
	}
	defer windows.CloseHandle(proc)
	if err := windows.AssignProcessToJobObject(runnerJobHandle, proc); err != nil {
		return fmt.Errorf("assign runner %d to job: %w", pid, err)
	}
	return nil
}

// closeRunnerJob releases the job handle (used by tests; production keeps it
// open until process exit so KILL_ON_JOB_CLOSE fires on any death).
func closeRunnerJob() {
	if runnerJobHandle != 0 {
		_ = windows.CloseHandle(runnerJobHandle)
		runnerJobHandle = 0
	}
}