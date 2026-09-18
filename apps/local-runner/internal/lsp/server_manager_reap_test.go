package lsp

import (
	"context"
	"os"
	"runtime"
	"testing"
)

// TestServerManagerStopReapsKill locks the corrected Stop contract behind
// CA-885: after Stop the OS process is always reaped (ProcessState set via
// the watcher Wait). The Exited() bit is platform-defined — a SIGKILLed
// Unix process is reaped but did not "exit", while Windows
// TerminateProcess reports Exited true — so it must never gate the reap
// assertion (the TestServerManagerStopKillsProcess bug).
func TestServerManagerStopReapsKill(t *testing.T) {
	m := lspTestManager(t)
	// hangkill: EOF-proof helper, so SIGKILL from Stop is the only death.
	lspHelperStart(t, m, "hangkill")
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if m.cmd == nil || m.cmd.ProcessState == nil {
		t.Fatal("OS process was not reaped")
	}
	if runtime.GOOS == "windows" {
		if !m.cmd.ProcessState.Exited() {
			t.Fatal("windows kill must report Exited")
		}
	} else {
		if m.cmd.ProcessState.Exited() {
			t.Fatal("unix SIGKILL must not report Exited (reaped, not exited)")
		}
		if m.cmd.ProcessState.Success() {
			t.Fatal("killed process must not report Success")
		}
	}
}

// TestServerManagerStopReapsExitedServer covers the other shape: a server
// that already exited on its own is still reaped (not leaked) by Stop.
func TestServerManagerStopReapsExitedServer(t *testing.T) {
	m := lspTestManager(t)
	dir := t.TempDir()
	m.Env = []string{"GO_WANT_LSP_HELPER=1", "GO_LSP_HELPER_MODE=exit1"}
	if err := m.Start(context.Background(), os.Args[0],
		[]string{"-test.run=TestLSPHelperProcess"}, dir); err != nil {
		t.Fatalf("Start: %v", err)
	}
	lspWaitFor(t, "process exit", func() bool { return !m.IsRunning() })
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if m.cmd == nil || m.cmd.ProcessState == nil {
		t.Fatal("exited process was not reaped")
	}
}
