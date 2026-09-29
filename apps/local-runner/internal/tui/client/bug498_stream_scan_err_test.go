package client_test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/client"
)

// BUG-498: openStream must log scanner.Err() before closing — a >8MB data
// line trips the raised buffer cap; without the log the stream failure is
// indistinguishable from a clean end in diagnostics. Live-armed on a real
// TCP wire via httptest (the production scanner path is exercised
// byte-for-byte).
func TestBug498_TUIStreamOverCapLineLogsScanError(t *testing.T) {
	var logBuf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(orig)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/events/stream") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// One data line > 8MB scanner cap.
		fmt.Fprintf(w, "data: %s\n\n", strings.Repeat("z", 9*1024*1024))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	got := 0
	for range c.StreamRun(ctx, "r1", 0) {
		got++
	}
	if got != 0 {
		t.Fatalf("oversized frame must not yield events, got %d", got)
	}
	if !strings.Contains(logBuf.String(), "event stream scan ended with error") {
		t.Fatalf("scanner.Err() not logged; log tail: %q", logBuf.String())
	}
}
