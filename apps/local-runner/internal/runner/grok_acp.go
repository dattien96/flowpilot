package runner

import "strings"

// Task-206 (CP-46 P-2/P-4, Task-206 T-2): standalone Grok ACP primitives.
//
// This file is a STANDALONE copy of the *shapes* already proven for Gemini in
// gemini_acp_transport.go — it does not import from, extract from, or modify
// that file (CP-46 P-0/P-4: "do NOT refactor gemini_acp_transport.go"; Gemini's
// transport stays byte-identical). Unifying the two ACP implementations is an
// explicit non-goal of CP-46.
//
// Every shape below was verified against a real `grok agent stdio` process
// (Grok Build 0.2.93) during Task-206 authoring; see testdata/grok_acp/.

// grokACPInitializeParams builds the `initialize` request params. fs.{read,write}
// TextFile are hard-set false (CP-46 P-6): Grok performs file I/O with its own
// tools and routes each write/edit through session/request_permission, so
// FlowPilot never needs to implement an ACP client-side filesystem server.
func grokACPInitializeParams() map[string]interface{} {
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

// grokACPSessionNewParams builds the `session/new` request params.
func grokACPSessionNewParams(cwd string, mcpServers []interface{}) map[string]interface{} {
	if mcpServers == nil {
		mcpServers = []interface{}{}
	}
	return map[string]interface{}{
		"cwd":        cwd,
		"mcpServers": mcpServers,
	}
}

// grokACPSessionLoadParams builds the resume request params. Live-verified
// (Task-206 T-2b): Grok's resume request is the ACP-standard `session/load`
// shape (sessionId/cwd/mcpServers) — NOT a distinct x.ai/* variant. Whether a
// session id remains resumable across a GROK_HOME change is still open (CP-46
// Q-2/R-7) and is not decided by this shape alone.
func grokACPSessionLoadParams(sessionID, cwd string, mcpServers []interface{}) map[string]interface{} {
	if mcpServers == nil {
		mcpServers = []interface{}{}
	}
	return map[string]interface{}{
		"sessionId":  strings.TrimSpace(sessionID),
		"cwd":        cwd,
		"mcpServers": mcpServers,
	}
}

// grokACPPromptParams builds the `session/prompt` request params. Grok's prompt
// content blocks use the same {type:"text",text} shape ACP/Gemini already use;
// image blocks are intentionally not built here — Vision stays false until
// initialize's promptCapabilities.image is proven true (live-verified false,
// CP-46 Q-1 / GR-27).
func grokACPPromptParams(sessionID, prompt string) map[string]interface{} {
	return map[string]interface{}{
		"sessionId": sessionID,
		"prompt": []map[string]string{
			{"type": "text", "text": prompt},
		},
	}
}

// grokACPResponseSessionID reads sessionId out of a session/new, session/load,
// or session/prompt result (Task-207 T-5/GR-32: session/prompt may return a
// different sessionId than session/new, which the adapter must adopt).
//
// Live-verified during Task-213 re-verification against a real `grok agent
// stdio` process: session/new puts sessionId at the top level of result, but
// session/load and session/prompt only carry it nested under result._meta —
// the top-level field is absent from both. Without the _meta fallback, resume
// (session/load) always failed with "no sessionId", and the GR-32
// post-prompt session-id-adoption check silently never fired.
func grokACPResponseSessionID(message map[string]interface{}) string {
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

// extractGrokACPText extracts the text delta from a `session/update`
// notification whose update.sessionUpdate == "agent_message_chunk" (the
// user-visible streamed reply text; agent_thought_chunk is reasoning and is
// intentionally not extracted here — see grok_event_mapper.go).
func extractGrokACPText(message map[string]interface{}) string {
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

// grokACPPromptResultText reads the terminal text out of a session/prompt
// result, if the result itself carries content (defensive; live-verified
// results carry only stopReason/_meta, with the actual reply text streamed via
// agent_message_chunk notifications beforehand).
func grokACPPromptResultText(result map[string]interface{}) string {
	if text, ok := result["text"].(string); ok && text != "" {
		return text
	}
	content, ok := result["content"].([]interface{})
	if !ok {
		return ""
	}
	var out strings.Builder
	for _, entry := range content {
		block, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		if blockType, _ := block["type"].(string); blockType != "text" {
			continue
		}
		if text, ok := block["text"].(string); ok {
			out.WriteString(text)
		}
	}
	return out.String()
}

// grokMCPServerEntry builds one ACP mcpServers[] entry for the runner-hosted
// FlowPilot MCP server (Task-209 T-1), reusing the same server name/path the
// Claude/Gemini adapters already point at (claudeMCPServerName, ClaudeMCPPath —
// provider-neutral, reused not mutated per CP-46 P-0).
func grokACPFlowPilotMCPServers(baseURL, token string) []interface{} {
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

// grokACPExtraMCPServers converts the shared claudeMcpServer config map (the
// same shape flowpilotClaudeExtraMCPServers/writeClaudeMCPConfig already build
// for Codex/Claude, google_drive_mcp_provider_config.go) into ACP mcpServers[]
// entries (Task-209 T-2). ACP entries are stdio-launched (command/args/env),
// mirroring config.toml's own [mcp_servers.*] shape rather than Claude's HTTP
// entry — Grok's mcpCapabilities only advertised http/sse for server-hosted
// tools (like the flowpilot entry above); externally-launched servers (Google
// Drive) go through the stdio launcher shape instead.
func grokACPExtraMCPServers(extra map[string]claudeMcpServer) []interface{} {
	if len(extra) == 0 {
		return nil
	}
	out := make([]interface{}, 0, len(extra))
	for name, server := range extra {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(server.Command) == "" {
			continue
		}
		entry := map[string]interface{}{
			"type":    "stdio",
			"name":    name,
			"command": server.Command,
			"args":    server.Args,
		}
		if len(server.Env) > 0 {
			env := make(map[string]interface{}, len(server.Env))
			for k, v := range server.Env {
				env[k] = v
			}
			entry["env"] = env
		}
		out = append(out, entry)
	}
	return out
}
