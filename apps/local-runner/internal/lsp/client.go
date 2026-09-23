package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// defaultRequestTimeout bounds every pending JSON-RPC request so a hung
// server can never deadlock the runner (Task-354 T-4).
const defaultRequestTimeout = 5 * time.Second

// maxHeaderLines caps header scanning so a garbage stream cannot spin the
// read loop forever.
const maxHeaderLines = 128

// rpcMessage is the shared wire shape for requests, responses and
// notifications. Exactly one of (ID+Method) / (ID+Result|Error) / (Method)
// is populated per message.
type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError is a JSON-RPC 2.0 error object.
type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return "lsp: unknown rpc error"
	}
	return fmt.Sprintf("lsp: rpc error %d: %s", e.Code, e.Message)
}

// DiagnosticsHandler receives server-pushed diagnostics.
type DiagnosticsHandler func(PublishDiagnosticsParams)

// Client is a JSON-RPC 2.0 client speaking LSP over stdio. It is safe for
// concurrent use: one background goroutine owns stdout and demuxes inbound
// messages to per-request channels (by ID) or to the notification handler
// (by method). Callers must arrange EOF on stdout (or kill the server
// process) to let the read loop exit; Close marks the client unusable.
type Client struct {
	stdin  io.Writer
	stdout io.Reader

	writeMu sync.Mutex

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan rpcResult
	onDiags DiagnosticsHandler

	closed atomic.Bool
}

type rpcResult struct {
	result json.RawMessage
	rpcErr *rpcError
}

// NewClient wraps an LSP server's stdin/stdout pipes and starts the read
// loop. onDiags may be nil and can be replaced later with
// SetDiagnosticsHandler.
func NewClient(stdin io.Writer, stdout io.Reader) *Client {
	c := &Client{
		stdin:   stdin,
		stdout:  stdout,
		pending: make(map[int64]chan rpcResult),
	}
	go c.readLoop()
	return c
}

// SetDiagnosticsHandler registers (or replaces) the callback invoked for
// every textDocument/publishDiagnostics notification.
func (c *Client) SetDiagnosticsHandler(h DiagnosticsHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDiags = h
}

// Close marks the client closed. In-flight requests fail fast; the read loop
// exits once stdout hits EOF or errors.
func (c *Client) Close() {
	c.closed.Store(true)
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		delete(c.pending, id)
		select {
		case ch <- rpcResult{rpcErr: &rpcError{Code: -32800, Message: "lsp: client closed"}}:
		default:
		}
	}
}

// Initialize performs the LSP initialize handshake and returns the server's
// capabilities.
func (c *Client) Initialize(ctx context.Context, params InitializeParams) (InitializeResult, error) {
	var out InitializeResult
	raw, err := c.request(ctx, "initialize", params)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("lsp: decode initialize result: %w", err)
	}
	return out, nil
}

// Shutdown sends the shutdown request followed by the exit notification, per
// the LSP lifecycle.
func (c *Client) Shutdown(ctx context.Context) error {
	if _, err := c.request(ctx, "shutdown", nil); err != nil {
		return err
	}
	return c.notify("exit", nil)
}

// Initialized sends the LSP `initialized` notification that completes the
// handshake (BUG-380). Servers like gopls defer workspace/package load until
// it arrives — without it every didOpen/didChange only yields the sev-2
// "No active builds" placeholder and real diagnostics never flow.
func (c *Client) Initialized() error {
	return c.notify("initialized", map[string]any{})
}

// DidOpen notifies the server that a document was opened.
func (c *Client) DidOpen(params DidOpenTextDocumentParams) error {
	return c.notify("textDocument/didOpen", params)
}

// DidChange notifies the server that an open document changed.
func (c *Client) DidChange(params DidChangeTextDocumentParams) error {
	return c.notify("textDocument/didChange", params)
}

// DidClose notifies the server that a document was closed.
func (c *Client) DidClose(params DidCloseTextDocumentParams) error {
	return c.notify("textDocument/didClose", params)
}

// DocumentSymbols requests textDocument/documentSymbol and returns the flat
// symbol rows (CP-67 P-2, Task-379 B-8.4). Servers that reply with the
// hierarchical DocumentSymbol[] shape are handled by the caller flattening —
// the runner's adapter maps this to flowgate.DocumentSymbol anchors.
func (c *Client) DocumentSymbols(ctx context.Context, uri string) ([]DocumentSymbolResultItem, error) {
	raw, err := c.request(ctx, "textDocument/documentSymbol", DocumentSymbolParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	if err != nil {
		return nil, err
	}
	var out []DocumentSymbolResultItem
	if len(raw) == 0 || string(raw) == "null" {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("lsp: decode documentSymbol result: %w", err)
	}
	return out, nil
}

// request sends a JSON-RPC request and waits for its response.
func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("lsp: client closed")
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultRequestTimeout)
		defer cancel()
	}
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan rpcResult, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("lsp: marshal %s params: %w", method, err)
		}
		rawParams = b
	}
	msg := rpcMessage{JSONRPC: "2.0", ID: &id, Method: method, Params: rawParams}
	if err := c.send(msg); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("lsp: %s timed out: %w", method, ctx.Err())
	case res := <-ch:
		if res.rpcErr != nil {
			return nil, res.rpcErr
		}
		if len(res.result) == 0 {
			return json.RawMessage("null"), nil
		}
		return res.result, nil
	}
}

// notify sends a JSON-RPC notification (no response expected).
func (c *Client) notify(method string, params any) error {
	if c.closed.Load() {
		return fmt.Errorf("lsp: client closed")
	}
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("lsp: marshal %s params: %w", method, err)
		}
		rawParams = b
	}
	return c.send(rpcMessage{JSONRPC: "2.0", Method: method, Params: rawParams})
}

// send frames and writes one message. Callers must not hold c.mu.
func (c *Client) send(msg rpcMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("lsp: marshal message: %w", err)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return fmt.Errorf("lsp: write frame header: %w", err)
	}
	if _, err := c.stdin.Write(body); err != nil {
		return fmt.Errorf("lsp: write frame body: %w", err)
	}
	return nil
}

// readLoop owns stdout until EOF or a fatal read error.
func (c *Client) readLoop() {
	br := bufio.NewReader(c.stdout)
	for {
		body, err := readFrame(br)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				c.failAllPending(fmt.Errorf("lsp: server closed stdout: %w", err))
				return
			}
			// Malformed message: log and resync on the next frame instead of
			// dying — one corrupt message must not kill diagnostics (R-4).
			log.Printf("[lsp] malformed frame skipped: %v", err)
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			log.Printf("[lsp] malformed message skipped: %v", err)
			continue
		}
		c.dispatch(msg)
	}
}

// dispatch routes one decoded message to its pending request or to the
// notification handler.
func (c *Client) dispatch(msg rpcMessage) {
	if msg.ID != nil && msg.Method == "" {
		c.mu.Lock()
		ch, ok := c.pending[*msg.ID]
		c.mu.Unlock()
		if !ok {
			return
		}
		select {
		case ch <- rpcResult{result: msg.Result, rpcErr: msg.Error}:
		default:
		}
		return
	}
	if msg.Method != "" {
		c.handleNotification(msg.Method, msg.Params)
	}
}

// handleNotification dispatches server-to-client notifications.
func (c *Client) handleNotification(method string, params json.RawMessage) {
	switch method {
	case "textDocument/publishDiagnostics":
		var p PublishDiagnosticsParams
		if err := json.Unmarshal(params, &p); err != nil {
			log.Printf("[lsp] malformed publishDiagnostics skipped: %v", err)
			return
		}
		c.mu.Lock()
		h := c.onDiags
		c.mu.Unlock()
		if h != nil {
			h(p)
		}
	default:
		// Unknown notifications ($/progress, window/logMessage, ...) are
		// ignored by design at this layer.
	}
}

// failAllPending unblocks every in-flight request with err.
func (c *Client) failAllPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rpcErr := &rpcError{Code: -32800, Message: err.Error()}
	for id, ch := range c.pending {
		delete(c.pending, id)
		select {
		case ch <- rpcResult{rpcErr: rpcErr}:
		default:
		}
	}
}

// readFrame reads one Content-Length-framed message. Lines that are neither
// headers nor the blank separator are skipped so a corrupt stream can
// resync on the next valid frame.
func readFrame(br *bufio.Reader) ([]byte, error) {
	var contentLength = -1
	for i := 0; i < maxHeaderLines; i++ {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			break
		}
		name, value, found := strings.Cut(trimmed, ":")
		if !found {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("lsp: invalid Content-Length %q", value)
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("lsp: frame without Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(br, body); err != nil {
		return nil, err
	}
	return body, nil
}
