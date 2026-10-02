package runner

import (
	"context"
	"testing"
)

// BUG-593 (live run-139670): POST /client/workflow-runs/{id}/interrupt
// returned {"status":"cancelling"} but only cancelled in-flight ctx handles —
// no durable fence. The run stayed "running" and the flow engine spawned a
// brand-new contract-planner leg AFTER interrupt acceptance. §2 contract:
// "ctx.Err() alone is defense-in-depth, not the guard" — interrupt must write
// the durable run-stop fence so post-interrupt dispatch is fenced by CAS.
func TestBUG593_InterruptWritesDurableStopFence(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServer(t)
	store := NewMemoryDispatchStore()
	svc.dispatchStore = store

	runID := "run-bug593"
	if err := store.CreatePrepared(ctx, testPrepared(runID, "turn-1"), testEnvelope(runID, "turn-1")); err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:          runID,
		providerKey: ProviderKeyCodex,
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	if e := svc.Interrupt(runID); e != nil {
		t.Fatalf("interrupt: %v", e)
	}

	st, err := store.GetRunStopState(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Stopped {
		t.Fatal("interrupt must write the durable run-stop fence — " +
			"ctx cancels alone let the flow engine spawn new legs after acceptance")
	}
}

// The same fence must land on every child — parent interrupt without the
// child fence lets an already-bound child pass its own send CAS.
func TestBUG593_InterruptFencesChildren(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServer(t)
	store := NewMemoryDispatchStore()
	svc.dispatchStore = store

	parentID := "run-bug593-parent"
	childID := "run-bug593-child"
	if err := store.CreatePrepared(ctx, testPrepared(parentID, "turn-p"), testEnvelope(parentID, "turn-p")); err != nil {
		t.Fatal(err)
	}
	if err := store.CreatePrepared(ctx, testPrepared(childID, "turn-c"), testEnvelope(childID, "turn-c")); err != nil {
		t.Fatal(err)
	}

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:          parentID,
		providerKey: ProviderKeyCodex,
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:          childID,
		parentRunID: parentID,
		providerKey: ProviderKeyCodex,
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.mu.Unlock()

	if e := svc.Interrupt(parentID); e != nil {
		t.Fatalf("interrupt: %v", e)
	}

	st, err := store.GetRunStopState(ctx, childID)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Stopped {
		t.Fatal("child run must carry the durable stop fence after parent interrupt")
	}
}
