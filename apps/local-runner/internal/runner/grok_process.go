package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Task-206 (CP-46 P-2/P-3): the live shared `grok agent stdio` process +
// dispatcher layer. Modeled on codex_appserver.go's async dispatcher (waiters
// map, per-scope notification subs, single inbound handler, one read-loop,
// fail() drain) and codex_appserver_process.go's ensureCodexAppServer (one
// shared persistent process per scopeKey, teardown+respawn on account switch) —
// NOT Claude's spawn-per-turn model (CP-46 T-2).
//
// Unlike Codex's thread/turn split, Grok's session/prompt call itself blocks
// until the prompt turn ends and its own response carries the terminal
// stopReason + token usage (live-verified, Task-206 authoring). Streaming
// happens via session/update notifications delivered while that call is still
// in flight. The dispatcher's call()/notification-channel split already
// supports this: the adapter (Task-207) issues session/prompt via call() on its
// own goroutine while pumping the session's notification channel, exactly like
// codexAdapter.SendTurn pumps codexTurns while turn/start is in flight.

const grokAgentEnvFlag = "FLOWPILOT_GROK_AGENT"

// grokAgentEnabled reports whether the live Grok ACP path is turned on. Mirrors
// codexAppServerEnabled: gated so the default registry/tests never require a
// real grok binary.
func grokAgentEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(grokAgentEnvFlag)))
	return v == "1" || v == "true" || v == "yes"
}

// grokBinaryName is the `grok` binary; overridable for tests AND for machines
// where a different, unrelated tool named "grok" shadows the real xAI Grok
// Build CLI earlier on PATH (observed live during Task-206 authoring: the npm
// package `@vibe-kit/grok-cli` installs its own `grok` executable with no
// `agent stdio`/ACP support at all, while the real Grok Build binary lives at
// `~/.grok/bin/grok.exe`). Operators on such a machine MUST set
// FLOWPILOT_GROK_BIN to the real binary's absolute path.
var grokBinaryName = func() string {
	if b := strings.TrimSpace(os.Getenv("FLOWPILOT_GROK_BIN")); b != "" {
		return b
	}
	return "grok"
}

// grokInitTimeout bounds the initialize handshake so a wedged build can't hang
// the ensure.
var grokInitTimeout = 30 * time.Second

// ---- async dispatcher -------------------------------------------------------

type grokResponse struct {
	result map[string]any
	err    error
}

// grokInboundRequest is a server->client request (session/request_permission is
// the only one Task-206 routes generically; Task-208 adds the decision policy).
type grokInboundRequest struct {
	ID     any
	Method string
	Params map[string]any
}

// grokNotification is a session/update (or _x.ai/*) notification routed to the
// owning turn by sessionId.
type grokNotification struct {
	Method string
	Params map[string]any
}

type grokDispatcher struct {
	w       io.Writer
	writeMu sync.Mutex

	mu          sync.Mutex
	nextID      int64
	waiters     map[int64]chan grokResponse
	sessionSubs map[string]chan grokNotification
	closed      bool
	closeErr    error
	done        chan struct{}

	// inbound handles server->client requests on its own goroutine so a blocking
	// handler (an approval waiting on the user, Task-208) never stalls the read
	// loop.
	inbound func(grokInboundRequest)
}

func newGrokDispatcher(w io.Writer, inbound func(grokInboundRequest)) *grokDispatcher {
	return &grokDispatcher{
		w:           w,
		waiters:     map[int64]chan grokResponse{},
		sessionSubs: map[string]chan grokNotification{},
		done:        make(chan struct{}),
		inbound:     inbound,
	}
}

func (d *grokDispatcher) setInbound(f func(grokInboundRequest)) {
	d.inbound = f
}

func (d *grokDispatcher) start(r io.Reader) {
	go d.readLoop(r)
}

func (d *grokDispatcher) readLoop(r io.Reader) {
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
	d.fail(fmt.Errorf("grok agent stdio stream closed: %w", err))
}

func (d *grokDispatcher) dispatch(msg map[string]any) {
	method, hasMethod := msg["method"].(string)
	idValue, hasID := msg["id"]

	switch {
	case hasMethod && hasID:
		// inbound server->client request (session/request_permission)
		params, _ := msg["params"].(map[string]any)
		if d.inbound != nil {
			go d.inbound(grokInboundRequest{ID: idValue, Method: method, Params: params})
		}
	case hasMethod && !hasID:
		// notification -> route by sessionId to the owning turn
		params, _ := msg["params"].(map[string]any)
		sessionID := grokSessionIDFromParams(params)
		d.mu.Lock()
		ch := d.sessionSubs[sessionID]
		d.mu.Unlock()
		if ch != nil {
			select {
			case ch <- grokNotification{Method: method, Params: params}:
			default: // bounded buffer full — consumer reconnects/recovers, never block the loop
			}
		}
	case !hasMethod && hasID:
		// response -> deliver to the waiter
		id := jsonRPCIDToInt(idValue)
		d.mu.Lock()
		waiter := d.waiters[id]
		delete(d.waiters, id)
		d.mu.Unlock()
		if waiter != nil {
			if errObj, ok := msg["error"]; ok {
				waiter <- grokResponse{err: fmt.Errorf("%s", jsonRpcErrorMessage(map[string]any{"error": errObj}))}
			} else {
				result, _ := msg["result"].(map[string]any)
				waiter <- grokResponse{result: result}
			}
		}
	}
}

// call sends a client->server request and waits for its response, the caller's
// ctx, or dispatcher death. For `session/prompt` this call blocks for the
// entire prompt turn (live-verified) — callers pump the session's notification
// channel concurrently on another goroutine, exactly like the Codex adapter.
func (d *grokDispatcher) call(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	d.mu.Lock()
	if d.closed {
		err := d.closeErr
		d.mu.Unlock()
		return nil, err
	}
	d.nextID++
	id := d.nextID
	ch := make(chan grokResponse, 1)
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

func (d *grokDispatcher) notify(method string, params map[string]any) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (d *grokDispatcher) reply(id any, result map[string]any) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (d *grokDispatcher) replyError(id any, message string) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32000, "message": message}})
}

func (d *grokDispatcher) write(msg map[string]any) error {
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

// registerSession subscribes to notifications for a session (bounded buffer).
func (d *grokDispatcher) registerSession(sessionID string) (chan grokNotification, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, d.closeErr
	}
	ch := make(chan grokNotification, 256)
	d.sessionSubs[sessionID] = ch
	return ch, nil
}

func (d *grokDispatcher) unregisterSession(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.sessionSubs, sessionID) // closed only by fail(); normal end just detaches
}

// fail marks the dispatcher dead and drains every pending waiter + session
// channel with the error, so a process death never leaves a turn blocked.
func (d *grokDispatcher) fail(err error) {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	d.closeErr = err
	close(d.done)
	waiters := d.waiters
	subs := d.sessionSubs
	d.waiters = map[int64]chan grokResponse{}
	d.sessionSubs = map[string]chan grokNotification{}
	d.mu.Unlock()

	for _, ch := range waiters {
		select {
		case ch <- grokResponse{err: err}:
		default:
		}
	}
	for _, ch := range subs {
		close(ch) // signals the turn pump that the stream died
	}
}

func (d *grokDispatcher) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

// grokSessionIDFromParams extracts the owning session id from a notification's
// params. Live-verified key is "sessionId" on every session/update AND every
// _x.ai/session_notification / _x.ai/mcp/* frame observed.
func grokSessionIDFromParams(params map[string]any) string {
	if params == nil {
		return ""
	}
	if id, ok := params["sessionId"].(string); ok {
		return id
	}
	return ""
}

// ---- log redaction (Task-206 T-6 / DOD-6) ----------------------------------
//
// Live-verified during CP-46 authoring: launching `grok agent stdio` inside a
// real project directory caused Grok to auto-discover and echo plaintext
// credentials (a Google Drive OAuth refresh token + client secret) via
// `_x.ai/mcp/servers_updated`. This is a launch blocker (CP-46 R-1) — any debug
// log of a raw ACP frame MUST go through this redactor first.

// grokCredentialShapedKeys lists JSON keys whose values are redacted wholesale
// before a frame is ever logged — deliberately broad (headers/env/tokens/
// secrets/credentials/auth) since Grok's own extension surface can carry
// arbitrary ambient MCP server configs (mcpServers[].headers/env, auth files).
var grokCredentialShapedKeys = map[string]bool{
	"headers": true, "env": true, "token": true, "tokens": true,
	"secret": true, "secrets": true, "credential": true, "credentials": true,
	"authorization": true, "apikey": true, "api_key": true,
	"access_token": true, "refresh_token": true, "client_secret": true,
	"clientsecret": true, "auth": true,
}

// redactGrokValue walks a decoded JSON value and replaces any credential-shaped
// key's value with "[redacted]", recursing into maps/slices.
func redactGrokValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if grokCredentialShapedKeys[strings.ToLower(k)] {
				out[k] = "[redacted]"
				continue
			}
			out[k] = redactGrokValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactGrokValue(val)
		}
		return out
	default:
		return v
	}
}

// redactGrokFrameForLog parses a raw ACP line and returns a redacted JSON
// string safe to log. Malformed input is redacted to a fixed placeholder
// rather than logged raw.
func redactGrokFrameForLog(rawLine string) string {
	var msg map[string]any
	if err := json.Unmarshal([]byte(rawLine), &msg); err != nil {
		return "[unparsable grok acp frame omitted]"
	}
	redacted := redactGrokValue(msg)
	b, err := json.Marshal(redacted)
	if err != nil {
		return "[unloggable grok acp frame omitted]"
	}
	return string(b)
}

// logGrokFrameDebug is the ONLY sanctioned place to log a raw Grok ACP frame;
// every call site must route through redactGrokFrameForLog first.
func logGrokFrameDebug(direction, rawLine string) {
	log.Printf("[grok-acp] %s %s", direction, redactGrokFrameForLog(rawLine))
}

// ---- process lifecycle -------------------------------------------------------

type grokProcessHandle struct {
	scopeKey   string
	dispatcher *grokDispatcher
	adapter    *grokAdapter
	initResult map[string]any
	kill       func()
}

func (h *grokProcessHandle) close() {
	if h == nil {
		return
	}
	if h.dispatcher != nil {
		h.dispatcher.fail(fmt.Errorf("grok agent process torn down"))
	}
	if h.kill != nil {
		h.kill()
	}
}

// ensureGrokProcess returns the shared grok agent stdio process handle for a
// scope, spawning a fresh process (and tearing down any handle bound to a
// different scope — the account-switch recreate) when needed. Reuses a live
// handle for the same scope.
func (r *Runner) ensureGrokProcess(ctx context.Context, scopeKey, cwd string, extraEnv map[string]string) (*grokProcessHandle, error) {
	r.grokProcessMu.Lock()
	defer r.grokProcessMu.Unlock()

	if r.grokProcess != nil && r.grokProcess.scopeKey == scopeKey && !r.grokProcess.dispatcher.isClosed() {
		return r.grokProcess, nil
	}
	if r.grokProcess != nil {
		r.grokProcess.close()
		r.grokProcess = nil
	}

	if grokHome := strings.TrimSpace(extraEnv["GROK_HOME"]); grokHome != "" {
		if mode, bypasses := grokConfigPermissionModeBypassesGating(grokHome); bypasses {
			log.Printf("[grok-acp] WARNING: %s/config.toml has [ui] permission_mode=%q — Grok will not send "+
				"session/request_permission at all for this account, so YOLO=false deny-by-default is NOT "+
				"enforced end-to-end (live-verified during Task-213 re-verification). FlowPilot does not "+
				"rewrite this file (mirrors the Claude ensureClaudeConfigSettings 'never clobber' precedent) — "+
				"the account owner must set permission_mode to \"default\" (or run `/always-approve off` in the "+
				"grok TUI) for gating to actually take effect.", grokHome, mode)
		}
	}

	cmd := commandContextFn(ctx, grokBinaryName(), "agent", "stdio")
	cmd.Env = grokProcessEnv(extraEnv)
	cmd.Dir = cwd

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("grok agent stdio stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("grok agent stdio stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("grok agent stdio start: %w", err)
	}
	kill := func() { _ = cmd.Process.Kill() }

	dispatcher := newGrokDispatcher(stdin, nil)
	dispatcher.start(stdout)

	initCtx, cancel := context.WithTimeout(ctx, grokInitTimeout)
	defer cancel()
	initResult, err := dispatcher.call(initCtx, "initialize", grokACPInitializeParams())
	if err != nil {
		dispatcher.fail(err)
		kill()
		return nil, fmt.Errorf("grok initialize: %w", err)
	}

	adapter := newGrokAdapter(dispatcher, cwd)
	adapter.grokHome = strings.TrimSpace(extraEnv["GROK_HOME"])
	adapter.initResult = initResult

	h := &grokProcessHandle{scopeKey: scopeKey, dispatcher: dispatcher, adapter: adapter, initResult: initResult, kill: kill}
	r.grokProcess = h
	return h, nil
}

// grokProcessEnv builds the launch environment for `grok agent stdio`.
// Task-206 T-4 (security-critical, CP-46 R-1): the two GROK_*_MCPS_ENABLED
// flags disable ambient compat-scanning of Claude/Cursor config files, and
// GROK_HOME isolates the account. Live-verified caveat (Task-206 authoring):
// these two flags do NOT suppress Grok's own marketplace-plugin-provided MCP
// servers (observed: an "atlassian" server auto-probed from the xAI Official
// marketplace's auto-installed plugin even with mcpServers:[] and both flags
// set) — that ambient source is a separate, still-open risk tracked under
// CP-46 R-1/Task-209's MCP-visibility validation, not fully closed by this env.
func grokProcessEnv(extraEnv map[string]string) []string {
	env := os.Environ()
	env = append(env,
		"GROK_CLAUDE_MCPS_ENABLED=false",
		"GROK_CURSOR_MCPS_ENABLED=false",
	)
	for key, value := range extraEnv {
		if strings.TrimSpace(key) == "" {
			continue
		}
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	return env
}

// grokConfigPermissionModeBypassesGating inspects <grokHome>/config.toml for
// a [ui] permission_mode value that makes Grok stop sending
// session/request_permission entirely. Live-verified during Task-213
// re-verification against a real, previously-connected account: with
// permission_mode="always-approve" a write tool call executed with ZERO
// permission-channel round-trip (Task-208's deny-by-default is unreachable —
// there is nothing to deny); with permission_mode="default" the same write
// correctly triggered session/request_permission and a runner deny blocked
// it. This is diagnostic-only — FlowPilot never rewrites the file, mirroring
// ensureClaudeConfigSettings's "never clobber an existing settings.json"
// precedent for the identical Claude-side risk (BUG-069/CA-079 class).
//
// Deliberately conservative: only the one value actually observed to bypass
// gating is treated as unsafe. Grok's own `/always-approve` slash command
// (seen in initialize's availableCommands: "Toggle always-approve mode (skip
// all permission prompts)") is the mechanism that writes this value, so it is
// trusted as the complete bypass vocabulary until another value is proven to
// behave the same way.
func grokConfigPermissionModeBypassesGating(grokHome string) (mode string, bypasses bool) {
	raw, err := os.ReadFile(filepath.Join(grokHome, "config.toml"))
	if err != nil {
		return "", false
	}
	inUISection := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inUISection = trimmed == "[ui]"
			continue
		}
		if !inUISection || !strings.HasPrefix(trimmed, "permission_mode") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(key) != "permission_mode" {
			continue
		}
		mode = strings.Trim(strings.TrimSpace(value), `"`)
		return mode, strings.EqualFold(mode, "always-approve")
	}
	return "", false
}
