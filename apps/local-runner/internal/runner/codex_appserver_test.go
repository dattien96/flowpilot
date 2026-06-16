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

// fakeCodex is a scripted stand-in for the `codex app-server` process, speaking
// newline-delimited JSON-RPC over in-memory pipes — so the dispatcher + adapter are
// tested without a real Codex binary (06 Part D verifies the real wire later).
type fakeCodex struct {
	out      *io.PipeWriter
	outMu    sync.Mutex
	requests chan map[string]any
}

func startFakeCodex(t *testing.T, inbound func(codexInboundRequest)) (*codexDispatcher, *fakeCodex) {
	t.Helper()
	cR, cW := io.Pipe() // client → server
	sR, sW := io.Pipe() // server → client
	d := newCodexDispatcher(cW, inbound)
	d.start(sR)
	fc := &fakeCodex{out: sW, requests: make(chan map[string]any, 1024)}
	// Always drain the client→server pipe so the dispatcher's writes never block
	// on the synchronous io.Pipe, even in tests that script no responses.
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
			case fc.requests <- m:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		_ = sW.Close()
		_ = cW.Close()
	})
	return d, fc
}

func (fc *fakeCodex) send(msg map[string]any) {
	b, _ := json.Marshal(msg)
	b = append(b, '\n')
	fc.outMu.Lock()
	defer fc.outMu.Unlock()
	_, _ = fc.out.Write(b)
}

func (fc *fakeCodex) reply(id any, result map[string]any) {
	fc.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (fc *fakeCodex) notify(method string, params map[string]any) {
	fc.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// serve dispatches each drained client request to handle, on a goroutine.
func (fc *fakeCodex) serve(handle func(fc *fakeCodex, msg map[string]any)) {
	go func() {
		for m := range fc.requests {
			handle(fc, m)
		}
	}()
}

func (fc *fakeCodex) closeServer() { _ = fc.out.Close() }

// ---- dispatcher tests ------------------------------------------------------

func TestDispatcherCallResponse(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		fc.reply(m["id"], map[string]any{"echo": m["method"]})
	})
	res, err := d.call(context.Background(), "thread/start", map[string]any{"cwd": "/x"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res["echo"] != "thread/start" {
		t.Fatalf("unexpected result: %v", res)
	}
}

func TestDispatcherConcurrentCalls(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		fc.reply(m["id"], map[string]any{"id": m["id"]})
	})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := d.call(context.Background(), "ping", nil); err != nil {
				t.Errorf("concurrent call: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestDispatcherNotificationRoutingByThread(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	ch, err := d.registerThread("t1")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	fc.notify("turn.delta", map[string]any{"threadId": "t1", "text": "hi"})
	fc.notify("turn.delta", map[string]any{"threadId": "other", "text": "nope"}) // wrong thread, dropped

	select {
	case n := <-ch:
		if n.Method != "turn.delta" || n.Params["text"] != "hi" {
			t.Fatalf("unexpected notification: %+v", n)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestDispatcherInboundRequestRouted(t *testing.T) { // T-37
	replied := make(chan map[string]any, 1)
	var d *codexDispatcher
	d, fc := startFakeCodex(t, func(req codexInboundRequest) {
		// reply-capable handler (the third message category)
		_ = d.reply(req.ID, map[string]any{"decision": "approve"})
	})
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		if _, ok := m["result"]; ok {
			replied <- m
		}
	})
	// server → client request (has method AND id)
	fc.send(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "approval/request", "params": map[string]any{"threadId": "t1"}})

	select {
	case m := <-replied:
		result, _ := m["result"].(map[string]any)
		if jsonRPCIDToInt(m["id"]) != 99 || result["decision"] != "approve" {
			t.Fatalf("unexpected reply: %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("inbound request was not routed to a reply-capable handler")
	}
}

func TestDispatcherProcessDeathDrains(t *testing.T) { // T-38
	d, fc := startFakeCodex(t, nil)
	ch, _ := d.registerThread("t1")

	// a call with no response, then kill the server stream
	callErr := make(chan error, 1)
	go func() {
		_, err := d.call(context.Background(), "turn/start", nil)
		callErr <- err
	}()
	time.Sleep(20 * time.Millisecond)
	fc.closeServer() // EOF → readLoop ends → fail() drains

	select {
	case err := <-callErr:
		if err == nil {
			t.Fatal("expected pending call to fail on process death")
		}
	case <-time.After(time.Second):
		t.Fatal("pending call was not drained on process death")
	}
	// thread channel is closed
	select {
	case _, ok := <-ch:
		if ok {
			// may receive nothing; closed channel yields ok=false
		}
	case <-time.After(time.Second):
		t.Fatal("thread channel not closed on process death")
	}
	if !d.isClosed() {
		t.Fatal("dispatcher should be closed")
	}
}

func TestDispatcherCallContextCancel(t *testing.T) {
	d, _ := startFakeCodex(t, nil) // server never replies
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	if _, err := d.call(ctx, "thread/list", nil); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

// ---- adapter integration ---------------------------------------------------

type captureBridge struct {
	mu               sync.Mutex
	events           []ProviderEvent
	approvalRequests []ApprovalDetails
	approveWith      string
}

func (b *captureBridge) Emit(ev ProviderEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, ev)
}
func (b *captureBridge) RequestApproval(details ApprovalDetails) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.approvalRequests = append(b.approvalRequests, details)
	return b.approveWith, nil
}
func (b *captureBridge) AskQuestion(string, []QuestionOption, bool) ([]string, error) {
	return nil, nil
}
func (b *captureBridge) types() []ProviderEventType {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]ProviderEventType, len(b.events))
	for i, e := range b.events {
		out[i] = e.Type
	}
	return out
}

func (b *captureBridge) approvalRequestCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.approvalRequests)
}

func (b *captureBridge) lastApprovalRequest() ApprovalDetails {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.approvalRequests) == 0 {
		return ApprovalDetails{}
	}
	return b.approvalRequests[len(b.approvalRequests)-1]
}

func TestCodexAdapterTurnStreams(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	adapter := newCodexAdapter(d, "/workspace")
	// re-point inbound to the adapter (startFakeCodex passed nil)
	d.setInbound(adapter.handleInbound)

	fc.serve(func(fc *fakeCodex, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "thread/start":
			fc.reply(m["id"], map[string]any{"threadId": "th1"})
		case "turn/start":
			fc.reply(m["id"], map[string]any{"turnId": "ct1"})
			fc.notify("turn.delta", map[string]any{"threadId": "th1", "text": "working..."})
			fc.notify("file.changed", map[string]any{"threadId": "th1", "path": "a.go", "changeType": "modified"})
			fc.notify("turn.completed", map[string]any{"threadId": "th1", "finalMessage": "done"})
		}
	})

	bridge := &captureBridge{}
	err := adapter.SendTurn(context.Background(), TurnRequest{RunID: "r1", Prompt: "hi"}, bridge)
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	got := bridge.types()
	if len(got) == 0 || got[len(got)-1] != EventTurnCompleted {
		t.Fatalf("expected terminal turn_completed, got %v", got)
	}
	if !containsType(got, EventMessageDelta) || !containsType(got, EventFileChanged) {
		t.Fatalf("missing mapped events: %v", got)
	}
	// adapter must NOT leak Codex's turn id onto events (runner core stamps it)
	bridge.mu.Lock()
	for _, e := range bridge.events {
		if e.ProviderTurnID != "" {
			bridge.mu.Unlock()
			t.Fatalf("event carried provider turn id %q; should be stamped by runner core", e.ProviderTurnID)
		}
	}
	bridge.mu.Unlock()
}

func TestCodexAdapterAcceptsGeneratedAppServerThreadAndTurnShapes(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	adapter := newCodexAdapter(d, "/workspace")
	d.setInbound(adapter.handleInbound)

	observedInputArray := make(chan bool, 1)
	observedThreadParams := make(chan map[string]any, 1)
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "thread/start":
			if params, ok := m["params"].(map[string]any); ok {
				observedThreadParams <- params
			}
			fc.reply(m["id"], map[string]any{
				"thread": map[string]any{"id": "th-live"},
			})
		case "turn/start":
			params, _ := m["params"].(map[string]any)
			_, ok := params["input"].([]any)
			observedInputArray <- ok
			fc.reply(m["id"], map[string]any{
				"turn": map[string]any{"id": "turn-live"},
			})
			fc.notify("item/agentMessage/delta", map[string]any{"threadId": "th-live", "turnId": "turn-live", "delta": "working..."})
			fc.notify("item/completed", map[string]any{
				"threadId": "th-live",
				"turnId":   "turn-live",
				"item":     map[string]any{"type": "agentMessage", "text": "done"},
			})
			fc.notify("turn/completed", map[string]any{
				"threadId": "th-live",
				"turn": map[string]any{
					"id": "turn-live",
					"items": []any{
						map[string]any{"type": "agentMessage", "text": "done"},
					},
				},
			})
		}
	})

	bridge := &captureBridge{}
	if err := adapter.SendTurn(context.Background(), TurnRequest{RunID: "r1", Prompt: "hi", ModelName: "gpt-5.4", ReasoningEffort: "HIGH", YoloMode: true}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	select {
	case params := <-observedThreadParams:
		if params["model"] != "gpt-5.4" {
			t.Fatalf("thread/start model = %v, want gpt-5.4", params["model"])
		}
		if params["reasoningEffort"] != "high" || params["modelReasoningEffort"] != "high" {
			t.Fatalf("thread/start reasoning params = %v/%v, want high/high", params["reasoningEffort"], params["modelReasoningEffort"])
		}
		if params["sandbox"] != "danger-full-access" || params["approvalMode"] != "never" {
			t.Fatalf("thread/start yolo params = sandbox:%v approval:%v", params["sandbox"], params["approvalMode"])
		}
	default:
		t.Fatal("did not observe thread/start")
	}
	select {
	case ok := <-observedInputArray:
		if !ok {
			t.Fatal("turn/start input was not encoded as generated app-server UserInput[]")
		}
	default:
		t.Fatal("did not observe turn/start")
	}
	got := bridge.types()
	if !containsType(got, EventMessageDelta) || !containsType(got, EventMessageCompleted) || got[len(got)-1] != EventTurnCompleted {
		t.Fatalf("expected generated app-server events to map, got %v", got)
	}
}

func TestCodexAdapterApprovalRoundTrip(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	adapter := newCodexAdapter(d, "/workspace")
	d.setInbound(adapter.handleInbound)

	decisionReplies := make(chan string, 1)
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "thread/start":
			fc.reply(m["id"], map[string]any{"threadId": "th1"})
		case "turn/start":
			fc.reply(m["id"], map[string]any{"turnId": "ct1"})
			// server asks for approval (server→client request). The real app-server
			// method is execCommandApproval (verified against codex-cli 0.137.0).
			fc.send(map[string]any{"jsonrpc": "2.0", "id": 500, "method": "execCommandApproval",
				"params": map[string]any{"threadId": "th1", "command": "rm -rf x"}})
		default:
			if res, ok := m["result"].(map[string]any); ok {
				if dec, ok := res["decision"].(string); ok {
					decisionReplies <- dec
					fc.notify("turn.completed", map[string]any{"threadId": "th1", "finalMessage": "ok"})
				}
			}
		}
	})

	bridge := &captureBridge{approveWith: "approve"}
	if err := adapter.SendTurn(context.Background(), TurnRequest{RunID: "r1", Prompt: "go"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	select {
	case dec := <-decisionReplies:
		// The bridge approves with the internal "approve"; the adapter must translate
		// it to Codex's ReviewDecision value "approved" on the wire (BUG-064).
		if dec != "approved" {
			t.Fatalf("decision = %q, want approved", dec)
		}
	default:
		t.Fatal("approval decision was not replied to Codex")
	}
}

func TestCodexAdapterV2ApprovalRoundTrip(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	adapter := newCodexAdapter(d, "/workspace")
	d.setInbound(adapter.handleInbound)

	decisionReplies := make(chan string, 1)
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "thread/start":
			fc.reply(m["id"], map[string]any{"threadId": "th1"})
		case "turn/start":
			fc.reply(m["id"], map[string]any{"turnId": "ct1"})
			fc.send(map[string]any{"jsonrpc": "2.0", "id": 500, "method": "item/commandExecution/requestApproval",
				"params": map[string]any{"threadId": "th1", "turnId": "ct1", "itemId": "item1", "command": "echo hi"}})
		default:
			if res, ok := m["result"].(map[string]any); ok {
				if dec, ok := res["decision"].(string); ok {
					decisionReplies <- dec
					fc.notify("turn.completed", map[string]any{"threadId": "th1", "finalMessage": "ok"})
				}
			}
		}
	})

	bridge := &captureBridge{approveWith: "approve"}
	if err := adapter.SendTurn(context.Background(), TurnRequest{RunID: "r1", Prompt: "go"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	select {
	case dec := <-decisionReplies:
		if dec != "accept" {
			t.Fatalf("decision = %q, want accept", dec)
		}
	default:
		t.Fatal("approval decision was not replied to Codex")
	}
}

func TestCodexAdapterPermissionsApprovalRoundTrip(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	adapter := newCodexAdapter(d, "/workspace")
	d.setInbound(adapter.handleInbound)

	approvalReplies := make(chan map[string]any, 1)
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "thread/start":
			fc.reply(m["id"], map[string]any{"threadId": "th1"})
		case "turn/start":
			fc.reply(m["id"], map[string]any{"turnId": "ct1"})
			fc.send(map[string]any{"jsonrpc": "2.0", "id": 500, "method": "item/permissions/requestApproval",
				"params": map[string]any{
					"threadId": "th1",
					"turnId":   "ct1",
					"itemId":   "item1",
					"cwd":      "/workspace",
					"reason":   "Allow google-drive MCP access",
					"permissions": map[string]any{
						"network": map[string]any{"enabled": true},
					},
				}})
		default:
			if res, ok := m["result"].(map[string]any); ok {
				approvalReplies <- res
				fc.notify("turn.completed", map[string]any{"threadId": "th1", "finalMessage": "ok"})
			}
		}
	})

	bridge := &captureBridge{approveWith: "approve"}
	if err := adapter.SendTurn(context.Background(), TurnRequest{RunID: "r1", Prompt: "use drive"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	select {
	case res := <-approvalReplies:
		if _, hasDecision := res["decision"]; hasDecision {
			t.Fatalf("permissions approval must not use decision response: %+v", res)
		}
		if res["scope"] != "turn" {
			t.Fatalf("scope = %v, want turn", res["scope"])
		}
		permissions, _ := res["permissions"].(map[string]any)
		network, _ := permissions["network"].(map[string]any)
		if network["enabled"] != true {
			t.Fatalf("network permission = %+v, want enabled true", network)
		}
	default:
		t.Fatal("permission approval was not replied to Codex")
	}
}

func TestCodexAdapterMcpElicitationApprovalRoundTrip(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	adapter := newCodexAdapter(d, "/workspace")
	d.setInbound(adapter.handleInbound)

	approvalReplies := make(chan map[string]any, 1)
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "thread/start":
			fc.reply(m["id"], map[string]any{"threadId": "th1"})
		case "turn/start":
			fc.reply(m["id"], map[string]any{"turnId": "ct1"})
			fc.send(map[string]any{"jsonrpc": "2.0", "id": 500, "method": "mcpServer/elicitation/request",
				"params": map[string]any{
					"threadId":   "th1",
					"turnId":     "ct1",
					"serverName": "google-drive",
					"mode":       "url",
					"message":    "Allow Google Drive MCP access",
					"url":        "https://example.test/approve",
				}})
		default:
			if res, ok := m["result"].(map[string]any); ok {
				approvalReplies <- res
				fc.notify("turn.completed", map[string]any{"threadId": "th1", "finalMessage": "ok"})
			}
		}
	})

	bridge := &captureBridge{approveWith: "approve"}
	if err := adapter.SendTurn(context.Background(), TurnRequest{RunID: "r1", Prompt: "use drive"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if got := bridge.approvalRequestCount(); got != 1 {
		t.Fatalf("approval requests = %d, want 1", got)
	}
	details := bridge.lastApprovalRequest()
	if details.Command != "MCP server: google-drive" || details.Reason != "Allow Google Drive MCP access" {
		t.Fatalf("approval details = %+v", details)
	}
	select {
	case res := <-approvalReplies:
		if res["action"] != "accept" {
			t.Fatalf("action = %v, want accept", res["action"])
		}
		if _, hasDecision := res["decision"]; hasDecision {
			t.Fatalf("mcp elicitation approval must not use decision response: %+v", res)
		}
	default:
		t.Fatal("mcp elicitation approval was not replied to Codex")
	}
}

func TestCodexAdapterYoloApprovalMatrix(t *testing.T) {
	cases := []struct {
		name                 string
		yolo                 bool
		approvalMethod       string
		approvalParams       map[string]any
		wantSandbox          string
		wantApprovalMode     string
		wantApprovalRequests int
		wantDecision         string
		wantPermissionScope  string
	}{
		{
			name:             "write tool yolo off prompts",
			yolo:             false,
			approvalMethod:   "item/commandExecution/requestApproval",
			approvalParams:   map[string]any{"threadId": "th1", "turnId": "ct1", "itemId": "item1", "command": "echo hi"},
			wantSandbox:      "workspace-write",
			wantApprovalMode: "untrusted",

			wantApprovalRequests: 1,
			wantDecision:         "accept",
		},
		{
			name:                 "write tool yolo on does not prompt",
			yolo:                 true,
			approvalMethod:       "item/commandExecution/requestApproval",
			approvalParams:       map[string]any{"threadId": "th1", "turnId": "ct1", "itemId": "item1", "command": "echo hi"},
			wantSandbox:          "danger-full-access",
			wantApprovalMode:     "never",
			wantApprovalRequests: 0,
		},
		{
			name:           "mcp permission yolo off prompts",
			yolo:           false,
			approvalMethod: "item/permissions/requestApproval",
			approvalParams: map[string]any{
				"threadId": "th1",
				"turnId":   "ct1",
				"itemId":   "item1",
				"cwd":      "/workspace",
				"reason":   "Allow google-drive MCP access",
				"permissions": map[string]any{
					"network": map[string]any{"enabled": true},
				},
			},
			wantSandbox:          "workspace-write",
			wantApprovalMode:     "untrusted",
			wantApprovalRequests: 1,
			wantPermissionScope:  "turn",
		},
		{
			name:           "mcp permission yolo on does not prompt",
			yolo:           true,
			approvalMethod: "item/permissions/requestApproval",
			approvalParams: map[string]any{
				"threadId": "th1",
				"turnId":   "ct1",
				"itemId":   "item1",
				"cwd":      "/workspace",
				"reason":   "Allow google-drive MCP access",
				"permissions": map[string]any{
					"network": map[string]any{"enabled": true},
				},
			},
			wantSandbox:          "danger-full-access",
			wantApprovalMode:     "never",
			wantApprovalRequests: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, fc := startFakeCodex(t, nil)
			adapter := newCodexAdapter(d, "/workspace")
			d.setInbound(adapter.handleInbound)

			threadParams := make(chan map[string]any, 1)
			approvalReplies := make(chan map[string]any, 1)
			fc.serve(func(fc *fakeCodex, m map[string]any) {
				method, _ := m["method"].(string)
				switch method {
				case "thread/start":
					params, _ := m["params"].(map[string]any)
					threadParams <- params
					fc.reply(m["id"], map[string]any{"threadId": "th1"})
				case "turn/start":
					fc.reply(m["id"], map[string]any{"turnId": "ct1"})
					if !tc.yolo {
						fc.send(map[string]any{"jsonrpc": "2.0", "id": 500, "method": tc.approvalMethod, "params": tc.approvalParams})
						return
					}
					fc.notify("turn.completed", map[string]any{"threadId": "th1", "finalMessage": "ok"})
				default:
					if res, ok := m["result"].(map[string]any); ok {
						approvalReplies <- res
						fc.notify("turn.completed", map[string]any{"threadId": "th1", "finalMessage": "ok"})
					}
				}
			})

			bridge := &captureBridge{approveWith: "approve"}
			if err := adapter.SendTurn(context.Background(), TurnRequest{RunID: "r1", Prompt: "go", YoloMode: tc.yolo}, bridge); err != nil {
				t.Fatalf("SendTurn: %v", err)
			}
			select {
			case params := <-threadParams:
				if params["sandbox"] != tc.wantSandbox || params["approvalMode"] != tc.wantApprovalMode {
					t.Fatalf("thread/start sandbox=%v approvalMode=%v, want %s/%s", params["sandbox"], params["approvalMode"], tc.wantSandbox, tc.wantApprovalMode)
				}
			default:
				t.Fatal("did not observe thread/start params")
			}
			if got := bridge.approvalRequestCount(); got != tc.wantApprovalRequests {
				t.Fatalf("approval requests = %d, want %d", got, tc.wantApprovalRequests)
			}
			if tc.wantDecision == "" && tc.wantPermissionScope == "" {
				select {
				case res := <-approvalReplies:
					t.Fatalf("unexpected approval reply in YOLO case: %+v", res)
				default:
				}
				return
			}
			select {
			case res := <-approvalReplies:
				if tc.wantDecision != "" && res["decision"] != tc.wantDecision {
					t.Fatalf("decision = %v, want %s", res["decision"], tc.wantDecision)
				}
				if tc.wantPermissionScope != "" && res["scope"] != tc.wantPermissionScope {
					t.Fatalf("scope = %v, want %s", res["scope"], tc.wantPermissionScope)
				}
			default:
				t.Fatal("approval reply was not sent")
			}
		})
	}
}

func TestCodexReviewDecisionMapsToAppServerEnum(t *testing.T) {
	cases := map[string]string{
		"approve":              "approved",
		"approved":             "approved",
		"approve_for_session":  "approved_for_session",
		"approved_for_session": "approved_for_session",
		"deny":                 "denied",
		"denied":               "denied",
		"abort":                "abort",
		"":                     "denied", // unknown/empty fails safe
		"garbage":              "denied",
	}
	for in, want := range cases {
		if got := codexReviewDecision("execCommandApproval", in); got != want {
			t.Errorf("codexReviewDecision(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCodexReviewDecisionMapsV2AppServerEnum(t *testing.T) {
	cases := map[string]string{
		"approve":              "accept",
		"approved":             "accept",
		"approve_for_session":  "acceptForSession",
		"approved_for_session": "acceptForSession",
		"deny":                 "decline",
		"denied":               "decline",
		"abort":                "cancel",
		"":                     "decline",
		"garbage":              "decline",
	}
	for in, want := range cases {
		if got := codexReviewDecision("item/commandExecution/requestApproval", in); got != want {
			t.Errorf("codexReviewDecision(%q) = %q, want %q", in, got, want)
		}
	}
}

func containsType(types []ProviderEventType, want ProviderEventType) bool {
	for _, t := range types {
		if t == want {
			return true
		}
	}
	return false
}
