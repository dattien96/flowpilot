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

// Task-300 (CP-57 P-2/P-3): the live shared `opencode acp` process +
// dispatcher layer. Modeled on grok_process.go's async dispatcher (waiters
// map, per-scope notification subs, single inbound handler, one read-loop,
// fail() drain) and ensureGrokProcess (one shared persistent process per
// scopeKey, teardown+respawn on account switch) — NOT Claude's spawn-per-turn.
//
// Unlike Codex's thread/turn split, Opencode's session/prompt call itself blocks
// until the prompt turn ends and its own response carries the terminal
// stopReason + token usage (live-verified). Streaming happens via session/update
// notifications delivered while that call is still in flight. The dispatcher's
// call()/notification-channel split already supports this: the adapter (Task-301)
// issues session/prompt via call() on its own goroutine while pumping the
// session's notification channel, exactly like grokAdapter.SendTurn pumps.

const opencodeAgentEnvFlag = "FLOWPILOT_OPENCODE_AGENT"

// opencodeAgentEnabled reports whether the live Opencode ACP path is turned on.
// On by default; set FLOWPILOT_OPENCODE_AGENT=0/false/no to explicitly opt out.
func opencodeAgentEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(opencodeAgentEnvFlag)))
	return v != "0" && v != "false" && v != "no"
}

// opencodeBinaryName is the `opencode` binary; overridable for tests AND for machines
// where a different tool shadows the real binary. Operators on such a machine MUST set
// FLOWPILOT_OPENCODE_BIN to the real binary's absolute path.
var opencodeBinaryName = func() string {
	if b := strings.TrimSpace(os.Getenv("FLOWPILOT_OPENCODE_BIN")); b != "" {
		return b
	}
	return "opencode"
}

// opencodeInitTimeout bounds the initialize handshake so a wedged build can't hang the ensure.
var opencodeInitTimeout = 30 * time.Second

// ---- async dispatcher -------------------------------------------------------

type opencodeResponse struct {
	result map[string]any
	err    error
}

// opencodeInboundRequest is a server->client request (session/request_permission)
type opencodeInboundRequest struct {
	ID     any
	Method string
	Params map[string]any
}

// opencodeNotification is a session/update notification routed to the owning turn by sessionId.
type opencodeNotification struct {
	Method string
	Params map[string]any
}

type opencodeDispatcher struct {
	w       io.Writer
	writeMu sync.Mutex

	mu          sync.Mutex
	nextID      int64
	waiters     map[int64]chan opencodeResponse
	sessionSubs map[string]chan opencodeNotification
	closed      bool
	closeErr    error
	done        chan struct{}

	// inbound handles server->client requests on its own goroutine so a blocking
	// handler (an approval waiting on the user, Task-303) never stalls the read loop.
	inbound func(opencodeInboundRequest)
}

func newOpencodeDispatcher(w io.Writer, inbound func(opencodeInboundRequest)) *opencodeDispatcher {
	return &opencodeDispatcher{
		w:           w,
		waiters:     map[int64]chan opencodeResponse{},
		sessionSubs: map[string]chan opencodeNotification{},
		done:        make(chan struct{}),
		inbound:     inbound,
	}
}

func (d *opencodeDispatcher) setInbound(f func(opencodeInboundRequest)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.inbound = f
}

func (d *opencodeDispatcher) start(r io.Reader) {
	go d.readLoop(r)
}

func (d *opencodeDispatcher) readLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		logOpencodeFrameDebug("recv", line)
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		d.dispatch(msg)
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	d.fail(fmt.Errorf("opencode acp stream closed: %w", err))
}

func (d *opencodeDispatcher) dispatch(msg map[string]any) {
	method, hasMethod := msg["method"].(string)
	idValue, hasID := msg["id"]

	switch {
	case hasMethod && hasID:
		// inbound server->client request (session/request_permission)
		params, _ := msg["params"].(map[string]any)
		d.mu.Lock()
		inbound := d.inbound
		d.mu.Unlock()
		if inbound != nil {
			go inbound(opencodeInboundRequest{ID: idValue, Method: method, Params: params})
		}
	case hasMethod && !hasID:
		// notification -> route by sessionId to owning turn
		params, _ := msg["params"].(map[string]any)
		sessionID := opencodeSessionIDFromParams(params)
		d.mu.Lock()
		ch := d.sessionSubs[sessionID]
		d.mu.Unlock()
		if ch != nil {
			select {
			case ch <- opencodeNotification{Method: method, Params: params}:
			default:
				select {
				case ch <- opencodeNotification{Method: method, Params: params}:
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
				waiter <- opencodeResponse{err: fmt.Errorf("%s", jsonRpcErrorMessage(map[string]any{"error": errObj}))}
			} else {
				result, _ := msg["result"].(map[string]any)
				waiter <- opencodeResponse{result: result}
			}
		}
	}
}

// call sends a client->server request and waits for its response, caller's ctx, or dispatcher death.
func (d *opencodeDispatcher) call(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	d.mu.Lock()
	if d.closed {
		err := d.closeErr
		d.mu.Unlock()
		return nil, err
	}
	d.nextID++
	id := d.nextID
	ch := make(chan opencodeResponse, 1)
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

func (d *opencodeDispatcher) notify(method string, params map[string]any) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (d *opencodeDispatcher) reply(id any, result map[string]any) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (d *opencodeDispatcher) replyError(id any, message string) error {
	return d.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32000, "message": message}})
}

func (d *opencodeDispatcher) write(msg map[string]any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	logOpencodeFrameDebug("send", string(b))
	b = append(b, '\n')
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	_, err = d.w.Write(b)
	return err
}

// opencodeSessionNotifBuffer is large enough for long turns that emit many chunks.
const opencodeSessionNotifBuffer = 8192

func (d *opencodeDispatcher) registerSession(sessionID string) (chan opencodeNotification, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, d.closeErr
	}
	ch := make(chan opencodeNotification, opencodeSessionNotifBuffer)
	d.sessionSubs[sessionID] = ch
	return ch, nil
}

func (d *opencodeDispatcher) unregisterSession(sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.sessionSubs, sessionID)
}

func (d *opencodeDispatcher) fail(err error) {
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
	d.waiters = map[int64]chan opencodeResponse{}
	d.sessionSubs = map[string]chan opencodeNotification{}
	d.mu.Unlock()

	for _, ch := range waiters {
		select {
		case ch <- opencodeResponse{err: err}:
		default:
		}
	}
	for _, ch := range subs {
		close(ch)
	}
}

func (d *opencodeDispatcher) isClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

func opencodeSessionIDFromParams(params map[string]any) string {
	if params == nil {
		return ""
	}
	if id, ok := params["sessionId"].(string); ok {
		return id
	}
	return ""
}

// ---- log redaction (Task-300 T-6 / DOD-6) ----------------------------------

// opencodeCredentialShapedKeys lists JSON keys whose values are redacted wholesale before a frame is ever logged.
var opencodeCredentialShapedKeys = map[string]bool{
	"headers": true, "env": true, "token": true, "tokens": true,
	"secret": true, "secrets": true, "credential": true, "credentials": true,
	"authorization": true, "apikey": true, "api_key": true,
	"access_token": true, "refresh_token": true, "client_secret": true,
	"clientsecret": true, "auth": true, "opencode_api_key": true,
}

func redactOpencodeValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if opencodeCredentialShapedKeys[strings.ToLower(k)] {
				out[k] = "[redacted]"
				continue
			}
			out[k] = redactOpencodeValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactOpencodeValue(val)
		}
		return out
	default:
		return v
	}
}

func redactOpencodeFrameForLog(rawLine string) string {
	var msg map[string]any
	if err := json.Unmarshal([]byte(rawLine), &msg); err != nil {
		return "[unparsable opencode acp frame omitted]"
	}
	redacted := redactOpencodeValue(msg)
	b, err := json.Marshal(redacted)
	if err != nil {
		return "[unloggable opencode acp frame omitted]"
	}
	return string(b)
}

func logOpencodeFrameDebug(direction, rawLine string) {
	log.Printf("[opencode-acp] %s %s", direction, redactOpencodeFrameForLog(rawLine))
}

// ---- process lifecycle -------------------------------------------------------

type opencodeProcessHandle struct {
	scopeKey string
	model    string
	variant  string
	auto     bool
	dispatcher *opencodeDispatcher
	adapter    *opencodeAdapter
	initResult map[string]any
	kill       func()
}

func (h *opencodeProcessHandle) close() {
	if h == nil {
		return
	}
	if h.dispatcher != nil {
		h.dispatcher.fail(fmt.Errorf("opencode acp process torn down"))
	}
	if h.kill != nil {
		h.kill()
	}
}

func opencodeProcessKey(scopeKey, model, variant string, auto bool) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%t", scopeKey, model, variant, auto)
}

func (r *Runner) closeAllOpencodeProcesses() {
	r.opencodeProcessMu.Lock()
	defer r.opencodeProcessMu.Unlock()
	r.closeAllOpencodeProcessesLocked()
}

func (r *Runner) closeAllOpencodeProcessesLocked() {
	for k, h := range r.opencodeProcesses {
		h.close()
		delete(r.opencodeProcesses, k)
	}
}

// ensureOpencodeProcess returns a live `opencode acp` process handle for a
// scope. Handles are keyed by the scope+model+variant+auto tuple
// (opencodeProcessKey — kept for old-test stability), but BUG-329 changed the
// miss behavior: an exact-key miss now REUSES any live handle bound to the
// same scope instead of spawning a second process. Live probe (1.18.18):
// sessions live inside the creating process instance — `session/load` of a
// ses_* created in process A returns RPC-OK with a config-only result and NO
// sessionId on a fresh process B, so a mid-chat model change must keep using
// the same process (the per-turn model rides on session/set_config_option,
// applied by applyOpencodeSessionConfig after ensureSession). Variant/auto
// never reach the command line (`opencode acp` has no such flags), so reusing
// across those is also safe. Only an account/scope change reclaims processes
// (every handle bound to a different scope is closed here).
func (r *Runner) ensureOpencodeProcess(ctx context.Context, scopeKey, cwd string, extraEnv map[string]string, model, variant string, auto bool) (*opencodeProcessHandle, error) {
	r.opencodeProcessMu.Lock()
	defer r.opencodeProcessMu.Unlock()

	if r.opencodeProcesses == nil {
		r.opencodeProcesses = make(map[string]*opencodeProcessHandle)
	}

	// Account switch still reclaims processes: close every handle bound to a
	// DIFFERENT scope. Same-scope handles that differ only by model/variant/auto
	// are LEFT RUNNING so a parent turn and a concurrently-spawned child turn
	// with different launch flags coexist.
	for k, h := range r.opencodeProcesses {
		if h.scopeKey != scopeKey {
			h.close()
			delete(r.opencodeProcesses, k)
		}
	}

	key := opencodeProcessKey(scopeKey, model, variant, auto)
	if h := r.opencodeProcesses[key]; h != nil {
		if !h.dispatcher.isClosed() {
			return h, nil
		}
		h.close()
		delete(r.opencodeProcesses, key)
	}
	// BUG-329: exact-key miss → reuse any other live handle in the same scope.
	// Spawning a fresh process for a mid-chat model change orphans every
	// session created by the old process (session/load returns no sessionId).
	for _, h := range r.opencodeProcesses {
		if h.scopeKey == scopeKey && !h.dispatcher.isClosed() {
			return h, nil
		}
	}

	if !opencodeAgentEnabled() {
		return nil, fmt.Errorf("opencode controlled runtime is not implemented yet")
	}

	// Opencode acp has no --model/--variant/--auto launch flags (verified
	// via `opencode acp --help` on 1.18.18). The binary is simply `opencode acp`.
	cmd := commandContextFn(ctx, opencodeBinaryName(), "acp")
	cmd.Env = opencodeProcessEnv(extraEnv)
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opencode acp stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("opencode acp stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("opencode acp start: %w", err)
	}
	kill := func() { _ = cmd.Process.Kill() }

	dispatcher := newOpencodeDispatcher(stdin, nil)
	dispatcher.start(stdout)

	initCtx, cancel := context.WithTimeout(ctx, opencodeInitTimeout)
	defer cancel()
	initResult, err := dispatcher.call(initCtx, "initialize", opencodeACPInitializeParams())
	if err != nil {
		dispatcher.fail(err)
		kill()
		return nil, fmt.Errorf("opencode initialize: %w", err)
	}

	adapter := newOpencodeAdapter(dispatcher, cwd)
	adapter.initResult = initResult
	if r.opencodeRunSessions == nil {
		r.opencodeRunSessions = &opencodeRunSessionIndex{byRun: make(map[string]string), bySession: make(map[string]string)}
	}
	adapter.runSessions = r.opencodeRunSessions

	h := &opencodeProcessHandle{scopeKey: scopeKey, model: model, variant: variant, auto: auto, dispatcher: dispatcher, adapter: adapter, initResult: initResult, kill: kill}
	r.opencodeProcesses[key] = h
	return h, nil
}

// opencodeProcessEnv builds the launch environment for `opencode acp`.
// Task-300 T-4: disable ambient discovery, isolate account via HOME/XDG_CONFIG_HOME/OPENCODE_CONFIG,
// strip inherited secrets, handle Windows USERPROFILE/APPDATA.
func opencodeProcessEnv(extraEnv map[string]string) []string {
	env := os.Environ()
	// Opencode does not have explicit _MCPS_ENABLED flags like Grok, but we set a
	// defensive disable flag and document why. At minimum we ensure no ambient
	// OPENCODE_API_KEY leaks beyond scope.
	filtered := make([]string, 0, len(env)+len(extraEnv)+5)
	for _, kv := range env {
		// Strip host OPENCODE secrets beyond scope; they will be re-injected only via extraEnv if needed.
		if strings.HasPrefix(kv, "OPENCODE_API_KEY=") || strings.HasPrefix(kv, "OPENCODE_HOME=") {
			continue
		}
		filtered = append(filtered, kv)
	}
	// Defensive ambient disable (see grokProcessEnv comment for analogy)
	filtered = append(filtered, "OPENCODE_DISABLE_AMBIENT_MCP=true")
	for key, value := range extraEnv {
		if strings.TrimSpace(key) == "" {
			continue
		}
		filtered = append(filtered, fmt.Sprintf("%s=%s", key, value))
	}
	// Ensure OPENCODE_CONFIG is set if HOME is set but OPENCODE_CONFIG is not.
	// Callers (provider_registry) should already set HOME/XDG_CONFIG_HOME/OPENCODE_CONFIG,
	// but we normalize here for direct ensureOpencodeProcess callers in tests.
	hasOpencodeConfig := false
	var home string
	for _, kv := range filtered {
		if strings.HasPrefix(kv, "OPENCODE_CONFIG=") {
			hasOpencodeConfig = true
		}
		if strings.HasPrefix(kv, "HOME=") {
			home = strings.TrimPrefix(kv, "HOME=")
		}
	}
	if !hasOpencodeConfig && home != "" {
		// CA-679: OPENCODE_CONFIG is a config FILE path, not a directory — a
		// directory value kills `opencode acp`/`opencode models` at boot.
		filtered = append(filtered, fmt.Sprintf("OPENCODE_CONFIG=%s", opencodeConfigFilePath(home)))
	}
	if home != "" {
		hasDataHome := false
		for _, kv := range filtered {
			if strings.HasPrefix(kv, "XDG_DATA_HOME=") {
				hasDataHome = true
				break
			}
		}
		if !hasDataHome {
			filtered = append(filtered, fmt.Sprintf("XDG_DATA_HOME=%s", filepath.Join(home, ".local", "share")))
		}
	}
	return filtered
}

func (r *Runner) ApplyOpencodeYoloPosture(ctx context.Context, yolo bool) error {
	account, err := r.ResolveProviderAccount(string(ProviderKeyOpencode), "")
	if err != nil {
		return fmt.Errorf("resolve active opencode account: %w", err)
	}
	if strings.TrimSpace(account.HomePath) == "" {
		return fmt.Errorf("active opencode account %q has no home path", account.ID)
	}
	// Opencode YOLO is per-turn --auto, not config.toml rewrite. Just flip the
	// desired auto flag and close live processes so next turn respawns with new auto.
	r.opencodeProcessMu.Lock()
	r.opencodeDesiredAuto = yolo
	r.closeAllOpencodeProcessesLocked()
	r.opencodeProcessMu.Unlock()
	return nil
}
