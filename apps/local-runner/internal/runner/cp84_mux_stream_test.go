package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// CP-84 / Task-429 gap-fill: HTTP-level tests for handleAllEventsStream.
// All pre-existing mux tests drive subscribeRunUpdates/drainRunUpdates
// internals; none exercised the SSE wire: chunk boundaries, Complete flag,
// resync-closes-stream, heartbeat, cancel/write-failure unsubscribe (T-4/T-7).
// Additive file — no existing test touched.
// ============================================================================

// sseEvent is one parsed `event:`/`data:` pair off the wire.
type sseEvent struct {
	event string
	data  string
}

// readSSEFrame blocks until one complete SSE frame (terminated by a blank
// line) arrives on r. Returns io.EOF (or ctx error) when the stream ends.
func readSSEFrame(t *testing.T, r *bufio.Reader) sseEvent {
	t.Helper()
	var ev sseEvent
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("stream read failed mid-frame: %v (partial frame %+v)", err, ev)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if ev.event != "" || ev.data != "" {
				return ev
			}
		case strings.HasPrefix(line, "event: "):
			ev.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			ev.data += strings.TrimPrefix(line, "data: ")
		case strings.HasPrefix(line, ":"):
			// heartbeat / comment line — keep reading
		default:
			t.Fatalf("malformed SSE line %q", line)
		}
	}
}

// startMuxStream serves handleAllEventsStream on a real httptest server and
// returns the response body + line reader. Callers close the body / cancel the
// context to end the stream.
func startMuxStream(t *testing.T, svc *InteractiveService) (*httptest.Server, *http.Response, *bufio.Reader, context.CancelFunc) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /client/events/stream", svc.handleAllEventsStream)
	srv := httptest.NewServer(mux)
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/client/events/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}
	return srv, resp, bufio.NewReader(resp.Body), cancel
}

// TestMuxHTTP_ChunkedSnapshotCommitsOnlyOnComplete: >runUpdateSnapshotChunk
// lanes must arrive as multiple snapshot frames sharing one snapshotId, with
// Complete=true on the last frame ONLY (T-4 atomic commit contract).
func TestMuxHTTP_ChunkedSnapshotCommitsOnlyOnComplete(t *testing.T) {
	svc := NewInteractiveService()
	const n = runUpdateSnapshotChunk + 7 // 39 → 2 chunks
	for i := 0; i < n; i++ {
		mkDecisionRun(svc, fmt.Sprintf("run-mux-chunk-%03d", i), "proj-1")
	}

	srv, resp, reader, cancel := startMuxStream(t, svc)
	defer srv.Close()
	defer resp.Body.Close()
	defer cancel()

	var snapID string
	var totalRuns int
	var frames []RunRealtimeFrame
	for {
		ev := readSSEFrame(t, reader)
		if ev.event != string(RunRealtimeSnapshot) {
			t.Fatalf("expected snapshot frame, got event=%q data=%s", ev.event, ev.data)
		}
		var f RunRealtimeFrame
		if err := json.Unmarshal([]byte(ev.data), &f); err != nil {
			t.Fatalf("snapshot payload not JSON: %v", err)
		}
		frames = append(frames, f)
		if snapID == "" {
			snapID = f.SnapshotID
		}
		if f.SnapshotID != snapID {
			t.Fatalf("chunk carried different snapshotId %q vs %q", f.SnapshotID, snapID)
		}
		totalRuns += len(f.Runs)
		if f.Complete {
			break
		}
	}
	if len(frames) != 2 {
		t.Fatalf("39 lanes should arrive in 2 chunks of 32+7, got %d frames", len(frames))
	}
	if len(frames[0].Runs) != runUpdateSnapshotChunk {
		t.Fatalf("first chunk = %d runs, want %d", len(frames[0].Runs), runUpdateSnapshotChunk)
	}
	if frames[0].Complete {
		t.Fatal("first chunk must NOT be marked complete")
	}
	if totalRuns != n {
		t.Fatalf("snapshot delivered %d runs, want %d", totalRuns, n)
	}
}

// TestMuxHTTP_EmptySnapshotIsSingleCompleteFrame: no lanes → exactly one
// snapshot frame with Complete=true and no Runs (T-4).
func TestMuxHTTP_EmptySnapshotIsSingleCompleteFrame(t *testing.T) {
	svc := NewInteractiveService()
	srv, resp, reader, cancel := startMuxStream(t, svc)
	defer srv.Close()
	defer resp.Body.Close()
	defer cancel()

	ev := readSSEFrame(t, reader)
	if ev.event != string(RunRealtimeSnapshot) {
		t.Fatalf("expected snapshot, got %q", ev.event)
	}
	var f RunRealtimeFrame
	if err := json.Unmarshal([]byte(ev.data), &f); err != nil {
		t.Fatal(err)
	}
	if !f.Complete || len(f.Runs) != 0 || f.SnapshotID == "" {
		t.Fatalf("empty snapshot malformed: %+v", f)
	}
}

// TestMuxHTTP_UpsertFlowsAfterDirtyMark: a meaningful state change lands on
// the wire as an upsert frame carrying the latest projection (T-1/T-5).
func TestMuxHTTP_UpsertFlowsAfterDirtyMark(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-mux-live", "proj-1")

	srv, resp, reader, cancel := startMuxStream(t, svc)
	defer srv.Close()
	defer resp.Body.Close()
	defer cancel()

	// Drain the single snapshot frame first.
	ev := readSSEFrame(t, reader)
	if ev.event != string(RunRealtimeSnapshot) {
		t.Fatalf("expected snapshot, got %q", ev.event)
	}

	svc.mu.Lock()
	rs.status = RunStatusWaitingApproval
	svc.approvals["appr-mux-live"] = pendingApprovalRec("appr-mux-live", rs.id)
	svc.markRunRealtimeDirtyLocked(rs.id)
	svc.mu.Unlock()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ev = readSSEFrame(t, reader)
		if ev.event != string(RunRealtimeUpsert) {
			continue
		}
		var f RunRealtimeFrame
		if err := json.Unmarshal([]byte(ev.data), &f); err != nil {
			t.Fatal(err)
		}
		if f.RunID != rs.id || f.Run == nil {
			continue
		}
		if f.Run.Status != RunStatusWaitingApproval || len(f.Run.Decisions) != 1 {
			t.Fatalf("upsert projection wrong: %+v", f.Run)
		}
		return
	}
	t.Fatal("no upsert frame arrived after dirty mark")
}

// TestMuxHTTP_RemoveFlowsWhenLaneTerminates: a run going terminal with no
// actionable decisions emits a remove tombstone on the wire (T-9).
func TestMuxHTTP_RemoveFlowsWhenLaneTerminates(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-mux-term", "proj-1")

	srv, resp, reader, cancel := startMuxStream(t, svc)
	defer srv.Close()
	defer resp.Body.Close()
	defer cancel()
	readSSEFrame(t, reader) // snapshot

	svc.mu.Lock()
	rs.status = RunStatusCompleted
	svc.markRunRealtimeDirtyLocked(rs.id)
	svc.mu.Unlock()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ev := readSSEFrame(t, reader)
		var f RunRealtimeFrame
		if err := json.Unmarshal([]byte(ev.data), &f); err != nil {
			t.Fatal(err)
		}
		if ev.event == string(RunRealtimeRemove) && f.RunID == rs.id {
			return
		}
	}
	t.Fatal("no remove frame arrived for terminal lane")
}

// TestMuxHTTP_OverflowEmitsResyncThenClosesStream: a subscriber whose dirty set
// exceeds runUpdateDirtyCap must receive a retryable resync frame and the
// server must end the stream so the client reconnects for a fresh snapshot
// (T-5). This is the HTTP-level proof — unit tests only checked drain output.
func TestMuxHTTP_OverflowEmitsResyncThenClosesStream(t *testing.T) {
	svc := NewInteractiveService()
	mkDecisionRun(svc, "run-mux-overflow", "proj-1")

	srv, resp, reader, cancel := startMuxStream(t, svc)
	defer srv.Close()
	defer resp.Body.Close()
	defer cancel()
	readSSEFrame(t, reader) // snapshot

	// Overflow the subscriber's dirty set with distinct runIDs — the map is
	// keyed on runID so ghost ids count toward the cap without creating runs.
	svc.mu.Lock()
	for i := 0; i < runUpdateDirtyCap+1; i++ {
		svc.markRunRealtimeDirtyLocked(fmt.Sprintf("ghost-%d", i))
	}
	svc.mu.Unlock()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ev := readSSEFrame(t, reader)
		if ev.event != string(RunRealtimeResync) {
			continue
		}
		var f RunRealtimeFrame
		if err := json.Unmarshal([]byte(ev.data), &f); err != nil {
			t.Fatal(err)
		}
		if !f.Retryable {
			t.Fatal("resync frame must be retryable so the client reconnects")
		}
		// The handler must close after resync — the next read must hit EOF.
		if _, err := reader.ReadString('\n'); err != io.EOF {
			t.Fatalf("stream must end after resync, got err=%v", err)
		}
		return
	}
	t.Fatal("no resync frame before deadline")
}

// TestMuxHTTP_CancelUnsubscribesSubscriber: request cancellation must drop the
// subscriber from the registry (T-4 cleanup proof at the HTTP layer).
func TestMuxHTTP_CancelUnsubscribesSubscriber(t *testing.T) {
	svc := NewInteractiveService()
	mkDecisionRun(svc, "run-mux-cancel", "proj-1")

	srv, resp, _, cancel := startMuxStream(t, svc)
	defer srv.Close()
	if n := svc.runUpdateSubCount(); n != 1 {
		t.Fatalf("subscribers = %d, want 1", n)
	}
	cancel()
	_ = resp.Body.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if svc.runUpdateSubCount() == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("subscriber still registered after request cancel")
}

// TestMuxHTTP_WriteFailureUnsubscribesSubscriber: when the client disappears
// mid-stream, the next write fails and the handler unwinds the subscriber
// (T-7). Close the body, then force a dirty drain so a write is attempted.
func TestMuxHTTP_WriteFailureUnsubscribesSubscriber(t *testing.T) {
	svc := NewInteractiveService()
	rs := mkDecisionRun(svc, "run-mux-wfail", "proj-1")

	srv, resp, _, cancel := startMuxStream(t, svc)
	defer srv.Close()
	defer cancel()
	if n := svc.runUpdateSubCount(); n != 1 {
		t.Fatalf("subscribers = %d, want 1", n)
	}
	// Client goes away without cancelling the request context — the handler
	// only learns on the next write.
	_ = resp.Body.Close()

	svc.mu.Lock()
	rs.status = RunStatusWaitingApproval
	svc.markRunRealtimeDirtyLocked(rs.id)
	svc.mu.Unlock()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if svc.runUpdateSubCount() == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("subscriber still registered after write failure")
}
