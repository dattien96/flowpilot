package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Task-400 DOD-1: golden fixtures captured live against devin 3000.10.31 must
// round-trip through the typed structs.

func devinFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "devin_acp", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

func TestDevinACPTypesRoundTrip(t *testing.T) {
	// initialize result
	var init DevinInitializeResult
	if err := json.Unmarshal(devinFixture(t, "initialize_result.json"), &init); err != nil {
		t.Fatalf("unmarshal initialize: %v", err)
	}
	if !init.AgentCapabilities.LoadSession {
		t.Fatal("expected loadSession true")
	}
	if init.AgentCapabilities.MCPCapabilities.HTTP || init.AgentCapabilities.MCPCapabilities.SSE {
		t.Fatal("expected mcp http/sse false (stdio-only)")
	}
	if !init.AgentCapabilities.PromptCapabilities.Image {
		t.Fatal("expected promptCapabilities.image true")
	}
	if len(init.AuthMethods) == 0 || init.AuthMethods[0].ID != "devin-browser" {
		t.Fatalf("expected devin-browser auth method, got %+v", init.AuthMethods)
	}
	if init.Meta["mcpConfigPath"] == nil {
		t.Fatal("expected _meta.mcpConfigPath")
	}
	if b, err := json.Marshal(init); err != nil {
		t.Fatalf("marshal initialize: %v", err)
	} else {
		var round map[string]any
		if err := json.Unmarshal(b, &round); err != nil {
			t.Fatalf("round-trip initialize: %v", err)
		}
		if round["protocolVersion"] == nil {
			t.Fatal("expected protocolVersion after round-trip")
		}
	}

	// session/new result — slug sessionId + modes + configOptions
	var newRes DevinSessionNewResult
	if err := json.Unmarshal(devinFixture(t, "session_new_result.json"), &newRes); err != nil {
		t.Fatalf("unmarshal session/new: %v", err)
	}
	if newRes.SessionID == "" {
		t.Fatal("expected sessionId")
	}
	if !isDevinRealSessionID(newRes.SessionID) {
		t.Fatalf("live sessionId %q should parse as a real devin id", newRes.SessionID)
	}
	if newRes.Modes == nil || newRes.Modes.CurrentModeID == "" {
		t.Fatal("expected modes.currentModeId")
	}
	if len(newRes.Modes.AvailableModes) != 5 {
		t.Fatalf("expected 5 available modes, got %d", len(newRes.Modes.AvailableModes))
	}
	var modeIDs []string
	for _, m := range newRes.Modes.AvailableModes {
		modeIDs = append(modeIDs, m.ID)
	}
	for _, want := range []string{"accept-edits", "smart", "ask", "plan", "bypass"} {
		found := false
		for _, got := range modeIDs {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected mode %q in availableModes %v", want, modeIDs)
		}
	}
	var modelOpt *DevinConfigOption
	for i := range newRes.ConfigOptions {
		if newRes.ConfigOptions[i].ID == "model" {
			modelOpt = &newRes.ConfigOptions[i]
		}
	}
	if modelOpt == nil || len(modelOpt.Options) == 0 {
		t.Fatal("expected model configOption with options")
	}
	if !devinChoiceSupportsImages(modelOpt.Options[0]) {
		t.Fatal("expected supportsImages _meta on model options")
	}

	// session/prompt result
	var promptRes DevinPromptResult
	if err := json.Unmarshal(devinFixture(t, "session_prompt_result.json"), &promptRes); err != nil {
		t.Fatalf("unmarshal prompt result: %v", err)
	}
	if promptRes.StopReason != "end_turn" {
		t.Fatalf("expected end_turn, got %s", promptRes.StopReason)
	}
	if promptRes.Usage == nil || promptRes.Usage.TotalTokens == 0 {
		t.Fatal("expected usage tokens")
	}

	// session/list result
	var listRes DevinSessionListResult
	if err := json.Unmarshal(devinFixture(t, "session_list_result.json"), &listRes); err != nil {
		t.Fatalf("unmarshal session/list: %v", err)
	}
	if len(listRes.Sessions) == 0 {
		t.Fatal("expected sessions in list result")
	}
	if listRes.Sessions[0].SessionID == "" {
		t.Fatal("expected sessionId in list entry")
	}
}

func TestDevinACPParamBuilders(t *testing.T) {
	// initialize advertises fs false (write safety) and terminal false.
	init := devinACPInitializeParams()
	caps, _ := init["clientCapabilities"].(map[string]interface{})
	fs, _ := caps["fs"].(map[string]interface{})
	if fs["readTextFile"] != false || fs["writeTextFile"] != false {
		t.Fatalf("fs capabilities must be false (writes gate through session/request_permission), got %v", fs)
	}
	if caps["terminal"] != false {
		t.Fatal("terminal must be false")
	}

	// authenticate carries the only advertised method id.
	auth := devinACPAuthenticateParams()
	if auth["methodId"] != "devin-browser" {
		t.Fatalf("methodId: %v", auth["methodId"])
	}

	// session/new: cwd + non-nil mcpServers; no model/mode params.
	newParams := devinACPSessionNewParams("/ws", nil)
	if newParams["cwd"] != "/ws" {
		t.Fatalf("cwd: %v", newParams["cwd"])
	}
	if srv, ok := newParams["mcpServers"].([]interface{}); !ok || srv == nil {
		t.Fatal("mcpServers must be a non-nil array")
	}
	if _, has := newParams["model"]; has {
		t.Fatal("model must NOT be a session/new param — it is a config option")
	}
	if _, has := newParams["mode"]; has {
		t.Fatal("mode must NOT be a session/new param — it is a config option")
	}

	// session/load carries sessionId+cwd+mcpServers.
	load := devinACPSessionLoadParams("  working-pentagon ", "/ws", nil)
	if load["sessionId"] != "working-pentagon" {
		t.Fatalf("sessionId trim: %v", load["sessionId"])
	}

	// session/list keys by canonical cwd.
	list := devinACPSessionListParams("/private/tmp")
	if list["cwd"] != "/private/tmp" {
		t.Fatalf("list cwd: %v", list["cwd"])
	}

	// set_config_option uses the live-verified {sessionId, configId, value}.
	cfg := devinACPSessionSetConfigParams("s", "model", "swe-2-high")
	if cfg["configId"] != "model" || cfg["value"] != "swe-2-high" || cfg["sessionId"] != "s" {
		t.Fatalf("set_config_option params: %v", cfg)
	}

	// prompt with image attachment.
	p := devinACPPromptParamsWithAttachments("s", "hi", []PromptAttachment{{Kind: "image", Data: "AAAA", MimeType: "image/png"}})
	blocks, _ := p["prompt"].([]map[string]string)
	if len(blocks) != 2 || blocks[1]["type"] != "image" || blocks[1]["data"] != "AAAA" {
		t.Fatalf("prompt blocks: %v", blocks)
	}
	// Non-image / empty attachments are skipped.
	p2 := devinACPPromptParamsWithAttachments("s", "hi", []PromptAttachment{{Kind: "file", Data: "x"}, {Kind: "image"}})
	blocks2, _ := p2["prompt"].([]map[string]string)
	if len(blocks2) != 1 {
		t.Fatalf("expected only text block, got %v", blocks2)
	}
}

func TestDevinACPResponseSessionID(t *testing.T) {
	msg := map[string]interface{}{"result": map[string]interface{}{"sessionId": "frost-plywood"}}
	if got := devinACPResponseSessionID(msg); got != "frost-plywood" {
		t.Fatalf("top-level sessionId: %q", got)
	}
	meta := map[string]interface{}{"result": map[string]interface{}{"_meta": map[string]interface{}{"sessionId": "dark-zinnia"}}}
	if got := devinACPResponseSessionID(meta); got != "dark-zinnia" {
		t.Fatalf("_meta sessionId: %q", got)
	}
	if got := devinACPResponseSessionIDFromResult(nil); got != "" {
		t.Fatalf("nil result: %q", got)
	}
}

func TestDevinACPExtractText(t *testing.T) {
	var msg map[string]any
	if err := json.Unmarshal(devinFixture(t, "update_agent_message_chunk.json"), &msg); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if got := devinACPExtractText(msg); got != "PROBE" {
		t.Fatalf("full message extract: %q", got)
	}
	params, _ := msg["params"].(map[string]any)
	update, _ := params["update"].(map[string]any)
	if got := devinACPExtractText(update); got != "PROBE" {
		t.Fatalf("update map extract: %q", got)
	}
	// thought chunks extract nothing (not user-visible text).
	var thought map[string]any
	if err := json.Unmarshal(devinFixture(t, "update_agent_thought_chunk.json"), &thought); err != nil {
		t.Fatalf("thought fixture: %v", err)
	}
	if got := devinACPExtractText(thought); got != "" {
		t.Fatalf("thought chunk must yield empty text, got %q", got)
	}
}

func TestDevinACPHelpers(t *testing.T) {
	if !devinACPIsPermissionRequest("session/request_permission") {
		t.Fatal("session/request_permission must route to approval")
	}
	if devinACPIsPermissionRequest("session/update") {
		t.Fatal("session/update is not a permission request")
	}
	if !devinACPIsExtensionNotification("_cognition.ai/output") {
		t.Fatal("_cognition.ai/* must classify as extension")
	}
	if !devinACPIsExtensionNotification("_cognition.ai/mcp/serversChanged") {
		t.Fatal("nested _cognition.ai path must classify as extension")
	}
	if devinACPIsExtensionNotification("session/update") {
		t.Fatal("session/update is not an extension")
	}
}

func TestDevinSessionModeFromResult(t *testing.T) {
	var res map[string]any
	if err := json.Unmarshal(devinFixture(t, "session_new_result.json"), &res); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if got := devinSessionModeFromResult(res); got != "accept-edits" {
		t.Fatalf("modes.currentModeId: %q", got)
	}
	// config-option fallback when modes block is absent.
	fallback := map[string]any{"configOptions": []any{map[string]any{"id": "mode", "currentValue": "plan"}}}
	if got := devinSessionModeFromResult(fallback); got != "plan" {
		t.Fatalf("configOption fallback: %q", got)
	}
}

func TestDevinSessionIDShape(t *testing.T) {
	// Live-observed slug ids are real.
	for _, id := range []string{"working-pentagon", "frost-plywood", "sore-router", "sturdy-fang"} {
		if !isDevinRealSessionID(id) {
			t.Fatalf("live slug %q must be a real session id", id)
		}
	}
	// Synthetic FlowPilot ids and paths are not.
	for _, id := range []string{"", "thread-abc123", ".", "..", "a/b", "a\\b", "Has Upper"} {
		if isDevinRealSessionID(id) {
			t.Fatalf("%q must not count as a real devin session id", id)
		}
	}
}

func TestDevinStdioMCPServerEntry(t *testing.T) {
	entry := devinACPStdioMCPServerEntry("flowpilot", "/bin/flowpilot", []string{"devin-mcp-stdio", "--url", "http://x"}, map[string]string{"A": "1"})
	if entry == nil {
		t.Fatal("expected entry")
	}
	if entry["type"] != nil || entry["name"] != "flowpilot" || entry["command"] != "/bin/flowpilot" {
		t.Fatalf("entry: %v", entry)
	}
	env, _ := entry["env"].([]interface{})
	if len(env) != 1 {
		t.Fatalf("env: %v", env)
	}
	if devinACPStdioMCPServerEntry("", "/bin/x", nil, nil) != nil {
		t.Fatal("empty name must yield nil")
	}
	if devinACPStdioMCPServerEntry("n", "", nil, nil) != nil {
		t.Fatal("empty command must yield nil")
	}
}

func TestDevinExtraMCPServersStdioOnly(t *testing.T) {
	// Devin ACP is stdio-only — url-only entries are skipped (they belong in
	// mcp_config.json), command entries convert to stdio.
	out := devinACPExtraMCPServers(map[string]claudeMcpServer{
		"stdio-one": {Command: "/bin/mcp", Args: []string{"serve"}, Env: map[string]string{"K": "V"}},
		"http-one":  {URL: "http://localhost:1/mcp"},
	})
	if len(out) != 1 {
		t.Fatalf("expected only the stdio entry, got %d", len(out))
	}
	m, _ := out[0].(map[string]interface{})
	if m["type"] != nil || m["name"] != "stdio-one" {
		t.Fatalf("entry: %v", m)
	}
}
