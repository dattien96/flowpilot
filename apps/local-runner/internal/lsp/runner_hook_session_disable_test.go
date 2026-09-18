package lsp

// CP-63 M-4 regression coverage: the reproduce/crash mitigation must be
// SESSION-WIDE. When a server's crash budget is spent (one auto-restart, then
// lsp.disabled), a later CheckFiles for the same (root, platform) must NOT
// respawn a fresh manager with a reset budget — the runner degrades silently
// to build/test validation for the rest of the session instead.
//
// Live evidence 2026-09-17 (runner-owned gopls, /tmp/lsp-m4-bed): after
// `lsp.disabled` the next write turn logged a brand-new `lsp.start` with a
// fresh crash budget — see CP-63-Test-Steps §4 M-4 and the additive fix in
// ServerSet.getOrStart.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLSPHelperCrashAfterInit is re-executed as a mock LSP server binary via
// os.Args[0]. It answers initialize like a minimal language server and THEN
// crashes, so the manager takes the auto-restart path (not the start-failure
// cooldown path). Additive helper: the shared TestLSPHelperProcess switch in
// server_manager_test.go is left untouched.
func TestLSPHelperCrashAfterInit(t *testing.T) {
	if os.Getenv("GO_WANT_LSP_HELPER") != "1" || os.Getenv("GO_LSP_HELPER_CRASH_AFTER_INIT") != "1" {
		return
	}
	// Crash shortly after start regardless of traffic: the auto-restart
	// replacement is never re-handshaked, so a request-driven crash would
	// leave generation 2 alive forever and the budget would never be spent.
	go func() {
		time.Sleep(700 * time.Millisecond)
		os.Exit(1)
	}()
	br := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	for {
		body, err := readFrame(br)
		if err != nil {
			return
		}
		var msg rpcMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}
		if msg.Method == "initialize" && msg.ID != nil {
			resp, _ := json.Marshal(rpcMessage{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result:  json.RawMessage(`{"capabilities":{},"serverInfo":{"name":"fake-lsp-crashy","version":"0.0"}}`),
			})
			fmt.Fprintf(out, "Content-Length: %d\r\n\r\n", len(resp))
			_, _ = out.Write(resp)
			_ = out.Flush()
			// Give the parent a moment to finish the handshake before the
			// crash, so Start+WaitReady succeed and the manager takes the
			// auto-restart path instead of the start-failure cooldown.
			time.Sleep(500 * time.Millisecond)
			os.Exit(1) // crash AFTER a successful handshake
		}
	}
}

func TestServerSetSessionWideDisableAfterCrashBudget(t *testing.T) {
	t.Setenv("GO_WANT_LSP_HELPER", "1")
	t.Setenv("GO_LSP_HELPER_CRASH_AFTER_INIT", "1")
	reg := Registry{
		"golang": {Platform: "golang", Binary: os.Args[0], Args: []string{"-test.run=TestLSPHelperCrashAfterInit"}, FileExtensions: []string{".go"}},
	}
	set := NewServerSet(reg)
	ws := t.TempDir()
	// Cleanup order: Close (kills helpers) runs before TempDir removal.
	t.Cleanup(set.Close)
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(ws)
	if err != nil {
		t.Fatal(err)
	}
	key := serverKey{root: root, platform: "golang"}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First check: the crashy server is started, auto-restarted once, then the
	// crash budget is spent and the manager disables itself. The hook degrades
	// silently either way.
	if got := set.CheckFiles(ctx, ws, []string{"a.go"}); got != "" {
		t.Fatalf("crashy-server check = %q, want empty (silent degradation)", got)
	}
	lspWaitFor(t, "crash budget spent (manager disabled)", func() bool {
		set.mu.Lock()
		defer set.mu.Unlock()
		srv, ok := set.servers[key]
		return ok && srv.Manager.Disabled()
	})

	// The session-wide contract: a later check must NOT respawn the server.
	if got := set.CheckFiles(ctx, ws, []string{"a.go"}); got != "" {
		t.Fatalf("post-disable check = %q, want empty (silent degradation)", got)
	}
	set.mu.Lock()
	_, alive := set.servers[key]
	disabled := set.disabled[key]
	set.mu.Unlock()
	if alive {
		t.Fatal("post-disable check respawned a server; session-wide disable does not hold")
	}
	if !disabled {
		t.Fatal("session disable flag was not recorded for the crashed server")
	}
}

