package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Phase 3 (04-03): Codex app-server JSON-RPC param builders + the async
// dispatcher that multiplexes a single shared stdio across many threads/turns.
//
// IMPORTANT: this REPLACES the synchronous single-in-flight model
// (`readJsonRpcResponseWithHandler` / `SendMu` / id `3`, sessions.go:157) — that
// model cannot multiplex a shared app-server. Only the `writeJsonRpcRequest` /
// `readJsonRpcMessage` framing is reused (newline-delimited JSON).
//
// Codex method/param names below are the assumed app-server shapes; verify them
// against the installed Codex build (06 Part D).

// ---- JSON-RPC param builders (next to geminiACP*Params) --------------------

func codexInitializeParams() map[string]any {
	return map[string]any{
		"clientInfo": map[string]any{"name": "flowpilot", "version": "1.0"},
	}
}

// codexThreadStartParams carries cwd, the YOLO-derived sandbox + approval mode
// (04-04), and the mcpServers list (FlowPilot proxy + required MCPs + ask_user).
func codexThreadStartParams(cwd, sandbox, approvalMode string, mcpServers []any) map[string]any {
	if mcpServers == nil {
		mcpServers = []any{}
	}
	p := map[string]any{"cwd": cwd, "mcpServers": mcpServers}
	if sandbox != "" {
		p["sandbox"] = sandbox
	}
	if approvalMode != "" {
		p["approvalMode"] = approvalMode
		p["approvalPolicy"] = approvalMode
	}
	return p
}

func codexThreadResumeParams(threadID string) map[string]any {
	return map[string]any{"threadId": threadID}
}

func codexThreadListParams(cwd string) map[string]any {
	return map[string]any{"cwd": cwd}
}

func codexThreadReadParams(threadID string) map[string]any {
	return map[string]any{"threadId": threadID}
}

func codexTurnStartParams(threadID, prompt string, skill *SkillSelection) map[string]any {
	p := map[string]any{
		"threadId": threadID,
		"input": []any{
			map[string]any{
				"type":          "text",
				"text":          prompt,
				"text_elements": []any{},
			},
		},
	}
	if skill != nil && skill.Name != "" {
		p["skill"] = skill.Name
	}
	return p
}

func codexInterruptParams(threadID, turnID string) map[string]any {
	p := map[string]any{"threadId": threadID}
	if turnID != "" {
		p["turnId"] = turnID
	}
	return p
}

// ---- async dispatcher ------------------------------------------------------

type codexResponse struct {
	result map[string]any
	err    error
}

// codexInboundRequest is a server→client request (has both method and id) that the
// dispatcher routes to a reply-capable handler — the THIRD message category
// (approval/elicitation). The handler must call Reply/ReplyError with the id.
type codexInboundRequest struct {
	ID     any
	Method string
	Params map[string]any
}

// codexNotification is a server notification routed to the owning turn by thread id.
type codexNotification struct {
	Method string
	Params map[string]any
}

type codexDispatcher struct {
	w       io.Writer
	writeMu sync.Mutex

	mu         sync.Mutex
	nextID     int64
	waiters    map[int64]chan codexResponse
	threadSubs map[string]chan codexNotification
	closed     bool
	closeErr   error
	done       chan struct{}

	// inbound handles server→client requests; called on its own goroutine so a
	// blocking handler (e.g. an approval waiting on the user) never stalls the
	// read loop. Approval/ask_user reply logic is wired in Phase 4.
	inbound func(codexInboundRequest)
}

func newCodexDispatcher(w io.Writer, inbound func(codexInboundRequest)) *codexDispatcher {
	return &codexDispatcher{
		w:          w,
		waiters:    map[int64]chan codexResponse{},
		threadSubs: map[string]chan codexNotification{},
		done:       make(chan struct{}),
		inbound:    inbound,
	}
}

// setInbound wires the server→client request handler (set once before start()).
func (d *codexDispatcher) setInbound(f func(codexInboundRequest)) {
	d.inbound = f
}

// start launches the single read-loop goroutine.
func (d *codexDispatcher) start(r io.Reader) {
	go d.readLoop(r)
}

func (d *codexDispatcher) readLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // skip malformed frame, keep the loop alive
		}
		d.dispatch(msg)
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	d.fail(fmt.Errorf("codex app-server stream closed: %w", err))
}

func (d *codexDispatcher) dispatch(msg map[string]any) {
	method, hasMethod := msg["method"].(string)
	idValue, hasID := msg["id"]

	switch {
	case hasMethod && hasID:
		// inbound server→client request (third category) — reply required
		params, _ := msg["params"].(map[string]any)
		if d.inbound != nil {
			go d.inbound(codexInboundRequest{ID: idValue, Method: method, Params: params})
		}
	case hasMethod && !hasID:
		// notification → route by thread id to the owning turn
		params, _ := msg["params"].(map[string]any)
		threadID := codexThreadIDFromParams(params)
		d.mu.Lock()
		ch := d.threadSubs[threadID]
		d.mu.Unlock()
		if ch != nil {
			select {
			case ch <- codexNotification{Method: method, Params: params}:
			default: // bounded buffer full — consumer reconnects/recovers, never block the loop
			}
		}
	case !hasMethod && hasID:
		// response → deliver to the waiter
		id := jsonRPCIDToInt(idValue)
		d.mu.Lock()
		waiter := d.waiters[id]
		delete(d.waiters, id)
		d.mu.Unlock()
		if waiter != nil {
			if errObj, ok := msg["error"]; ok {
				waiter <- codexResponse{err: fmt.Errorf("%s", jsonRpcErrorMessage(map[string]any{"error": errObj}))}
			} else {
				result, _ := msg["result"].(map[string]any)
				waiter <- codexResponse{result: result}
			}
		}
	}
}

// call sends a client→server request and waits for its response, the caller's ctx,
// or dispatcher death (whichever first). No leaked waiters.
func (d *codexDispatcher) call(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	d.mu.Lock()
	if d.closed {
		err := d.closeErr
		d.mu.Unlock()
		return nil, err
	}
	d.nextID++
	id := d.nextID
	ch := make(chan codexResponse, 1)
	d.waiters[id] = ch
	d.mu.Unlock()

	if err := d.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		d.mu.Lock()
		delete(d.waiters, id)
		d.mu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		return resp.result, resp.err
	case <-ctx.Done():
		d.mu.Lock()
		delete(d.waiters, id)
		d.mu.Unlock()
		return nil, ctx.Err()
	case <-d.done:
		return nil, d.closeErr
	}
}

func (d *codexDispatcher) notify(method string, params map[string]any) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (d *codexDispatcher) reply(id any, result map[string]any) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (d *codexDispatcher) replyError(id any, message string) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32000, "message": message}})
}

func (d *codexDispatcher) write(msg map[string]any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	_, err = d.w.Write(b)
	return err
}

// registerThread subscribes to notifications for a thread (bounded buffer).
func (d *codexDispatcher) registerThread(threadID string) (chan codexNotification, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, d.closeErr
	}
	ch := make(chan codexNotification, 256)
	d.threadSubs[threadID] = ch
	return ch, nil
}

func (d *codexDispatcher) unregisterThread(threadID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.threadSubs, threadID) // closed only by fail(); normal end just detaches
}

// fail marks the dispatcher dead and drains every pending waiter + thread channel
// with the error — so a process death never leaves a turn blocked.
func (d *codexDispatcher) fail(err error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	d.closeErr = err
	close(d.done)
	waiters := d.waiters
	subs := d.threadSubs
	d.waiters = map[int64]chan codexResponse{}
	d.threadSubs = map[string]chan codexNotification{}
	d.mu.Unlock()

	for _, ch := range waiters {
		select {
		case ch <- codexResponse{err: err}:
		default:
		}
	}
	for _, ch := range subs {
		close(ch) // signals the turn pump that the stream died
	}
}

func (d *codexDispatcher) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

// ---- helpers ---------------------------------------------------------------

func jsonRPCIDToInt(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	}
	return 0
}

// codexThreadIDFromParams extracts the owning thread id from a notification's
// params (assumed key "threadId"; verify against the installed build).
func codexThreadIDFromParams(params map[string]any) string {
	if params == nil {
		return ""
	}
	if id, ok := params["threadId"].(string); ok {
		return id
	}
	if id, ok := params["conversationId"].(string); ok {
		return id
	}
	if thread, ok := params["thread"].(map[string]any); ok {
		if id, ok := thread["id"].(string); ok {
			return id
		}
	}
	return ""
}

func codexThreadIDFromResponse(result map[string]any) string {
	return codexThreadIDFromParams(result)
}

func codexTurnIDFromResponse(result map[string]any) string {
	if result == nil {
		return ""
	}
	if id, ok := result["turnId"].(string); ok {
		return id
	}
	if turn, ok := result["turn"].(map[string]any); ok {
		if id, ok := turn["id"].(string); ok {
			return id
		}
	}
	return ""
}
