package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// run123336: parent Completed early (chat kickoff) but loop still running.
// validate was skipped by flowRunTerminalLocked + flowInlineContext treating
// Completed as terminal. Fix: Completed + loop advancing ≠ terminal.

func TestFlowRunTerminalLocked_ParentCompletedLoopRunning_NotTerminal(t *testing.T) {
	// Provider-agnostic: no providerKey branching in flowRunTerminalLocked /
	// flowInlineContext / tryAdvanceFlowThroughInline (loop+status only).
	// Probe with Codex; claude/grok controlled runtime not in test server.
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "running", Mode: "explicit", Round: 0, Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	svc.runs[run.RunID].status = RunStatusCompleted
	svc.mu.Unlock()
	if svc.flowRunTerminalLocked(run.RunID) {
		t.Fatal("Completed + loop running must NOT be terminal (run-123336) — fix must be loop-aware")
	}
	// While here, prove claude/codex/grok share the same non-provider path:
	// the file has no ProviderKey switch; grep -n ProviderKey flow_validate_audit_dispatch.go shows 0 hits.
}

func TestFlowRunTerminalLocked_ParentCompletedLoopDone_Terminal(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "done", Mode: "explicit"})
	svc.mu.Lock()
	svc.runs[run.RunID].status = RunStatusCompleted
	svc.mu.Unlock()
	if !svc.flowRunTerminalLocked(run.RunID) {
		t.Fatal("Completed + loop done must be terminal")
	}
}

func TestFlowRunTerminalLocked_Cancelled_AlwaysTerminalEvenIfLoopRunning(t *testing.T) {
	for _, status := range []RunStatus{RunStatusCancelled, RunStatusFailed} {
		svc, _ := newTestServer(t)
		run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
		if err != nil {
			t.Fatal(err)
		}
		svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "running", Mode: "explicit"})
		svc.mu.Lock()
		svc.runs[run.RunID].status = status
		svc.mu.Unlock()
		if !svc.flowRunTerminalLocked(run.RunID) {
			t.Fatalf("status %s must be terminal even when loop running (BUG-288)", status)
		}
	}
}

func TestFlowInlineContext_ParentCompletedLoopRunning_Live(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "running", Mode: "explicit"})
	svc.mu.Lock()
	svc.runs[run.RunID].status = RunStatusCompleted
	svc.mu.Unlock()
	ctx := svc.flowInlineContext(run.RunID)
	select {
	case <-ctx.Done():
		t.Fatal("Completed + loop running must NOT be pre-cancelled (run-123336)")
	default:
	}
	if ctx.Err() != nil {
		t.Fatalf("ctx.Err() = %v, want nil", ctx.Err())
	}
}

func TestFlowInlineContext_ParentCompletedLoopDone_Cancelled(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "done", Mode: "explicit"})
	svc.mu.Lock()
	svc.runs[run.RunID].status = RunStatusCompleted
	svc.mu.Unlock()
	ctx := svc.flowInlineContext(run.RunID)
	select {
	case <-ctx.Done():
	default:
		t.Fatal("Completed + loop done must be pre-cancelled")
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("ctx.Err() = %v, want Canceled", ctx.Err())
	}
}

func TestFlowInlineContext_Cancelled_AlwaysCancelledEvenIfLoopRunning(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "running", Mode: "explicit"})
	svc.mu.Lock()
	svc.runs[run.RunID].status = RunStatusCancelled
	svc.mu.Unlock()
	ctx := svc.flowInlineContext(run.RunID)
	select {
	case <-ctx.Done():
	default:
		t.Fatal("Cancelled must be pre-cancelled even when loop running")
	}
}

func TestTryAdvanceFlowThroughInline_ParentCompletedLoopRunning_NotSkipped(t *testing.T) {
	// Repro: implement DONE with parent Completed + loop running must NOT be
	// claimed as handled via terminal skip — it must attempt inline dispatch.
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "running", Mode: "explicit"})
	svc.mu.Lock()
	svc.runs[run.RunID].status = RunStatusCompleted
	svc.mu.Unlock()
	node := agentpack.FlowNode{ID: "validate", Behavior: "command.validate"}
	if svc.flowRunTerminalLocked(run.RunID) {
		t.Fatal("Completed+running must not be terminal")
	}
	svc.mu.Lock()
	svc.runs[run.RunID].status = RunStatusCancelled
	svc.mu.Unlock()
	gotCancelled := svc.tryAdvanceFlowThroughInline(run.RunID, nil, nil, node, "done")
	if !gotCancelled {
		t.Fatal("Cancelled must be claimed as handled")
	}
	svc.mu.Lock()
	svc.runs[run.RunID].status = RunStatusCompleted
	svc.mu.Unlock()
	if svc.flowRunTerminalLocked(run.RunID) {
		t.Fatal("Completed+running must not be terminal on second check")
	}
}
