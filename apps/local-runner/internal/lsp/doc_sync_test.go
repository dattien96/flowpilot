package lsp

import (
	"bufio"
	"encoding/json"
	"io"
	"testing"
	"time"
)

// lspDocFrame is one notification observed from a DocumentSyncManager client.
type lspDocFrame struct {
	method string
	params json.RawMessage
}

// lspDocHarness builds a DocumentSyncManager over pipes with a draining
// fake server that records every outbound frame.
func lspDocHarness(t *testing.T) (*DocumentSyncManager, chan lspDocFrame) {
	t.Helper()
	clientOutR, clientOutW := io.Pipe()
	testOutR, testOutW := io.Pipe()
	c := NewClient(clientOutW, testOutR)
	frames := make(chan lspDocFrame, 64)
	go func() {
		br := bufio.NewReader(clientOutR)
		for {
			body, err := readFrame(br)
			if err != nil {
				return
			}
			var msg rpcMessage
			if err := json.Unmarshal(body, &msg); err != nil {
				continue
			}
			frames <- lspDocFrame{method: msg.Method, params: msg.Params}
		}
	}()
	t.Cleanup(func() {
		c.Close()
		_ = clientOutR.Close()
		_ = clientOutW.Close()
		_ = testOutR.Close()
		_ = testOutW.Close()
	})
	return NewDocumentSyncManager(c), frames
}

func lspNextFrame(t *testing.T, frames chan lspDocFrame) lspDocFrame {
	t.Helper()
	select {
	case f := <-frames:
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for client frame")
		return lspDocFrame{}
	}
}

func lspNoFrame(t *testing.T, frames chan lspDocFrame) {
	t.Helper()
	select {
	case f := <-frames:
		t.Fatalf("unexpected client frame: %s", f.method)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestDocSyncOpenSendsDidOpen(t *testing.T) {
	mgr, frames := lspDocHarness(t)
	if err := mgr.OpenDocument("file:///tmp/a.go", "go", "package a\n"); err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}
	f := lspNextFrame(t, frames)
	if f.method != "textDocument/didOpen" {
		t.Fatalf("method = %q, want textDocument/didOpen", f.method)
	}
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(f.params, &params); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if params.TextDocument.URI != "file:///tmp/a.go" || params.TextDocument.LanguageID != "go" ||
		params.TextDocument.Version != 1 || params.TextDocument.Text != "package a\n" {
		t.Fatalf("didOpen params = %+v", params)
	}
	if !mgr.IsOpen("file:///tmp/a.go") || mgr.OpenCount() != 1 {
		t.Fatal("document should be tracked as open")
	}
}

func TestDocSyncChangeIncrementsVersion(t *testing.T) {
	mgr, frames := lspDocHarness(t)
	if err := mgr.OpenDocument("file:///tmp/a.go", "go", "v1"); err != nil {
		t.Fatal(err)
	}
	lspNextFrame(t, frames) // didOpen

	if err := mgr.ChangeDocument("file:///tmp/a.go", "v2"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ChangeDocument("file:///tmp/a.go", "v3"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		version int32
		text    string
	}{{2, "v2"}, {3, "v3"}} {
		f := lspNextFrame(t, frames)
		if f.method != "textDocument/didChange" {
			t.Fatalf("method = %q, want textDocument/didChange", f.method)
		}
		var params DidChangeTextDocumentParams
		if err := json.Unmarshal(f.params, &params); err != nil {
			t.Fatal(err)
		}
		if params.TextDocument.Version != want.version || len(params.ContentChanges) != 1 || params.ContentChanges[0].Text != want.text {
			t.Fatalf("didChange params = %+v", params)
		}
	}
	if got := mgr.Version("file:///tmp/a.go"); got != 3 {
		t.Fatalf("version = %d, want 3", got)
	}
}

func TestDocSyncCloseSendsDidClose(t *testing.T) {
	mgr, frames := lspDocHarness(t)
	if err := mgr.OpenDocument("file:///tmp/a.go", "go", "v1"); err != nil {
		t.Fatal(err)
	}
	lspNextFrame(t, frames) // didOpen

	if err := mgr.CloseDocument("file:///tmp/a.go"); err != nil {
		t.Fatalf("CloseDocument: %v", err)
	}
	f := lspNextFrame(t, frames)
	if f.method != "textDocument/didClose" {
		t.Fatalf("method = %q, want textDocument/didClose", f.method)
	}
	var params DidCloseTextDocumentParams
	if err := json.Unmarshal(f.params, &params); err != nil {
		t.Fatal(err)
	}
	if params.TextDocument.URI != "file:///tmp/a.go" {
		t.Fatalf("didClose params = %+v", params)
	}
	if mgr.IsOpen("file:///tmp/a.go") {
		t.Fatal("document should be forgotten after close")
	}
	// Closing an unknown document is a silent no-op.
	if err := mgr.CloseDocument("file:///tmp/a.go"); err != nil {
		t.Fatalf("second close: %v", err)
	}
	lspNoFrame(t, frames)
}

func TestDocSyncDoubleOpenIsIdempotent(t *testing.T) {
	mgr, frames := lspDocHarness(t)
	if err := mgr.OpenDocument("file:///tmp/a.go", "go", "v1"); err != nil {
		t.Fatal(err)
	}
	lspNextFrame(t, frames) // didOpen
	if err := mgr.OpenDocument("file:///tmp/a.go", "go", "v1-again"); err != nil {
		t.Fatalf("double open: %v", err)
	}
	lspNoFrame(t, frames)
	if got := mgr.Version("file:///tmp/a.go"); got != 1 {
		t.Fatalf("version = %d, want 1 after idempotent open", got)
	}
	if err := mgr.ChangeDocument("file:///tmp/a.go", "v2"); err != nil {
		t.Fatal(err)
	}
	f := lspNextFrame(t, frames)
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(f.params, &params); err != nil {
		t.Fatal(err)
	}
	if params.TextDocument.Version != 2 {
		t.Fatalf("version after change = %d, want 2", params.TextDocument.Version)
	}
}
