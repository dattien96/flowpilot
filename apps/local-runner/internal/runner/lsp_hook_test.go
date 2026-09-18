package runner

import (
	"context"
	"testing"
	"time"
)

// lspFakeChecker is a scripted lspChecker for gate-path tests.
type lspFakeChecker struct {
	msg       string
	calls     int
	lastRoot  string
	lastFiles []string
}

func (f *lspFakeChecker) CheckFiles(_ context.Context, root string, files []string) string {
	f.calls++
	f.lastRoot = root
	f.lastFiles = append([]string(nil), files...)
	return f.msg
}

func TestRunnerInjectsLSPDiagnosticsAfterWrite(t *testing.T) {
	svc := NewInteractiveService()
	run, apiErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	fake := &lspFakeChecker{msg: "a.go:1:1 error: undefined: Foo"}
	svc.lspChecker = fake

	// lspDiagnosticsForTurn routes workspace + files to the checker.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if got := svc.lspDiagnosticsForTurn(ctx, "/ws/proj", []string{"a.go"}); got != fake.msg {
		t.Fatalf("diagnostics = %q, want %q", got, fake.msg)
	}
	if fake.calls != 1 || fake.lastRoot != "/ws/proj" || len(fake.lastFiles) != 1 || fake.lastFiles[0] != "a.go" {
		t.Fatalf("checker got root=%q files=%v calls=%d", fake.lastRoot, fake.lastFiles, fake.calls)
	}

	// blockTurnForLSPDiagnostics arms the standard reprompt fields.
	rs := svc.runs[run.RunID]
	rs.lastTurnStepID = "coder"
	if !svc.blockTurnForLSPDiagnostics(run.RunID, rs.gateEpoch, "turn-1", fake.msg) {
		t.Fatal("blockTurnForLSPDiagnostics must report blocked")
	}
	if rs.pendingGateRepromptPrompt != fake.msg {
		t.Fatalf("reprompt prompt = %q", rs.pendingGateRepromptPrompt)
	}
	if rs.pendingGateRepromptStepID != "coder" {
		t.Fatalf("reprompt step = %q", rs.pendingGateRepromptStepID)
	}
	if rs.pendingGateRepromptGen != 1 {
		t.Fatalf("reprompt gen = %d, want 1", rs.pendingGateRepromptGen)
	}
	found := false
	for _, e := range rs.events {
		if e.Type == EventFlowGateViolation && e.Status == "reprompt" && e.ProviderTurnID == "turn-1" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a reprompt EventFlowGateViolation event")
	}

	// Step fallback: no lastTurnStepID -> run stepID.
	rs2 := svc.runs[run.RunID]
	rs2.lastTurnStepID = ""
	rs2.stepID = "implement"
	rs2.pendingGateRepromptGen = 0
	if !svc.blockTurnForLSPDiagnostics(run.RunID, rs2.gateEpoch, "turn-2", fake.msg) {
		t.Fatal("fallback block must report blocked")
	}
	if rs2.pendingGateRepromptStepID != "implement" {
		t.Fatalf("reprompt step fallback = %q, want implement", rs2.pendingGateRepromptStepID)
	}
}

func TestRunnerSkipsLSPHookWhenNoServer(t *testing.T) {
	svc := NewInteractiveService() // lspChecker nil -> DefaultSet, no binaries
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Nonexistent workspace detects as "general" (unregistered) regardless
	// of which real binaries exist on PATH.
	if got := svc.lspDiagnosticsForTurn(ctx, "/definitely/not/here-ws", []string{"a.go"}); got != "" {
		t.Fatalf("diagnostics = %q, want empty", got)
	}
	if got := svc.lspDiagnosticsForTurn(ctx, "/ws", nil); got != "" {
		t.Fatalf("empty files = %q, want empty", got)
	}
	if got := svc.lspDiagnosticsForTurn(nil, "", []string{"a.go"}); got != "" {
		t.Fatalf("empty workspace = %q, want empty", got)
	}
	// A checker that reports clean also yields no reprompt input.
	svc.lspChecker = &lspFakeChecker{msg: ""}
	if got := svc.lspDiagnosticsForTurn(ctx, "/ws", []string{"a.go"}); got != "" {
		t.Fatalf("clean checker = %q, want empty", got)
	}
}
