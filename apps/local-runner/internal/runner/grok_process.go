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

// grokAgentEnabled reports whether the live Grok ACP path is turned on. On by
// default; set FLOWPILOT_GROK_AGENT=0/false/no to explicitly opt back out
// (e.g. test/demo environments without a real grok binary).
func grokAgentEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(grokAgentEnvFlag)))
	return v != "0" && v != "false" && v != "no"
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
		logGrokFrameDebug("recv", line)
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
			default:
				// Buffer full: block briefly rather than drop. Image turns stream
				// hundreds of agent_message_chunk frames (run-96217: 400+); a hard
				// drop left the TUI stuck on thinking… with only tool rows.
				select {
				case ch <- grokNotification{Method: method, Params: params}:
				case <-d.done:
				}
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
	logGrokFrameDebug("send", string(b))
	b = append(b, '\n')
	d.writeMu.Lock()
	defer d.writeMu.Unlock()
	_, err = d.w.Write(b)
	return err
}

// grokSessionNotifBuffer is large enough for long Grok turns that emit
// hundreds of agent_thought_chunk + agent_message_chunk frames before the
// consumer drains them (run-96217 image path-fallback: 400+ message chunks).
const grokSessionNotifBuffer = 8192

// registerSession subscribes to notifications for a session (bounded buffer).
func (d *grokDispatcher) registerSession(sessionID string) (chan grokNotification, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, d.closeErr
	}
	ch := make(chan grokNotification, grokSessionNotifBuffer)
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
	scopeKey string
	// model/reasoningEffort record what this running process was actually
	// launched with (grok agent --model/--reasoning-effort), so ensureGrokProcess
	// can detect a turn-level change and respawn -- see ensureGrokProcess.
	model           string
	reasoningEffort string
	// alwaysApprove records whether this running process was launched with
	// --always-approve (Task-218/YOLO=true). Compared alongside model/
	// reasoningEffort so a YOLO flip also forces a respawn, the same way a
	// model/effort change already does -- otherwise a live process launched
	// under the old posture would keep running with it until something else
	// happened to bump scope/model/effort.
	alwaysApprove bool
	dispatcher    *grokDispatcher
	adapter       *grokAdapter
	initResult    map[string]any
	kill          func()
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

// grokProcessKey is the composite reuse key for a live grok process: two turns
// share one process only when scope, model, reasoning-effort, AND always-approve
// posture all match, because each is a `grok agent` launch-time flag.
func grokProcessKey(scopeKey, model, reasoningEffort string, alwaysApprove bool) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%t", scopeKey, model, reasoningEffort, alwaysApprove)
}

// closeAllGrokProcesses tears down every live grok process and clears the map.
// Acquires grokProcessMu; used on a YOLO posture change and by tests for cleanup.
func (r *Runner) closeAllGrokProcesses() {
	r.grokProcessMu.Lock()
	defer r.grokProcessMu.Unlock()
	r.closeAllGrokProcessesLocked()
}

// closeAllGrokProcessesLocked is closeAllGrokProcesses's body; the caller must
// already hold grokProcessMu.
func (r *Runner) closeAllGrokProcessesLocked() {
	for k, h := range r.grokProcesses {
		h.close()
		delete(r.grokProcesses, k)
	}
}

// ensureGrokProcess returns a grok agent stdio process handle for a
// scope+model+reasoningEffort+alwaysApprove combination, spawning a fresh
// process when none matches. Handles are keyed by that full tuple (grokProcessKey)
// and COEXIST: a live handle is reused only when scope, model, reasoningEffort,
// AND alwaysApprove all match; a mismatch spawns an additional process rather than
// tearing the existing one down. Only an account/scope change reclaims processes
// (every handle bound to a different scope is closed here), so a parent turn and a
// concurrently-spawned child turn on the same account but different launch flags no
// longer kill each other's process.
//
// model/reasoningEffort stay in grokProcessKey because they are also
// `grok agent` launch-time flags (`-m/--model`, `--reasoning-effort`) and
// existing tests require a respawn when they change. Grok CLI 1.0.3+ also
// supports ACP `session/set_model` (live-verified 2026-08-13: same sessionId,
// history kept). The adapter calls that RPC after session/new or session/load
// so a process respawned with the new --model still applies the turn's model
// to the loaded session. Do not put model on session/new (CA-445: cwd +
// mcpServers only). The 0.2.93 fixture in testdata/grok_acp/ predates this
// method; missing-method errors must not fail the turn.
//
// alwaysApprove (Task-218) mirrors the same launch-time-only constraint: it is
// the caller's current desired YOLO posture (Runner.grokDesiredAlwaysApprove),
// applied as --always-approve when true. It is also compared in the reuse
// check below so a YOLO flip forces a respawn exactly like a model/effort
// change does — Grok's ACP protocol has no session-level way to change this
// mid-process either. History still follows the ACP session id: the new
// process must session/load this run's id (Runner.grokRunSessions), then
// session/set_model when ModelName is set. Do not session/new (run-93161).
func (r *Runner) ensureGrokProcess(ctx context.Context, scopeKey, cwd string, extraEnv map[string]string, model, reasoningEffort string, alwaysApprove bool) (*grokProcessHandle, error) {
	r.grokProcessMu.Lock()
	defer r.grokProcessMu.Unlock()

	if r.grokProcesses == nil {
		r.grokProcesses = make(map[string]*grokProcessHandle)
	}

	// Account switch still reclaims processes: close every handle bound to a
	// DIFFERENT scope. Same-scope handles that differ only by model/effort/approve
	// are LEFT RUNNING so a parent turn and a concurrently-spawned child turn with
	// different launch flags coexist instead of tearing each other's process down.
	for k, h := range r.grokProcesses {
		if h.scopeKey != scopeKey {
			h.close()
			delete(r.grokProcesses, k)
		}
	}

	key := grokProcessKey(scopeKey, model, reasoningEffort, alwaysApprove)
	if h := r.grokProcesses[key]; h != nil {
		if !h.dispatcher.isClosed() {
			return h, nil
		}
		// Dead process for this key (crashed or torn down): drop it and respawn.
		h.close()
		delete(r.grokProcesses, key)
	}

	// AUTO-ENFORCE YOLO=false gating (Task-218, SS-08 YOLO-is-SSOT): Grok has no
	// CLI flag for the gated direction (confirmed live, Task-208 DOD-5), so when
	// a process is about to spawn under YOLO=false the ONLY lever is the account's
	// own config.toml. If it currently bypasses the permission channel
	// (permission_mode="always-approve", set by the user's own `/always-approve`
	// TUI toggle), rewrite it to "default" HERE — before `grok` reads it at
	// startup — so gating is enforced on every launch automatically, without
	// waiting for the user to flip the desktop YOLO toggle (which drives
	// ApplyGrokYoloPosture). Deliberately conservative: only rewrites when the
	// value genuinely bypasses, so a config that is already default/absent/other
	// is never touched. No-op-safe (setGrokConfigPermissionMode returns
	// changed=false when the value already matches). YOLO=true never touches the
	// file — --always-approve below covers bypass regardless of the file.
	if grokHome := strings.TrimSpace(extraEnv["GROK_HOME"]); grokHome != "" && !alwaysApprove {
		if mode, bypasses := grokConfigPermissionModeBypassesGating(grokHome); bypasses {
			if changed, werr := setGrokConfigPermissionMode(grokHome, "default"); werr != nil {
				log.Printf("[grok-acp] WARNING: could not auto-enforce YOLO=false gating: rewriting %s/config.toml "+
					"permission_mode (was %q) failed: %v — Grok may still skip session/request_permission this launch.",
					grokHome, mode, werr)
			} else if changed {
				log.Printf("[grok-acp] auto-enforced YOLO=false gating: rewrote %s/config.toml permission_mode %q -> "+
					"\"default\" so Grok sends session/request_permission and the runner deny-by-default takes effect.",
					grokHome, mode)
			}
		}
	}

	args := []string{"agent"}
	if m := strings.TrimSpace(model); m != "" {
		args = append(args, "--model", m)
	}
	if e := strings.TrimSpace(reasoningEffort); e != "" {
		args = append(args, "--reasoning-effort", e)
	}
	if alwaysApprove {
		args = append(args, "--always-approve")
	}
	args = append(args, "stdio")
	cmd := commandContextFn(ctx, grokBinaryName(), args...)
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
	if r.grokRunSessions == nil {
		r.grokRunSessions = &grokRunSessionIndex{byRun: make(map[string]string)}
	}
	adapter.runSessions = r.grokRunSessions

	h := &grokProcessHandle{scopeKey: scopeKey, model: model, reasoningEffort: reasoningEffort, alwaysApprove: alwaysApprove, dispatcher: dispatcher, adapter: adapter, initResult: initResult, kill: kill}
	r.grokProcesses[key] = h
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
// it. This reader itself is diagnostic-only and never writes anything — but
// unlike Claude's settings.json (never touched at all, matching
// ensureClaudeConfigSettings's "never clobber" precedent), Grok's config.toml
// IS deliberately rewritten elsewhere, by setGrokConfigPermissionMode /
// ApplyGrokYoloPosture (Task-218), because there is no `grok agent stdio` flag
// that can force gating back on the way Codex's `-c approval_policy=...` or
// Claude's `--permission-mode` can — this file is the only lever left for
// that direction, so it can no longer be treated as fully hands-off.
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

// setGrokConfigPermissionMode rewrites <grokHome>/config.toml's [ui]
// permission_mode to desiredMode (Task-218). This is the only lever FlowPilot
// has to force Grok's gating back on for an account that has its own
// permission_mode="always-approve" persisted (e.g. via the account owner's own
// `/always-approve` slash command) — there is no `grok agent stdio` flag for
// this direction (confirmed live, Task-208 DOD-5), unlike Codex's
// `-c approval_policy=...` or Claude's `--permission-mode`, which never touch
// a file on disk at all.
//
// Deliberately minimal: this is a line-based patch of exactly the one key,
// mirroring grokConfigPermissionModeBypassesGating's own line-scanning read
// above rather than a full TOML parse/re-marshal — every other line (other
// keys, comments, section ordering, blank lines) is preserved byte-for-byte.
// If the [ui] section exists but the key is missing, the key is inserted
// right after the section header; if the section itself is missing, a new
// [ui] section is appended at the end of the file; if the file doesn't exist
// at all, a minimal one is created. Returns changed=false, nil if the value
// already matches (no write performed) — avoids racing/thrashing a
// concurrently open grok TUI for no reason.
//
// Written atomically (temp file in the same directory + os.Rename), mirroring
// local_file_session_store.go's own sessions.ndjson rewrite.
func setGrokConfigPermissionMode(grokHome, desiredMode string) (changed bool, err error) {
	path := filepath.Join(grokHome, "config.toml")
	raw, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return false, readErr
	}

	lines := strings.Split(string(raw), "\n")
	if len(raw) == 0 {
		lines = nil
	}

	uiSectionLine := -1
	keyLine := -1
	inUISection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inUISection = trimmed == "[ui]"
			if inUISection {
				uiSectionLine = i
			}
			continue
		}
		if !inUISection || !strings.HasPrefix(trimmed, "permission_mode") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(key) != "permission_mode" {
			continue
		}
		keyLine = i
		break
	}

	desiredLine := fmt.Sprintf("permission_mode = %q", desiredMode)

	switch {
	case keyLine >= 0:
		if strings.TrimSpace(lines[keyLine]) == desiredLine {
			return false, nil
		}
		lines[keyLine] = desiredLine
	case uiSectionLine >= 0:
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:uiSectionLine+1]...)
		out = append(out, desiredLine)
		out = append(out, lines[uiSectionLine+1:]...)
		lines = out
	default:
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "[ui]", desiredLine)
	}

	content := strings.Join(lines, "\n")
	if err := os.MkdirAll(grokHome, 0o755); err != nil {
		return false, err
	}
	tmpPath := path + ".tmp"
	tmp, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return false, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return false, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return false, err
	}
	return true, nil
}

// ApplyGrokYoloPosture is the entry point for the desktop's Grok-only YOLO
// toggle (Task-218; POST /provider-accounts/grok-yolo-posture). Unlike
// Claude/Codex, where flipping YOLO only changes a CLI flag passed on the next
// launch, Grok's YOLO=false direction has no such flag — the account's own
// config.toml must be rewritten and the live process respawned for gating to
// actually take effect, which is why this call is synchronous/awaited by the
// UI (with a loading modal) instead of a fire-and-forget local state flip.
//
// This endpoint is the EAGER path: it applies the change immediately (rewrite +
// respawn now) so the modal reflects a real, completed change. Enforcement is
// ALSO automatic on every launch — ensureGrokProcess rewrites an always-approve
// config to "default" whenever it spawns under YOLO=false, so gating holds even
// if the user never touches this toggle. This endpoint just makes it instant.
//
// yolo=true never touches config.toml: --always-approve (passed by
// ensureGrokProcess whenever grokDesiredAlwaysApprove is true) bypasses
// regardless of whatever the file says, so there's nothing to gain from also
// rewriting it, and every unnecessary write is one more chance to race a
// concurrently open grok TUI for no benefit.
//
// Concurrency note: grok processes are now keyed by scope+model+effort+approve
// (ensureGrokProcess), so concurrent same-account runs that differ only by
// model/reasoningEffort each get their own process instead of thrashing one
// shared handle. This eager posture endpoint still closes ALL of them, because a
// YOLO change is account-global; the next turn on each configuration respawns
// under the new posture.
func (r *Runner) ApplyGrokYoloPosture(ctx context.Context, yolo bool) error {
	account, err := r.ResolveProviderAccount(string(ProviderKeyGrok), "")
	if err != nil {
		return fmt.Errorf("resolve active grok account: %w", err)
	}
	if strings.TrimSpace(account.HomePath) == "" {
		return fmt.Errorf("active grok account %q has no home path", account.ID)
	}

	if !yolo {
		if _, err := setGrokConfigPermissionMode(account.HomePath, "default"); err != nil {
			return fmt.Errorf("rewrite %s/config.toml: %w", account.HomePath, err)
		}
	}

	r.grokProcessMu.Lock()
	r.grokDesiredAlwaysApprove = yolo
	// A posture change is account-global (it may have rewritten config.toml above),
	// so close every live process; the next turn respawns under the new posture.
	r.closeAllGrokProcessesLocked()
	r.grokProcessMu.Unlock()
	return nil
}
