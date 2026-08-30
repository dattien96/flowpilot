package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/client"
)

// BUG-337 F-2: TUI SSE scanner must handle large frames (>> 64 KB).
// Grok read_file on a PNG returns ImageContent base64 (MB) as tool_completed.Output.
// The serialized ProviderEvent on a single data: line exceeds bufio.Scanner's
// 64 KB default and the stream would close without any further events.
// F-2 bumps the buffer to 8 MB (same as grok_process.go/opencode_process.go).

func TestBug337_SSE_LargeFrameSurvives(t *testing.T) {
	const wantTurnID = "turn-large"
	largeText := strings.Repeat("x", 200*1024) // 200 KB > 64 KB

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
			json.NewEncoder(w).Encode(map[string]string{"turnId": wantTurnID})
		case strings.Contains(r.URL.Path, "/events/stream"):
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			ev := client.ProviderEvent{ID: "e-large", Seq: 1, Type: "tool_completed", ProviderTurnID: wantTurnID, Text: largeText}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			// Also send terminal so SendTurn can return successfully
			ev2 := client.ProviderEvent{ID: "e-term", Seq: 2, Type: "turn_completed", ProviderTurnID: wantTurnID, FinalMessage: "done"}
			data2, _ := json.Marshal(ev2)
			fmt.Fprintf(w, "data: %s\n\n", data2)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	evCh, errCh := c.SendTurn(ctx, client.TurnInput{RunID: "run-large", Prompt: "hi"})
	var received []client.ProviderEvent
	for ev := range evCh {
		received = append(received, ev)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("SendTurn error (large frame must survive): %v", err)
	}
	if len(received) != 2 {
		t.Fatalf("received %d events, want 2 (large frame + terminal)", len(received))
	}
	if len(received[0].Text) != len(largeText) {
		t.Fatalf("large text length mismatch: got %d want %d", len(received[0].Text), len(largeText))
	}
}

// BUG-337 F-3: EOF without turn_completed/turn_failed must surface as error,
// not as silent success. Before the fix the TUI would settle "done" with no
// answer (run-383622 stopped after tool_completed, never got turn_completed).

func TestBug337_SSE_EOFWithoutTerminalIsError(t *testing.T) {
	const wantTurnID = "turn-eof"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/turns"):
			json.NewEncoder(w).Encode(map[string]string{"turnId": wantTurnID})
		case strings.Contains(r.URL.Path, "/events/stream"):
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			ev := client.ProviderEvent{ID: "e1", Seq: 1, Type: "message_delta", ProviderTurnID: wantTurnID, Text: "partial"}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
			// Close without terminal event
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	evCh, errCh := c.SendTurn(ctx, client.TurnInput{RunID: "run-eof", Prompt: "hi"})
	var received []client.ProviderEvent
	for ev := range evCh {
		received = append(received, ev)
	}
	err := <-errCh
	if err == nil {
		t.Fatalf("expected error when stream ends without terminal event, got nil (received %d events)", len(received))
	}
	if !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("error must mention terminal, got %v", err)
	}
	if len(received) != 1 {
		t.Fatalf("should have received the partial delta before error, got %d", len(received))
	}
}
