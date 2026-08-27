package runner

import (
	"encoding/json"
	"strings"
)

// Task-301 T-3 (CP-57 P-4/P-12): maps Opencode ACP `session/update` notifications
// into normalized ProviderEvent(s). Modeled on grok_event_mapper.go but adapted
// for Opencode's wire shapes captured live against 1.18.18.
//
// Opencode's session/update envelope is `session/update{sessionId, update:{sessionUpdate:<kind>}}`
// where sessionUpdate is one of: agent_message_chunk, tool_call, tool_call_update,
// usage_update, available_commands_update, user_message_chunk.

func mapOpencodeNotification(n opencodeNotification) ([]ProviderEvent, bool) {
	switch n.Method {
	case "session/update":
		return mapOpencodeSessionUpdate(n.Params)
	default:
		return nil, false
	}
}

func mapOpencodeSessionUpdate(params map[string]any) ([]ProviderEvent, bool) {
	update, _ := params["update"].(map[string]any)
	if update == nil {
		return nil, false
	}
	kind, _ := update["sessionUpdate"].(string)
	switch kind {
	case "agent_message_chunk":
		text := opencodeTextContent(update["content"])
		if text == "" {
			return nil, false
		}
		return []ProviderEvent{{Type: EventMessageDelta, Text: text}}, true

	case "agent_thought_chunk":
		return nil, false

	case "tool_call":
		title, _ := update["title"].(string)
		return []ProviderEvent{{
			Type:     EventToolStarted,
			ToolName: opencodeToolDisplayName(update, title),
			Input:    update["rawInput"],
		}}, true

	case "tool_call_update":
		return mapOpencodeToolCallUpdate(update)

	case "usage_update":
		// Surface token usage via event if needed; adapter also maps final result usage.
		// For now emit token_usage_updated from usage_update (used/size/cost).
		// This is additive; if not needed, caller can ignore.
		if token := opencodeUsageUpdateToSnapshot(update); token != nil {
			return []ProviderEvent{{Type: EventTokenUsageUpdated, TokenUsage: token}}, true
		}
		return nil, false

	default:
		return nil, false
	}
}

func mapOpencodeToolCallUpdate(update map[string]any) ([]ProviderEvent, bool) {
	status, _ := update["status"].(string)
	if status == "" {
		return nil, false
	}
	title, _ := update["title"].(string)
	mutationKind := opencodeToolMutationKind(update)

	events := []ProviderEvent{{
		Type:     EventToolCompleted,
		ToolName: opencodeToolDisplayName(update, title),
		Status:   opencodeToolStatus(status),
		Output:   opencodeToolOutput(update),
	}}

	if mutationKind != "" {
		for _, path := range opencodeMutationPaths(update) {
			events = append(events, ProviderEvent{
				Type:       EventFileChanged,
				Path:       path,
				ChangeType: mutationKind,
			})
		}
	}
	return events, true
}

func opencodeIsFileMutationKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "edit", "write", "create", "delete":
		return true
	default:
		return false
	}
}

func opencodeToolMutationKind(update map[string]any) string {
	if update == nil {
		return ""
	}
	if kind, _ := update["kind"].(string); opencodeIsFileMutationKind(kind) {
		return strings.ToLower(strings.TrimSpace(kind))
	}
	if title, _ := update["title"].(string); title != "" {
		if mapped := opencodeMutationKindFromToolName(title); mapped != "" {
			return mapped
		}
	}
	return ""
}

func opencodeMutationKindFromToolName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "write", "write_file", "create", "create_file":
		return "write"
	case "delete", "delete_file":
		return "delete"
	case "edit", "search_replace", "strreplace", "str_replace", "apply_patch", "edit_file":
		return "edit"
	case "read", "read_file", "list", "glob", "grep":
		return ""
	default:
		return ""
	}
}

func opencodeMutationPaths(update map[string]any) []string {
	if update == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, p := range opencodeLocationPaths(update["locations"]) {
		add(p)
	}
	if len(out) == 0 {
		for _, p := range opencodePathsFromRawInput(update["rawInput"]) {
			add(p)
		}
	}
	return out
}

func opencodeLocationPaths(v any) []string {
	locations, _ := v.([]any)
	if len(locations) == 0 {
		return nil
	}
	out := make([]string, 0, len(locations))
	for _, raw := range locations {
		loc, _ := raw.(map[string]any)
		if loc == nil {
			continue
		}
		if path, _ := loc["path"].(string); strings.TrimSpace(path) != "" {
			out = append(out, strings.TrimSpace(path))
		}
	}
	return out
}

func opencodePathsFromRawInput(v any) []string {
	switch raw := v.(type) {
	case map[string]any:
		return opencodePathsFromRawInputMap(raw)
	case string:
		s := strings.TrimSpace(raw)
		if s == "" {
			return nil
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			return nil
		}
		return opencodePathsFromRawInputMap(m)
	default:
		return nil
	}
}

func opencodePathsFromRawInputMap(m map[string]any) []string {
	if m == nil {
		return nil
	}
	keys := []string{"path", "file_path", "filepath", "filePath", "target_file", "target_path", "filename"}
	var out []string
	for _, k := range keys {
		if s, _ := m[k].(string); strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func opencodeToolStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "success":
		return "success"
	case "failed", "error":
		return "failed"
	case "in_progress", "running", "pending":
		return status
	default:
		return status
	}
}

func opencodeToolOutput(update map[string]any) any {
	if out, ok := update["rawOutput"]; ok {
		return out
	}
	return update["content"]
}

func opencodeToolDisplayName(update map[string]any, fallbackTitle string) string {
	if fallbackTitle != "" {
		return fallbackTitle
	}
	return "tool"
}

func opencodeTextContent(v any) string {
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

func opencodeUsageUpdateToSnapshot(update map[string]any) *TokenUsageSnapshot {
	if update == nil {
		return nil
	}
	used, _ := toInt64(update["used"])
	size, _ := toInt64(update["size"])
	if used == 0 && size == 0 {
		return nil
	}
	// Map used as total tokens approximation; size as context window
	var window *int64
	if size > 0 {
		w := int64(size)
		window = &w
	}
	breakdown := &TokenUsageBreakdown{
		TotalTokens: int64(used),
	}
	return &TokenUsageSnapshot{
		Last:               breakdown,
		Total:              breakdown,
		ModelContextWindow: window,
	}
}

// opencodePromptResultTokenUsage maps a session/prompt response's usage fields
// (inputTokens/outputTokens/totalTokens/thoughtTokens/cachedReadTokens) plus
// optional contextWindow into a TokenUsageSnapshot.
func opencodePromptResultTokenUsage(usage map[string]any, contextWindow *int64) *TokenUsageSnapshot {
	if usage == nil {
		return nil
	}
	total, okTotal := toInt64(usage["totalTokens"])
	input, okInput := toInt64(usage["inputTokens"])
	output, okOutput := toInt64(usage["outputTokens"])
	thought, okThought := toInt64(usage["thoughtTokens"])
	cached, okCached := toInt64(usage["cachedReadTokens"])
	if !okTotal && !okInput && !okOutput && !okThought && !okCached && contextWindow == nil {
		return nil
	}
	breakdown := &TokenUsageBreakdown{
		TotalTokens:           total,
		InputTokens:           input,
		OutputTokens:          output,
		ReasoningOutputTokens: thought,
		CachedInputTokens:     cached,
	}
	return &TokenUsageSnapshot{
		Last:               breakdown,
		Total:              breakdown,
		ModelContextWindow: contextWindow,
	}
}

// opencodeContextWindowFromInit reads context window from initialize result if exposed.
// Opencode's initialize does not directly expose totalContextTokens like Grok;
// fallback to usage_update size or nil.
func opencodeContextWindowFromInit(initResult map[string]any) *int64 {
	if initResult == nil {
		return nil
	}
	// Try to find in agentCapabilities (not present for opencode) or return nil
	return nil
}

// Helper to emit turn completed/failed based on stopReason
func opencodeStopReasonToEvent(stopReason string) ProviderEventType {
	switch strings.ToLower(strings.TrimSpace(stopReason)) {
	case "end_turn", "stop", "completed", "success":
		return EventTurnCompleted
	case "error", "failed", "aborted":
		return EventTurnFailed
	default:
		if strings.TrimSpace(stopReason) == "" {
			return EventTurnCompleted
		}
		return EventTurnCompleted
	}
}
