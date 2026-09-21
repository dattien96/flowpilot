package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Task-401 tests: the Devin adapter on top of the Task-400 dispatcher.

type fakeDevinBridge struct {
	events    []ProviderEvent
	approvals []ApprovalDetails
	decision  string
}

func (b *fakeDevinBridge) Emit(ev ProviderEvent) { b.events = append(b.events, ev) }
func (b *fakeDevinBridge) Accepted(receipt ReceiptEvidence) {}
func (b *fakeDevinBridge) Terminal(proof TerminalEvidence) {}
func (b *fakeDevinBridge) RequestApproval(details ApprovalDetails) (string, error) {
	b.approvals = append(b.approvals, details)
	if b.decision != "" {
		return b.decision, nil
	}
	return "approve", nil
}
func (b *fakeDevinBridge) AskQuestion(prompt string, options []QuestionOption, multiSelect bool) ([]string, error) {
	return nil, nil
}
func (b *fakeDevinBridge) SpawnAgent(in SpawnAgentInput) (SpawnAgentResult, error) {
	return SpawnAgentResult{}, nil
}
func (b *fakeDevinBridge) SubmitFlowControl(in FlowControlInput) (FlowControlResult, error) {
	return FlowControlResult{}, nil
}

// serveDevinBaseline wires the fake to answer session/new with the live
// fixture and lets the caller handle prompt/etc.
func serveDevinBaseline(fd *fakeDevin, onPrompt func(m map[string]any)) {
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			var res map[string]any
			data, _ := os.ReadFile(filepath.Join("testdata", "devin_acp", "session_new_result.json"))
			_ = json.Unmarshal(data, &res)
			fd.reply(m["id"], res)
		case "session/load":
			var res map[string]any
			data, _ := os.ReadFile(filepath.Join("testdata", "devin_acp", "session_load_result.json"))
			_ = json.Unmarshal(data, &res)
			fd.reply(m["id"], res)
		case "session/set_config_option":
			fd.reply(m["id"], map[string]any{"configOptions": []any{}})
		case "session/prompt":
			if onPrompt != nil {
				onPrompt(m)
			}
		}
	})
}

func TestDevinAdapterSendTurnStreamsAndCompletes(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	serveDevinBaseline(fd, func(m map[string]any) {
		go func() {
			fd.notify("session/update", map[string]any{"sessionId": "working-pentagon", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "Hello"}}})
			time.Sleep(10 * time.Millisecond)
			fd.reply(m["id"], map[string]any{"stopReason": "end_turn", "usage": map[string]any{"inputTokens": float64(10), "outputTokens": float64(5), "totalTokens": float64(15)}})
		}()
	})
	a := newDevinAdapter(d, "/tmp")
	store := &recordingSessionStore{}
	a.sessionStore = store
	bridge := &fakeDevinBridge{}
	req := TurnRequest{RunID: "run-1", Prompt: "hello", Cwd: "/tmp"}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	var sawDelta, sawCompleted bool
	var finalMsg string
	for _, ev := range bridge.events {
		if ev.Type == EventMessageDelta && strings.Contains(ev.Text, "Hello") {
			sawDelta = true
		}
		if ev.Type == EventTurnCompleted {
			sawCompleted = true
			finalMsg = ev.FinalMessage
		}
	}
	if !sawDelta {
		t.Fatalf("expected message_delta, got %+v", bridge.events)
	}
	if !sawCompleted {
		t.Fatalf("expected turn_completed, got %+v", bridge.events)
	}
	if finalMsg == "" {
		t.Fatal("expected final message")
	}
	if len(store.records) == 0 {
		t.Fatal("expected session persist")
	}
	if store.records[0].ProviderKey != string(ProviderKeyDevin) {
		t.Fatalf("providerKey: %q", store.records[0].ProviderKey)
	}
}

func TestDevinAdapterSendTurnWithToolAndFileEvents(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	serveDevinBaseline(fd, func(m map[string]any) {
		go func() {
			fd.notify("session/update", map[string]any{"sessionId": "working-pentagon", "update": map[string]any{"sessionUpdate": "tool_call", "title": "write_file", "toolCallId": "call_1", "rawInput": map[string]any{"filePath": "/tmp/a.txt"}}})
			fd.notify("session/update", map[string]any{"sessionId": "working-pentagon", "update": map[string]any{"sessionUpdate": "tool_call_update", "status": "completed", "title": "write_file", "kind": "edit", "toolCallId": "call_1", "locations": []any{map[string]any{"path": "/tmp/a.txt"}}}})
			fd.reply(m["id"], map[string]any{"stopReason": "end_turn"})
		}()
	})
	a := newDevinAdapter(d, "/tmp")
	bridge := &fakeDevinBridge{}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-1", Prompt: "write file", Cwd: "/tmp"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	var sawToolStarted, sawToolCompleted, sawFileChanged bool
	for _, ev := range bridge.events {
		switch ev.Type {
		case EventToolStarted:
			sawToolStarted = true
		case EventToolCompleted:
			sawToolCompleted = true
		case EventFileChanged:
			if ev.Path == "/tmp/a.txt" {
				sawFileChanged = true
			}
		}
	}
	if !sawToolStarted || !sawToolCompleted || !sawFileChanged {
		t.Fatalf("expected tool_started+tool_completed+file_changed, got %+v", bridge.events)
	}
}

// Session resume: second turn on the same run uses session/load with the
// slug id captured from the first turn (devin/<slug>).
func TestDevinAdapterResumesSlugSession(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	var loadCalls []string
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fd.reply(m["id"], map[string]any{"sessionId": "quiet-otter"})
		case "session/load":
			params, _ := m["params"].(map[string]any)
			loadCalls = append(loadCalls, params["sessionId"].(string))
			fd.reply(m["id"], map[string]any{"modes": map[string]any{"currentModeId": "accept-edits"}})
		case "session/set_config_option":
			fd.reply(m["id"], map[string]any{"configOptions": []any{}})
		case "session/prompt":
			go func() {
				fd.notify("session/update", map[string]any{"sessionId": "quiet-otter", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "ok"}}})
				fd.reply(m["id"], map[string]any{"stopReason": "end_turn"})
			}()
		}
	})
	a := newDevinAdapter(d, "/tmp")
	a.runSessions = &devinRunSessionIndex{byRun: map[string]string{}, bySession: map[string]string{}}
	bridge := &fakeDevinBridge{}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-1", Prompt: "one", Cwd: "/tmp"}, bridge); err != nil {
		t.Fatalf("turn1: %v", err)
	}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-1", ProviderSessionID: "quiet-otter", Prompt: "two", Cwd: "/tmp"}, bridge); err != nil {
		t.Fatalf("turn2: %v", err)
	}
	if len(loadCalls) != 1 || loadCalls[0] != "quiet-otter" {
		t.Fatalf("expected session/load quiet-otter, got %v", loadCalls)
	}
}

// A synthetic thread-* id or another run's session must force session/new.
func TestDevinAdapterSyntheticAndForeignSessionsForceNew(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	var methods []string
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		if m["method"] == "session/new" || m["method"] == "session/load" {
			methods = append(methods, m["method"].(string))
		}
		switch m["method"] {
		case "session/new":
			fd.reply(m["id"], map[string]any{"sessionId": "fresh-slug"})
		case "session/load":
			fd.reply(m["id"], map[string]any{})
		case "session/set_config_option":
			fd.reply(m["id"], map[string]any{})
		case "session/prompt":
			fd.reply(m["id"], map[string]any{"stopReason": "end_turn", "text": "x"})
		}
	})
	a := newDevinAdapter(d, "/tmp")
	a.runSessions = &devinRunSessionIndex{byRun: map[string]string{"other-run": "owned-slug"}, bySession: map[string]string{"owned-slug": "other-run"}}
	bridge := &fakeDevinBridge{}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-1", ProviderSessionID: "thread-9", Prompt: "hi"}, bridge); err != nil {
		t.Fatalf("thread-*: %v", err)
	}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-1", ProviderSessionID: "owned-slug", Prompt: "hi"}, bridge); err != nil {
		t.Fatalf("foreign-owned: %v", err)
	}
	if len(methods) != 2 || methods[0] != "session/new" || methods[1] != "session/new" {
		t.Fatalf("expected 2x session/new, got %v", methods)
	}
}

// Model + mode are applied via session/set_config_option per turn.
func TestDevinAdapterAppliesModelAndModeConfig(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	var cfgCalls []map[string]any
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fd.reply(m["id"], map[string]any{"sessionId": "calm-finch"})
		case "session/set_config_option":
			params, _ := m["params"].(map[string]any)
			cfgCalls = append(cfgCalls, map[string]any{"configId": params["configId"], "value": params["value"]})
			fd.reply(m["id"], map[string]any{})
		case "session/prompt":
			fd.reply(m["id"], map[string]any{"stopReason": "end_turn", "text": "x"})
		}
	})
	a := newDevinAdapter(d, "/tmp")
	bridge := &fakeDevinBridge{}
	req := TurnRequest{RunID: "run-1", Prompt: "hi", ModelName: "devin/claude-opus-5-high", YoloMode: true}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	var modelSet, modeSet string
	for _, c := range cfgCalls {
		if c["configId"] == "model" {
			modelSet = c["value"].(string)
		}
		if c["configId"] == "mode" {
			modeSet = c["value"].(string)
		}
	}
	if modelSet != "claude-opus-5-high" {
		t.Fatalf("expected devin/ prefix stripped -> claude-opus-5-high, got %q", modelSet)
	}
	if modeSet != "bypass" {
		t.Fatalf("yolo=true must map mode bypass, got %q", modeSet)
	}
}

func TestDevinResolveSessionMode(t *testing.T) {
	cases := []struct {
		name           string
		yolo           bool
		forceBridge    bool
		chatPosture    string
		flowNodePostur string
		want           string
	}{
		{"code default", false, false, "", "", "smart"},
		{"code explicit", false, false, "code", "", "smart"},
		{"yolo bypass", true, false, "", "", "bypass"},
		{"yolo + forceShellBridge gates", true, true, "", "", "accept-edits"},
		{"scan posture", false, false, "scan", "", "ask"},
		{"plan posture", true, false, "plan", "", "plan"},
		{"scan beats yolo", true, false, "scan", "", "ask"},
		{"read_only flow node", true, false, "", "read_only", "ask"},
		{"verdict_only flow node", false, false, "", "verdict_only", "ask"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveDevinSessionMode(tc.yolo, tc.forceBridge, tc.chatPosture, tc.flowNodePostur); got != tc.want {
				t.Fatalf("resolveDevinSessionMode = %q, want %q", got, tc.want)
			}
		})
	}
}

// YOLO auto-approves tool permission in-adapter; non-YOLO routes to the bridge.
func TestDevinAdapterPermissionGateAndYolo(t *testing.T) {
	newAdapter := func() (*devinAdapter, *fakeDevin) {
		d, fd := startFakeDevin(t, nil)
		serveDevinBaseline(fd, func(m map[string]any) {
			go func() {
				// Emit a permission request mid-turn, then wait for our reply
				// (the fake inbound path replies via the adapter's inbound).
				fd.send(map[string]any{"jsonrpc": "2.0", "id": 77, "method": "session/request_permission", "params": map[string]any{
					"sessionId": "working-pentagon",
					"toolCall":  map[string]any{"toolCallId": "tc", "title": "bash", "kind": "execute", "rawInput": map[string]any{"command": "rm -rf x"}},
					"options": []any{
						map[string]any{"optionId": "allow", "kind": "allow_once"},
						map[string]any{"optionId": "reject", "kind": "reject_once"},
					},
				}})
				time.Sleep(20 * time.Millisecond)
				fd.notify("session/update", map[string]any{"sessionId": "working-pentagon", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "done"}}})
				fd.reply(m["id"], map[string]any{"stopReason": "end_turn"})
			}()
		})
		return newDevinAdapter(d, "/tmp"), fd
	}

	t.Run("yolo auto-approves without bridge", func(t *testing.T) {
		a, _ := newAdapter()
		bridge := &fakeDevinBridge{}
		req := TurnRequest{RunID: "r", Prompt: "x", YoloMode: true}
		if err := a.SendTurn(context.Background(), req, bridge); err != nil {
			t.Fatalf("SendTurn: %v", err)
		}
		if len(bridge.approvals) != 0 {
			t.Fatal("yolo must auto-approve in-adapter, never reaching the bridge")
		}
	})

	t.Run("deny terminates cleanly", func(t *testing.T) {
		a, _ := newAdapter()
		bridge := &fakeDevinBridge{decision: "deny"}
		req := TurnRequest{RunID: "r", Prompt: "x", YoloMode: false}
		if err := a.SendTurn(context.Background(), req, bridge); err != nil {
			t.Fatalf("SendTurn: %v", err)
		}
		if len(bridge.approvals) != 1 {
			t.Fatalf("expected one approval card, got %d", len(bridge.approvals))
		}
		if bridge.approvals[0].Kind != "exec" {
			t.Fatalf("expected exec kind, got %q", bridge.approvals[0].Kind)
		}
		// Denied + text streamed → still completes (no blank turn).
		var sawCompleted bool
		for _, ev := range bridge.events {
			if ev.Type == EventTurnCompleted {
				sawCompleted = true
			}
		}
		if !sawCompleted {
			t.Fatal("expected turn_completed after deny")
		}
	})

	t.Run("approve path emits approval detail", func(t *testing.T) {
		a, _ := newAdapter()
		bridge := &fakeDevinBridge{decision: "approve"}
		req := TurnRequest{RunID: "r", Prompt: "x"}
		if err := a.SendTurn(context.Background(), req, bridge); err != nil {
			t.Fatalf("SendTurn: %v", err)
		}
		if len(bridge.approvals) != 1 || bridge.approvals[0].Command != "rm -rf x" {
			t.Fatalf("approval details: %+v", bridge.approvals)
		}
	})
}

// Quota/usage-limit stopReason and RPC errors must fail the turn (BUG-361
// parity), not blank-complete.
func TestDevinAdapterQuotaFailure(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	serveDevinBaseline(fd, func(m map[string]any) {
		fd.reply(m["id"], map[string]any{"stopReason": "rate_limit"})
	})
	a := newDevinAdapter(d, "/tmp")
	bridge := &fakeDevinBridge{}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "r", Prompt: "x"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	var failed bool
	var msg string
	for _, ev := range bridge.events {
		if ev.Type == EventTurnFailed {
			failed = true
			msg = ev.Error
		}
	}
	if !failed || !strings.Contains(msg, "usage limit") {
		t.Fatalf("expected usage-limit turn_failed, got %+v", bridge.events)
	}
}

// devin/ prefix must be stripped before hitting the wire; bare ids pass.
func TestDevinModelIDForACP(t *testing.T) {
	if got := devinModelIDForACP("devin/swe-2-high"); got != "swe-2-high" {
		t.Fatalf("strip prefix: %q", got)
	}
	if got := devinModelIDForACP("swe-2-high"); got != "swe-2-high" {
		t.Fatalf("bare id: %q", got)
	}
	if got := devinModelIDForACP("  "); got != "" {
		t.Fatalf("blank: %q", got)
	}
}

// Reasoning effort rides inside the model id (swe-2-<effort> suffix swap).
func TestDevinModelForEffort(t *testing.T) {
	cases := []struct{ model, effort, want string }{
		{"swe-2-high", "low", "swe-2-low"},
		{"swe-2-high", "high", ""},          // already high → no-op
		{"swe-2-high", "xhigh", "swe-2-xhigh"},
		{"claude-opus-5-medium", "max", "claude-opus-5-max"},
		{"adaptive", "high", "adaptive-high"}, // family-only id gets suffix
		{"swe-2-high", "bogus", ""},           // unknown effort → keep
		{"swe-2-high", "", ""},                // no effort → keep
	}
	for _, tc := range cases {
		if got := devinModelForEffort(tc.model, tc.effort); got != tc.want {
			t.Fatalf("devinModelForEffort(%q,%q)=%q want %q", tc.model, tc.effort, got, tc.want)
		}
	}
}

// The stdio shim entry is the only FlowPilot MCP transport (http unsupported).
func TestDevinAdapterMCPShimEntry(t *testing.T) {
	d, _ := startFakeDevin(t, nil)
	a := newDevinAdapter(d, "/tmp")
	a.mcpBaseURL = func() string { return "http://127.0.0.1:7777" }
	a.mcpShimCommand = func() (string, []string) { return "/bin/fake", []string{"devin-mcp-stdio"} }
	entry := a.devinFlowPilotMCPEntry("tok123")
	if entry == nil {
		t.Fatal("expected stdio entry")
	}
	if entry["type"] != nil || entry["name"] != "flowpilot" {
		t.Fatalf("entry: %v", entry)
	}
	args, _ := entry["args"].([]string)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "devin-mcp-stdio") || !strings.Contains(joined, "token=tok123") {
		t.Fatalf("args must carry shim + token url: %v", args)
	}
	// No url/token → nil entry (never inject a broken server).
	a2 := newDevinAdapter(d, "/tmp")
	a2.mcpShimCommand = func() (string, []string) { return "/bin/fake", []string{"x"} }
	if a2.devinFlowPilotMCPEntry("") != nil {
		t.Fatal("empty token must yield nil")
	}
}

// Denied permission with no streamed text replays via session/load, then
// falls back to the honest no-reply notice (CA-712/713 analog).
func TestDevinAdapterDeniedEmptyAnswerNotice(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	var sawLoad bool
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fd.reply(m["id"], map[string]any{"sessionId": "still-coral"})
		case "session/load":
			sawLoad = true
			fd.reply(m["id"], map[string]any{})
		case "session/set_config_option":
			fd.reply(m["id"], map[string]any{})
		case "session/prompt":
			go func() {
				fd.send(map[string]any{"jsonrpc": "2.0", "id": 55, "method": "session/request_permission", "params": map[string]any{
					"sessionId": "still-coral",
					"toolCall":  map[string]any{"title": "bash", "kind": "execute"},
					"options":   []any{map[string]any{"optionId": "allow", "kind": "allow_once"}, map[string]any{"optionId": "reject", "kind": "reject_once"}},
				}})
				time.Sleep(15 * time.Millisecond)
				fd.reply(m["id"], map[string]any{"stopReason": "end_turn"})
			}()
		}
	})
	a := newDevinAdapter(d, "/tmp")
	bridge := &fakeDevinBridge{decision: "deny"}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "r", Prompt: "x"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if !sawLoad {
		t.Fatal("expected session/load replay attempt after denied empty answer")
	}
	var notice string
	for _, ev := range bridge.events {
		if ev.Type == EventMessageDelta && strings.Contains(ev.Text, "no reply text") {
			notice = ev.Text
		}
		if ev.Type == EventTurnCompleted {
			if strings.TrimSpace(ev.FinalMessage) == "" {
				t.Fatal("blank finalMessage after deny — the notice must fill it")
			}
		}
	}
	if notice == "" {
		t.Fatalf("expected no-reply notice, got %+v", bridge.events)
	}
}

// Devin lazy MCP lifecycle: the stdio shim only comes up after session/prompt
// begins, so SendTurn must NOT block on the Claude-style tools/list readiness
// gate. A token that never signals ready used to cost the full 30s timeout.
func TestDevinSendTurnDoesNotWaitForClaudeMCPReadyTimeout(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	var promptSent bool
	serveDevinBaseline(fd, func(m map[string]any) {
		promptSent = true
		fd.reply(m["id"], map[string]any{"stopReason": "end_turn", "text": "ok"})
	})
	a := newDevinAdapter(d, t.TempDir())
	a.mcpServer = newClaudeMCPServer()
	a.mcpBaseURL = func() string { return "http://127.0.0.1:7777" }
	a.mcpShimCommand = func() (string, []string) { return "/bin/fake", []string{"devin-mcp-stdio"} }
	bridge := &fakeDevinBridge{}
	start := time.Now()
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-1", Prompt: "hi"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if !promptSent {
		t.Fatal("session/prompt never dispatched")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("SendTurn blocked %s — readiness gate must not gate Devin prompts", elapsed)
	}
}

// Removing the readiness gate must not remove the wiring: session/new still
// carries the stdio shim entry with the per-turn token, the token stays
// registered for the whole prompt, and is released after the turn.
func TestDevinSendTurnPreservesMCPServerEntryWithoutReadyWait(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	var mcpToken string
	var a *devinAdapter
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			params, _ := m["params"].(map[string]any)
			servers, _ := params["mcpServers"].([]any)
			for _, s := range servers {
				entry, _ := s.(map[string]any)
				args, _ := entry["args"].([]any)
				for _, arg := range args {
					str, _ := arg.(string)
					if i := strings.Index(str, "token="); i >= 0 {
						mcpToken = str[i+len("token="):]
					}
				}
			}
			fd.reply(m["id"], map[string]any{"sessionId": "lazy-otter"})
		case "session/set_config_option":
			fd.reply(m["id"], map[string]any{})
		case "session/prompt":
			// Mid-prompt the token must still resolve to this bridge.
			if mcpToken == "" {
				t.Error("no token in session/new mcpServers")
			} else if a.mcpServer.bridgeFor(mcpToken) == nil {
				t.Error("token unregistered while prompt in flight")
			}
			fd.reply(m["id"], map[string]any{"stopReason": "end_turn", "text": "ok"})
		}
	})
	a = newDevinAdapter(d, t.TempDir())
	a.mcpServer = newClaudeMCPServer()
	a.mcpBaseURL = func() string { return "http://127.0.0.1:7777" }
	a.mcpShimCommand = func() (string, []string) { return "/bin/fake", []string{"devin-mcp-stdio"} }
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-1", Prompt: "hi"}, &fakeDevinBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if mcpToken == "" {
		t.Fatal("session/new lost the FlowPilot stdio shim entry")
	}
	if a.mcpServer.bridgeFor(mcpToken) != nil {
		t.Fatal("token still registered after turn — unregister leak")
	}
}

// Resume path: .devin/mcp_config.local.json is the only MCP source
// session/load honors, so it must exist before load and carry the shim.
func TestDevinResumeWritesLocalMCPConfigBeforeSessionLoad(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	cwd := t.TempDir()
	var loadSawConfig bool
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/load":
			if _, err := os.Stat(filepath.Join(cwd, ".devin", "mcp_config.local.json")); err == nil {
				loadSawConfig = true
			}
			fd.reply(m["id"], map[string]any{"modes": map[string]any{"currentModeId": "accept-edits"}})
		case "session/set_config_option":
			fd.reply(m["id"], map[string]any{})
		case "session/prompt":
			fd.reply(m["id"], map[string]any{"stopReason": "end_turn", "text": "ok"})
		}
	})
	a := newDevinAdapter(d, cwd)
	a.mcpServer = newClaudeMCPServer()
	a.mcpBaseURL = func() string { return "http://127.0.0.1:7777" }
	a.mcpShimCommand = func() (string, []string) { return "/bin/fake", []string{"devin-mcp-stdio"} }
	req := TurnRequest{RunID: "run-1", ProviderSessionID: "quiet-otter", Prompt: "hi", Cwd: cwd}
	if err := a.SendTurn(context.Background(), req, &fakeDevinBridge{}); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if !loadSawConfig {
		t.Fatal("session/load ran before mcp_config.local.json was written")
	}
	doc, _ := readDevinMcpConfig(filepath.Join(cwd, ".devin", "mcp_config.local.json"))
	if doc == nil {
		t.Fatal("mcp_config.local.json missing")
	}
	raw, _ := json.Marshal(doc)
	if !strings.Contains(string(raw), "devin-mcp-stdio") {
		t.Fatalf("local config lost the flowpilot shim: %s", raw)
	}
}
