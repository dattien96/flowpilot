package runner

import "strings"

// Phase 3 (04-03): maps Codex app-server notifications → normalized ProviderEvent.
// The runner core + clients only ever see ProviderEvent; Codex wire shapes stay
// here. Correlation (run/step/session ids) and the monotonic per-run `seq` are
// stamped by runner core when the event is emitted (interactive_service.emitLocked)
// — this mapper only sets the type-specific fields + providerTurnId.
//
// The Codex method/param names are the assumed app-server schema; verify them
// against the installed build (06 Part D). Centralizing them here means a schema
// drift is a one-file change.

// mapCodexNotification converts a routed Codex notification into a ProviderEvent.
// Returns ok=false for notifications that carry no client-facing event.
func mapCodexNotification(n codexNotification) (ProviderEvent, bool) {
	p := n.Params
	str := func(k string) string {
		if p == nil {
			return ""
		}
		s, _ := p[k].(string)
		return s
	}
	turnID := str("turnId")

	switch n.Method {
	case "error":
		msg := codexErrorMessage(p)
		if msg == "" {
			return ProviderEvent{}, false
		}
		return ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: msg, Recoverable: false}, true

	case "turn.started", "codex/turn.started", "turn/started":
		return ProviderEvent{Type: EventTurnStarted, ProviderTurnID: turnID}, true

	case "turn.delta", "codex/agent_message_chunk", "agent_message_chunk", "item/agentMessage/delta":
		text := str("text")
		if text == "" {
			text = str("delta")
		}
		if text == "" {
			return ProviderEvent{}, false
		}
		return ProviderEvent{Type: EventMessageDelta, ProviderTurnID: turnID, Text: text}, true

	case "thread/tokenUsage/updated":
		usage := codexTokenUsage(paramAny(p, "tokenUsage"))
		if usage == nil {
			return ProviderEvent{}, false
		}
		return ProviderEvent{
			Type:           EventTokenUsageUpdated,
			ProviderTurnID: turnID,
			TokenUsage:     usage,
		}, true

	case "turn.message", "codex/agent_message", "agent_message":
		return ProviderEvent{Type: EventMessageCompleted, ProviderTurnID: turnID, Text: str("text")}, true

	case "item/completed":
		return mapCodexCompletedItem(p, turnID)

	case "tool.started", "mcp.tool.started":
		return ProviderEvent{Type: EventToolStarted, ProviderTurnID: turnID, ToolName: str("name"), Input: paramAny(p, "input")}, true

	case "tool.completed", "mcp.tool.completed":
		return ProviderEvent{
			Type: EventToolCompleted, ProviderTurnID: turnID,
			ToolName: str("name"), Status: defaultStatus(str("status")), Output: paramAny(p, "output"),
		}, true

	// Command execution maps to DISTINCT command events (04-03 / PP-29) with exit status.
	case "command.started", "exec.started", "codex/exec_command_begin":
		name := str("command")
		if name == "" {
			name = "command"
		}
		return ProviderEvent{Type: EventToolStarted, ProviderTurnID: turnID, ToolName: name, Input: paramAny(p, "command")}, true

	case "command.completed", "exec.completed", "codex/exec_command_end":
		status := "success"
		if code, ok := toInt(paramAny(p, "exitCode")); ok && code != 0 {
			status = "failed"
		}
		if s := str("status"); s != "" {
			status = s
		}
		name := str("command")
		if name == "" {
			name = "command"
		}
		return ProviderEvent{
			Type: EventToolCompleted, ProviderTurnID: turnID, ToolName: name,
			Status: status, Output: paramAny(p, "output"),
		}, true

	case "file.changed", "codex/file_change":
		return ProviderEvent{Type: EventFileChanged, ProviderTurnID: turnID, Path: str("path"), ChangeType: str("changeType")}, true

	case "turn.failed", "codex/turn.failed":
		rec, _ := p["recoverable"].(bool)
		return ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: str("error"), Recoverable: rec}, true

	case "turn.completed", "codex/turn.completed", "turn/completed":
		if msg := codexTurnErrorMessage(paramAny(p, "turn")); msg != "" {
			return ProviderEvent{Type: EventTurnFailed, ProviderTurnID: turnID, Error: msg, Recoverable: false}, true
		}
		final := str("finalMessage")
		if final == "" {
			final = str("text")
		}
		if final == "" {
			final = lastAgentMessageText(paramAny(p, "turn"))
		}
		return ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: turnID, FinalMessage: final}, true
	}
	return ProviderEvent{}, false
}

// mapCodexRolloutLine converts one on-disk Codex rollout JSONL entry into zero or
// more ProviderEvents for transcript replay (Task-067). The rollout file is the
// durable session log written by the Codex CLI; its shape differs from the live
// app-server protocol handled by mapCodexNotification. Each turn is recorded twice
// — as canonical `response_item` entries and as a parallel `event_msg` UI stream —
// so we read only the response_item entries to avoid double-rendering. As with the
// Claude transcript replay, only assistant output is emitted (user prompts are
// re-rendered client-side), plus tool/command activity.
func mapCodexRolloutLine(raw map[string]any) []ProviderEvent {
	if t, _ := raw["type"].(string); t != "response_item" {
		return nil
	}
	p, _ := raw["payload"].(map[string]any)
	if p == nil {
		return nil
	}
	switch pt, _ := p["type"].(string); pt {
	case "message":
		role, _ := p["role"].(string)
		text := codexRolloutMessageText(p["content"])
		if text == "" {
			return nil
		}
		switch role {
		case "assistant":
			return []ProviderEvent{{Type: EventMessageCompleted, Text: text}}
		case "user":
			// CLI-injected context frames (AGENTS.md, <INSTRUCTIONS>, <environment_context>)
			// are stored as role:user in the rollout but must not render as prompt bubbles
			// (BUG-083 F-2).  ProviderTurnID is stamped by loadCodexTranscriptEvents.
			if isCodexInjectedContext(text) {
				return nil
			}
			return []ProviderEvent{{Type: EventTurnStarted, Prompt: text}}
		default:
			return nil // developer/system are not client-facing
		}
	case "function_call", "custom_tool_call":
		name := stringDefault(stringAny(p, "name"), "tool")
		return []ProviderEvent{{Type: EventToolStarted, ToolName: name, Input: p["arguments"]}}
	case "function_call_output", "custom_tool_call_output":
		// Completed events carry no tool name (correlated by order, as in the Claude
		// mapper); the structured exit code is not in the rollout, so assume success.
		return []ProviderEvent{{Type: EventToolCompleted, Status: "success", Output: p["output"]}}
	case "web_search_call":
		return []ProviderEvent{
			{Type: EventToolStarted, ToolName: "web_search"},
			{Type: EventToolCompleted, ToolName: "web_search", Status: "success"},
		}
	}
	return nil // reasoning and other items are not client-facing transcript events
}

// codexRolloutMessageText joins the text of a rollout message's content blocks
// (output_text for assistant, input_text for user; input_image carries no text).
func codexRolloutMessageText(v any) string {
	blocks, _ := v.([]any)
	var parts []string
	for _, b := range blocks {
		block, _ := b.(map[string]any)
		if block == nil {
			continue
		}
		switch bt, _ := block["type"].(string); bt {
		case "output_text", "input_text", "text":
			if t, _ := block["text"].(string); t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, "")
}

// isCodexInjectedContext reports whether a role:user rollout message is a
// CLI-injected preamble rather than a user-typed prompt.  The Codex CLI
// injects AGENTS.md / CLAUDE.md instructions and a per-session
// <environment_context> block as a user-role message before every turn; these
// must not be rendered as prompt bubbles (BUG-083 F-2).
func isCodexInjectedContext(text string) bool {
	return strings.Contains(text, "<INSTRUCTIONS>") ||
		strings.Contains(text, "<environment_context>") ||
		strings.HasPrefix(strings.TrimSpace(text), "# AGENTS.md instructions for") ||
		strings.HasPrefix(strings.TrimSpace(text), "# CLAUDE.md instructions for")
}

func mapCodexCompletedItem(p map[string]any, turnID string) (ProviderEvent, bool) {
	item, _ := paramAny(p, "item").(map[string]any)
	if item == nil {
		return ProviderEvent{}, false
	}
	itemType, _ := item["type"].(string)
	switch itemType {
	case "agentMessage":
		text, _ := item["text"].(string)
		return ProviderEvent{Type: EventMessageCompleted, ProviderTurnID: turnID, Text: text}, true
	case "commandExecution":
		status := defaultStatus(stringAny(item, "status"))
		if code, ok := toInt(item["exitCode"]); ok && code != 0 {
			status = "failed"
		}
		return ProviderEvent{
			Type: EventToolCompleted, ProviderTurnID: turnID,
			ToolName: stringDefault(stringAny(item, "command"), "command"),
			Status:   status,
			Output:   item["aggregatedOutput"],
		}, true
	case "fileChange":
		return ProviderEvent{
			Type: EventFileChanged, ProviderTurnID: turnID,
			Path:       firstFileChangePath(item["changes"]),
			ChangeType: stringDefault(stringAny(item, "status"), "modified"),
		}, true
	case "mcpToolCall":
		return ProviderEvent{
			Type: EventToolCompleted, ProviderTurnID: turnID,
			ToolName: stringDefault(stringAny(item, "tool"), stringAny(item, "server")),
			Status:   defaultStatus(stringAny(item, "status")),
			Output:   item["result"],
		}, true
	}
	return ProviderEvent{}, false
}

func paramAny(p map[string]any, key string) any {
	if p == nil {
		return nil
	}
	return p[key]
}

func defaultStatus(s string) string {
	if s == "" {
		return "success"
	}
	if s == "completed" {
		return "success"
	}
	return s
}

func stringAny(p map[string]any, key string) string {
	if p == nil {
		return ""
	}
	s, _ := p[key].(string)
	return s
}

func stringDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func firstFileChangePath(v any) string {
	changes, _ := v.([]any)
	if len(changes) == 0 {
		return ""
	}
	first, _ := changes[0].(map[string]any)
	if first == nil {
		return ""
	}
	for _, key := range []string{"path", "file", "uri"} {
		if s, _ := first[key].(string); s != "" {
			return s
		}
	}
	return ""
}

func lastAgentMessageText(v any) string {
	turn, _ := v.(map[string]any)
	items, _ := turn["items"].([]any)
	for i := len(items) - 1; i >= 0; i-- {
		item, _ := items[i].(map[string]any)
		if item == nil || stringAny(item, "type") != "agentMessage" {
			continue
		}
		return stringAny(item, "text")
	}
	return ""
}

func codexErrorMessage(p map[string]any) string {
	if msg := stringAny(p, "message"); msg != "" {
		return msg
	}
	errObj, _ := paramAny(p, "error").(map[string]any)
	return stringAny(errObj, "message")
}

func codexTurnErrorMessage(v any) string {
	turn, _ := v.(map[string]any)
	if turn == nil {
		return ""
	}
	status, _ := turn["status"].(string)
	errObj, _ := turn["error"].(map[string]any)
	msg := stringAny(errObj, "message")
	if status == "failed" && msg == "" {
		return "Codex turn failed."
	}
	return msg
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case int32:
		return int64(n), true
	}
	return 0, false
}

func codexTokenUsage(v any) *TokenUsageSnapshot {
	root, _ := v.(map[string]any)
	if root == nil {
		return nil
	}
	usage := &TokenUsageSnapshot{
		Last:  codexTokenUsageBreakdown(root["last"]),
		Total: codexTokenUsageBreakdown(root["total"]),
	}
	if window, ok := toInt64(root["modelContextWindow"]); ok {
		usage.ModelContextWindow = &window
	}
	if usage.Last == nil && usage.Total == nil && usage.ModelContextWindow == nil {
		return nil
	}
	return usage
}

func codexTokenUsageBreakdown(v any) *TokenUsageBreakdown {
	root, _ := v.(map[string]any)
	if root == nil {
		return nil
	}
	cached, okCached := toInt64(root["cachedInputTokens"])
	input, okInput := toInt64(root["inputTokens"])
	output, okOutput := toInt64(root["outputTokens"])
	reasoning, okReasoning := toInt64(root["reasoningOutputTokens"])
	total, okTotal := toInt64(root["totalTokens"])
	if !okCached && !okInput && !okOutput && !okReasoning && !okTotal {
		return nil
	}
	return &TokenUsageBreakdown{
		CachedInputTokens:     cached,
		InputTokens:           input,
		OutputTokens:          output,
		ReasoningOutputTokens: reasoning,
		TotalTokens:           total,
	}
}
