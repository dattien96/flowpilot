package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// CA-535: the providers scan on the runner spawns CLI probes. On Windows those
// used to steal console input for the probe's full duration, so the TUI's
// session unlock must never wait the entire fast-path window on a slow scan.
// This test proves cmdLoadSessionDefaults returns well before a slow /providers
// completes (the 2s providers budget, not the 8s accounts window).
func TestCmdLoadSessionDefaults_ProvidersScanDoesNotHoldUnlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/client/projects":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[]`)
		case "/client/provider-accounts":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[]`)
		case "/providers":
			// Slow probe scan (3s) that outlives the 2s providers budget.
			time.Sleep(3 * time.Second)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	m := New(config.ChatConfig{}, srv.URL)
	cmd := m.cmdLoadSessionDefaults()
	if cmd == nil {
		t.Fatal("nil cmd")
	}
	start := time.Now()
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		sd, ok := msg.(SessionDefaultsMsg)
		if !ok {
			t.Fatalf("msg type %T", msg)
		}
		elapsed := time.Since(start)
		// 3s probe scan > 2s providers budget: unlock must arrive by ~2s+buffer,
		// never by waiting out the full probe.
		if elapsed > 4500*time.Millisecond {
			t.Fatalf("session unlock waited on the providers scan: elapsed=%v", elapsed)
		}
		// A providers ctx deadline is not a dial failure — it must not be
		// reported as "runner offline" (mirrors CA-514 catalog rule).
		if runnerDialDeadErr(sd.CatalogErr) {
			t.Fatalf("providers timeout must not be dial-dead: %q", sd.CatalogErr)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("cmdLoadSessionDefaults hung on slow providers scan")
	}
}
