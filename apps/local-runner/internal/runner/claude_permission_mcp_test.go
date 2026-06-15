package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	args := claudeArgs(resolveYoloPosture(false), "", "", nil)
	if argIndex(args, "--strict-mcp-config") < 0 {
		t.Fatalf("--strict-mcp-config must always be present: %v", args)
	}
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

func TestEnsureClaudeConfigSettingsDoesNotClobber(t *testing.T) {
	dir := t.TempDir()
	custom := `{"permissions":{"allow":["Bash(*)"]},"theme":"dark"}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(custom), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := ensureClaudeConfigSettings(dir); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
	if !strings.Contains(string(raw), `"theme":"dark"`) {
		t.Fatalf("must not clobber an existing settings.json, got %s", raw)
	}
}
