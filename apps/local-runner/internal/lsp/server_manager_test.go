package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLSPHelperProcess is re-executed as a mock LSP server binary via
// os.Args[0]. It never runs as a real test: without GO_WANT_LSP_HELPER it
// returns immediately.
func TestLSPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_LSP_HELPER") != "1" {
		return
	}
	switch os.Getenv("GO_LSP_HELPER_MODE") {
	case "exit1":
		os.Exit(1)
	case "lspserver":
		lspHelperServe()
	case "diagnostics":
		lspHelperDiagnostics()
	default: // hang: block until stdin closes, never answer
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
}

// lspHelperServe answers initialize like a minimal language server.
func lspHelperServe() {
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
			lspHelperMaybeCapture(msg.Params)
			resp, _ := json.Marshal(rpcMessage{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result:  json.RawMessage(`{"capabilities":{},"serverInfo":{"name":"fake-lsp","version":"0.0"}}`),
			})
			fmt.Fprintf(out, "Content-Length: %d\r\n\r\n", len(resp))
			_, _ = out.Write(resp)
			_ = out.Flush()
		}
		if msg.Method == "exit" {
			return
		}
	}
}

// lspHelperMaybeCapture dumps initialize params to GO_LSP_HELPER_CAPTURE so
// tests can assert what the client sent (e.g. initializationOptions).
func lspHelperMaybeCapture(params json.RawMessage) {
	if path := os.Getenv("GO_LSP_HELPER_CAPTURE"); path != "" {
		_ = os.WriteFile(path, params, 0o644)
	}
}

// lspHelperDiagnostics answers initialize and publishes diagnostics on
// every didOpen/didChange. GO_LSP_HELPER_DIAG controls the payload:
// "clean" (or empty) publishes an empty list; otherwise each |-separated
// chunk becomes one error diagnostic on successive lines.
func lspHelperDiagnostics() {
	br := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	publish := func(uri string) {
		var diags []string
		if msg := os.Getenv("GO_LSP_HELPER_DIAG"); msg != "" && msg != "clean" {
			for i, chunk := range strings.Split(msg, "|") {
				q, _ := json.Marshal(strings.TrimSpace(chunk))
				diags = append(diags, `{"range":{"start":{"line":`+strconv.Itoa(i)+`,"character":0},"end":{"line":`+strconv.Itoa(i)+`,"character":5}},"severity":1,"message":`+string(q)+`}`)
			}
		}
		uriQ, _ := json.Marshal(uri)
		body, _ := json.Marshal(rpcMessage{
			JSONRPC: "2.0",
			Method:  "textDocument/publishDiagnostics",
			Params:  json.RawMessage(`{"uri":` + string(uriQ) + `,"diagnostics":[` + strings.Join(diags, ",") + `]}`),
		})
		fmt.Fprintf(out, "Content-Length: %d\r\n\r\n", len(body))
		_, _ = out.Write(body)
		_ = out.Flush()
	}
	for {
		body, err := readFrame(br)
		if err != nil {
			return
		}
		var msg rpcMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}
		switch msg.Method {
		case "initialize":
			if msg.ID != nil {
				resp, _ := json.Marshal(rpcMessage{JSONRPC: "2.0", ID: msg.ID, Result: json.RawMessage(`{"capabilities":{},"serverInfo":{"name":"fake-lsp","version":"0.0"}}`)})
				fmt.Fprintf(out, "Content-Length: %d\r\n\r\n", len(resp))
				_, _ = out.Write(resp)
				_ = out.Flush()
			}
		case "textDocument/didOpen":
			var p DidOpenTextDocumentParams
			if json.Unmarshal(msg.Params, &p) == nil {
				publish(p.TextDocument.URI)
			}
		case "textDocument/didChange":
			var p DidChangeTextDocumentParams
			if json.Unmarshal(msg.Params, &p) == nil {
				publish(p.TextDocument.URI)
			}
		case "exit":
			return
		}
	}
}

// lspTestManager returns a ServerManager with a fast health tick.
func lspTestManager(t *testing.T) *ServerManager {
	t.Helper()
	m := &ServerManager{healthInterval: 20 * time.Millisecond}
	t.Cleanup(func() { _ = m.Stop() })
	return m
}

// lspHelperStart spawns the test binary in the given helper mode.
func lspHelperStart(t *testing.T, m *ServerManager, mode string) {
	t.Helper()
	dir := t.TempDir()
	// Register Stop AFTER TempDir: cleanup runs LIFO, so the helper (whose
	// cwd is dir) dies before TempDir removal. Otherwise Windows refuses
	// the removal while the child still lives in that dir and the test
	// fails in cleanup with no assertion message.
	t.Cleanup(func() { _ = m.Stop() })
	m.Env = []string{"GO_WANT_LSP_HELPER=1", "GO_LSP_HELPER_MODE=" + mode}
	if err := m.Start(context.Background(), os.Args[0],
		[]string{"-test.run=TestLSPHelperProcess"}, dir); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// lspWaitFor polls cond until true or the deadline hits.
func lspWaitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestServerManagerStartSpawnsProcess(t *testing.T) {
	m := lspTestManager(t)
	lspHelperStart(t, m, "hang")
	if !m.IsRunning() {
		t.Fatal("IsRunning = false after Start")
	}
	if m.Client() == nil {
		t.Fatal("Client() = nil after Start")
	}
	if m.cmd == nil || m.cmd.Process == nil {
		t.Fatal("no OS process spawned")
	}
}

func TestServerManagerStopKillsProcess(t *testing.T) {
	m := lspTestManager(t)
	lspHelperStart(t, m, "hang")
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if m.IsRunning() {
		t.Fatal("IsRunning = true after Stop")
	}
	// Idempotent second stop.
	if err := m.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if m.cmd.ProcessState == nil || !m.cmd.ProcessState.Exited() {
		t.Fatal("OS process was not reaped")
	}
}

func TestServerManagerRestartRecreatesClient(t *testing.T) {
	m := lspTestManager(t)
	lspHelperStart(t, m, "hang")
	first := m.Client()
	if err := m.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if !m.IsRunning() {
		t.Fatal("IsRunning = false after Restart")
	}
	if second := m.Client(); second == nil || second == first {
		t.Fatal("Restart must replace the Client")
	}
}

func TestServerManagerAutoRestartOnCrash(t *testing.T) {
	m := lspTestManager(t)
	lspHelperStart(t, m, "exit1")
	// Crash -> one restart -> second crash -> disabled.
	lspWaitFor(t, "crash recovery to settle", func() bool {
		return !m.IsRunning() && m.Restarts() == 1
	})
	if got := m.Restarts(); got != 1 {
		t.Fatalf("Restarts() = %d, want 1", got)
	}
}

func TestServerManagerMaxOneRestartPerSession(t *testing.T) {
	m := lspTestManager(t)
	lspHelperStart(t, m, "exit1")
	lspWaitFor(t, "disable after budget spent", func() bool { return !m.IsRunning() })
	// Let several more health ticks pass: the budget must not grow.
	time.Sleep(300 * time.Millisecond)
	if got := m.Restarts(); got != 1 {
		t.Fatalf("Restarts() = %d, want exactly 1", got)
	}
	if m.IsRunning() {
		t.Fatal("disabled manager must stay down")
	}
}

func TestServerManagerWaitReadyTimesOut(t *testing.T) {
	m := lspTestManager(t)
	lspHelperStart(t, m, "hang") // never answers initialize
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if err := m.WaitReady(ctx, 200*time.Millisecond); err == nil {
		t.Fatal("WaitReady against a silent server must time out")
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("WaitReady took %v, expected fast timeout", elapsed)
	}
}

func TestServerManagerWaitReadySucceeds(t *testing.T) {
	m := lspTestManager(t)
	lspHelperStart(t, m, "lspserver")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.WaitReady(ctx, 0); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
}

func TestServerManagerIsRunningReflectsState(t *testing.T) {
	m := lspTestManager(t)
	if m.IsRunning() {
		t.Fatal("fresh manager must not report running")
	}
	lspHelperStart(t, m, "hang")
	if !m.IsRunning() {
		t.Fatal("IsRunning = false after Start")
	}
	_ = m.Stop()
	if m.IsRunning() {
		t.Fatal("IsRunning = true after Stop")
	}
}
