package runner

// BUG-475 (CP-84/CP-51): a dispatch-attention authority READ FAILURE is not an
// empty decision set. dispatchAttentionByRun must propagate the error; the
// mux must never emit a decision-clearing authoritative frame (empty snapshot
// or remove) built on an unreadable store — it answers with a retryable
// resync instead, and the previous attention survives until a complete
// healthy snapshot lands.

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
)

// bug475AttentionStore wraps a real DispatchStore and fails ListAttention on
// demand — simulating a corrupt/unreadable durable dispatch authority.
type bug475AttentionStore struct {
	DispatchStore
	failAttention atomic.Bool
}

func (s *bug475AttentionStore) ListAttention(ctx context.Context) ([]AttentionItem, error) {
	if s.failAttention.Load() {
		return nil, errors.New("injected dispatch attention authority failure")
	}
	return s.DispatchStore.ListAttention(ctx)
}

func newBUG475Svc(t *testing.T, runID string) (*InteractiveService, *bug475AttentionStore) {
	t.Helper()
	svc := NewInteractiveService()
	store := &bug475AttentionStore{DispatchStore: NewMemoryDispatchStore()}
	if _, err := store.OpenRepair(context.Background(), runID, "crash mid-settle needs operator repair", nil, ""); err != nil {
		t.Fatalf("OpenRepair: %v", err)
	}
	svc.dispatchStore = store
	rs := mkDecisionRun(svc, runID, "proj-1")
	svc.mu.Lock()
	rs.status = RunStatusCompleted // terminal lane: only dispatch attention keeps it relevant
	svc.mu.Unlock()
	return svc, store
}

// A snapshot built while the attention store is unreadable must NOT arrive as
// an authoritative lane set — the subscriber is closed retryable so the
// handler emits resync instead of a decision-clearing snapshot.
func TestBUG475_SnapshotStoreFailureIsRetryableResync(t *testing.T) {
	svc, store := newBUG475Svc(t, "run-475-snap")

	store.failAttention.Store(true)
	subID, _, snapshot := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID)

	svc.mu.Lock()
	sub := svc.runUpdateSubs[subID]
	closed, retryable := sub != nil && sub.closed, sub != nil && sub.retryable
	svc.mu.Unlock()
	if !closed || !retryable {
		t.Fatalf("authority-failed subscribe must register closed+retryable (closed=%v retryable=%v); "+
			"snapshot would project %d lanes with empty dispatch decisions — a client reconcile "+
			"would erase the live repair item", closed, retryable, len(snapshot))
	}
	if len(snapshot) != 0 {
		t.Fatalf("no lane set may be returned on authority failure, got %d", len(snapshot))
	}
}

// A dirty-drain failure must not emit a remove for a terminal repair-required
// lane; it closes the subscriber retryable so the client re-snapshots.
func TestBUG475_DrainStoreFailureEmitsResyncNotRemove(t *testing.T) {
	svc, store := newBUG475Svc(t, "run-475-drain")

	subID, _, snapshot := svc.subscribeRunUpdates()
	if len(snapshot) != 1 {
		t.Fatalf("healthy snapshot must contain the repair lane, got %d", len(snapshot))
	}
	var repairFound bool
	for _, d := range snapshot[0].Decisions {
		if d.Kind == DecisionKindDispatch && d.Dispatch != nil && d.Dispatch.AttentionKind == "repair_required" {
			repairFound = true
		}
	}
	if !repairFound {
		t.Fatalf("healthy snapshot missing repair_required decision: %+v", snapshot[0].Decisions)
	}

	svc.markRunRealtimeDirty("run-475-drain")
	store.failAttention.Store(true)
	frames := svc.drainRunUpdates(subID)
	for _, f := range frames {
		if f.Kind == RunRealtimeRemove && f.RunID == "run-475-drain" {
			t.Fatalf("authority failure must never tombstone the repair lane: %+v", frames)
		}
	}
	if len(frames) != 1 || frames[0].Kind != RunRealtimeResync || !frames[0].Retryable {
		t.Fatalf("expected single retryable resync on authority failure, got %+v", frames)
	}
}

// Recovery: after the store heals, a fresh snapshot must carry the same
// repair_required decision again (stable ID/revision, no duplicates).
func TestBUG475_RecoveryResnapshotRestoresRepairDecision(t *testing.T) {
	svc, store := newBUG475Svc(t, "run-475-heal")

	store.failAttention.Store(true)
	subID, _, _ := svc.subscribeRunUpdates()
	svc.unsubscribeRunUpdates(subID)

	store.failAttention.Store(false)
	subID2, _, snapshot := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID2)
	var found *RunRealtimeProjection
	for i := range snapshot {
		if snapshot[i].RunID == "run-475-heal" {
			found = &snapshot[i]
		}
	}
	if found == nil {
		t.Fatal("healed snapshot lost the repair lane")
	}
	repairs := 0
	for _, d := range found.Decisions {
		if d.Kind == DecisionKindDispatch {
			repairs++
			if d.ID != "dispatch-repair:run-475-heal" || d.Revision == "" {
				t.Fatalf("repair decision identity unstable: %+v", d)
			}
		}
	}
	if repairs != 1 {
		t.Fatalf("want exactly one repair decision, got %d", repairs)
	}
}

// HTTP E2E: authority failure on the wire → the first frame is a retryable
// resync (never a completing snapshot); after healing, a reconnect gets the
// full authoritative lane set.
func TestBUG475_HTTPStreamResyncThenHealthySnapshot(t *testing.T) {
	svc, store := newBUG475Svc(t, "run-475-http")
	store.failAttention.Store(true)

	srv, resp, reader, cancel := startMuxStream(t, svc)
	ev := readSSEFrame(t, reader)
	if ev.event != string(RunRealtimeResync) {
		t.Fatalf("first frame on authority failure must be resync, got event=%q data=%s", ev.event, ev.data)
	}
	var f RunRealtimeFrame
	if err := json.Unmarshal([]byte(ev.data), &f); err != nil || !f.Retryable {
		t.Fatalf("resync frame must be retryable: data=%s err=%v", ev.data, err)
	}
	resp.Body.Close()
	cancel()
	srv.Close()

	store.failAttention.Store(false)
	srv2, resp2, reader2, cancel2 := startMuxStream(t, svc)
	defer srv2.Close()
	defer resp2.Body.Close()
	defer cancel2()
	var sawRepair bool
	for {
		ev := readSSEFrame(t, reader2)
		if ev.event != string(RunRealtimeSnapshot) {
			t.Fatalf("expected snapshot after heal, got %q", ev.event)
		}
		var sf RunRealtimeFrame
		if err := json.Unmarshal([]byte(ev.data), &sf); err != nil {
			t.Fatalf("bad snapshot payload: %v", err)
		}
		for _, p := range sf.Runs {
			for _, d := range p.Decisions {
				if d.Kind == DecisionKindDispatch && d.Dispatch != nil && d.Dispatch.AttentionKind == "repair_required" {
					sawRepair = true
				}
			}
		}
		if sf.Complete {
			break
		}
	}
	if !sawRepair {
		t.Fatal("healed snapshot must restore the repair_required decision")
	}
}
