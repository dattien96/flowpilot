package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// lspTestPair wires a Client to in-memory pipes and returns the client plus
// the test-side pipe ends. Cleanup closes the pipes so the client read loop
// exits.
func lspTestPair(t *testing.T) (*Client, *bufio.Reader, io.Writer) {
	t.Helper()
	clientOutR, clientOutW := io.Pipe() // client writes -> test reads
	testOutR, testOutW := io.Pipe()     // test writes -> client reads
	c := NewClient(clientOutW, testOutR)
	t.Cleanup(func() {
		c.Close()
		_ = clientOutR.Close()
		_ = clientOutW.Close()
		_ = testOutR.Close()
		_ = testOutW.Close()
	})
	return c, bufio.NewReader(clientOutR), testOutW
}

// lspTestReadMessage reads one framed message the client sent.
func lspTestReadMessage(t *testing.T, r *bufio.Reader) rpcMessage {
	t.Helper()
	body, err := readFrame(r)
	if err != nil {
		t.Fatalf("read client frame: %v", err)
	}
	var msg rpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("decode client frame: %v", err)
	}
	return msg
}

// lspTestWriteMessage frames one message toward the client.
func lspTestWriteMessage(t *testing.T, w io.Writer, msg rpcMessage) {
	t.Helper()
	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal server frame: %v", err)
	}
	if _, err := w.Write([]byte("Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n")); err != nil {
		t.Fatalf("write server frame header: %v", err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatalf("write server frame body: %v", err)
	}
}

func intToString(n int) string { return strconv.Itoa(n) }

func lspTestCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func strptr(s string) string { return s }

func TestClientSendsInitializeRequest(t *testing.T) {
	c, fromClient, toClient := lspTestPair(t)
	root := "file:///tmp/proj"

	type res struct {
		out InitializeResult
		err error
	}
	done := make(chan res, 1)
	go func() {
		out, err := c.Initialize(lspTestCtx(t), InitializeParams{RootURI: &root})
		done <- res{out, err}
	}()

	msg := lspTestReadMessage(t, fromClient)
	if msg.Method != "initialize" {
		t.Fatalf("method = %q, want initialize", msg.Method)
	}
	if msg.ID == nil {
		t.Fatal("initialize must carry a request id")
	}
	var params InitializeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("decode initialize params: %v", err)
	}
	if params.RootURI == nil || *params.RootURI != root {
		t.Fatalf("rootUri = %v, want %q", params.RootURI, root)
	}

	lspTestWriteMessage(t, toClient, rpcMessage{JSONRPC: "2.0", ID: msg.ID, Result: json.RawMessage(`{"capabilities":{}}`)})
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Initialize: %v", r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Initialize did not complete")
	}
}

func TestClientReceivesInitializeResponse(t *testing.T) {
	c, fromClient, toClient := lspTestPair(t)

	done := make(chan InitializeResult, 1)
	go func() {
		out, err := c.Initialize(lspTestCtx(t), InitializeParams{})
		if err != nil {
			t.Errorf("Initialize: %v", err)
			return
		}
		done <- out
	}()

	msg := lspTestReadMessage(t, fromClient)
	lspTestWriteMessage(t, toClient, rpcMessage{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result:  json.RawMessage(`{"capabilities":{},"serverInfo":{"name":"gopls","version":"v0.1"}}`),
	})

	select {
	case out := <-done:
		if out.ServerInfo == nil || out.ServerInfo.Name != "gopls" {
			t.Fatalf("serverInfo = %+v, want gopls", out.ServerInfo)
		}
		if out.ServerInfo.Version == nil || *out.ServerInfo.Version != "v0.1" {
			t.Fatalf("version = %v, want v0.1", out.ServerInfo.Version)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Initialize did not complete")
	}
}

func TestClientSendsDidOpenNotification(t *testing.T) {
	c, fromClient, _ := lspTestPair(t)

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.DidOpen(DidOpenTextDocumentParams{
			TextDocument: TextDocumentItem{URI: "file:///tmp/a.go", LanguageID: "go", Version: 1, Text: "package a\n"},
		})
	}()

	msg := lspTestReadMessage(t, fromClient)
	if msg.Method != "textDocument/didOpen" {
		t.Fatalf("method = %q, want textDocument/didOpen", msg.Method)
	}
	if msg.ID != nil {
		t.Fatal("didOpen must be a notification (no id)")
	}
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("decode didOpen params: %v", err)
	}
	if params.TextDocument.URI != "file:///tmp/a.go" || params.TextDocument.LanguageID != "go" || params.TextDocument.Text != "package a\n" {
		t.Fatalf("didOpen params = %+v", params)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("DidOpen: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DidOpen did not return")
	}
}

func TestClientSendsDidChangeNotification(t *testing.T) {
	c, fromClient, _ := lspTestPair(t)

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.DidChange(DidChangeTextDocumentParams{
			TextDocument:   VersionedTextDocumentIdentifier{URI: "file:///tmp/a.go", Version: 2},
			ContentChanges: []ContentChangeEvent{{Text: "package a\n\nfunc F() {}\n"}},
		})
	}()

	msg := lspTestReadMessage(t, fromClient)
	if msg.Method != "textDocument/didChange" {
		t.Fatalf("method = %q, want textDocument/didChange", msg.Method)
	}
	if msg.ID != nil {
		t.Fatal("didChange must be a notification (no id)")
	}
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("decode didChange params: %v", err)
	}
	if params.TextDocument.Version != 2 || len(params.ContentChanges) != 1 || params.ContentChanges[0].Text != "package a\n\nfunc F() {}\n" {
		t.Fatalf("didChange params = %+v", params)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("DidChange: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DidChange did not return")
	}
}

func TestClientReceivesDiagnosticsNotification(t *testing.T) {
	c, _, toClient := lspTestPair(t)

	got := make(chan PublishDiagnosticsParams, 1)
	c.SetDiagnosticsHandler(func(p PublishDiagnosticsParams) { got <- p })

	lspTestWriteMessage(t, toClient, rpcMessage{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params: json.RawMessage(`{"uri":"file:///tmp/a.go","diagnostics":[` +
			`{"range":{"start":{"line":41,"character":9},"end":{"line":41,"character":12}},"severity":1,"message":"undefined: Foo"}]}`),
	})

	select {
	case p := <-got:
		if p.URI != "file:///tmp/a.go" {
			t.Fatalf("uri = %q", p.URI)
		}
		if len(p.Diagnostics) != 1 {
			t.Fatalf("diagnostics = %+v", p.Diagnostics)
		}
		d := p.Diagnostics[0]
		if d.Severity != DiagnosticSeverityError || d.Message != "undefined: Foo" {
			t.Fatalf("diagnostic = %+v", d)
		}
		if d.Range.Start.Line != 41 || d.Range.Start.Character != 9 {
			t.Fatalf("range = %+v", d.Range)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("diagnostics callback did not fire")
	}
}

func TestClientHandlesMalformedResponse(t *testing.T) {
	c, fromClient, toClient := lspTestPair(t)

	done := make(chan error, 1)
	go func() {
		_, err := c.Initialize(lspTestCtx(t), InitializeParams{})
		done <- err
	}()

	// 1. Garbage line before a valid frame: the reader must resync.
	if _, err := toClient.Write([]byte("this is not an lsp frame\n")); err != nil {
		t.Fatalf("write garbage: %v", err)
	}
	// 2. Valid framing but corrupt JSON body.
	if _, err := toClient.Write([]byte("Content-Length: 5\r\n\r\n{bad}")); err != nil {
		t.Fatalf("write corrupt body: %v", err)
	}
	// 3. Invalid Content-Length value.
	if _, err := toClient.Write([]byte("Content-Length: abc\r\n\r\n")); err != nil {
		t.Fatalf("write bad header: %v", err)
	}

	// The client must still serve a clean roundtrip afterwards.
	msg := lspTestReadMessage(t, fromClient)
	if msg.Method != "initialize" {
		t.Fatalf("method = %q, want initialize", msg.Method)
	}
	lspTestWriteMessage(t, toClient, rpcMessage{JSONRPC: "2.0", ID: msg.ID, Result: json.RawMessage(`{"capabilities":{}}`)})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Initialize after malformed input: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client wedged after malformed input")
	}
}

func TestClientHandlesConcurrentRequests(t *testing.T) {
	c, fromClient, toClient := lspTestPair(t)

	const n = 16
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Initialize(lspTestCtx(t), InitializeParams{})
			errs <- err
		}()
	}

	// Fake server: answer every request with its own id.
	go func() {
		for i := 0; i < n; i++ {
			body, err := readFrame(fromClient)
			if err != nil {
				return
			}
			var msg rpcMessage
			if err := json.Unmarshal(body, &msg); err != nil || msg.ID == nil {
				continue
			}
			resp, _ := json.Marshal(rpcMessage{JSONRPC: "2.0", ID: msg.ID, Result: json.RawMessage(`{"capabilities":{}}`)})
			if _, err := toClient.Write([]byte("Content-Length: " + intToString(len(resp)) + "\r\n\r\n")); err != nil {
				return
			}
			if _, err := toClient.Write(resp); err != nil {
				return
			}
		}
	}()

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Initialize: %v", err)
		}
	}
}

func TestClientShutdownSendsExitNotification(t *testing.T) {
	c, fromClient, toClient := lspTestPair(t)

	done := make(chan error, 1)
	go func() {
		done <- c.Shutdown(lspTestCtx(t))
	}()

	first := lspTestReadMessage(t, fromClient)
	if first.Method != "shutdown" || first.ID == nil {
		t.Fatalf("first message = method %q id %v, want shutdown request", first.Method, first.ID)
	}
	lspTestWriteMessage(t, toClient, rpcMessage{JSONRPC: "2.0", ID: first.ID, Result: json.RawMessage(`null`)})

	second := lspTestReadMessage(t, fromClient)
	if second.Method != "exit" {
		t.Fatalf("second message = %q, want exit notification", second.Method)
	}
	if second.ID != nil {
		t.Fatal("exit must be a notification (no id)")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not complete")
	}
}

func TestProtocolMessageFraming(t *testing.T) {
	body := "héllo wörld ✓" // multibyte: framing counts bytes, not runes
	raw := "Content-Length: " + intToString(len([]byte(body))) + "\r\n\r\n" + body
	got, err := readFrame(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if string(got) != body {
		t.Fatalf("body = %q, want %q", got, body)
	}

	// Extra/unknown headers are tolerated.
	raw2 := "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\nContent-Length: 2\r\n\r\n{}"
	got2, err := readFrame(bufio.NewReader(strings.NewReader(raw2)))
	if err != nil {
		t.Fatalf("readFrame with extra headers: %v", err)
	}
	if string(got2) != "{}" {
		t.Fatalf("body = %q, want {}", got2)
	}
}

func TestProtocolParseContentLength(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"lowercase header", "content-length: 3\r\n\r\nabc", "abc", false},
		{"spaced value", "Content-Length:   3  \r\n\r\nabc", "abc", false},
		{"garbage line resync", "not a header\nContent-Length: 3\r\n\r\nabc", "abc", false},
		{"missing length", "Content-Type: x\r\n\r\nabc", "", true},
		{"invalid length", "Content-Length: abc\r\n\r\n", "", true},
		{"negative length", "Content-Length: -1\r\n\r\n", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readFrame(bufio.NewReader(strings.NewReader(tc.raw)))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got body %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readFrame: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("body = %q, want %q", got, tc.want)
			}
		})
	}
}
