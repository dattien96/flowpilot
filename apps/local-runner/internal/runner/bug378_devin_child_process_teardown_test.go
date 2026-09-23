package runner

import (
	"io"
	"testing"
)

// BUG-378: a child run's terminal event tears down its isolated `devin acp`
// process — the opencode path already does this (BUG-334); devin handles were
// never closed and leaked for the runner's lifetime.
func TestBug378_ChildTerminalEventClosesDevinProcess(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	rs := newP4ChildRun(svc, "child-devin-1", parentID, dir, head)
	svc.agentOrchestrator.registerChild(parentID, rs.id)

	// Seed the runner's devin process table: one chat-scope handle and one
	// isolated handle for the child run.
	svc.runner = &Runner{}
	svc.runner.devinProcesses = map[string]*devinProcessHandle{}
	seed := func(segment string) *devinProcessHandle {
		scopeKey := devinSegmentedScope("acct-1", segment)
		h := &devinProcessHandle{
			scopeKey:     scopeKey,
			scopeBase:    "acct-1",
			scopeSegment: segment,
			model:        "devin/swe-2-high",
			dispatcher:   newDevinDispatcher(io.Discard, nil),
		}
		svc.runner.devinProcesses[devinProcessKey(scopeKey, h.model, "")] = h
		return h
	}
	parent := seed("")
	child := seed("child-devin-1")
	if len(svc.runner.devinProcesses) != 2 {
		t.Fatalf("setup: expected 2 handles, got %d", len(svc.runner.devinProcesses))
	}

	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{Type: EventTurnCompleted, FinalMessage: "CHILD_OK"})
	svc.mu.Unlock()

	if !child.dispatcher.isClosed() {
		t.Fatal("child devin acp process must be closed on the child's terminal event")
	}
	if len(svc.runner.devinProcesses) != 1 {
		t.Fatalf("child handle must be removed from the process table, got %d handles", len(svc.runner.devinProcesses))
	}
	if parent.dispatcher.isClosed() {
		t.Fatal("chat-scope devin process must never be touched by child teardown")
	}
}

// Near-miss: non-terminal events must not tear the child's process down.
func TestBug378_ChildNonTerminalEventKeepsDevinProcess(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	rs := newP4ChildRun(svc, "child-devin-2", parentID, dir, head)
	svc.agentOrchestrator.registerChild(parentID, rs.id)

	svc.runner = &Runner{}
	svc.runner.devinProcesses = map[string]*devinProcessHandle{}
	scopeKey := devinSegmentedScope("acct-1", "child-devin-2")
	child := &devinProcessHandle{
		scopeKey: scopeKey, scopeBase: "acct-1", scopeSegment: "child-devin-2",
		model: "devin/swe-2-high", dispatcher: newDevinDispatcher(io.Discard, nil),
	}
	svc.runner.devinProcesses[devinProcessKey(scopeKey, child.model, "")] = child

	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{Type: EventMessageDelta, Text: "working…"})
	svc.mu.Unlock()

	if child.dispatcher.isClosed() {
		t.Fatal("non-terminal event must not close the child's devin process")
	}
	if len(svc.runner.devinProcesses) != 1 {
		t.Fatal("child handle must remain")
	}
}
