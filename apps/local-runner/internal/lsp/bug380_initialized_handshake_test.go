package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBUG380HelperProcess is this file's own helper-process entry point so the
// handshake instrumentation lives entirely in the additive fixture — the
// pre-existing server_manager_test.go helpers stay byte-identical to baseline
// (BUG-443: additive-tests-only contract).
func TestBUG380HelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_LSP_HELPER") != "1" {
		return
	}
	bug380HelperServe()
}

// bug380HelperServe is the minimal language server loop from lspHelperServe,
// extended to record every notification method (initialized, didOpen, exit, …)
// so the parent test can assert the LSP lifecycle handshake.
func bug380HelperServe() {
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
				Result:  json.RawMessage(`{"capabilities":{},"serverInfo":{"name":"fake-lsp","version":"0.0"}}`),
			})
			fmt.Fprintf(out, "Content-Length: %d\r\n\r\n", len(resp))
			_, _ = out.Write(resp)
			_ = out.Flush()
		}
		if msg.ID == nil && msg.Method != "" {
			bug380RecordMethod(msg.Method)
		}
		if msg.Method == "exit" {
			return
		}
	}
}

// bug380RecordMethod appends one notification method name per line to
// GO_LSP_HELPER_METHODS (BUG-380 handshake assertions).
func bug380RecordMethod(method string) {
	if path := os.Getenv("GO_LSP_HELPER_METHODS"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		defer f.Close()
		_, _ = f.WriteString(method + "\n")
	}
}

// BUG-380: WaitReady returned after `initialize` without sending the LSP
// `initialized` notification — gopls defers workspace/package load until it
// arrives, so every didOpen/didChange produced only the sev-2 "No active
// builds" placeholder and the whole post-write diagnostics pipeline was dead.
// The handshake must complete: initialize → initialized notification.
func TestBug380_WaitReadySendsInitializedNotification(t *testing.T) {
	m := lspTestManager(t)
	dir := t.TempDir()
	methodsFile := filepath.Join(dir, "methods.txt")
	m.Env = []string{
		"GO_WANT_LSP_HELPER=1",
		"GO_LSP_HELPER_METHODS=" + methodsFile,
	}
	if err := m.Start(context.Background(), os.Args[0],
		[]string{"-test.run=TestBUG380HelperProcess"}, dir); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.WaitReady(ctx, 0); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	// The notification write races the WaitReady return — poll briefly.
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, _ := os.ReadFile(methodsFile)
		if strings.Contains(string(data), "initialized") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("initialized notification never sent; methods=%q", string(data))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// BUG-380 secondary: WaitForDiagnostics returned on ANY URI's publish
// (generation bump) — a publish for a different file ended the wait before
// the target file's diagnostics arrived. The URI-scoped wait must only
// return once THE target URI has been published since the call.
func TestBug380_WaitForDiagnosticsForURIIgnoresOtherFiles(t *testing.T) {
	dc := NewDiagnosticsCollector(nil)
	target := "file:///w/target.go"
	other := "file:///w/other.go"

	done := make(chan error, 1)
	go func() {
		done <- dc.WaitForDiagnosticsForURI(context.Background(), target, 2*time.Second)
	}()

	// A publish for a different URI must NOT end the wait.
	dc.Handle(PublishDiagnosticsParams{URI: other, Diagnostics: []Diagnostic{{Message: "unrelated", Severity: 1}}})
	select {
	case err := <-done:
		t.Fatalf("wait ended on a foreign URI publish: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	// The target publish ends it.
	dc.Handle(PublishDiagnosticsParams{URI: target, Diagnostics: []Diagnostic{{Message: "real error", Severity: 1}}})
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitForDiagnosticsForURI: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("target URI publish did not end the wait")
	}
}

// BUG-380: URI wait must not return early for a URI already published BEFORE
// the call (stale generation) — only a publish AFTER the call counts.
func TestBug380_WaitForDiagnosticsForURIRequiresFreshPublish(t *testing.T) {
	dc := NewDiagnosticsCollector(nil)
	target := "file:///w/target.go"
	dc.Handle(PublishDiagnosticsParams{URI: target, Diagnostics: []Diagnostic{{Message: "old", Severity: 1}}})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := dc.WaitForDiagnosticsForURI(ctx, target, 0); err == nil {
		t.Fatal("returned on a stale publish — no fresh publish for the URI happened after the call")
	}
}
