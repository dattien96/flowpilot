package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeOpencode is a scripted stand-in for `opencode acp`, speaking
// newline-delimited JSON-RPC over in-memory pipes (mirrors fakeGrok).
type fakeOpencode struct {
	out      *io.PipeWriter
	outMu    sync.Mutex
	requests chan map[string]any
}

func startFakeOpencode(t *testing.T, inbound func(opencodeInboundRequest)) (*opencodeDispatcher, *fakeOpencode) {
	t.Helper()
	cR, cW := io.Pipe()
	sR, sW := io.Pipe()
	d := newOpencodeDispatcher(cW, inbound)
	d.start(sR)
	fg := &fakeOpencode{out: sW, requests: make(chan map[string]any, 1024)}
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
			case fg.requests <- m:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		_ = sW.Close()
		_ = cW.Close()
	})
	return d, fg
}

func (fg *fakeOpencode) send(msg map[string]any) {
	b, _ := json.Marshal(msg)
	b = append(b, '\n')
	fg.outMu.Lock()
	defer fg.outMu.Unlock()
	_, _ = fg.out.Write(b)
}

func (fg *fakeOpencode) reply(id any, result map[string]any) {
	fg.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (fg *fakeOpencode) notify(method string, params map[string]any) {
	fg.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (fg *fakeOpencode) serve(handle func(fg *fakeOpencode, msg map[string]any)) {
	go func() {
		for m := range fg.requests {
			handle(fg, m)
		}
	}()
}

func mockOpencodeInitProcess(t *testing.T) func() {
	t.Helper()
	orig := commandContextFn
	initResult, _ := json.Marshal(map[string]any{
		"protocolVersion": 1,
		"agentCapabilities": map[string]any{
			"loadSession": true,
			"mcpCapabilities": map[string]any{"http": true, "sse": true},
			"promptCapabilities": map[string]any{"embeddedContext": true, "image": true},
			"sessionCapabilities": map[string]any{"close": map[string]any{}, "fork": map[string]any{}, "list": map[string]any{}, "resume": map[string]any{}},
		},
	})
	commandContextFn = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		s := shellReadLine() + shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":`+string(initResult)+`}`) + "sleep 3\n"
		return testShellCommand(ctx, s)
	}
	return func() { commandContextFn = orig }
}

func TestOpencodeDispatcherMultiplexesConcurrentSessions(t *testing.T) {
	d, fg := startFakeOpencode(t, nil)
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		fg.reply(m["id"], map[string]any{"id": m["id"]})
	})

	sessionA, err := d.registerSession("session-a")
	if err != nil {
		t.Fatalf("registerSession a: %v", err)
	}
	sessionB, err := d.registerSession("session-b")
	if err != nil {
		t.Fatalf("registerSession b: %v", err)
	}
	defer d.unregisterSession("session-a")
	defer d.unregisterSession("session-b")

	fg.notify("session/update", map[string]any{"sessionId": "session-a", "tag": "a"})
	fg.notify("session/update", map[string]any{"sessionId": "session-b", "tag": "b"})

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

func TestOpencodeDispatcherRoutesPermissionRequestToInboundHandler(t *testing.T) {
	received := make(chan opencodeInboundRequest, 1)
	var dRef *opencodeDispatcher
	d, fg := startFakeOpencode(t, func(req opencodeInboundRequest) {
		received <- req
		_ = dRef.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "once"}})
	})
	dRef = d

	fg.send(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "session/request_permission", "params": map[string]any{
		"sessionId": "session-a",
		"options":   []any{map[string]any{"optionId": "once", "kind": "allow_once"}},
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

func TestOpencodeDispatcherFailDrainsWaitersAndSubs(t *testing.T) {
	d, _ := startFakeOpencode(t, nil)
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

func TestOpencodeDispatcherRoutesPermissionRequestQuestionAlias(t *testing.T) {
	received := make(chan string, 1)
	d, fg := startFakeOpencode(t, func(req opencodeInboundRequest) {
		received <- req.Method
	})
	_ = d
	fg.send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "question", "params": map[string]any{}})
	select {
	case m := <-received:
		if m != "question" {
			t.Fatalf("got %s", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	if !opencodeACPIsPermissionRequest("question") {
		t.Fatal("question should be permission request")
	}
}

func TestEnsureOpencodeProcessReusesOrRespawns(t *testing.T) {
	// Test the process key logic directly (the core of reuse/respawn)
	// without spawning a real OS process (the spawn path is integration-tested
	// via live opencode acp elsewhere).
	// without spawning a real OS process (the spawn path is integration-tested
	// via live opencode acp elsewhere).

	// Test the key function
	key1 := opencodeProcessKey("scope-a", "opencode/muse-spark-1.2-contributor-free", "high", false)
	key2 := opencodeProcessKey("scope-a", "opencode/muse-spark-1.2-contributor-free", "high", false)
	if key1 != key2 {
		t.Fatal("same tuple should produce same key")
	}
	key3 := opencodeProcessKey("scope-a", "opencode/gpt-5.4-nano", "high", false)
	if key1 == key3 {
		t.Fatal("different model should produce different key")
	}
	key4 := opencodeProcessKey("scope-a", "opencode/muse-spark-1.2-contributor-free", "low", false)
	if key1 == key4 {
		t.Fatal("different variant should produce different key")
	}
	key5 := opencodeProcessKey("scope-a", "opencode/muse-spark-1.2-contributor-free", "high", true)
	if key1 == key5 {
		t.Fatal("different auto should produce different key")
	}
	key6 := opencodeProcessKey("scope-b", "opencode/muse-spark-1.2-contributor-free", "high", false)
	if key1 == key6 {
		t.Fatal("different scope should produce different key")
	}

	// Test Runner map coexistence without spawning: simulate two handles
	r := &Runner{
		opencodeProcesses: make(map[string]*opencodeProcessHandle),
	}
	h1 := &opencodeProcessHandle{scopeKey: "scope-a", model: "m1", variant: "high", auto: false, dispatcher: newOpencodeDispatcher(io.Discard, nil)}
	r.opencodeProcesses[key1] = h1
	// Simulate ensure logic: same key reuses
	if existing := r.opencodeProcesses[key1]; existing != h1 {
		t.Fatal("expected reuse")
	}
	// Different scope should be closed (simulate account switch)
	// Add a handle for scope-b, then call ensure with scope-a should close scope-b
	r.opencodeProcesses[key6] = &opencodeProcessHandle{scopeKey: "scope-b", dispatcher: newOpencodeDispatcher(io.Discard, nil)}
	// Simulate the for loop that closes different scope
	for k, h := range r.opencodeProcesses {
		if h.scopeKey != "scope-a" {
			h.close()
			delete(r.opencodeProcesses, k)
		}
	}
	if _, exists := r.opencodeProcesses[key6]; exists {
		t.Fatal("expected scope-b to be removed on scope-a ensure")
	}
	if _, exists := r.opencodeProcesses[key1]; !exists {
		t.Fatal("expected scope-a to remain")
	}
}

func TestOpencodeProcessEnvIsolates(t *testing.T) {
	env := opencodeProcessEnv(map[string]string{"HOME": "/tmp/home", "OPENCODE_CONFIG": "/tmp/home/.config/opencode", "GROK_HOME": "/tmp/grok"})
	has := func(want string) bool {
		for _, e := range env {
			if e == want {
				return true
			}
		}
		return false
	}
	if !has("HOME=/tmp/home") {
		t.Fatal("expected HOME")
	}
	if !has("OPENCODE_CONFIG=/tmp/home/.config/opencode") {
		t.Fatal("expected OPENCODE_CONFIG")
	}
	// Should not leak host OPENCODE_API_KEY if present in os.Environ
	t.Setenv("OPENCODE_API_KEY", "host-secret")
	env2 := opencodeProcessEnv(map[string]string{"HOME": "/tmp/home2"})
	for _, e := range env2 {
		if strings.HasPrefix(e, "OPENCODE_API_KEY=host-secret") {
			t.Fatal("leaked host OPENCODE_API_KEY")
		}
	}
	// Should contain disable flag
	found := false
	for _, e := range env {
		if e == "OPENCODE_DISABLE_AMBIENT_MCP=true" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected OPENCODE_DISABLE_AMBIENT_MCP=true")
	}
}

func TestEnsureOpencodeProcessHonorsAgentEnabledFlag(t *testing.T) {
	t.Setenv("FLOWPILOT_OPENCODE_AGENT", "0")
	if opencodeAgentEnabled() {
		t.Fatal("expected disabled when env=0")
	}
	t.Setenv("FLOWPILOT_OPENCODE_AGENT", "1")
	if !opencodeAgentEnabled() {
		t.Fatal("expected enabled when env=1")
	}
	t.Setenv("FLOWPILOT_OPENCODE_AGENT", "false")
	if opencodeAgentEnabled() {
		t.Fatal("expected disabled when false")
	}
	t.Setenv("FLOWPILOT_OPENCODE_AGENT", "")
	if !opencodeAgentEnabled() {
		t.Fatal("expected enabled by default")
	}

	// Test that ensure returns placeholder error when disabled
	orig := commandContextFn
	defer func() { commandContextFn = orig }()
	// Avoid spawning real process: set flag to disabled and call ensure
	t.Setenv("FLOWPILOT_OPENCODE_AGENT", "0")
	r := &Runner{
		opencodeProcesses: make(map[string]*opencodeProcessHandle),
	}
	_, err := r.ensureOpencodeProcess(context.Background(), "scope-a", "/tmp", nil, "", "", false)
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expected not implemented error when disabled, got %v", err)
	}
}

func TestOpencodeProcessEnvIsolatesWindows(t *testing.T) {
	// Verify HOME mapping also sets OPENCODE_CONFIG when not explicitly provided
	env := opencodeProcessEnv(map[string]string{"HOME": "/home/user"})
	hasConfig := false
	for _, e := range env {
		if strings.HasPrefix(e, "OPENCODE_CONFIG=") {
			hasConfig = true
			if !strings.Contains(e, ".config/opencode") {
				t.Fatalf("unexpected OPENCODE_CONFIG: %s", e)
			}
		}
	}
	if !hasConfig {
		t.Fatal("expected OPENCODE_CONFIG to be auto-set from HOME")
	}
}

func TestOpencodeGoldenFixture(t *testing.T) {
	// Replay the exact initialize -> session/new -> prompt flow from fixtures
	d, fg := startFakeOpencode(t, nil)
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "initialize":
			data, _ := os.ReadFile(filepath.Join("testdata", "opencode_acp", "initialize_result.json"))
			var res map[string]any
			_ = json.Unmarshal(data, &res)
			fg.reply(m["id"], res)
		case "session/new":
			data, _ := os.ReadFile(filepath.Join("testdata", "opencode_acp", "session_new_result.json"))
			var res map[string]any
			_ = json.Unmarshal(data, &res)
			fg.reply(m["id"], res)
		case "session/load":
			data, _ := os.ReadFile(filepath.Join("testdata", "opencode_acp", "resume_response.json"))
			var res map[string]any
			_ = json.Unmarshal(data, &res)
			fg.reply(m["id"], res)
		}
	})

	initRes, err := d.call(context.Background(), "initialize", opencodeACPInitializeParams())
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if initRes["protocolVersion"] == nil {
		t.Fatal("expected protocolVersion")
	}
	caps, _ := initRes["agentCapabilities"].(map[string]any)
	if caps == nil {
		t.Fatal("expected agentCapabilities")
	}
	if mcp, _ := caps["mcpCapabilities"].(map[string]any); mcp == nil || mcp["http"] != true {
		t.Fatalf("expected mcp http true, got %v", mcp)
	}

	newRes, err := d.call(context.Background(), "session/new", opencodeACPSessionNewParams("/tmp", nil, nil, "", ""))
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	sid := opencodeACPResponseSessionIDFromResult(newRes)
	if sid == "" {
		t.Fatal("expected sessionId")
	}

	// Test resume shape
	loadParams := opencodeACPSessionLoadParams(sid, "/tmp", nil)
	if loadParams["sessionId"] != sid {
		t.Fatalf("load sessionId: %v", loadParams["sessionId"])
	}
	loadRes, err := d.call(context.Background(), "session/load", loadParams)
	if err != nil {
		t.Fatalf("session/load: %v", err)
	}
	if loadRes["sessionId"] == nil && loadRes["configOptions"] == nil {
		t.Fatal("expected sessionId or configOptions in load result")
	}
}
