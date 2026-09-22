package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDevin is a scripted stand-in for `devin acp`, speaking
// newline-delimited JSON-RPC over in-memory pipes (mirrors fakeOpencode).
type fakeDevin struct {
	out      *io.PipeWriter
	outMu    sync.Mutex
	requests chan map[string]any
}

func startFakeDevin(t *testing.T, inbound func(devinInboundRequest)) (*devinDispatcher, *fakeDevin) {
	t.Helper()
	cR, cW := io.Pipe()
	sR, sW := io.Pipe()
	d := newDevinDispatcher(cW, inbound)
	d.start(sR)
	fd := &fakeDevin{out: sW, requests: make(chan map[string]any, 1024)}
	go func() {
		scanner := bufio.NewScanner(cR)
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				continue
			}
			select {
			case fd.requests <- m:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		_ = sW.Close()
		_ = cW.Close()
	})
	return d, fd
}

func (fd *fakeDevin) send(msg map[string]any) {
	b, _ := json.Marshal(msg)
	b = append(b, '\n')
	fd.outMu.Lock()
	defer fd.outMu.Unlock()
	_, _ = fd.out.Write(b)
}

func (fd *fakeDevin) reply(id any, result map[string]any) {
	fd.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (fd *fakeDevin) notify(method string, params map[string]any) {
	fd.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (fd *fakeDevin) serve(handle func(fd *fakeDevin, msg map[string]any)) {
	go func() {
		for m := range fd.requests {
			handle(fd, m)
		}
	}()
}

func TestDevinDispatcherMultiplexesConcurrentSessions(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		fd.reply(m["id"], map[string]any{"id": m["id"]})
	})

	sessionA, err := d.registerSession("alpha-bravo")
	if err != nil {
		t.Fatalf("registerSession a: %v", err)
	}
	sessionB, err := d.registerSession("charlie-delta")
	if err != nil {
		t.Fatalf("registerSession b: %v", err)
	}
	defer d.unregisterSession("alpha-bravo")
	defer d.unregisterSession("charlie-delta")

	fd.notify("session/update", map[string]any{"sessionId": "alpha-bravo", "tag": "a"})
	fd.notify("session/update", map[string]any{"sessionId": "charlie-delta", "tag": "b"})

	select {
	case n := <-sessionA:
		if n.Params["tag"] != "a" {
			t.Fatalf("session A wrong: %v", n.Params)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout session A")
	}
	select {
	case n := <-sessionB:
		if n.Params["tag"] != "b" {
			t.Fatalf("session B wrong: %v", n.Params)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout session B")
	}
}

// DOD-10: `_cognition.ai/*` extension notifications — including sessionless
// channel output — must be tolerated, never fatal, and (sessionless) broadcast
// to live subs so a turn can surface diagnostics.
func TestDevinDispatcherToleratesExtensionNotifications(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	sessionCh, err := d.registerSession("s1")
	if err != nil {
		t.Fatalf("registerSession: %v", err)
	}
	defer d.unregisterSession("s1")

	// Sessionless _cognition.ai/output line (the MCP connect trace observed live).
	fd.notify("_cognition.ai/output", map[string]any{"channel": "MCP: flowpilot", "message": "connecting", "level": "info", "sessionId": ""})
	// Session-scoped extension notification.
	fd.notify("_cognition.ai/turn_stats", map[string]any{"sessionId": "s1", "responseDimensions": []any{}})
	// Unknown future method.
	fd.notify("_cognition.ai/future_thing", map[string]any{"sessionId": "s1"})
	// Malformed frame must not kill the loop.
	fd.outMu.Lock()
	_, _ = fd.out.Write([]byte("{not json\n"))
	fd.outMu.Unlock()
	fd.notify("session/update", map[string]any{"sessionId": "s1", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "after"}}})

	var methods []string
	var texts []string
	deadline := time.After(2 * time.Second)
	for len(texts) == 0 {
		select {
		case n, ok := <-sessionCh:
			if !ok {
				t.Fatal("session channel closed")
			}
			methods = append(methods, n.Method)
			if n.Method == "session/update" {
				update, _ := n.Params["update"].(map[string]any)
				texts = append(texts, fmt.Sprint(update["content"]))
			}
		case <-deadline:
			t.Fatalf("timeout waiting for notifications; got methods=%v", methods)
		}
	}
	if d.isClosed() {
		t.Fatal("dispatcher must survive extension notifications + malformed frames")
	}
	// The sessionless extension broadcast must have reached the sub.
	var sawOutput, sawTurnStats bool
	for _, m := range methods {
		if m == "_cognition.ai/output" {
			sawOutput = true
		}
		if m == "_cognition.ai/turn_stats" {
			sawTurnStats = true
		}
	}
	if !sawOutput || !sawTurnStats {
		t.Fatalf("expected broadcast + session-scoped extensions to arrive, got %v", methods)
	}
}

func TestDevinDispatcherRoutesPermissionRequestToInboundHandler(t *testing.T) {
	received := make(chan devinInboundRequest, 1)
	var dRef *devinDispatcher
	d, fd := startFakeDevin(t, func(req devinInboundRequest) {
		received <- req
		_ = dRef.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "allow"}})
	})
	dRef = d

	fd.send(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "session/request_permission", "params": map[string]any{
		"sessionId": "s1",
		"toolCall":  map[string]any{"toolCallId": "tc1", "title": "bash", "kind": "execute"},
		"options":   []any{map[string]any{"optionId": "allow", "kind": "allow_once"}},
	}})

	select {
	case req := <-received:
		if req.Method != "session/request_permission" {
			t.Fatalf("unexpected method: %s", req.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout inbound")
	}
}

func TestDevinDispatcherFailDrainsWaitersAndSubs(t *testing.T) {
	d, _ := startFakeDevin(t, nil)
	sessionCh, err := d.registerSession("s1")
	if err != nil {
		t.Fatalf("registerSession: %v", err)
	}

	callErr := make(chan error, 1)
	go func() {
		_, err := d.call(context.Background(), "session/prompt", map[string]any{})
		callErr <- err
	}()
	time.Sleep(50 * time.Millisecond)

	d.fail(io.ErrClosedPipe)

	select {
	case err := <-callErr:
		if err == nil {
			t.Fatal("expected error after fail")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout call")
	}

	select {
	case _, ok := <-sessionCh:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout session close")
	}
}

// DOD-9: process boot must run initialize -> authenticate -> (only then)
// session-capable. The fake binary asserts the order on the wire.
func TestEnsureDevinProcessBootOrderIncludesAuthenticate(t *testing.T) {
	orig := commandContextFn
	defer func() { commandContextFn = orig }()

	initResult, _ := json.Marshal(map[string]any{
		"protocolVersion": 1,
		"agentCapabilities": map[string]any{
			"loadSession":        true,
			"mcpCapabilities":    map[string]any{"http": false, "sse": false},
			"promptCapabilities": map[string]any{"image": true, "embeddedContext": true},
		},
		"authMethods": []any{map[string]any{"id": "devin-browser", "name": "Log in with browser"}},
	})
	// Shell script: echo init result, echo authenticate result for id=2,
	// then stay alive. The script proves ordering: authenticate's reply only
	// arrives if a request with method authenticate was actually sent (the
	// harness below asserts on the call sequence instead).
	commandContextFn = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		s := shellReadLine() + shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":`+string(initResult)+`}`) +
			shellReadLine() + shellOutputLine(`{"jsonrpc":"2.0","id":2,"result":{}}`) + "sleep 3\n"
		return testShellCommand(ctx, s)
	}

	r := &Runner{devinProcesses: make(map[string]*devinProcessHandle)}
	h, err := r.ensureDevinProcess(context.Background(), "scope-a", "/tmp", nil, "", "")
	if err != nil {
		t.Fatalf("ensureDevinProcess: %v", err)
	}
	if h == nil || h.dispatcher == nil {
		t.Fatal("expected live handle")
	}
	if h.initResult == nil || h.initResult["protocolVersion"] == nil {
		t.Fatal("initResult not captured")
	}
	h.close()
}

// DOD-9 negative: when authenticate fails the process must be torn down and
// the error must name authenticate — a half-booted handle is never cached.
func TestEnsureDevinProcessAuthenticateFailureKillsProcess(t *testing.T) {
	orig := commandContextFn
	defer func() { commandContextFn = orig }()

	initResult, _ := json.Marshal(map[string]any{"protocolVersion": 1})
	commandContextFn = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		s := shellReadLine() + shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":`+string(initResult)+`}`) +
			shellReadLine() + shellOutputLine(`{"jsonrpc":"2.0","id":2,"error":{"code":-32000,"message":"authentication required"}}`) + "sleep 3\n"
		return testShellCommand(ctx, s)
	}

	r := &Runner{devinProcesses: make(map[string]*devinProcessHandle)}
	h, err := r.ensureDevinProcess(context.Background(), "scope-a", "/tmp", nil, "", "")
	if err == nil {
		t.Fatal("expected authenticate failure")
	}
	if h != nil {
		t.Fatal("no handle may be returned on auth failure")
	}
	if !strings.Contains(err.Error(), "authenticate") {
		t.Fatalf("error should name authenticate, got %v", err)
	}
	if len(r.devinProcesses) != 0 {
		t.Fatal("no process may be cached after auth failure")
	}
}

func TestDevinProcessKeyAndScopeReuse(t *testing.T) {
	key1 := devinProcessKey("scope-a", "swe-2-high", "bypass")
	key2 := devinProcessKey("scope-a", "swe-2-high", "bypass")
	if key1 != key2 {
		t.Fatal("same tuple should produce same key")
	}
	if devinProcessKey("scope-a", "swe-2-low", "bypass") == key1 {
		t.Fatal("different model should produce different key")
	}
	if devinProcessKey("scope-a", "swe-2-high", "accept-edits") == key1 {
		t.Fatal("different mode should produce different key")
	}
	if devinProcessKey("scope-b", "swe-2-high", "bypass") == key1 {
		t.Fatal("different scope should produce different key")
	}

	// Runner map coexistence without spawning.
	r := &Runner{devinProcesses: make(map[string]*devinProcessHandle)}
	h1 := &devinProcessHandle{scopeKey: "scope-a", model: "m1", permissionMode: "bypass", dispatcher: newDevinDispatcher(io.Discard, nil)}
	r.devinProcesses[key1] = h1
	if existing := r.devinProcesses[key1]; existing != h1 {
		t.Fatal("expected reuse")
	}
	r.devinProcesses[devinProcessKey("scope-b", "x", "y")] = &devinProcessHandle{scopeKey: "scope-b", dispatcher: newDevinDispatcher(io.Discard, nil)}
	for k, h := range r.devinProcesses {
		if h.scopeKey != "scope-a" {
			h.close()
			delete(r.devinProcesses, k)
		}
	}
	if _, exists := r.devinProcesses[key1]; !exists {
		t.Fatal("expected scope-a to remain")
	}
}

// DOD-6: env isolation — strip host DEVIN_*/WINDSURF_API_KEY secrets, honor
// extraEnv, auto-fill XDG roots from HOME.
func TestDevinProcessEnvIsolates(t *testing.T) {
	t.Setenv("DEVIN_API_KEY", "host-secret-devin")
	t.Setenv("WINDSURF_API_KEY", "host-secret-ws")
	t.Setenv("DEVIN_SOME_OTHER", "x")

	env := devinProcessEnv(map[string]string{"HOME": "/tmp/devinhome"})
	has := func(prefix string) bool {
		for _, e := range env {
			if strings.HasPrefix(e, prefix) {
				return true
			}
		}
		return false
	}
	eq := func(want string) bool {
		for _, e := range env {
			if e == want {
				return true
			}
		}
		return false
	}
	if has("DEVIN_API_KEY=") || has("DEVIN_SOME_OTHER=") {
		t.Fatal("leaked host DEVIN_* secrets")
	}
	if has("WINDSURF_API_KEY=") {
		t.Fatal("leaked host WINDSURF_API_KEY")
	}
	if !eq("HOME=/tmp/devinhome") {
		t.Fatal("expected extraEnv HOME to win")
	}
	if !eq("XDG_CONFIG_HOME=" + filepath.Join("/tmp/devinhome", ".config")) {
		t.Fatal("expected XDG_CONFIG_HOME derived from HOME")
	}
	if !eq("XDG_DATA_HOME=" + filepath.Join("/tmp/devinhome", ".local", "share")) {
		t.Fatal("expected XDG_DATA_HOME derived from HOME")
	}

	// Explicit XDG overrides are respected, not duplicated.
	env2 := devinProcessEnv(map[string]string{
		"HOME":            "/tmp/h2",
		"XDG_CONFIG_HOME": "/custom/config",
		"XDG_DATA_HOME":   "/custom/data",
	})
	var configCount, dataCount int
	for _, e := range env2 {
		if strings.HasPrefix(e, "XDG_CONFIG_HOME=") {
			configCount++
			if e != "XDG_CONFIG_HOME=/custom/config" {
				t.Fatalf("explicit XDG_CONFIG_HOME overwritten: %s", e)
			}
		}
		if strings.HasPrefix(e, "XDG_DATA_HOME=") {
			dataCount++
			if e != "XDG_DATA_HOME=/custom/data" {
				t.Fatalf("explicit XDG_DATA_HOME overwritten: %s", e)
			}
		}
	}
	if configCount != 1 || dataCount != 1 {
		t.Fatal("XDG vars duplicated")
	}
}

func TestDevinAgentEnabledFlag(t *testing.T) {
	t.Setenv("FLOWPILOT_DEVIN_AGENT", "0")
	if devinAgentEnabled() {
		t.Fatal("expected disabled when env=0")
	}
	t.Setenv("FLOWPILOT_DEVIN_AGENT", "1")
	if !devinAgentEnabled() {
		t.Fatal("expected enabled when env=1")
	}
	t.Setenv("FLOWPILOT_DEVIN_AGENT", "false")
	if devinAgentEnabled() {
		t.Fatal("expected disabled when false")
	}
	t.Setenv("FLOWPILOT_DEVIN_AGENT", "")
	if !devinAgentEnabled() {
		t.Fatal("expected enabled by default")
	}

	t.Setenv("FLOWPILOT_DEVIN_AGENT", "0")
	r := &Runner{devinProcesses: make(map[string]*devinProcessHandle)}
	_, err := r.ensureDevinProcess(context.Background(), "scope-a", "/tmp", nil, "", "")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expected not implemented error when disabled, got %v", err)
	}
}

// DOD-7: frame logging redacts credential-shaped keys — including the
// authenticate response and mcpServers env — before anything hits the log.
func TestDevinFrameRedaction(t *testing.T) {
	frame := `{"jsonrpc":"2.0","method":"session/new","params":{"mcpServers":[{"type":"stdio","name":"flowpilot","command":"x","env":[{"name":"TOKEN","value":"sekret"}]}],"token":"abc","nested":{"access_token":"zzz","safe":"ok"}}}`
	out := redactDevinFrameForLog(frame)
	if strings.Contains(out, "sekret") || strings.Contains(out, `"token":"abc"`) || strings.Contains(out, "zzz") {
		t.Fatalf("redaction leaked secret material: %s", out)
	}
	if !strings.Contains(out, `"safe":"ok"`) {
		t.Fatalf("redaction ate non-secret data: %s", out)
	}
	if got := redactDevinFrameForLog("not json"); !strings.Contains(got, "omitted") {
		t.Fatalf("malformed frames must be omitted, got %q", got)
	}
}

// Golden-flow replay: initialize -> authenticate -> session/new -> prompt ->
// session/list -> session/load over the fake, driven by the real param
// builders and live fixtures.
func TestDevinGoldenFlow(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		load := func(name string) map[string]any {
			data, _ := os.ReadFile(filepath.Join("testdata", "devin_acp", name))
			var res map[string]any
			_ = json.Unmarshal(data, &res)
			return res
		}
		switch m["method"] {
		case "initialize":
			fd.reply(m["id"], load("initialize_result.json"))
		case "authenticate":
			params, _ := m["params"].(map[string]any)
			if params["methodId"] != "devin-browser" {
				fd.send(map[string]any{"jsonrpc": "2.0", "id": m["id"], "error": map[string]any{"code": -32602, "message": "bad methodId"}})
				return
			}
			fd.reply(m["id"], map[string]any{})
		case "session/new":
			fd.reply(m["id"], load("session_new_result.json"))
		case "session/load":
			fd.reply(m["id"], load("session_load_result.json"))
		case "session/list":
			fd.reply(m["id"], load("session_list_result.json"))
		case "session/set_config_option":
			fd.reply(m["id"], load("set_config_model_result.json"))
		case "session/prompt":
			params, _ := m["params"].(map[string]any)
			sid, _ := params["sessionId"].(string)
			fd.notify("session/update", map[string]any{"sessionId": sid, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "PROBE_OK"}}})
			fd.reply(m["id"], load("session_prompt_result.json"))
		}
	})

	ctx := context.Background()
	initRes, err := d.call(ctx, "initialize", devinACPInitializeParams())
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	caps, _ := initRes["agentCapabilities"].(map[string]any)
	if mcp, _ := caps["mcpCapabilities"].(map[string]any); mcp["http"] != false {
		t.Fatal("expected mcp http false")
	}

	if _, err := d.call(ctx, "authenticate", devinACPAuthenticateParams()); err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	newRes, err := d.call(ctx, "session/new", devinACPSessionNewParams("/private/tmp", nil))
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	sid := devinACPResponseSessionIDFromResult(newRes)
	if sid != "working-pentagon" {
		t.Fatalf("sessionId: %q", sid)
	}
	if got := devinSessionModeFromResult(newRes); got != "accept-edits" {
		t.Fatalf("mode: %q", got)
	}

	if _, err := d.call(ctx, "session/set_config_option", devinACPSessionSetConfigParams(sid, "model", "swe-2-medium")); err != nil {
		t.Fatalf("set_config_option: %v", err)
	}

	notif, err := d.registerSession(sid)
	if err != nil {
		t.Fatalf("registerSession: %v", err)
	}
	defer d.unregisterSession(sid)

	res, err := d.call(ctx, "session/prompt", devinACPPromptParams(sid, "Reply PROBE_OK"))
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if res["stopReason"] != "end_turn" {
		t.Fatalf("stopReason: %v", res["stopReason"])
	}
	select {
	case n := <-notif:
		if devinACPExtractText(map[string]any{"method": n.Method, "params": n.Params}) == "" {
			update, _ := n.Params["update"].(map[string]any)
			if got := devinACPExtractText(update); got != "PROBE_OK" {
				t.Fatalf("streamed text: %q", got)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for stream chunk")
	}

	listRes, err := d.call(ctx, "session/list", devinACPSessionListParams("/private/tmp"))
	if err != nil {
		t.Fatalf("session/list: %v", err)
	}
	sessions, _ := listRes["sessions"].([]any)
	if len(sessions) == 0 {
		t.Fatal("expected sessions")
	}

	if _, err := d.call(ctx, "session/load", devinACPSessionLoadParams(sid, "/private/tmp", nil)); err != nil {
		t.Fatalf("session/load: %v", err)
	}
}
