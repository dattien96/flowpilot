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

func TestStreamLive_KeepsEventsAfterTurnCompleted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/events/stream") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		write := func(ev client.ProviderEvent) {
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			if flusher != nil {
				flusher.Flush()
			}
		}
		write(client.ProviderEvent{ID: "e1", Seq: 1, Type: "turn_completed", FinalMessage: "hub done"})
		write(client.ProviderEvent{ID: "e2", Seq: 2, Type: "agent_graph_updated"})
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch := client.New(srv.URL).StreamLive(ctx, "run-91842", 0)
	var types []string
	deadline := time.After(2 * time.Second)
	for len(types) < 2 {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("StreamLive closed after %v; want events after turn_completed", types)
			}
			types = append(types, ev.Type)
		case <-deadline:
			t.Fatalf("timeout waiting for post-turn event; got %v", types)
		}
	}
	if types[0] != "turn_completed" || types[1] != "agent_graph_updated" {
		t.Fatalf("events=%v", types)
	}
}
