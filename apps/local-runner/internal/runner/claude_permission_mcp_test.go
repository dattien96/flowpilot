package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Validates the FlowPilot approve/ask_user MCP handlers + config isolation against the
// REAL contract captured from claude 2.1.177 (07 Appendix A spike):
//   approve args = {tool_name, input{...}, tool_use_id}
//   approve reply = a text content block with JSON {"behavior":"deny"|"allow", ...}

func parseClaudeDecision(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("result has no content: %+v", result)
	}
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	var decision map[string]any
	if err := json.Unmarshal([]byte(text), &decision); err != nil {
		t.Fatalf("decision text is not JSON (%q): %v", text, err)
	}
	return decision
}

func TestHandleClaudeApproveDenyBlocks(t *testing.T) {
	args := map[string]any{
		"tool_name":   "Write",
		"input":       map[string]any{"file_path": "/w/spike.txt", "content": "hi"},
		"tool_use_id": "toolu_1",
	}
	b := &fakeClaudeBridge{approval: "deny"}
	decision := parseClaudeDecision(t, handleClaudeApprove(args, b))
	if decision["behavior"] != "deny" {
		t.Fatalf("expected deny, got %+v", decision)
	}
	// the bridge saw the gated tool labelled by its file_path / tool name
	if len(b.approvalCalls) != 1 || b.approvalCalls[0].Command != "/w/spike.txt" || b.approvalCalls[0].Reason != "Write" {
		t.Fatalf("approval details = %+v", b.approvalCalls)
	}
}

func TestHandleClaudeApproveAllowPassesInput(t *testing.T) {
	input := map[string]any{"command": "echo hi"}
	args := map[string]any{"tool_name": "Bash", "input": input, "tool_use_id": "toolu_2"}
	b := &fakeClaudeBridge{approval: "approve"}
	decision := parseClaudeDecision(t, handleClaudeApprove(args, b))
	if decision["behavior"] != "allow" {
		t.Fatalf("expected allow, got %+v", decision)
	}
	if _, ok := decision["updatedInput"].(map[string]any); !ok {
		t.Fatalf("allow must carry updatedInput, got %+v", decision)
	}
	if b.approvalCalls[0].Command != "echo hi" {
		t.Fatalf("Bash command should be the label, got %q", b.approvalCalls[0].Command)
	}
}

func TestHandleClaudeApproveBridgeErrorDenies(t *testing.T) {
	args := map[string]any{"tool_name": "Bash", "input": map[string]any{"command": "rm -rf /"}}
	b := &fakeClaudeBridge{approvalErr: errApprovalExpired}
	decision := parseClaudeDecision(t, handleClaudeApprove(args, b))
	if decision["behavior"] != "deny" {
		t.Fatalf("bridge error must deny (engine never hangs), got %+v", decision)
	}
}

func TestHandleClaudeAskUser(t *testing.T) {
	args := map[string]any{"prompt": "Pick", "options": []any{"A", "B"}}
	b := &fakeClaudeBridge{answer: []string{"A"}}
	result := handleClaudeAskUser(args, b)
	content, _ := result["content"].([]any)
	block, _ := content[0].(map[string]any)
	if txt, _ := block["text"].(string); txt != "A" {
		t.Fatalf("ask_user result = %q, want A", txt)
	}
	if b.questionPrompt != "Pick" || len(b.questionOpts) != 2 {
		t.Fatalf("AskQuestion got prompt=%q opts=%d", b.questionPrompt, len(b.questionOpts))
	}
}

func TestClaudeArgsIncludesStrictMcpConfig(t *testing.T) {
	args := claudeArgs(resolveYoloPosture(false), "", "", "", "", nil)
	if argIndex(args, "--strict-mcp-config") < 0 {
		t.Fatalf("--strict-mcp-config must always be present: %v", args)
	}
}

// TestClaudeArgsDisablesBuiltinAskUserQuestion guards the live-flow defect: Claude's built-in
// AskUserQuestion tool runs in the headless CLI with no TTY (returns "user did not answer" and
// renders as plain text), bypassing FlowPilot's bridge so no QuestionCard appears. claudeArgs
// MUST disable it in BOTH YOLO states so the model is forced onto mcp__flowpilot__ask_user.
func TestClaudeArgsDisablesBuiltinAskUserQuestion(t *testing.T) {
	for _, yolo := range []bool{false, true} {
		args := claudeArgs(resolveYoloPosture(yolo), "", "", "", "", nil)
		if !flagHasValue(args, "--disallowed-tools", "AskUserQuestion") {
			t.Fatalf("yolo=%v: built-in AskUserQuestion must be disabled so ask_user MCP tool is used: %v", yolo, args)
		}
	}
}

// TestClaudeAskUserToolAdvertisesSchema guards that the ask_user MCP tool exposes its
// prompt/options/multiSelect parameters via tools/list — an empty schema left the model unsure
// how to call it and biased it toward the (now-disabled) built-in AskUserQuestion.
func TestClaudeAskUserToolAdvertisesSchema(t *testing.T) {
	var askUser map[string]any
	for _, def := range claudeMCPToolDefs() {
		if m, ok := def.(map[string]any); ok && m["name"] == "ask_user" {
			askUser = m
		}
	}
	if askUser == nil {
		t.Fatalf("claudeMCPToolDefs must include an ask_user tool")
	}
	schema, _ := askUser["inputSchema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	for _, key := range []string{"prompt", "options", "multiSelect"} {
		if _, ok := props[key]; !ok {
			t.Fatalf("ask_user inputSchema must advertise %q: %+v", key, schema)
		}
	}
	req, _ := schema["required"].([]any)
	if len(req) != 1 || req[0] != "prompt" {
		t.Fatalf("ask_user inputSchema must require prompt, got %+v", req)
	}
}

// TestClaudeMCPServerPromptGate guards the core live-flow fix: claude connects --mcp-config
// servers asynchronously, so SendTurn must withhold the prompt until tools/list arrives or
// ask_user is never in the first turn's tool set. waitReady must block until the connection
// signal, unblock the instant tools/list is dispatched, and bound itself on timeout + ctx.
func TestClaudeMCPServerPromptGate(t *testing.T) {
	t.Run("unblocks when tools/list is dispatched", func(t *testing.T) {
		s := newClaudeMCPServer()
		tok := s.register(&fakeClaudeBridge{})
		defer s.unregister(tok)

		// Not ready before the handshake reaches tools/list.
		if s.waitReady(context.Background(), tok, 30*time.Millisecond) {
			t.Fatalf("waitReady must not report ready before tools/list")
		}

		done := make(chan bool, 1)
		go func() { done <- s.waitReady(context.Background(), tok, 2*time.Second) }()
		// Simulate claude's client fetching the tool list for this token (the real wire path).
		if _, rpcErr := s.dispatch("tools/list", map[string]any{"id": 1}, tok); rpcErr != nil {
			t.Fatalf("tools/list dispatch error: %+v", rpcErr)
		}
		select {
		case ok := <-done:
			if !ok {
				t.Fatalf("waitReady must report ready once tools/list is dispatched")
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("waitReady did not unblock after tools/list")
		}
	})

	t.Run("times out without a connection", func(t *testing.T) {
		s := newClaudeMCPServer()
		tok := s.register(&fakeClaudeBridge{})
		defer s.unregister(tok)
		start := time.Now()
		if s.waitReady(context.Background(), tok, 40*time.Millisecond) {
			t.Fatalf("waitReady must time out (return false) when claude never connects")
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("waitReady overran its timeout: %v", elapsed)
		}
	})

	t.Run("returns on ctx cancel", func(t *testing.T) {
		s := newClaudeMCPServer()
		tok := s.register(&fakeClaudeBridge{})
		defer s.unregister(tok)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if s.waitReady(ctx, tok, time.Hour) {
			t.Fatalf("waitReady must return false promptly when ctx is cancelled")
		}
	})

	t.Run("unknown token returns false", func(t *testing.T) {
		s := newClaudeMCPServer()
		if s.waitReady(context.Background(), "no-such-token", 10*time.Millisecond) {
			t.Fatalf("waitReady must return false for an unregistered token")
		}
	})
}

func TestEnsureClaudeConfigSettingsWritesGatingPosture(t *testing.T) {
	dir := t.TempDir()
	if err := ensureClaudeConfigSettings(dir); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("settings not JSON: %v", err)
	}
	perms, _ := cfg["permissions"].(map[string]any)
	if perms == nil || perms["defaultMode"] != "default" {
		t.Fatalf("expected defaultMode=default, got %+v", cfg)
	}
	allow, _ := perms["allow"].([]any)
	if len(allow) != 0 {
		t.Fatalf("gating posture must have an empty allow-list, got %+v", allow)
	}
}

// TestEnsureClaudeConfigSettingsClearsAllowRules is the regression guard for BUG-069:
// Claude CLI persists approved tools as allow-rules in settings.json, which then bypass
// YOLO=false gating. ensureClaudeConfigSettings must always clear the allow list even
// when settings.json already exists, while preserving non-permission fields.
func TestEnsureClaudeConfigSettingsClearsAllowRules(t *testing.T) {
	dir := t.TempDir()
	// Seed a settings.json that looks like what Claude writes after a user approves a
	// tool (allow-rule) while also carrying a user preference ("theme").
	existing := `{"permissions":{"allow":["Bash(*)"],"defaultMode":"default"},"theme":"dark"}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := ensureClaudeConfigSettings(dir); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	// Allow-rules must be cleared so gating engages.
	perms, _ := cfg["permissions"].(map[string]any)
	allow, _ := perms["allow"].([]any)
	if len(allow) != 0 {
		t.Fatalf("allow-rules not cleared: %v", allow)
	}
	if perms["defaultMode"] != "default" {
		t.Fatalf("defaultMode must be default, got %v", perms["defaultMode"])
	}
	// Non-permission fields must be preserved.
	if cfg["theme"] != "dark" {
		t.Fatalf("non-permission field 'theme' must be preserved, got %v", cfg["theme"])
	}
}
