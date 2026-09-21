package runner

import (
	"sort"
	"strings"
)

// Task-400 (CP-70 T-1/T-2): standalone Devin ACP primitives.
//
// Modeled on opencode_acp.go — same JSON-RPC/NDJSON transport, same method
// names — but adapted for the Devin wire contract captured live against devin
// 3000.10.31 (see testdata/devin_acp/*.json):
//
//   - Boot is initialize -> authenticate{methodId:"devin-browser"} ->
//     session/new. Devin ACP does NOT read the local CLI credential store;
//     the host is the sole credential source and must call authenticate
//     (live stderr: "ACP host is the sole source of credentials").
//   - session/new -> {sessionId:"<adjective-noun slug>", modes:{currentModeId,
//     availableModes:[accept-edits/smart/ask/plan/bypass]}, configOptions:
//     [mode,model]} — the model option carries the full ~380-entry catalog.
//   - Per-session model AND mode switching ride on
//     session/set_config_option {sessionId, configId, value} (verified live:
//     configId "model" and "mode" both work mid-session).
//   - mcpCapabilities{http:false,sse:false} — only stdio MCP servers can be
//     injected via session/new; remote MCP belongs in mcp_config.json.
//   - Custom notifications `_cognition.ai/*` (output, turn_stats,
//     mcp/serversChanged, agent_stopped, thinking_complete) flow freely —
//     the dispatcher must tolerate unknown methods.

// devinACPInitializeParams builds the `initialize` request params. fs text
// capabilities are hard-set false (same rule as OpenCode, CP-57 P-6): Devin
// performs file I/O with its own tools and routes each write through
// session/request_permission — advertising fs.write_text_file would open a
// second write channel that bypasses the approval bridge and read-only
// postures. terminal is false; FlowPilot implements no ACP terminal server.
func devinACPInitializeParams() map[string]interface{} {
	return map[string]interface{}{
		"protocolVersion": 1,
		"clientCapabilities": map[string]interface{}{
			"fs": map[string]interface{}{
				"readTextFile":  false,
				"writeTextFile": false,
			},
			"terminal": false,
		},
		"clientInfo": map[string]interface{}{
			"name":    "flowpilot",
			"version": "1.0",
		},
	}
}

// devinACPAuthenticateParams builds the `authenticate` request params. The
// only method Devin advertises is "devin-browser" (PKCE). Authenticate must
// be called after initialize and before session/new — session/new without it
// stalls forever (live-verified).
func devinACPAuthenticateParams() map[string]interface{} {
	return map[string]interface{}{
		"methodId": "devin-browser",
	}
}

// devinACPSessionNewParams builds the `session/new` request params. Live
// shape is {"cwd": "...", "mcpServers": [...]}. Model/mode are NOT params —
// they are session config options applied via session/set_config_option.
func devinACPSessionNewParams(cwd string, mcpServers []interface{}) map[string]interface{} {
	if mcpServers == nil {
		mcpServers = []interface{}{}
	}
	return map[string]interface{}{
		"cwd":        cwd,
		"mcpServers": mcpServers,
	}
}

// devinACPSessionLoadParams builds the `session/load` resume params
// (sessionId/cwd/mcpServers — same shape as session/new plus the id).
func devinACPSessionLoadParams(sessionID, cwd string, mcpServers []interface{}) map[string]interface{} {
	if mcpServers == nil {
		mcpServers = []interface{}{}
	}
	return map[string]interface{}{
		"sessionId":  strings.TrimSpace(sessionID),
		"cwd":        cwd,
		"mcpServers": mcpServers,
	}
}

// devinACPSessionListParams builds the `session/list` params. Devin keys
// session listing by the canonical cwd — on macOS /tmp resolves to
// /private/tmp, so callers must pass the already-canonicalized cwd.
func devinACPSessionListParams(cwd string) map[string]interface{} {
	return map[string]interface{}{"cwd": cwd}
}

// devinACPPromptParams builds the `session/prompt` params for a text turn.
func devinACPPromptParams(sessionID, prompt string) map[string]interface{} {
	return devinACPPromptParamsWithAttachments(sessionID, prompt, nil)
}

// devinACPPromptParamsWithAttachments builds `session/prompt` params with the
// text block followed by one ACP image block per image attachment. Devin
// advertises promptCapabilities.image=true at protocol level and per-model
// supportsImages in the model option _meta; the caller is responsible for
// gating attachments on the selected model (devinModelSupportsImages).
func devinACPPromptParamsWithAttachments(sessionID, prompt string, atts []PromptAttachment) map[string]interface{} {
	blocks := []map[string]string{
		{"type": "text", "text": prompt},
	}
	for _, att := range atts {
		if att.Kind != "image" || strings.TrimSpace(att.Data) == "" {
			continue
		}
		blocks = append(blocks, map[string]string{
			"type":     "image",
			"data":     att.Data,
			"mimeType": att.MimeType,
		})
	}
	return map[string]interface{}{
		"sessionId": sessionID,
		"prompt":    blocks,
	}
}

// devinACPSessionSetConfigParams builds `session/set_config_option` params.
// Live-verified shape {sessionId, configId, value}: configId "model" accepts
// a catalog id ("swe-2-medium"), configId "mode" accepts
// accept-edits/smart/ask/plan/bypass. The response echoes the full
// configOptions array with updated currentValue.
func devinACPSessionSetConfigParams(sessionID, key, value string) map[string]interface{} {
	return map[string]interface{}{
		"sessionId": strings.TrimSpace(sessionID),
		"configId":  strings.TrimSpace(key),
		"value":     strings.TrimSpace(value),
	}
}

// devinACPResponseSessionID reads sessionId out of a session/new,
// session/load, or session/prompt result — top level first, _meta fallback.
func devinACPResponseSessionID(message map[string]interface{}) string {
	result, ok := message["result"].(map[string]interface{})
	if !ok {
		return ""
	}
	return devinACPResponseSessionIDFromResult(result)
}

func devinACPResponseSessionIDFromResult(result map[string]interface{}) string {
	if result == nil {
		return ""
	}
	if sessionID, ok := result["sessionId"].(string); ok && strings.TrimSpace(sessionID) != "" {
		return strings.TrimSpace(sessionID)
	}
	if meta, ok := result["_meta"].(map[string]interface{}); ok {
		if sessionID, ok := meta["sessionId"].(string); ok {
			return strings.TrimSpace(sessionID)
		}
	}
	return ""
}

// devinACPExtractText extracts text delta from a `session/update` notification
// whose update.sessionUpdate == "agent_message_chunk". Returns "" for other
// update types. Accepts either the full message or the update map.
func devinACPExtractText(update map[string]interface{}) string {
	if _, hasMethod := update["method"].(string); hasMethod {
		params, ok := update["params"].(map[string]interface{})
		if !ok {
			return ""
		}
		inner, ok := params["update"].(map[string]interface{})
		if !ok {
			return ""
		}
		return devinACPExtractText(inner)
	}
	if params, ok := update["params"].(map[string]interface{}); ok {
		if inner, ok := params["update"].(map[string]interface{}); ok {
			return devinACPExtractText(inner)
		}
	}
	if updateType, ok := update["sessionUpdate"].(string); ok {
		if updateType != "agent_message_chunk" {
			return ""
		}
		content, ok := update["content"].(map[string]interface{})
		if !ok {
			return ""
		}
		if contentType, _ := content["type"].(string); contentType != "text" {
			return ""
		}
		if text, ok := content["text"].(string); ok {
			return text
		}
	}
	return ""
}

// devinACPIsPermissionRequest reports whether a server->client method is a
// permission request routed to bridge.RequestApproval.
func devinACPIsPermissionRequest(method string) bool {
	return strings.TrimSpace(method) == "session/request_permission"
}

// devinACPIsClientCapabilityRequest reports whether a server->client request
// asks for a client capability FlowPilot implements: fs/read_text_file and
// fs/write_text_file are advertised in initialize (Devin emits them while
// running its own tools). fs.write_text_file must be gated by posture the
// same way session/request_permission is — the adapter owns that decision.
func devinACPIsClientCapabilityRequest(method string) bool {
	switch strings.TrimSpace(method) {
	case "fs/read_text_file", "fs/write_text_file":
		return true
	default:
		return false
	}
}

// devinACPIsExtensionNotification reports whether a method is a Devin custom
// notification (`_cognition.ai/*`). These are informational only — output
// channel lines, turn stats, MCP server lifecycle — and must be tolerated
// (never error-replied, never fatal to the dispatcher).
func devinACPIsExtensionNotification(method string) bool {
	return strings.HasPrefix(strings.TrimSpace(method), "_cognition.ai/")
}

// devinACPFlowPilotMCPServers builds one ACP mcpServers[] entry for the
// FlowPilot MCP server. Devin's mcpCapabilities{http:false,sse:false} mean
// the HTTP loopback server CANNOT be injected — the FlowPilot tools reach
// Devin through a stdio shim instead (devin_mcp_stdio.go, Task-403), and the
// runner-hosted HTTP entry stays registered in ~/.config/devin/
// mcp_config.json for the REPL path. This helper returns the stdio entry
// when a shim command is provided.
func devinACPStdioMCPServerEntry(name, command string, args []string, env map[string]string) map[string]interface{} {
	name = strings.TrimSpace(name)
	command = strings.TrimSpace(command)
	if name == "" || command == "" {
		return nil
	}
	// Devin's McpServer is an untagged enum — the stdio variant is selected
	// purely by field shape: command+args+env must ALL be present (env is a
	// required field, live-verified R1: omitting it or adding a "type"
	// discriminator both fail with -32602 "did not match any variant").
	entry := map[string]interface{}{
		"name":    name,
		"command": command,
		"args":    args,
		"env":     devinACPNameValueList(env),
	}
	return entry
}

// devinACPExtraMCPServers converts the shared claudeMcpServer config map into
// ACP mcpServers[] entries. Devin only supports stdio transport over ACP —
// url-only entries are skipped (they belong in mcp_config.json).
func devinACPExtraMCPServers(extra map[string]claudeMcpServer) []interface{} {
	if len(extra) == 0 {
		return nil
	}
	out := make([]interface{}, 0, len(extra))
	for name, server := range extra {
		name = strings.TrimSpace(name)
		command := strings.TrimSpace(server.Command)
		if name == "" || command == "" {
			continue
		}
		entry := map[string]interface{}{
			"name":    name,
			"command": command,
			"args":    server.Args,
			"env":     devinACPNameValueList(server.Env),
		}
		out = append(out, entry)
	}
	return out
}

// devinACPNameValueList converts a string map to ACP's array-of-{name,value}.
func devinACPNameValueList(m map[string]string) []interface{} {
	if len(m) == 0 {
		return []interface{}{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]interface{}, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]interface{}{"name": k, "value": m[k]})
	}
	return out
}

// devinConfigOptionFromResult returns the configOptions entry with the given
// id out of a session/new, session/load, set_config_option result, or a
// config_option_update notification's update map.
func devinConfigOptionFromResult(result map[string]interface{}, id string) map[string]interface{} {
	if result == nil {
		return nil
	}
	raw, _ := result["configOptions"].([]interface{})
	for _, entry := range raw {
		m, _ := entry.(map[string]interface{})
		if m == nil {
			continue
		}
		if optID, _ := m["id"].(string); optID == id {
			return m
		}
	}
	return nil
}

// devinConfigOptionChoices extracts the {value,name,supportsImages} choices
// of one config option — the model catalog lives on the "model" option.
func devinConfigOptionChoices(option map[string]interface{}) []DevinConfigChoice {
	if option == nil {
		return nil
	}
	raw, _ := option["options"].([]interface{})
	out := make([]DevinConfigChoice, 0, len(raw))
	for _, entry := range raw {
		m, _ := entry.(map[string]interface{})
		if m == nil {
			continue
		}
		v, _ := m["value"].(string)
		if strings.TrimSpace(v) == "" {
			continue
		}
		choice := DevinConfigChoice{Value: v}
		choice.Name, _ = m["name"].(string)
		choice.Description, _ = m["description"].(string)
		if meta, ok := m["_meta"].(map[string]interface{}); ok {
			choice.Meta = meta
		}
		out = append(out, choice)
	}
	return out
}

// devinChoiceSupportsImages reads _meta["cognition.ai/supportsImages"] off a
// model option choice (live-verified per-model vision flag).
func devinChoiceSupportsImages(choice DevinConfigChoice) bool {
	if choice.Meta == nil {
		return false
	}
	v, _ := choice.Meta["cognition.ai/supportsImages"].(bool)
	return v
}

// devinSessionModeFromResult returns the current session mode id out of a
// session/new or session/load result ({modes:{currentModeId}}), falling back
// to the "mode" config option's currentValue.
func devinSessionModeFromResult(result map[string]interface{}) string {
	if result == nil {
		return ""
	}
	if modes, ok := result["modes"].(map[string]interface{}); ok {
		if cur, _ := modes["currentModeId"].(string); strings.TrimSpace(cur) != "" {
			return strings.TrimSpace(cur)
		}
	}
	if opt := devinConfigOptionFromResult(result, "mode"); opt != nil {
		if cur, _ := opt["currentValue"].(string); strings.TrimSpace(cur) != "" {
			return strings.TrimSpace(cur)
		}
	}
	return ""
}
