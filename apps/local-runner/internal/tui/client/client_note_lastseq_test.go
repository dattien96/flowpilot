// Tests CA-520: the per-run SSE cursor must be seeded from a resume snapshot so
// a continue turn after /open streams from the snapshot, not from 0 (which
// replayed the entire prior turn into the live transcript).
package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/client"
)

// TestNoteLastSeq_SeedsSendTurnCursor is the CA-520 core: after NoteLastSeq
// raises the cursor to 50, SendTurn must request the SSE stream at afterSeq=50
// instead of 0 — exactly what /open does when the resume handle carries
// LastEventSeq.
func TestNoteLastSeq_SeedsSendTurnCursor(t *testing.T) {
	var gotAfter atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
			json.NewEncoder(w).Encode(map[string]string{"turnId": "turn-1"})
		case strings.Contains(r.URL.Path, "/events/stream"):
			gotAfter.Store(r.URL.Query().Get("afterSeq"))
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			flusher, _ := w.(http.Flusher)
			for _, ev := range []client.ProviderEvent{
				{ID: "e51", Seq: 51, Type: "turn_started", Prompt: "hi"},
				{ID: "e52", Seq: 52, Type: "message_delta", ProviderTurnID: "turn-1", Text: "answer"},
				{ID: "e53", Seq: 53, Type: "turn_completed", ProviderTurnID: "turn-1", FinalMessage: "answer"},
			} {
				data, _ := json.Marshal(ev)
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	c.NoteLastSeq("run-1", 50)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	evCh, errCh := c.SendTurn(ctx, client.TurnInput{RunID: "run-1", Prompt: "hi"})

	var received []client.ProviderEvent
	for ev := range evCh {
		received = append(received, ev)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn error: %v", err)
	}

	if got := gotAfter.Load(); got != "50" {
		t.Fatalf("afterSeq=%v, want 50 (cursor must be seeded, not 0)", got)
	}
	if len(received) != 3 {
		t.Fatalf("received %d events, want 3", len(received))
	}
}

// TestNoteLastSeq_NeverDecreases locks the monotonic contract: an older snapshot
// (40) must never rewind a cursor already advanced to 50.
func TestNoteLastSeq_NeverDecreases(t *testing.T) {
	var gotAfter atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
			json.NewEncoder(w).Encode(map[string]string{"turnId": "turn-1"})
		case strings.Contains(r.URL.Path, "/events/stream"):
			gotAfter.Store(r.URL.Query().Get("afterSeq"))
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	c.NoteLastSeq("run-1", 50)
	c.NoteLastSeq("run-1", 40) // stale snapshot must not rewind

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	evCh, errCh := c.SendTurn(ctx, client.TurnInput{RunID: "run-1", Prompt: "hi"})
	for range evCh {
	}
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn error: %v", err)
	}

	if got := gotAfter.Load(); got != "50" {
		t.Fatalf("afterSeq=%v, want 50 (NoteLastSeq must never decrease)", got)
	}
}

// TestSendTurn_AdvancesCursorForNextTurn locks the streaming side: after a first
// SendTurn consumed events up to seq 10, a second SendTurn on the same run must
// start after that cursor even without an explicit NoteLastSeq.
func TestSendTurn_AdvancesCursorForNextTurn(t *testing.T) {
	var afterSeqs []string
	mu := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
			json.NewEncoder(w).Encode(map[string]string{"turnId": "turn-1"})
		case strings.Contains(r.URL.Path, "/events/stream"):
			afterSeqs = append(afterSeqs, r.URL.Query().Get("afterSeq"))
			select {
			case mu <- struct{}{}:
			default:
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			flusher, _ := w.(http.Flusher)
			for _, ev := range []client.ProviderEvent{
				{ID: "e1", Seq: 1, Type: "turn_started", Prompt: "p1"},
				{ID: "e2", Seq: 2, Type: "message_delta", ProviderTurnID: "turn-1", Text: "a"},
				{ID: "e3", Seq: 3, Type: "turn_completed", ProviderTurnID: "turn-1", FinalMessage: "a"},
			} {
				data, _ := json.Marshal(ev)
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	evCh, errCh := c.SendTurn(ctx, client.TurnInput{RunID: "run-1", Prompt: "p1"})
	for range evCh {
	}
	if err := <-errCh; err != nil {
		t.Fatalf("first SendTurn error: %v", err)
	}

	// First stream starts at 0; events advanced the internal cursor to 3.
	if len(afterSeqs) != 1 || afterSeqs[0] != "0" {
		t.Fatalf("first afterSeq=%v, want [0]", afterSeqs)
	}

	evCh2, errCh2 := c.SendTurn(ctx, client.TurnInput{RunID: "run-1", Prompt: "p2"})
	for range evCh2 {
	}
	if err := <-errCh2; err != nil {
		t.Fatalf("second SendTurn error: %v", err)
	}
	if len(afterSeqs) != 2 || afterSeqs[1] != "3" {
		t.Fatalf("second afterSeq=%v, want 3 (cursor must advance across turns)", afterSeqs)
	}
}