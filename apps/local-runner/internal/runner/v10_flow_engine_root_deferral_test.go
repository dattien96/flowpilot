package runner

import (
	"testing"
)

// V10 residual follow-up (BUG-288 F-15): pin the root deferral matrix in
// emitLocked's EventTurnCompleted branch after the flow-engine-root fix.
//
// The branch is provider-agnostic by construction: its decision inputs are
// parentRunID, flowEngineDriven, turnStartedAfterLoopDone and the run's own
// event list — no ProviderKey value is ever branched on (only stamped onto
// events/telemetry). One representative provider is therefore sufficient;
// these tests use ProviderKeyCodex like the V10 residual suite.
//
// New file — no pre-existing test is modified (additive-tests-only).

func newV10RootRun(t *testing.T, svc *InteractiveService, flowDriven, afterLoopDone bool, events []ProviderEvent) *interactiveRun {
	t.Helper()
	root, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[root.RunID]
	rs.flowEngineDriven = flowDriven
	rs.turnStartedAfterLoopDone = afterLoopDone
	rs.turnStartGitHead = "abc123"
	rs.events = append(rs.events, events...)
	return rs
}

// Flow-engine root WITH code changes defers too (combined path: the fix must
// not depend on the absence of FileChanged events).
func TestV10FlowEngineRootWithCodeDefersUntilGate(t *testing.T) {
	svc, _ := newTestServer(t)
	rs := newV10RootRun(t, svc, true, false, []ProviderEvent{
		{Type: EventFileChanged, Path: "src/calc.go"},
	})
	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{Type: EventTurnCompleted, FinalMessage: "root done"})
	deferred := rs.pendingFlowGateSettle
	status := rs.status
	svc.mu.Unlock()
	if !deferred {
		t.Fatal("flow-engine root with code changes must defer Completed until gate")
	}
	if status == RunStatusCompleted {
		t.Fatalf("status = %q, want non-Completed while gate is pending", status)
	}
}

// Plain chat root without code still completes immediately (run-208282
// contract): the fix must not park ordinary "hi" turns behind a gate.
func TestV10PlainRootWithoutCodeCompletesImmediately(t *testing.T) {
	svc, _ := newTestServer(t)
	rs := newV10RootRun(t, svc, false, false, nil)
	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{Type: EventTurnCompleted, FinalMessage: "hi"})
	deferred := rs.pendingFlowGateSettle
	status := rs.status
	svc.mu.Unlock()
	if deferred {
		t.Fatal("plain codeless root must not arm the pending gate")
	}
	if status != RunStatusCompleted {
		t.Fatalf("status = %q, want Completed for a plain codeless turn", status)
	}
}

// Plain chat root WITH code still defers (run-208282 contract, unchanged by
// the fix — regression pin for the pre-existing branch).
func TestV10PlainRootWithCodeDefersUntilGate(t *testing.T) {
	svc, _ := newTestServer(t)
	rs := newV10RootRun(t, svc, false, false, []ProviderEvent{
		{Type: EventFileChanged, Path: "src/calc.go"},
	})
	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{Type: EventTurnCompleted, FinalMessage: "root done"})
	deferred := rs.pendingFlowGateSettle
	svc.mu.Unlock()
	if !deferred {
		t.Fatal("plain root with code changes must defer Completed until gate")
	}
}

// BUG-305 precedence preserved: a follow-up on an already-done loop
// completes immediately even on a flow-engine-driven root — the fix lives in
// the pre-loop-done branch only and must not swallow this path.
func TestV10FlowEngineRootAfterLoopDoneCompletesImmediately(t *testing.T) {
	svc, _ := newTestServer(t)
	rs := newV10RootRun(t, svc, true, true, []ProviderEvent{
		{Type: EventFileChanged, Path: "src/calc.go"},
	})
	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{Type: EventTurnCompleted, FinalMessage: "follow-up"})
	deferred := rs.pendingFlowGateSettle
	status := rs.status
	svc.mu.Unlock()
	if deferred {
		t.Fatal("post-loop-done follow-up must not arm the pending gate (BUG-305)")
	}
	if status != RunStatusCompleted {
		t.Fatalf("status = %q, want Completed for a post-loop-done follow-up", status)
	}
}
