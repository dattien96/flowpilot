package runner

import (
	"sort"
	"strings"
)

// Task-300 (CP-57 P-1/P-4, Task-300 T-1/T-2): standalone Opencode ACP primitives.
//
// This file is a STANDALONE copy of the shapes proven for Grok in
// grok_acp.go — it does not import from, extract from, or modify
// gemini_acp_transport.go (CP-57 P-0/P-4). Live wire shapes captured
// against opencode 1.18.18 (initialize / session/new / session/prompt /
// session/update / session/request_permission / turn_completed) in Task-300.
//
// Key live observations (see testdata/opencode_acp/):
// - initialize -> {protocolVersion, agentCapabilities{loadSession,mcpCapabilities{http,sse},promptCapabilities{embeddedContext,image},sessionCapabilities{close,fork,list,resume}}, authMethods, agentInfo}
// - session/new {cwd,mcpServers} -> {sessionId, configOptions[{id:"model",...},{id:"effort",...},{id:"mode",...}]}
// - session/prompt {sessionId, prompt:[{type:"text",text}]} -> streamed session/update + final {stopReason, usage{inputTokens,outputTokens,totalTokens,thoughtTokens}, _meta:{}} + usage_update
// - session/update {sessionUpdate:"agent_message_chunk", content:{type:"text",text}} is the streamed text
// - session/update {sessionUpdate:"tool_call"} / {sessionUpdate:"tool_call_update"} carry tool lifecycle
// - server->client request "session/request_permission" carries {sessionId, toolCall{toolCallId,title,kind,status,locations,rawInput}, options[{optionId,kind,name}]}
// - session/load {sessionId,cwd,mcpServers} is ACP-standard resume (verified via live capture)
// - model selection is via opencode.json config file (cwd-bound), not via session/new param — extra model/variant fields are accepted but ignored (preserved for future per-session routing).

// opencodeACPInitializeParams builds the `initialize` request params. fs.{read,write}
// TextFile are hard-set false (CP-57 P-6): Opencode performs file I/O with its own
// tools and routes each write/edit through session/request_permission, so
// FlowPilot never needs to implement an ACP client-side filesystem server.
func opencodeACPInitializeParams() map[string]interface{} {
	return map[string]interface{}{
		"protocolVersion": 1,
		"clientCapabilities": map[string]interface{}{
			"fs": map[string]interface{}{
				"readTextFile":  false,
				"writeTextFile": false,
			},
		},
		"clientInfo": map[string]interface{}{
			"name":    "flowpilot",
			"version": "1.0",
		},
	}
}

// opencodeACPSessionNewParams builds the `session/new` request params.
// Live shape is {"cwd": "...", "mcpServers": []}. Extra keys permission/model/variant
// are appended only when non-empty so golden fixtures capture the full intent
// without breaking a server that strictly validates the base shape (verified: extra
// model field is ignored, not rejected).
func opencodeACPSessionNewParams(cwd string, mcpServers []interface{}, permission []interface{}, model, variant string) map[string]interface{} {
	if mcpServers == nil {
		mcpServers = []interface{}{}
	}
	params := map[string]interface{}{
		"cwd":        cwd,
		"mcpServers": mcpServers,
	}
	if permission != nil {
		params["permission"] = permission
	}
	if strings.TrimSpace(model) != "" {
		params["model"] = strings.TrimSpace(model)
	}
	if strings.TrimSpace(variant) != "" {
		params["variant"] = strings.TrimSpace(variant)
	}
	return params
}

// opencodeACPSessionLoadParams builds the resume request params. Live-verified:
// Opencode's resume request is the ACP-standard `session/load` shape
// (sessionId/cwd/mcpServers) — same as Grok's session/load, not a distinct
// x.ai/* variant.
func opencodeACPSessionLoadParams(sessionID, cwd string, mcpServers []interface{}) map[string]interface{} {
	if mcpServers == nil {
		mcpServers = []interface{}{}
	}
	return map[string]interface{}{
		"sessionId":  strings.TrimSpace(sessionID),
		"cwd":        cwd,
		"mcpServers": mcpServers,
	}
}

// opencodeACPPromptParams builds the `session/prompt` request params. Prompt
// content blocks use {type:"text",text} shape same as Grok; image blocks are
// intentionally not built here — Vision stays false until
// promptCapabilities.image is proven true (live-verified true, but adapter
// keeps false until Task-301 proves attachment round-trip).
func opencodeACPPromptParams(sessionID, prompt string) map[string]interface{} {
	return map[string]interface{}{
		"sessionId": sessionID,
		"prompt": []map[string]string{
			{"type": "text", "text": prompt},
		},
	}
}

// opencodeACPResponseSessionID reads sessionId out of a session/new, session/load,
// or session/prompt result. Live-verified: session/new puts sessionId at top
// level of result; other methods may nest under _meta. Checks top-level first
// then _meta fallback like grokACPResponseSessionID (Task-207 DOD-6).
func opencodeACPResponseSessionID(message map[string]interface{}) string {
	result, ok := message["result"].(map[string]interface{})
	if !ok {
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

// opencodeACPResponseSessionIDFromResult is a convenience variant that operates
// directly on the result map (used by adapter after call() returns result).
func opencodeACPResponseSessionIDFromResult(result map[string]interface{}) string {
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

// extractOpencodeACPText extracts text delta from a `session/update` notification
// whose update.sessionUpdate == "agent_message_chunk" (user-visible streamed reply).
// Returns "" for other update types.
func extractOpencodeACPText(message map[string]interface{}) string {
	method, _ := message["method"].(string)
	if method != "session/update" {
		return ""
	}
	params, ok := message["params"].(map[string]interface{})
	if !ok {
		return ""
	}
	update, ok := params["update"].(map[string]interface{})
	if !ok {
		return ""
	}
	if updateType, _ := update["sessionUpdate"].(string); updateType != "agent_message_chunk" {
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
	return ""
}

// opencodeACPExtractText is the public alias expected by mapper (mirrors
// grokACPExtractText naming). Handles both raw message and update map shapes.
func opencodeACPExtractText(update map[string]interface{}) string {
	// Accept either full message {method,params:{update:{...}}} or just the update map itself.
	if _, hasMethod := update["method"].(string); hasMethod {
		return extractOpencodeACPText(update)
	}
	// Direct update map case: check sessionUpdate field
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
		return ""
	}
	// Also handle params wrapper without method: {params:{update:{...}}}
	if params, ok := update["params"].(map[string]interface{}); ok {
		if inner, ok := params["update"].(map[string]interface{}); ok {
			if updateType, _ := inner["sessionUpdate"].(string); updateType == "agent_message_chunk" {
				content, ok := inner["content"].(map[string]interface{})
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
		}
	}
	return ""
}

// opencodeACPIsPermissionRequest reports whether a server->client method is a
// permission request that must be routed to bridge.RequestApproval.
// Live-verified: Opencode emits "session/request_permission" (same as Grok).
// Also accept "question" / "permission" as defensive aliases per spec Q-3.
func opencodeACPIsPermissionRequest(method string) bool {
	switch strings.TrimSpace(method) {
	case "session/request_permission", "question", "permission":
		return true
	default:
		return false
	}
}

// opencodeACPFlowPilotMCPServers builds one ACP mcpServers[] entry for the runner-hosted
// FlowPilot MCP server, reusing the same server name/path the Claude/Grok adapters
// already point at (claudeMCPServerName, ClaudeMCPPath — provider-neutral).
func opencodeACPFlowPilotMCPServers(baseURL, token string) []interface{} {
	base := strings.TrimSpace(baseURL)
	tok := strings.TrimSpace(token)
	if base == "" || tok == "" {
		return nil
	}
	return []interface{}{
		map[string]interface{}{
			"type":    "http",
			"name":    claudeMCPServerName,
			"url":     strings.TrimRight(base, "/") + ClaudeMCPPath + "?token=" + tok,
			"headers": []interface{}{},
		},
	}
}

// opencodeACPExtraMCPServers converts the shared claudeMcpServer config map into
// ACP mcpServers[] entries (same shape as grokACPExtraMCPServers).
func opencodeACPExtraMCPServers(extra map[string]claudeMcpServer) []interface{} {
	if len(extra) == 0 {
		return nil
	}
	out := make([]interface{}, 0, len(extra))
	for name, server := range extra {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		command := strings.TrimSpace(server.Command)
		url := strings.TrimSpace(server.URL)
		switch {
		case command != "":
			entry := map[string]interface{}{
				"type":    "stdio",
				"name":    name,
				"command": command,
				"args":    server.Args,
			}
			if len(server.Env) > 0 {
				entry["env"] = opencodeACPNameValueList(server.Env)
			}
			out = append(out, entry)
		case url != "":
			entry := map[string]interface{}{
				"type": "http",
				"name": name,
				"url":  url,
			}
			entry["headers"] = opencodeACPNameValueList(server.Headers)
			out = append(out, entry)
		default:
			continue
		}
	}
	return out
}

// opencodeACPNameValueList converts a string map to ACP's array-of-{name,value} shape.
func opencodeACPNameValueList(m map[string]string) []interface{} {
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

// opencodeACPSessionSetConfigParams builds params for setting a session config option.
// Live Opencode ACP exposes configOptions via session/new result (model/effort/mode).
// The per-turn model/effort switch is via session/set_config_option (or legacy
// session/set_config). Live shape (ACP v1, reviewer verified) is {sessionId, configId, value}.
// We try configId first; caller degrades on error to legacy key/name shapes.
func opencodeACPSessionSetConfigParams(sessionID, key, value string) map[string]interface{} {
	return map[string]interface{}{
		"sessionId": strings.TrimSpace(sessionID),
		"configId":  strings.TrimSpace(key),
		"value":     strings.TrimSpace(value),
	}
}

// opencodeACPSessionSetConfigParamsAlt is the alternative shape some builds expect
// (key/value). Kept for graceful fallback when server rejects configId.
func opencodeACPSessionSetConfigParamsAlt(sessionID, key, value string) map[string]interface{} {
	return map[string]interface{}{
		"sessionId": strings.TrimSpace(sessionID),
		"key":       strings.TrimSpace(key),
		"value":     strings.TrimSpace(value),
	}
}

// opencodeACPFlowPilotMCPServerEntry is an alias for the single-entry builder
// expected by some specs (singular name).
func opencodeACPFlowPilotMCPServerEntry(baseURL, token string) map[string]interface{} {
	servers := opencodeACPFlowPilotMCPServers(baseURL, token)
	if len(servers) == 0 {
		return nil
	}
	if m, ok := servers[0].(map[string]interface{}); ok {
		return m
	}
	return nil
}

// opencodeEffortOptionsFromConfig (CA-689b) extracts the effort select from a
// session/set_config_option response's configOptions: the option values are
// the freshly-selected model's real variants and currentValue its default.
// Returns empty when the payload lacks an effort entry.
func opencodeEffortOptionsFromConfig(result map[string]any) ([]string, string) {
	if result == nil {
		return nil, ""
	}
	raw, _ := result["configOptions"].([]any)
	for _, entry := range raw {
		m, _ := entry.(map[string]any)
		if m == nil {
			continue
		}
		if id, _ := m["id"].(string); !strings.EqualFold(id, "effort") {
			continue
		}
		current, _ := m["currentValue"].(string)
		opts, _ := m["options"].([]any)
		var efforts []string
		for _, opt := range opts {
			om, _ := opt.(map[string]any)
			if om == nil {
				continue
			}
			v, _ := om["value"].(string)
			if v == "" {
				continue
			}
			efforts = append(efforts, v)
		}
		return efforts, current
	}
	return nil, ""
}
