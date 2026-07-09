package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGrok is a scripted stand-in for `grok agent stdio`, speaking
// newline-delimited JSON-RPC over in-memory pipes (mirrors fakeCodex,
// codex_appserver_test.go) so the dispatcher is tested without a real grok
// binary. Golden-fixture tests below replay the exact live-captured sequence
// (testdata/grok_acp/live_probe_raw.txt, captured against grok 0.2.93).
type fakeGrok struct {
	out      *io.PipeWriter
	outMu    sync.Mutex
	requests chan map[string]any
}

func startFakeGrok(t *testing.T, inbound func(grokInboundRequest)) (*grokDispatcher, *fakeGrok) {
	t.Helper()
	cR, cW := io.Pipe()
	sR, sW := io.Pipe()
	d := newGrokDispatcher(cW, inbound)
	d.start(sR)
	fg := &fakeGrok{out: sW, requests: make(chan map[string]any, 1024)}
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

func (fg *fakeGrok) send(msg map[string]any) {
	b, _ := json.Marshal(msg)
	b = append(b, '\n')
	fg.outMu.Lock()
	defer fg.outMu.Unlock()
	_, _ = fg.out.Write(b)
}

func (fg *fakeGrok) reply(id any, result map[string]any) {
	fg.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}
func (fg *fakeGrok) notify(method string, params map[string]any) {
	fg.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}
func (fg *fakeGrok) serve(handle func(fg *fakeGrok, msg map[string]any)) {
	go func() {
		for m := range fg.requests {
			handle(fg, m)
		}
	}()
}

// ---- dispatcher tests (Task-206 DOD-3) -------------------------------------

func TestGrokDispatcherCallResponse(t *testing.T) {
	d, fg := startFakeGrok(t, nil)
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		fg.reply(m["id"], map[string]any{"echo": m["method"]})
	})
	res, err := d.call(context.Background(), "initialize", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res["echo"] != "initialize" {
		t.Fatalf("unexpected result: %v", res)
	}
}

func TestGrokDispatcherMultiplexesConcurrentSessions(t *testing.T) {
	d, fg := startFakeGrok(t, nil)
	fg.serve(func(fg *fakeGrok, m map[string]any) {
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
			t.Fatalf("session A got wrong notification: %v", n.Params)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session A notification")
	}
	select {
	case n := <-sessionB:
		if n.Params["tag"] != "b" {
			t.Fatalf("session B got wrong notification: %v", n.Params)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session B notification")
	}
}

func TestGrokDispatcherRoutesInboundPermissionRequest(t *testing.T) {
	received := make(chan grokInboundRequest, 1)
	var dRef *grokDispatcher
	d, fg := startFakeGrok(t, func(req grokInboundRequest) {
		received <- req
		_ = dRef.reply(req.ID, map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": "allow-once"}})
	})
	dRef = d

	fg.send(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "session/request_permission", "params": map[string]any{
		"sessionId": "session-a",
		"options":   []any{map[string]any{"optionId": "allow-once", "kind": "allow_once"}},
	}})

	select {
	case req := <-received:
		if req.Method != "session/request_permission" {
			t.Fatalf("unexpected method: %s", req.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound permission request")
	}
}

func TestGrokDispatcherFailDrainsWaitersAndSessions(t *testing.T) {
	d, _ := startFakeGrok(t, nil)
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
			t.Fatal("expected an error after dispatcher fail")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for call to unblock after fail")
	}

	select {
	case _, ok := <-sessionCh:
		if ok {
			t.Fatal("expected session channel to be closed, got a value")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session channel to close")
	}
}

// ---- golden fixture: full live-captured sequence (Task-206 DOD-1) ---------
//
// Replays the exact initialize -> session/new -> session/prompt (with
// streaming session/update notifications, a tool_call/tool_call_update pair,
// and the terminal result) sequence captured live against grok 0.2.93.

func TestGrokGoldenFixtureInitializeAndSessionNew(t *testing.T) {
	d, fg := startFakeGrok(t, nil)
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "initialize":
			fg.reply(m["id"], liveGrokInitializeResult())
		case "session/new":
			fg.reply(m["id"], liveGrokSessionNewResult())
		}
	})

	initRes, err := d.call(context.Background(), "initialize", grokACPInitializeParams())
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	window := grokContextWindowFromInit(initRes)
	if window == nil || *window != 500000 {
		t.Fatalf("expected context window 500000, got %v", window)
	}

	newRes, err := d.call(context.Background(), "session/new", grokACPSessionNewParams("/tmp/x", nil))
	if err != nil {
		t.Fatalf("session/new: %v", err)
	}
	sessionID := grokACPResponseSessionID(map[string]any{"result": newRes})
	if sessionID == "" {
		t.Fatal("expected a sessionId")
	}
}

func TestGrokGoldenFixtureStreamedPromptAndToolCall(t *testing.T) {
	sessionID := "019f45c7-ad1a-7052-8a13-fdad979fe04b"
	d, fg := startFakeGrok(t, nil)

	notif, err := d.registerSession(sessionID)
	if err != nil {
		t.Fatalf("registerSession: %v", err)
	}
	defer d.unregisterSession(sessionID)

	fg.serve(func(fg *fakeGrok, m map[string]any) {
		if m["method"] != "session/prompt" {
			return
		}
		go func() {
			for _, frame := range liveGrokToolCallStreamFrames(sessionID) {
				fg.notify("session/update", frame)
			}
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}()
	})

	resultCh := make(chan map[string]any, 1)
	go func() {
		res, callErr := d.call(context.Background(), "session/prompt", grokACPPromptParams(sessionID, "read sample.txt"))
		if callErr != nil {
			t.Errorf("session/prompt call: %v", callErr)
			return
		}
		resultCh <- res
	}()

	var events []ProviderEvent
	// Generous bound: pure in-memory pipe ops, but a full `go test ./...` run
	// can put the scheduler under enough contention to make a short deadline
	// flaky on a loaded machine.
	deadline := time.After(15 * time.Second)
	var finalResult map[string]any
collect:
	for {
		select {
		case n := <-notif:
			mapped, ok := mapGrokNotification(n)
			if ok {
				events = append(events, mapped...)
			}
		case res := <-resultCh:
			finalResult = res
			break collect
		case <-deadline:
			t.Fatal("timed out collecting streamed events")
		}
	}

	var sawToolStarted, sawToolCompleted, sawFileChanged, sawDelta bool
	var deltaText string
	for _, ev := range events {
		switch ev.Type {
		case EventToolStarted:
			sawToolStarted = true
		case EventToolCompleted:
			sawToolCompleted = true
		case EventFileChanged:
			sawFileChanged = true
		case EventMessageDelta:
			sawDelta = true
			deltaText += ev.Text
		}
	}
	if !sawToolStarted || !sawToolCompleted {
		t.Fatalf("expected tool_started+tool_completed events, got %+v", events)
	}
	if sawFileChanged {
		t.Fatal("a read-only tool call must not emit file_changed")
	}
	if !sawDelta || !strings.Contains(deltaText, "OK") {
		t.Fatalf("expected a streamed message delta containing OK, got %q", deltaText)
	}
	if finalResult["stopReason"] != "end_turn" {
		t.Fatalf("expected stopReason end_turn, got %v", finalResult["stopReason"])
	}
	meta, _ := finalResult["_meta"].(map[string]any)
	usage := grokPromptResultTokenUsage(meta, nil)
	if usage == nil || usage.Total.TotalTokens == 0 {
		t.Fatalf("expected non-zero token usage, got %+v", usage)
	}
}

// ---- fixtures (captured live against grok 0.2.93; see testdata/grok_acp/) --

func liveGrokInitializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": 1,
		"agentCapabilities": map[string]any{
			"promptCapabilities": map[string]any{"image": false, "audio": false, "embeddedContext": true},
			"mcpCapabilities":    map[string]any{"http": true, "sse": true},
		},
		"_meta": map[string]any{
			"agentVersion": "0.2.93",
			"modelState": map[string]any{
				"currentModelId": "grok-4.5",
				"availableModels": []any{
					map[string]any{
						"modelId": "grok-4.5",
						"name":    "Grok 4.5",
						"_meta": map[string]any{
							"totalContextTokens":      float64(500000),
							"supportsReasoningEffort": true,
							"reasoningEfforts": []any{
								map[string]any{"id": "high", "value": "high", "label": "High Effort", "default": true},
								map[string]any{"id": "medium", "value": "medium", "label": "Medium Effort", "default": false},
								map[string]any{"id": "low", "value": "low", "label": "Low Effort", "default": false},
							},
						},
					},
				},
			},
		},
	}
}

func liveGrokSessionNewResult() map[string]any {
	return map[string]any{
		"sessionId": "019f45c5-db9d-7460-85ee-ab3a700c1a71",
		"models":    map[string]any{"currentModelId": "grok-4.5"},
	}
}

func liveGrokToolCallStreamFrames(sessionID string) []map[string]any {
	return []map[string]any{
		{"sessionId": sessionID, "update": map[string]any{
			"sessionUpdate": "tool_call",
			"toolCallId":    "call-f53ab3c3-0",
			"title":         "read_file",
			"rawInput":      map[string]any{"target_file": "sample.txt"},
			"_meta":         map[string]any{"x.ai/tool": map[string]any{"name": "read_file", "kind": "read"}},
		}},
		{"sessionId": sessionID, "update": map[string]any{
			"sessionUpdate": "tool_call_update",
			"toolCallId":    "call-f53ab3c3-0",
			"kind":          "read",
			"status":        "completed",
			"locations":     []any{map[string]any{"path": "sample.txt"}},
			"content":       []any{map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "hello world\n"}}},
		}},
		{"sessionId": sessionID, "update": map[string]any{
			"sessionUpdate": "agent_message_chunk",
			"content":       map[string]any{"type": "text", "text": "OK"},
		}},
	}
}

func liveGrokPromptResult(sessionID string) map[string]any {
	return map[string]any{
		"stopReason": "end_turn",
		"_meta": map[string]any{
			"sessionId":        sessionID,
			"requestId":        "9cfd168a-2bf8-415e-ba1b-bcdff40786d6",
			"promptId":         "9cfd168a-2bf8-415e-ba1b-bcdff40786d6",
			"totalTokens":      float64(17275),
			"modelId":          "grok-4.5",
			"inputTokens":      float64(17234),
			"outputTokens":     float64(39),
			"cachedReadTokens": float64(17024),
			"reasoningTokens":  float64(21),
		},
	}
}

// ---- process launch env / redaction tests (Task-206 DOD-5/DOD-6) ----------

func TestGrokProcessEnvDisablesAmbientMCPScanning(t *testing.T) {
	env := grokProcessEnv(map[string]string{"GROK_HOME": "/tmp/grok-home"})
	has := func(want string) bool {
		for _, e := range env {
			if e == want {
				return true
			}
		}
		return false
	}
	if !has("GROK_CLAUDE_MCPS_ENABLED=false") {
		t.Fatal("expected GROK_CLAUDE_MCPS_ENABLED=false in process env")
	}
	if !has("GROK_CURSOR_MCPS_ENABLED=false") {
		t.Fatal("expected GROK_CURSOR_MCPS_ENABLED=false in process env")
	}
	if !has("GROK_HOME=/tmp/grok-home") {
		t.Fatal("expected GROK_HOME to be set from extraEnv")
	}
}

func TestRedactGrokFrameForLogStripsCredentialShapedFields(t *testing.T) {
	raw := `{"jsonrpc":"2.0","method":"_x.ai/mcp/servers_updated","params":{"mcpServers":[{"name":"google-drive","headers":{"Authorization":"Bearer super-secret"},"env":{"REFRESH_TOKEN":"abc123"}}]}}`
	redacted := redactGrokFrameForLog(raw)
	if strings.Contains(redacted, "super-secret") || strings.Contains(redacted, "abc123") {
		t.Fatalf("credential leaked into redacted log line: %s", redacted)
	}
	if !strings.Contains(redacted, "[redacted]") {
		t.Fatalf("expected redaction marker, got: %s", redacted)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(redacted), &parsed); err != nil {
		t.Fatalf("redacted line is not valid JSON: %v", err)
	}
}

func TestRedactGrokFrameForLogHandlesMalformedInput(t *testing.T) {
	if redactGrokFrameForLog("not json") == "" {
		t.Fatal("expected a placeholder for malformed input, got empty string")
	}
}
