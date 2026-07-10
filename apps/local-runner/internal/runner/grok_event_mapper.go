package runner

import "strings"

// Task-207 (CP-46 P-4): maps Grok ACP `session/update` (and select `_x.ai/*`)
// notifications into normalized ProviderEvent(s). Modeled on
// mapCodexNotification (codex_event_mapper.go) but Grok unifies most signals
// under one `session/update{update:{sessionUpdate:<kind>}}` envelope rather
// than distinct top-level methods, and a single `tool_call_update` can carry
// BOTH a completion status and a file-path/kind that must become a distinct
// EventFileChanged — hence this mapper returns a slice, unlike Codex's single
// ProviderEvent return.
//
// All `sessionUpdate` discriminator values and field names below are
// live-verified against a real `grok agent stdio` process (0.2.93); see
// testdata/grok_acp/.

// mapGrokNotification converts a routed Grok notification into zero or more
// ProviderEvents. Returns ok=false (empty slice) for notifications that carry
// no client-facing event (e.g. `_x.ai/mcp/*`, `_x.ai/announcements/*`,
// `available_commands_update`, `user_message_chunk` — the last is the model's
// own echo of the prompt just sent, which would duplicate what the desktop
// already renders client-side).
func mapGrokNotification(n grokNotification) ([]ProviderEvent, bool) {
	switch n.Method {
	case "session/update":
		return mapGrokSessionUpdate(n.Params)
	default:
		// _x.ai/session_notification (turn_completed dup — the adapter uses the
		// session/prompt response for this instead), _x.ai/mcp/*,
		// _x.ai/announcements/*, _x.ai/settings/update, _x.ai/sessions/changed,
		// _x.ai/queue/changed, etc. are all internal/informational.
		return nil, false
	}
}

func mapGrokSessionUpdate(params map[string]any) ([]ProviderEvent, bool) {
	update, _ := params["update"].(map[string]any)
	if update == nil {
		return nil, false
	}
	kind, _ := update["sessionUpdate"].(string)
	switch kind {
	case "agent_message_chunk":
		text := grokTextContent(update["content"])
		if text == "" {
			return nil, false
		}
		return []ProviderEvent{{Type: EventMessageDelta, Text: text}}, true

	case "agent_thought_chunk":
		// Reasoning stream — internal-only per Task-207 open question (no desktop
		// UX guidance yet); not surfaced as a client-facing event.
		return nil, false

	case "tool_call":
		// toolCallId correlates with a later tool_call_update at the
		// adapter/permission layer; ProviderEvent has no such field, so it is
		// not carried on the client-facing event itself.
		title, _ := update["title"].(string)
		return []ProviderEvent{{
			Type:     EventToolStarted,
			ToolName: grokToolDisplayName(update, title),
			Input:    update["rawInput"],
		}}, true

	case "tool_call_update":
		return mapGrokToolCallUpdate(update)

	default:
		return nil, false
	}
}

// mapGrokToolCallUpdate handles a `tool_call_update`. Live-verified shape:
// {toolCallId, status, kind, title, locations:[{path}], content, rawOutput}.
// A file-mutating call (kind "edit"/"write"/"create"/"delete") emits BOTH a
// tool_completed AND a file_changed event; a read-only call (kind "read") emits
// only tool_completed.
func mapGrokToolCallUpdate(update map[string]any) ([]ProviderEvent, bool) {
	status, _ := update["status"].(string)
	if status == "" {
		// A mid-flight update with no status (still running) carries no
		// client-facing event yet.
		return nil, false
	}
	kind, _ := update["kind"].(string)
	title, _ := update["title"].(string)

	events := []ProviderEvent{{
		Type:     EventToolCompleted,
		ToolName: grokToolDisplayName(update, title),
		Status:   grokToolStatus(status),
		Output:   grokToolOutput(update),
	}}

	if grokIsFileMutationKind(kind) {
		if path := grokFirstLocationPath(update["locations"]); path != "" {
			events = append(events, ProviderEvent{
				Type:       EventFileChanged,
				Path:       path,
				ChangeType: kind,
			})
		}
	}
	return events, true
}

func grokIsFileMutationKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "edit", "write", "create", "delete":
		return true
	default:
		return false
	}
}

func grokToolStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "success":
		return "success"
	case "failed", "error":
		return "failed"
	default:
		return status
	}
}

func grokToolOutput(update map[string]any) any {
	if out, ok := update["rawOutput"]; ok {
		return out
	}
	return update["content"]
}

// grokToolDisplayName prefers the structured x.ai/tool name (stable, machine
// name) over the human title (which Grok formats per-call, e.g.
// "Read `C:\...\sample.txt`") so the desktop's tool icon/grouping logic gets a
// consistent name across the started/completed pair.
func grokToolDisplayName(update map[string]any, fallbackTitle string) string {
	meta, _ := update["_meta"].(map[string]any)
	if meta != nil {
		if toolMeta, ok := meta["x.ai/tool"].(map[string]any); ok {
			if name, _ := toolMeta["name"].(string); name != "" {
				return name
			}
		}
	}
	if fallbackTitle != "" {
		return fallbackTitle
	}
	return "tool"
}

func grokFirstLocationPath(v any) string {
	locations, _ := v.([]any)
	if len(locations) == 0 {
		return ""
	}
	first, _ := locations[0].(map[string]any)
	if first == nil {
		return ""
	}
	path, _ := first["path"].(string)
	return path
}

func grokTextContent(v any) string {
	content, _ := v.(map[string]any)
	if content == nil {
		return ""
	}
	if contentType, _ := content["type"].(string); contentType != "text" {
		return ""
	}
	text, _ := content["text"].(string)
	return text
}

// grokPromptResultTokenUsage maps a session/prompt response's `_meta` token
// fields (live-verified: totalTokens/inputTokens/outputTokens/
// cachedReadTokens/reasoningTokens) plus the model's totalContextTokens
// (live-verified in `initialize`) into a TokenUsageSnapshot (CP-46 P-14 /
// GR-24).
func grokPromptResultTokenUsage(meta map[string]any, contextWindow *int64) *TokenUsageSnapshot {
	if meta == nil {
		return nil
	}
	total, okTotal := toInt64(meta["totalTokens"])
	input, okInput := toInt64(meta["inputTokens"])
	output, okOutput := toInt64(meta["outputTokens"])
	cached, okCached := toInt64(meta["cachedReadTokens"])
	reasoning, okReasoning := toInt64(meta["reasoningTokens"])
	if !okTotal && !okInput && !okOutput && !okCached && !okReasoning && contextWindow == nil {
		return nil
	}
	breakdown := &TokenUsageBreakdown{
		TotalTokens:           total,
		InputTokens:           input,
		OutputTokens:          output,
		CachedInputTokens:     cached,
		ReasoningOutputTokens: reasoning,
	}
	return &TokenUsageSnapshot{
		Last:               breakdown,
		Total:              breakdown,
		ModelContextWindow: contextWindow,
	}
}

// grokContextWindowFromInit reads the current model's totalContextTokens out
// of a captured `initialize` result (live-verified: 500000 for grok-4.5).
func grokContextWindowFromInit(initResult map[string]any) *int64 {
	if initResult == nil {
		return nil
	}
	meta, _ := initResult["_meta"].(map[string]any)
	if meta == nil {
		return nil
	}
	modelState, _ := meta["modelState"].(map[string]any)
	if modelState == nil {
		return nil
	}
	currentModelID, _ := modelState["currentModelId"].(string)
	models, _ := modelState["availableModels"].([]any)
	for _, m := range models {
		model, _ := m.(map[string]any)
		if model == nil {
			continue
		}
		modelID, _ := model["modelId"].(string)
		if currentModelID != "" && modelID != currentModelID {
			continue
		}
		modelMeta, _ := model["_meta"].(map[string]any)
		if modelMeta == nil {
			continue
		}
		if window, ok := toInt64(modelMeta["totalContextTokens"]); ok {
			return &window
		}
	}
	return nil
}

// grokReasoningEffortID maps FlowPilot's canonical ReasoningEffort values to a
// Grok ACP effort id. Live-verified: grok-4.5's initialize.reasoningEfforts
// only lists high/medium/low (NOT the full none/minimal/low/medium/high/xhigh/
// max set CP-46 assumed) — none/minimal degrade to low, xhigh/max degrade to
// high. Returns ("", false) for a value with no reasonable mapping, so the
// caller omits the field and the model's own default applies (GR-35).
func grokReasoningEffortID(effort string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low", "minimal", "none":
		return "low", true
	case "medium":
		return "medium", true
	case "high", "xhigh", "max":
		return "high", true
	default:
		return "", false
	}
}
