package runner

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// F-1 (live finding, Task-438/439 test round): a killed `devin acp` parent
// can leave MCP-style grandchildren holding the stdout pipe, so the
// dispatcher's read loop sees no EOF for minutes and the dead handle keeps
// reporting "warm" — the next turn then writes into the void and fails late.
// Liveness must follow PROCESS EXIT (cmd.Wait), not just pipe EOF.
func TestEnsureDevinProcessExitMarksClosedWithoutStdoutEOF(t *testing.T) {
	isolateDevinPrewarmEnv(t)

	orig := commandContextFn
	commandContextFn = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		s := // Grandchild spawned BEFORE the handshake so it deterministically
			// outlives the parent while holding stdout — exactly what devin's
			// spawned MCP servers do on Windows.
			"sleep 60 &\n" +
				shellReadLine() +
				shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"devin-browser","name":"browser"}]}}`) +
				shellReadLine() +
				shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{}}`) +
				"sleep 60\n"
		return testShellCommand(ctx, s)
	}
	t.Cleanup(func() { commandContextFn = orig })

	r := &Runner{}
	t.Cleanup(r.closeAllDevinProcesses)
	h, err := r.ensureDevinProcess(context.Background(), "acct-deadproc", "", nil, "", "")
	if err != nil {
		t.Fatalf("ensureDevinProcess: %v", err)
	}
	if h.dispatcher.isClosed() {
		t.Fatal("fresh handle must be live")
	}

	h.kill() // kill the parent only — `sleep 60 &` keeps stdout open

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if h.dispatcher.isClosed() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dead process still reported live — liveness must follow process exit, not pipe EOF")
}
