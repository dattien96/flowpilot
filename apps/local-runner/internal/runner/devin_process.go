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

// Task-400 (CP-70 T-3/T-4/T-5): the live shared `devin acp` process +
// dispatcher layer. Modeled on opencode_process.go (waiters map, per-scope
// notification subs, single inbound handler, one read-loop, fail() drain) —
// NOT Claude's spawn-per-turn.
//
// Devin-specific contract (live-verified 3000.10.31):
//   - Boot is initialize -> authenticate{methodId:"devin-browser"} -> ready.
//     The ACP agent refuses the on-disk CLI credentials and stalls session/new
//     until authenticate succeeds, so the handshake is part of process setup.
//   - Sessions persist in the account's SQLite store
//     ($XDG_DATA_HOME/devin/cli/sessions.db) and sessionIds are slug-style
//     names — session/load resumes them in ANY devin acp process sharing the
//     account home, unlike opencode's process-local sessions.
//   - The agent emits `_cognition.ai/*` extension notifications freely; the
//     dispatcher tolerates every unknown method.

const devinAgentEnvFlag = "FLOWPILOT_DEVIN_AGENT"

// devinAgentEnabled reports whether the live Devin ACP path is turned on.
// On by default; set FLOWPILOT_DEVIN_AGENT=0/false/no to opt out.
func devinAgentEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(devinAgentEnvFlag)))
	return v != "0" && v != "false" && v != "no"
}

// devinBinaryName resolves the `devin` binary; overridable for tests and for
// machines where a different tool shadows the real binary
// (FLOWPILOT_DEVIN_BIN).
var devinBinaryName = func() string {
	if b := strings.TrimSpace(os.Getenv("FLOWPILOT_DEVIN_BIN")); b != "" {
		return b
	}
	return "devin"
}

// devinInitTimeout bounds the initialize handshake.
var devinInitTimeout = 30 * time.Second

// devinAuthTimeout bounds the authenticate handshake. The devin-browser PKCE
// flow finishes in ~3s when the machine's browser profile is already signed
// in, but may need a full browser round-trip otherwise — generous but bounded.
var devinAuthTimeout = 90 * time.Second

// ---- async dispatcher -------------------------------------------------------

type devinResponse struct {
	result map[string]any
	err    error
}

// devinInboundRequest is a server->client request (session/request_permission,
// fs/read_text_file, fs/write_text_file, terminal/*, ...).
type devinInboundRequest struct {
	ID     any
	Method string
	Params map[string]any
}

// devinNotification is a notification routed to the owning turn by sessionId.
// Notifications without a sessionId (e.g. _cognition.ai/output channel lines)
// are broadcast to every subscribed session — they carry process-level
// diagnostics a turn may legitimately surface.
type devinNotification struct {
	Method string
	Params map[string]any
}

type devinDispatcher struct {
	w       io.Writer
	writeMu sync.Mutex

	mu          sync.Mutex
	nextID      int64
	waiters     map[int64]chan devinResponse
	sessionSubs map[string]chan devinNotification
	closed      bool
	closeErr    error
	done        chan struct{}

	// inbound handles server->client requests on its own goroutine so a
	// blocking handler (an approval waiting on the user) never stalls the
	// read loop.
	inbound func(devinInboundRequest)
}

func newDevinDispatcher(w io.Writer, inbound func(devinInboundRequest)) *devinDispatcher {
	return &devinDispatcher{
		w:           w,
		waiters:     map[int64]chan devinResponse{},
		sessionSubs: map[string]chan devinNotification{},
		done:        make(chan struct{}),
		inbound:     inbound,
	}
}

func (d *devinDispatcher) setInbound(f func(devinInboundRequest)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.inbound = f
}

func (d *devinDispatcher) start(r io.Reader) {
	go d.readLoop(r)
}

func (d *devinDispatcher) readLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		logDevinFrameDebug("recv", line)
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			// Malformed frame: skip, never fatal (Task-400 DOD-10).
			continue
		}
		d.dispatch(msg)
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	d.fail(fmt.Errorf("devin acp stream closed: %w", err))
}

func (d *devinDispatcher) dispatch(msg map[string]any) {
	method, hasMethod := msg["method"].(string)
	idValue, hasID := msg["id"]

	switch {
	case hasMethod && hasID:
		// inbound server->client request (session/request_permission,
		// fs/read_text_file, fs/write_text_file, terminal/*, ...). The handler
		// owns the reply; unknown methods get a JSON-RPC error reply there.
		params, _ := msg["params"].(map[string]any)
		d.mu.Lock()
		inbound := d.inbound
		d.mu.Unlock()
		if inbound != nil {
			go inbound(devinInboundRequest{ID: idValue, Method: method, Params: params})
		}
	case hasMethod && !hasID:
		// notification -> route by sessionId; sessionless notifications
		// (extension channel output, mcp server lifecycle) broadcast to all
		// live session subs so the owning turn can decide what to surface.
		params, _ := msg["params"].(map[string]any)
		sessionID := devinSessionIDFromParams(params)
		d.mu.Lock()
		var targets []chan devinNotification
		if sessionID != "" {
			if ch := d.sessionSubs[sessionID]; ch != nil {
				targets = append(targets, ch)
			}
		} else {
			for _, ch := range d.sessionSubs {
				targets = append(targets, ch)
			}
		}
		d.mu.Unlock()
		for _, ch := range targets {
			select {
			case ch <- devinNotification{Method: method, Params: params}:
			default:
				select {
				case ch <- devinNotification{Method: method, Params: params}:
				case <-d.done:
				}
			}
		}
	case !hasMethod && hasID:
		// response -> deliver to waiter
		id := jsonRPCIDToInt(idValue)
		d.mu.Lock()
		waiter := d.waiters[id]
		delete(d.waiters, id)
		d.mu.Unlock()
		if waiter != nil {
			if errObj, ok := msg["error"]; ok {
				waiter <- devinResponse{err: fmt.Errorf("%s", jsonRpcErrorMessage(map[string]any{"error": errObj}))}
			} else {
				result, _ := msg["result"].(map[string]any)
				waiter <- devinResponse{result: result}
			}
		}
	}
}

// call sends a client->server request and waits for its response, caller's
// ctx, or dispatcher death.
func (d *devinDispatcher) call(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	d.mu.Lock()
	if d.closed {
		err := d.closeErr
		d.mu.Unlock()
		return nil, err
	}
	d.nextID++
	id := d.nextID
	ch := make(chan devinResponse, 1)
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

func (d *devinDispatcher) notify(method string, params map[string]any) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (d *devinDispatcher) reply(id any, result map[string]any) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (d *devinDispatcher) replyError(id any, message string) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32000, "message": message}})
}

func (d *devinDispatcher) write(msg map[string]any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	logDevinFrameDebug("send", string(b))
	b = append(b, '\n')
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	_, err = d.w.Write(b)
	return err
}

// devinSessionNotifBuffer is large enough for long turns that emit many chunks.
const devinSessionNotifBuffer = 8192

func (d *devinDispatcher) registerSession(sessionID string) (chan devinNotification, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, d.closeErr
	}
	ch := make(chan devinNotification, devinSessionNotifBuffer)
	d.sessionSubs[sessionID] = ch
	return ch, nil
}

func (d *devinDispatcher) unregisterSession(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.sessionSubs, sessionID)
}

func (d *devinDispatcher) fail(err error) {
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
	d.waiters = map[int64]chan devinResponse{}
	d.sessionSubs = map[string]chan devinNotification{}
	d.mu.Unlock()

	for _, ch := range waiters {
		select {
		case ch <- devinResponse{err: err}:
		default:
		}
	}
	for _, ch := range subs {
		close(ch)
	}
}

func (d *devinDispatcher) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

func devinSessionIDFromParams(params map[string]any) string {
	if params == nil {
		return ""
	}
	if id, ok := params["sessionId"].(string); ok {
		return id
	}
	return ""
}

// ---- log redaction (Task-400 DOD-7) -----------------------------------------

// devinCredentialShapedKeys lists JSON keys whose values are redacted
// wholesale before a frame is ever logged. Superset of the opencode list plus
// the fields the Devin auth/MCP paths can carry.
var devinCredentialShapedKeys = map[string]bool{
	"headers": true, "env": true, "token": true, "tokens": true,
	"secret": true, "secrets": true, "credential": true, "credentials": true,
	"authorization": true, "apikey": true, "api_key": true,
	"access_token": true, "refresh_token": true, "client_secret": true,
	"clientsecret": true, "auth": true, "code_verifier": true,
	"code_challenge": true, "devin_api_key": true, "windsurf_api_key": true,
	"sessiontoken": true, "session_token": true,
}

func redactDevinValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if devinCredentialShapedKeys[strings.ToLower(k)] {
				out[k] = "[redacted]"
				continue
			}
			out[k] = redactDevinValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactDevinValue(val)
		}
		return out
	default:
		return v
	}
}

func redactDevinFrameForLog(rawLine string) string {
	var msg map[string]any
	if err := json.Unmarshal([]byte(rawLine), &msg); err != nil {
		return "[unparsable devin acp frame omitted]"
	}
	redacted := redactDevinValue(msg)
	b, err := json.Marshal(redacted)
	if err != nil {
		return "[unloggable devin acp frame omitted]"
	}
	return string(b)
}

func logDevinFrameDebug(direction, rawLine string) {
	log.Printf("[devin-acp] %s %s", direction, redactDevinFrameForLog(rawLine))
}

// ---- process lifecycle -------------------------------------------------------

type devinProcessHandle struct {
	scopeKey string
	// scopeBase is the account-level scope (account.ID / env:$HOME) and
	// scopeSegment isolates run families on the same account ("|child:<id>"
	// for spawned child runs, "probe" for catalog probes, "" for chat turns)
	// — the BUG-334 rule: a child process must never tear down a live parent
	// process whose in-flight MCP tool call would die with it.
	scopeBase    string
	scopeSegment string
	model        string
	permissionMode string
	dispatcher   *devinDispatcher
	adapter      *devinAdapter
	initResult   map[string]any
	kill         func()
}

func (h *devinProcessHandle) close() {
	if h == nil {
		return
	}
	if h.dispatcher != nil {
		h.dispatcher.fail(fmt.Errorf("devin acp process torn down"))
	}
	if h.kill != nil {
		h.kill()
	}
}

func devinProcessKey(scopeKey, model, permissionMode string) string {
	return fmt.Sprintf("%s\x00%s\x00%s", scopeKey, model, permissionMode)
}

func (r *Runner) closeAllDevinProcesses() {
	r.devinProcessMu.Lock()
	defer r.devinProcessMu.Unlock()
	for k, h := range r.devinProcesses {
		h.close()
		delete(r.devinProcesses, k)
	}
}

// ensureDevinProcess returns a live `devin acp` process handle for a scope.
// The key tuple is scope+model+permissionMode for coexistence safety, but a
// miss REUSES any live same-scope handle (sessions are resumable slugs in the
// account's SQLite store, and model/mode are per-session config options — a
// fresh process is never needed for a mid-chat model switch).
func (r *Runner) ensureDevinProcess(ctx context.Context, scopeKey, cwd string, extraEnv map[string]string, model, permissionMode string) (*devinProcessHandle, error) {
	return r.ensureDevinProcessSegmented(ctx, scopeKey, "", cwd, extraEnv, model, permissionMode)
}

// devinSegmentedScope joins an account-level base with a run-family segment.
func devinSegmentedScope(base, segment string) string {
	if strings.TrimSpace(segment) == "" {
		return base
	}
	return base + "|child:" + segment
}

// ensureDevinProcessSegmented is ensureDevinProcess with a BUG-334 run-family
// segment: the account-switch reclaim closes handles whose BASE differs, so a
// child segment spawning its own process never kills a live parent process.
// Within one segment, exact-key misses reuse any live same-segment handle.
func (r *Runner) ensureDevinProcessSegmented(ctx context.Context, scopeBase, scopeSegment, cwd string, extraEnv map[string]string, model, permissionMode string) (*devinProcessHandle, error) {
	scopeKey := devinSegmentedScope(scopeBase, scopeSegment)
	r.devinProcessMu.Lock()
	defer r.devinProcessMu.Unlock()

	if r.devinProcesses == nil {
		r.devinProcesses = make(map[string]*devinProcessHandle)
	}

	for k, h := range r.devinProcesses {
		if devinScopeBaseOf(h) != scopeBase {
			h.close()
			delete(r.devinProcesses, k)
		}
	}

	key := devinProcessKey(scopeKey, model, permissionMode)
	if h := r.devinProcesses[key]; h != nil {
		if !h.dispatcher.isClosed() {
			return h, nil
		}
		h.close()
		delete(r.devinProcesses, key)
	}
	for _, h := range r.devinProcesses {
		if devinScopeBaseOf(h) == scopeBase && h.scopeSegment == scopeSegment && !h.dispatcher.isClosed() {
			return h, nil
		}
	}

	if !devinAgentEnabled() {
		return nil, fmt.Errorf("devin controlled runtime is not implemented yet")
	}

	// `devin acp` takes no launch flags for model/mode — both are per-session
	// config options applied via session/set_config_option (live-verified).
	cmd := commandContextFn(ctx, devinSpawnBinary(), "acp")
	cmd.Env = devinProcessEnv(extraEnv)
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("devin acp stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("devin acp stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("devin acp start: %w", err)
	}
	kill := func() { _ = cmd.Process.Kill() }

	dispatcher := newDevinDispatcher(stdin, nil)
	dispatcher.start(stdout)

	initCtx, cancel := context.WithTimeout(ctx, devinInitTimeout)
	initResult, err := dispatcher.call(initCtx, "initialize", devinACPInitializeParams())
	cancel()
	if err != nil {
		dispatcher.fail(err)
		kill()
		return nil, fmt.Errorf("devin initialize: %w", err)
	}

	// Task-400 T-5 (DOD-9): the ACP agent is the sole credential source for
	// this process — local CLI credentials are ignored and session/new stalls
	// until authenticate succeeds. authenticate rides its own generous timeout
	// (PKCE is ~3s warm, longer cold) and its failure must kill the process so
	// a half-authenticated handle is never cached.
	authCtx, authCancel := context.WithTimeout(ctx, devinAuthTimeout)
	_, authErr := dispatcher.call(authCtx, "authenticate", devinACPAuthenticateParams())
	authCancel()
	if authErr != nil {
		dispatcher.fail(authErr)
		kill()
		return nil, fmt.Errorf("devin authenticate (run `devin auth login` if the browser flow did not complete): %w", authErr)
	}

	adapter := newDevinAdapter(dispatcher, cwd)
	adapter.initResult = initResult
	if r.devinRunSessions == nil {
		r.devinRunSessions = &devinRunSessionIndex{byRun: make(map[string]string), bySession: make(map[string]string)}
	}
	adapter.runSessions = r.devinRunSessions

	h := &devinProcessHandle{scopeKey: scopeKey, scopeBase: scopeBase, scopeSegment: scopeSegment, model: model, permissionMode: permissionMode, dispatcher: dispatcher, adapter: adapter, initResult: initResult, kill: kill}
	r.devinProcesses[key] = h
	return h, nil
}

// devinScopeBaseOf tolerates test handle literals that only set scopeKey.
func devinScopeBaseOf(h *devinProcessHandle) string {
	if h == nil {
		return ""
	}
	if strings.TrimSpace(h.scopeBase) != "" {
		return h.scopeBase
	}
	return h.scopeKey
}

// CloseDevinProcessesForChildRun tears down the BUG-334 isolated process of
// one spawned child run after its terminal event, so short-lived children do
// not leak `devin acp` processes. Chat-scope (segment "") handles are never
// touched.
func (r *Runner) CloseDevinProcessesForChildRun(childRunID string) {
	suffix := "|child:" + strings.TrimSpace(childRunID)
	if suffix == "|child:" {
		return
	}
	r.devinProcessMu.Lock()
	defer r.devinProcessMu.Unlock()
	for k, h := range r.devinProcesses {
		if strings.HasSuffix(h.scopeKey, suffix) {
			h.close()
			delete(r.devinProcesses, k)
		}
	}
}

// devinProcessEnv builds the launch environment for `devin acp`.
// Task-400 T-4: isolate the account via HOME/XDG_CONFIG_HOME/XDG_DATA_HOME —
// Devin reads ~/.config/devin/ (config.json, mcp_config.json) and
// ~/.local/share/devin/ (credentials.toml, cli/sessions.db) through the
// standard XDG resolution. Host secrets are stripped so a managed account
// can never see the ambient WINDSURF_API_KEY/DEVIN_* the shell exported.
// There is no DEVIN_HOME or DEVIN_CONFIG_CONTENT mechanism (verified).
func devinProcessEnv(extraEnv map[string]string) []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env)+len(extraEnv)+4)
	for _, kv := range env {
		// Strip host secrets: every DEVIN_* variable (DEVIN_API_KEY etc.) and
		// the Windsurf key the Devin credential store shares a field name with.
		if strings.HasPrefix(kv, "DEVIN_") || strings.HasPrefix(kv, "WINDSURF_API_KEY=") {
			continue
		}
		filtered = append(filtered, kv)
	}
	for key, value := range extraEnv {
		if strings.TrimSpace(key) == "" {
			continue
		}
		filtered = append(filtered, fmt.Sprintf("%s=%s", key, value))
	}
	var home string
	for _, kv := range filtered {
		if strings.HasPrefix(kv, "HOME=") {
			home = strings.TrimPrefix(kv, "HOME=")
		}
	}
	if home != "" {
		// Normalize the XDG roots Devin actually resolves: config under
		// ~/.config, data (credentials + sessions.db) under ~/.local/share.
		hasConfig, hasData := false, false
		for _, kv := range filtered {
			if strings.HasPrefix(kv, "XDG_CONFIG_HOME=") {
				hasConfig = true
			}
			if strings.HasPrefix(kv, "XDG_DATA_HOME=") {
				hasData = true
			}
		}
		if !hasConfig {
			filtered = append(filtered, fmt.Sprintf("XDG_CONFIG_HOME=%s", filepath.Join(home, ".config")))
		}
		if !hasData {
			filtered = append(filtered, fmt.Sprintf("XDG_DATA_HOME=%s", filepath.Join(home, ".local", "share")))
		}
	}
	return filtered
}
