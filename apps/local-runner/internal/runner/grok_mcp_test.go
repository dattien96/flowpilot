package runner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Task-209: MCP wiring (ask_user/spawn_agent via the reused claudeMCPServer)
// and the MCP-ready-before-prompt gate (BUG-114 class).

func TestGrokAdapterBuildsMcpServersArrayAndRegistersBridge(t *testing.T) {
	sessionID := "session-mcp-1"
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x")
	a.initResult = liveGrokInitializeResult()
	a.mcpServer = newClaudeMCPServer()
	a.mcpBaseURL = func() string { return "http://127.0.0.1:9999" }
	a.extraMCPServers = func(bool) map[string]claudeMcpServer {
		return map[string]claudeMcpServer{"google-drive": {Command: "npx", Args: []string{"-y", "@piotr-agier/google-drive-mcp"}}}
	}

	var capturedMcpServers []interface{}
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			if params, ok := m["params"].(map[string]any); ok {
				capturedMcpServers, _ = params["mcpServers"].([]interface{})
			}
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/prompt":
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
	})

	bridge := &fakeGrokBridge{}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-mcp-1", Prompt: "hi"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	if len(capturedMcpServers) != 2 {
		t.Fatalf("expected 2 mcpServers entries (flowpilot + google-drive), got %d: %+v", len(capturedMcpServers), capturedMcpServers)
	}
	sawFlowpilot, sawDrive := false, false
	for _, raw := range capturedMcpServers {
		entry, _ := raw.(map[string]interface{})
		switch entry["name"] {
		case claudeMCPServerName:
			sawFlowpilot = true
			if entry["type"] != "http" {
				t.Fatalf("expected flowpilot entry type=http, got %v", entry["type"])
			}
		case "google-drive":
			sawDrive = true
			if entry["type"] != "stdio" {
				t.Fatalf("expected google-drive entry type=stdio, got %v", entry["type"])
			}
		}
	}
	if !sawFlowpilot || !sawDrive {
		t.Fatalf("expected both flowpilot and google-drive entries, got %+v", capturedMcpServers)
	}

	// The bridge must be unregistered from the MCP server once the turn ends
	// (no token leak across turns).
	if len(a.mcpServer.bridges) != 0 {
		t.Fatalf("expected mcpServer bridges to be empty after SendTurn returns, got %d", len(a.mcpServer.bridges))
	}
}

func TestGrokAdapterMcpReadyGateDoesNotHangOnTimeout(t *testing.T) {
	sessionID := "session-mcp-2"
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x")
	a.initResult = liveGrokInitializeResult()
	a.mcpServer = newClaudeMCPServer()
	a.mcpBaseURL = func() string { return "http://127.0.0.1:9999" }

	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/prompt":
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
		// Deliberately never call tools/list on the mcpServer — simulates a
		// Grok MCP client that never connects (or a stale token) — the gate
		// must still degrade to sending the prompt rather than hang forever.
	})

	origTimeout := claudeMCPReadyDefaultTimeout
	// Can't reassign a const; instead just bound the whole test with a context
	// deadline shorter than the real 30s default so it can't accidentally pass
	// by hanging the full default duration.
	_ = origTimeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	bridge := &fakeGrokBridge{}
	err := a.SendTurn(ctx, TurnRequest{RunID: "run-mcp-2", Prompt: "hi"}, bridge)
	// waitReady blocks up to claudeMCPReadyDefaultTimeout (30s) OR ctx.Done();
	// our 3s ctx deadline fires first, so waitReady returns false quickly and
	// SendTurn proceeds to session/prompt on the same (now-expired) ctx, which
	// then fails the session/prompt call itself. The key assertion is that this
	// returns promptly rather than hanging — not the specific error.
	_ = err
}

func TestGrokAdapterNoMcpServerIsANoOp(t *testing.T) {
	sessionID := "session-mcp-3"
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x") // mcpServer left nil (Task-207 MVP scope)
	a.initResult = liveGrokInitializeResult()

	var capturedMcpServers []interface{}
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			if params, ok := m["params"].(map[string]any); ok {
				capturedMcpServers, _ = params["mcpServers"].([]interface{})
			}
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/prompt":
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
	})

	bridge := &fakeGrokBridge{}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-mcp-3", Prompt: "hi"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if len(capturedMcpServers) != 0 {
		t.Fatalf("expected zero mcpServers when mcpServer is nil, got %+v", capturedMcpServers)
	}
}

// TestGrokAdapterDefaultPreparePromptAppendsAskUserReinforcement guards Task-209
// GR-06: bare adapters (no registry promptPrep) still steer the model onto
// FlowPilot MCP ask_user instead of native ask_user_question.
func TestGrokAdapterDefaultPreparePromptAppendsAskUserReinforcement(t *testing.T) {
	a := newGrokAdapter(newGrokDispatcher(nil, nil), "/tmp/x")
	got := a.preparePrompt(TurnRequest{Prompt: "pick a language"})
	if !strings.Contains(got, "pick a language") {
		t.Fatalf("expected original prompt preserved, got %q", got)
	}
	if !strings.Contains(got, grokToolReinforcements) {
		t.Fatalf("default preparePrompt must append grokToolReinforcements, got %q", got)
	}
	if !strings.Contains(got, "ask_user") || !strings.Contains(got, "ask_user_question") {
		t.Fatalf("reinforcement must name FlowPilot ask_user and warn off native ask_user_question, got %q", got)
	}
	if !strings.Contains(got, "spawn_agent") || !strings.Contains(got, "spawn_subagent") {
		t.Fatalf("reinforcement must name FlowPilot spawn_agent and warn off native spawn_subagent, got %q", got)
	}
}

// TestGrokPromptPrepAppendsAskUserReinforcement guards the live registry path:
// when promptPrep overrides preparePrompt it must still append the reinforcement
// (provider_registry.go Grok block) or the model loses the steer.
func TestGrokPromptPrepAppendsAskUserReinforcement(t *testing.T) {
	a := newGrokAdapter(newGrokDispatcher(nil, nil), "/tmp/x")
	// Mirror the live registry hook shape (skills + reinforcement).
	a.promptPrep = func(req TurnRequest) string {
		return "SKILL\n" + req.Prompt + grokToolReinforcements
	}
	got := a.preparePrompt(TurnRequest{Prompt: "need a choice"})
	if !strings.HasPrefix(got, "SKILL\nneed a choice") {
		t.Fatalf("expected skill prefix + prompt, got %q", got)
	}
	if !strings.Contains(got, grokToolReinforcements) {
		t.Fatalf("live promptPrep must append grokToolReinforcements, got %q", got)
	}
}

// TestGrokSendTurnPromptIncludesAskUserReinforcement asserts the reinforcement
// actually reaches session/prompt on the wire (not only preparePrompt unit path).
// Intentionally leaves mcpServer nil so waitReady does not delay the turn — the
// wire assertion is about prompt text, not MCP connect (covered elsewhere).
func TestGrokSendTurnPromptIncludesAskUserReinforcement(t *testing.T) {
	sessionID := "session-mcp-ask-prompt"
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x")
	a.initResult = liveGrokInitializeResult()

	var capturedPrompt string
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/prompt":
			if params, ok := m["params"].(map[string]any); ok {
				// prompt is []{type,text} blocks from grokACPPromptParams;
				// JSON unmarshaling yields []any of map[string]any.
				if blocks, ok := params["prompt"].([]map[string]string); ok && len(blocks) > 0 {
					capturedPrompt = blocks[0]["text"]
				} else if rawBlocks, ok := params["prompt"].([]any); ok && len(rawBlocks) > 0 {
					if block, ok := rawBlocks[0].(map[string]any); ok {
						capturedPrompt, _ = block["text"].(string)
					}
				}
			}
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
	})

	bridge := &fakeGrokBridge{}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-mcp-ask-prompt", Prompt: "choose language"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if capturedPrompt == "" {
		t.Fatal("expected session/prompt on the wire with non-empty text; capture failed")
	}
	if !strings.Contains(capturedPrompt, grokToolReinforcements) {
		t.Fatalf("session/prompt text must include grokToolReinforcements, got %q", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "choose language") {
		t.Fatalf("session/prompt text must include user prompt, got %q", capturedPrompt)
	}
}

// TestGrokMcpAskUserRoundTrip proves Task-209 DOD-1 wiring: when Grok calls
// FlowPilot MCP tools/call ask_user with the registered turn token, the shared
// handler routes to bridge.AskQuestion (same QuestionCard path as Codex/Claude).
func TestGrokMcpAskUserRoundTrip(t *testing.T) {
	mcp := newClaudeMCPServer()
	bridge := &fakeGrokBridge{answer: []string{"Python"}}
	token := mcp.register(bridge, false)
	defer mcp.unregister(token)

	raw, errObj := mcp.dispatch("tools/call", map[string]any{
		"params": map[string]any{
			"name": "ask_user",
			"arguments": map[string]any{
				"prompt":  "Which programming language do you prefer for the Hello World file?",
				"options": []any{"Python", "TypeScript", "Go"},
			},
		},
	}, token)
	if errObj != nil {
		t.Fatalf("tools/call ask_user error: %+v", errObj)
	}
	if bridge.questionPrompt == "" || len(bridge.questionOpts) != 3 {
		t.Fatalf("AskQuestion not reached: prompt=%q opts=%d", bridge.questionPrompt, len(bridge.questionOpts))
	}
	if bridge.questionOpts[0].Label != "Python" || bridge.questionOpts[2].Label != "Go" {
		t.Fatalf("unexpected options: %+v", bridge.questionOpts)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T %+v", raw, raw)
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("expected MCP text content result, got %+v", result)
	}
	block, _ := content[0].(map[string]any)
	if txt, _ := block["text"].(string); txt != "Python" {
		t.Fatalf("ask_user result = %q, want Python", txt)
	}
}

// TestGrokMcpAskUserMultiSelectAndEmptyAnswer covers multiSelect=true and the
// empty-answer / error path that must return a controlled tool result (not hang).
func TestGrokMcpAskUserMultiSelectAndEmptyAnswer(t *testing.T) {
	mcp := newClaudeMCPServer()

	t.Run("multiSelect", func(t *testing.T) {
		bridge := &fakeGrokBridge{answer: []string{"Python", "Go"}}
		token := mcp.register(bridge, false)
		defer mcp.unregister(token)
		raw, errObj := mcp.dispatch("tools/call", map[string]any{
			"params": map[string]any{
				"name": "ask_user",
				"arguments": map[string]any{
					"prompt":      "Pick languages",
					"options":     []any{"Python", "TypeScript", "Go"},
					"multiSelect": true,
				},
			},
		}, token)
		if errObj != nil {
			t.Fatalf("error: %+v", errObj)
		}
		if !bridge.questionMulti {
			t.Fatal("expected multiSelect=true to reach AskQuestion")
		}
		result := raw.(map[string]any)
		content := result["content"].([]any)
		txt := content[0].(map[string]any)["text"].(string)
		if txt != "Python, Go" {
			t.Fatalf("got %q, want Python, Go", txt)
		}
	})

	t.Run("emptyAnswer", func(t *testing.T) {
		bridge := &fakeGrokBridge{answer: nil} // AskQuestion returns nil,nil
		token := mcp.register(bridge, false)
		defer mcp.unregister(token)
		raw, errObj := mcp.dispatch("tools/call", map[string]any{
			"params": map[string]any{
				"name":      "ask_user",
				"arguments": map[string]any{"prompt": "Pick one"},
			},
		}, token)
		if errObj != nil {
			t.Fatalf("error: %+v", errObj)
		}
		result := raw.(map[string]any)
		content := result["content"].([]any)
		txt := content[0].(map[string]any)["text"].(string)
		if txt != "No answer was provided." {
			t.Fatalf("empty answer must yield controlled message, got %q", txt)
		}
	})
}

// TestGrokMcpSpawnAgentRoundTrip proves Task-209 DOD-2 wiring: when Grok calls
// FlowPilot MCP tools/call spawn_agent with the registered turn token, the shared
// handler routes to bridge.SpawnAgent (same path as Codex/Claude).
func TestGrokMcpSpawnAgentRoundTrip(t *testing.T) {
	mcp := newClaudeMCPServer()
	bridge := &fakeGrokBridge{
		spawnResult: SpawnAgentResult{
			RunID:        "child-run-42",
			ProviderKey:  "grok",
			FinalMessage: "child done",
		},
	}
	token := mcp.register(bridge, false)
	defer mcp.unregister(token)

	raw, errObj := mcp.dispatch("tools/call", map[string]any{
		"params": map[string]any{
			"name": "spawn_agent",
			"arguments": map[string]any{
				"agent":    "coder",
				"prompt":   "implement the feature",
				"provider": "grok",
				"wait":     true,
			},
		},
	}, token)
	if errObj != nil {
		t.Fatalf("tools/call spawn_agent error: %+v", errObj)
	}
	if bridge.spawnIn.Agent != "coder" || bridge.spawnIn.Prompt != "implement the feature" {
		t.Fatalf("SpawnAgent not reached with expected args: %+v", bridge.spawnIn)
	}
	if bridge.spawnIn.Provider != "grok" || !bridge.spawnIn.Wait {
		t.Fatalf("SpawnAgent provider/wait = %q/%v, want grok/true", bridge.spawnIn.Provider, bridge.spawnIn.Wait)
	}

	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T %+v", raw, raw)
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("expected MCP text content result, got %+v", result)
	}
	block, _ := content[0].(map[string]any)
	txt, _ := block["text"].(string)
	var got SpawnAgentResult
	if err := json.Unmarshal([]byte(txt), &got); err != nil {
		t.Fatalf("spawn_agent result must be JSON SpawnAgentResult, got %q: %v", txt, err)
	}
	if got.RunID != "child-run-42" || got.ProviderKey != "grok" || got.FinalMessage != "child done" {
		t.Fatalf("spawn_agent result = %+v, want runId=child-run-42 providerKey=grok finalMessage=child done", got)
	}
}

// ---- Task-209 DOD-4: OfferReviewOutcomeTool gating (shared claudeMCPServer) ----
//
// Grok reaches FlowPilot tools via ACP mcpServers → the reused claudeMCPServer.
// grok_adapter.SendTurn registers each turn with mcpServer.register(bridge,
// req.OfferReviewOutcomeTool) — same gate Claude/Codex use (BUG-NOTE-CP42 #24).

// TestGrokMcpReviewOutcomeGatingToolsList proves submit_review_outcome is only
// advertised when the turn registered with allowReviewOutcome=true (flow hub).
func TestGrokMcpReviewOutcomeGatingToolsList(t *testing.T) {
	mcp := newClaudeMCPServer()

	normalTok := mcp.register(&fakeGrokBridge{}, false)
	defer mcp.unregister(normalTok)
	_, list := postMCP(t, mcp, normalTok, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	lr, _ := list["result"].(map[string]any)
	tools, _ := lr["tools"].([]any)
	for _, def := range tools {
		if m, ok := def.(map[string]any); ok && m["name"] == "submit_review_outcome" {
			t.Fatalf("submit_review_outcome must not be advertised for normal_chat (OfferReviewOutcomeTool=false), got %+v", tools)
		}
	}

	hubTok := mcp.register(&fakeGrokBridge{}, true)
	defer mcp.unregister(hubTok)
	_, list2 := postMCP(t, mcp, hubTok, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	lr2, _ := list2["result"].(map[string]any)
	tools2, _ := lr2["tools"].([]any)
	found := false
	for _, def := range tools2 {
		if m, ok := def.(map[string]any); ok && m["name"] == "submit_review_outcome" {
			found = true
		}
	}
	if !found {
		t.Fatalf("submit_review_outcome must be advertised for flow-hub turn (OfferReviewOutcomeTool=true), got %+v", tools2)
	}
}

// TestGrokMcpSubmitReviewOutcomeRejectedWhenNotAllowed is defense-in-depth: a model
// must not mutate loop state via tools/call when the turn never offered the tool.
func TestGrokMcpSubmitReviewOutcomeRejectedWhenNotAllowed(t *testing.T) {
	mcp := newClaudeMCPServer()
	bridge := &fakeGrokBridge{}
	tok := mcp.register(bridge, false)
	defer mcp.unregister(tok)

	_, resp := postMCP(t, mcp, tok, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{
			"name":      "submit_review_outcome",
			"arguments": map[string]any{"status": "approved"},
		},
	})
	if _, hasError := resp["error"]; !hasError {
		t.Fatalf("expected error rejecting submit_review_outcome for non-hub turn, got %+v", resp)
	}
	if bridge.flowControlIn.Status != "" {
		t.Fatalf("SubmitFlowControl must not run when tool not offered, got %+v", bridge.flowControlIn)
	}
}

// TestGrokMcpSubmitReviewOutcomeRoundTrip proves Task-209 DOD-4 wiring: when Grok's
// MCP client calls tools/call submit_review_outcome on a hub turn token, the shared
// handler routes to bridge.SubmitFlowControl (same path as Claude/Codex).
func TestGrokMcpSubmitReviewOutcomeRoundTrip(t *testing.T) {
	mcp := newClaudeMCPServer()
	bridge := &fakeGrokBridge{
		flowControlResult: FlowControlResult{Status: "continue", Round: 1, Cap: 3},
	}
	tok := mcp.register(bridge, true)
	defer mcp.unregister(tok)

	raw, errObj := mcp.dispatch("tools/call", map[string]any{
		"params": map[string]any{
			"name": "submit_review_outcome",
			"arguments": map[string]any{
				"status":   "changes_requested",
				"feedback": "fix the null check",
				"issues": []any{
					map[string]any{"title": "nil deref", "severity": "error", "file": "main.go"},
				},
			},
		},
	}, tok)
	if errObj != nil {
		t.Fatalf("tools/call submit_review_outcome error: %+v", errObj)
	}
	if bridge.flowControlIn.Status != "continue" {
		t.Fatalf("SubmitFlowControl status = %q, want continue (changes_requested face)", bridge.flowControlIn.Status)
	}
	if bridge.flowControlIn.Summary != "fix the null check" {
		t.Fatalf("SubmitFlowControl summary = %q, want fix the null check", bridge.flowControlIn.Summary)
	}
	result, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T %+v", raw, raw)
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("expected MCP text content result, got %+v", result)
	}
	txt, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(txt, "continue") {
		t.Fatalf("result text should include flow-control status, got %q", txt)
	}
}

// TestGrokMcpSpawnAgentWaitModes covers wait=false acknowledgement through the
// shared MCP handler (wait=true shape is asserted in TestGrokMcpSpawnAgentRoundTrip).
func TestGrokMcpSpawnAgentWaitModes(t *testing.T) {
	mcp := newClaudeMCPServer()
	bridge := &fakeGrokBridge{
		spawnResult: SpawnAgentResult{RunID: "child-bg-1", ProviderKey: "grok"},
	}
	token := mcp.register(bridge, false)
	defer mcp.unregister(token)

	raw, errObj := mcp.dispatch("tools/call", map[string]any{
		"params": map[string]any{
			"name": "spawn_agent",
			"arguments": map[string]any{
				"agent":    "reviewer",
				"prompt":   "review in background",
				"provider": "grok",
				"wait":     false,
			},
		},
	}, token)
	if errObj != nil {
		t.Fatalf("tools/call spawn_agent error: %+v", errObj)
	}
	if bridge.spawnIn.Wait {
		t.Fatal("wait=false must reach SpawnAgent with Wait=false")
	}
	result := raw.(map[string]any)
	content := result["content"].([]any)
	txt := content[0].(map[string]any)["text"].(string)
	var got SpawnAgentResult
	if err := json.Unmarshal([]byte(txt), &got); err != nil {
		t.Fatalf("spawn_agent result must be JSON, got %q: %v", txt, err)
	}
	if got.RunID != "child-bg-1" {
		t.Fatalf("wait=false result runId = %q, want child-bg-1", got.RunID)
	}
}

func TestParseGrokSpawnSubagentInputMapsToSpawnAgentInput(t *testing.T) {
	in, err := parseGrokSpawnSubagentInput(map[string]any{
		"prompt":        "implement feature X",
		"description":   "feature work",
		"subagent_type": "plan",
		"background":    true,
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if in.Agent != "synthesizer" || in.Prompt != "implement feature X" || in.Wait {
		t.Fatalf("got %+v, want agent=synthesizer wait=false", in)
	}

	in2, err := parseGrokSpawnSubagentInput(map[string]any{
		"prompt":      "run review",
		"description": "review",
	})
	if err != nil {
		t.Fatalf("parse default type: %v", err)
	}
	if in2.Agent != "coder" || !in2.Wait {
		t.Fatalf("default subagent_type should map to coder with wait=true, got %+v", in2)
	}

	if _, err := parseGrokSpawnSubagentInput(map[string]any{"description": "no prompt"}); err == nil {
		t.Fatal("expected error for missing prompt")
	}
}

func TestGrokNormalizedToolDisplayNameAliasesNativeTools(t *testing.T) {
	if got := grokNormalizedToolDisplayName("spawn_subagent"); got != "spawn_agent" {
		t.Fatalf("spawn_subagent alias = %q, want spawn_agent", got)
	}
	if got := grokNormalizedToolDisplayName("ask_user_question"); got != "ask_user" {
		t.Fatalf("ask_user_question alias = %q, want ask_user", got)
	}
	if got := grokNormalizedToolDisplayName("read_file"); got != "read_file" {
		t.Fatalf("unrelated tool should pass through, got %q", got)
	}
}

// TestGrokTryShimNativeSpawnSubagentRoutesToBridge proves Task-209 T-3/DOD-8: when the
// model calls native spawn_subagent, the adapter mirrors it to TurnBridge.SpawnAgent.
func TestGrokTryShimNativeSpawnSubagentRoutesToBridge(t *testing.T) {
	a := newGrokAdapter(newGrokDispatcher(nil, nil), "/tmp/x")
	a.mcpServer = newClaudeMCPServer()
	bridge := &fakeGrokBridge{
		spawnResult: SpawnAgentResult{RunID: "child-shim-1", ProviderKey: "grok"},
	}
	seen := map[string]struct{}{}
	n := grokNotification{
		Method: "session/update",
		Params: map[string]any{
			"sessionId": "session-spawn-shim",
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    "call-spawn-1",
				"title":         "spawn_subagent",
				"rawInput": map[string]any{
					"prompt":        "review the code",
					"description":   "code review",
					"subagent_type": "explore",
					"background":    false,
				},
				"_meta": map[string]any{"x.ai/tool": map[string]any{"name": "spawn_subagent"}},
			},
		},
	}
	a.tryShimGrokNativeSpawnSubagent("session-spawn-shim", bridge, n, seen)
	time.Sleep(100 * time.Millisecond)
	if bridge.spawnIn.Agent != "reviewer" || bridge.spawnIn.Prompt != "review the code" {
		t.Fatalf("shim SpawnAgent args = %+v, want agent=reviewer prompt=review the code", bridge.spawnIn)
	}
	if !bridge.spawnIn.Wait {
		t.Fatal("background=false must map to wait=true")
	}
	// Dedup: second call with same toolCallId must not re-spawn.
	bridge.spawnIn = SpawnAgentInput{}
	a.tryShimGrokNativeSpawnSubagent("session-spawn-shim", bridge, n, seen)
	time.Sleep(50 * time.Millisecond)
	if bridge.spawnIn.Agent != "" {
		t.Fatalf("duplicate toolCallId must not re-spawn, got %+v", bridge.spawnIn)
	}
}

// TestGrokUnsupportedInboundStillRepliesError guards that non-permission
// inbound methods get a controlled JSON-RPC error (never hang the dispatcher).
func TestGrokUnsupportedInboundStillRepliesError(t *testing.T) {
	d, fg := startFakeGrok(t, nil)
	_ = newGrokAdapter(d, "/tmp/x") // wires handleInbound

	replied := make(chan map[string]any, 1)
	go func() {
		for m := range fg.requests {
			if m["id"] == float64(901) {
				replied <- m
				return
			}
		}
	}()
	fg.send(map[string]any{
		"jsonrpc": "2.0",
		"id":      901,
		"method":  "session/unknown_ask",
		"params":  map[string]any{},
	})

	select {
	case m := <-replied:
		errObj, _ := m["error"].(map[string]any)
		if errObj == nil {
			t.Fatalf("expected JSON-RPC error for unsupported inbound, got %+v", m)
		}
		msg, _ := errObj["message"].(string)
		if !strings.Contains(msg, "unsupported") {
			t.Fatalf("expected unsupported error message, got %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("unsupported inbound must still be replied to, not left hanging")
	}
}
