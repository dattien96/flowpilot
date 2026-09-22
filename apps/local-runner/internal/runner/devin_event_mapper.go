package runner

import (
	"encoding/json"
	"strings"
)

// Task-401 (CP-70 T-3): maps Devin ACP `session/update` notifications into
// normalized ProviderEvent(s). Modeled on opencode_event_mapper.go, adapted
// for the Devin wire shapes captured live against 3000.10.31.
//
// Devin's session/update envelope is
// session/update{sessionId, update:{sessionUpdate:<kind>}} where sessionUpdate
// is one of: agent_message_chunk, agent_thought_chunk, user_message_chunk,
// tool_call, tool_call_update, usage_update, session_info_update,
// current_mode_update, available_commands_update, config_option_update, plan.
// Devin additionally emits `_cognition.ai/*` extension notifications
// (output/turn_stats/mcp serversChanged/agent_stopped/thinking_complete) —
// informational only; they never map to ProviderEvents.

func mapDevinNotification(n devinNotification) ([]ProviderEvent, bool) {
	switch n.Method {
	case "session/update":
		return mapDevinSessionUpdate(n.Params)
	default:
		// _cognition.ai/* extensions and any future unknown methods are
		// tolerated: never error, never emit (Task-400 DOD-10).
		return nil, false
	}
}

func mapDevinSessionUpdate(params map[string]any) ([]ProviderEvent, bool) {
	update, _ := params["update"].(map[string]any)
	if update == nil {
		return nil, false
	}
	kind, _ := update["sessionUpdate"].(string)
	switch kind {
	case "agent_message_chunk":
		text := devinTextContent(update["content"])
		if text == "" {
			return nil, false
		}
		return []ProviderEvent{{Type: EventMessageDelta, Text: text}}, true

	case "agent_thought_chunk":
		// Devin streams chain-of-thought as agent_thought_chunk. FlowPilot's
		// provider-neutral event union has no thought channel — drop them
		// (same honest posture as the OpenCode mapper).
		return nil, false

	case "tool_call":
		title, _ := update["title"].(string)
		return []ProviderEvent{{
			Type:     EventToolStarted,
			ToolName: devinToolDisplayName(update, title),
			Input:    update["rawInput"],
		}}, true

	case "tool_call_update":
		return mapDevinToolCallUpdate(update)

	case "usage_update":
		if token := devinUsageUpdateToSnapshot(update); token != nil {
			return []ProviderEvent{{Type: EventTokenUsageUpdated, TokenUsage: token}}, true
		}
		return nil, false

	default:
		// user_message_chunk, session_info_update, current_mode_update,
		// available_commands_update, config_option_update, plan — tolerated.
		return nil, false
	}
}

func mapDevinToolCallUpdate(update map[string]any) ([]ProviderEvent, bool) {
	status, _ := update["status"].(string)
	if status == "" {
		return nil, false
	}
	title, _ := update["title"].(string)
	mutationKind := devinToolMutationKind(update)

	events := []ProviderEvent{{
		Type:     EventToolCompleted,
		ToolName: devinToolDisplayName(update, title),
		Status:   devinToolStatus(status),
		Output:   devinToolOutput(update),
	}}

	if mutationKind != "" {
		for _, path := range devinMutationPaths(update) {
			events = append(events, ProviderEvent{
				Type:       EventFileChanged,
				Path:       path,
				ChangeType: mutationKind,
			})
		}
	}
	return events, true
}

func devinIsFileMutationKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "edit", "write", "create", "delete":
		return true
	default:
		return false
	}
}

func devinToolMutationKind(update map[string]any) string {
	if update == nil {
		return ""
	}
	if kind, _ := update["kind"].(string); devinIsFileMutationKind(kind) {
		return strings.ToLower(strings.TrimSpace(kind))
	}
	if title, _ := update["title"].(string); title != "" {
		if mapped := devinMutationKindFromToolName(title); mapped != "" {
			return mapped
		}
	}
	return ""
}

func devinMutationKindFromToolName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "write", "write_file", "create", "create_file":
		return "write"
	case "delete", "delete_file":
		return "delete"
	case "edit", "search_replace", "strreplace", "str_replace", "apply_patch", "edit_file":
		return "edit"
	default:
		return ""
	}
}

func devinMutationPaths(update map[string]any) []string {
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
	for _, p := range devinLocationPaths(update["locations"]) {
		add(p)
	}
	if len(out) == 0 {
		for _, p := range devinPathsFromRawInput(update["rawInput"]) {
			add(p)
		}
	}
	return out
}

func devinLocationPaths(v any) []string {
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

func devinPathsFromRawInput(v any) []string {
	switch raw := v.(type) {
	case map[string]any:
		return devinPathsFromRawInputMap(raw)
	case string:
		s := strings.TrimSpace(raw)
		if s == "" {
			return nil
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			return nil
		}
		return devinPathsFromRawInputMap(m)
	default:
		return nil
	}
}

func devinPathsFromRawInputMap(m map[string]any) []string {
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

func devinToolStatus(status string) string {
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

func devinToolOutput(update map[string]any) any {
	var out any
	if v, ok := update["rawOutput"]; ok {
		out = v
	} else {
		out = update["content"]
	}
	return omitOversizedImageOutputDevin(out)
}

func omitOversizedImageOutputDevin(v any) any {
	if v == nil {
		return v
	}
	if s, ok := v.(string); ok {
		if len(s) > 64*1024 {
			return "[output omitted: " + itoaDevin(len(s)) + " bytes]"
		}
		return v
	}
	if m, ok := v.(map[string]any); ok {
		if data, ok := m["data"].(string); ok && len(data) > 10*1024 {
			if mt, _ := m["mimeType"].(string); mt != "" && containsFoldDevin(mt, "image") {
				return "[image omitted: " + itoaDevin(len(data)) + " bytes]"
			}
		}
		for _, val := range m {
			if s, ok := val.(string); ok && len(s) > 100*1024 {
				return "[output omitted: " + itoaDevin(len(s)) + " bytes]"
			}
		}
	}
	return v
}

func itoaDevin(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

func containsFoldDevin(s, substr string) bool {
	ls := len(s)
	lf := len(substr)
	if lf == 0 || ls < lf {
		return false
	}
	for i := 0; i <= ls-lf; i++ {
		match := true
		for j := 0; j < lf; j++ {
			c1 := s[i+j]
			c2 := substr[j]
			if c1 >= 'A' && c1 <= 'Z' {
				c1 += 'a' - 'A'
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 += 'a' - 'A'
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func devinToolDisplayName(update map[string]any, fallbackTitle string) string {
	if fallbackTitle != "" {
		return fallbackTitle
	}
	return "tool"
}

func devinTextContent(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if arr, ok := v.([]any); ok {
		var sb strings.Builder
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				if t, _ := m["text"].(string); t != "" {
					sb.WriteString(t)
					continue
				}
				if t, _ := m["content"].(string); t != "" {
					sb.WriteString(t)
				}
			} else if s, ok := item.(string); ok {
				sb.WriteString(s)
			}
		}
		return sb.String()
	}
	content, _ := v.(map[string]any)
	if content == nil {
		return ""
	}
	if typ, _ := content["type"].(string); typ != "" && typ != "text" && typ != "output_text" && typ != "content" {
		if t, _ := content["text"].(string); t != "" {
			return t
		}
		if t, _ := content["content"].(string); t != "" {
			return t
		}
		return ""
	}
	if t, _ := content["text"].(string); t != "" {
		return t
	}
	if t, _ := content["content"].(string); t != "" {
		return t
	}
	return ""
}

// devinUsageUpdateToSnapshot maps a usage_update notification. Live shape:
// {sessionUpdate:"usage_update", used, size,
// _meta:{"cognition.ai/inputTokens":N, "cognition.ai/outputTokens":N}} — used
// is the running token total, size the context window.
func devinUsageUpdateToSnapshot(update map[string]any) *TokenUsageSnapshot {
	if update == nil {
		return nil
	}
	used, _ := toInt64(update["used"])
	size, _ := toInt64(update["size"])
	var input, output int64
	if meta, ok := update["_meta"].(map[string]any); ok {
		input, _ = toInt64(meta["cognition.ai/inputTokens"])
		output, _ = toInt64(meta["cognition.ai/outputTokens"])
	}
	if used == 0 && size == 0 && input == 0 && output == 0 {
		return nil
	}
	var window *int64
	if size > 0 {
		w := int64(size)
		window = &w
	}
	total := used
	if total == 0 {
		total = input + output
	}
	breakdown := &TokenUsageBreakdown{
		TotalTokens:  total,
		InputTokens:  input,
		OutputTokens: output,
	}
	return &TokenUsageSnapshot{
		Last:               breakdown,
		Total:              breakdown,
		ModelContextWindow: window,
	}
}

// devinPromptResultTokenUsage maps a session/prompt response's usage fields
// ({totalTokens,inputTokens,outputTokens}) into a TokenUsageSnapshot.
func devinPromptResultTokenUsage(usage map[string]any, contextWindow *int64) *TokenUsageSnapshot {
	if usage == nil {
		return nil
	}
	total, okTotal := toInt64(usage["totalTokens"])
	input, okInput := toInt64(usage["inputTokens"])
	output, okOutput := toInt64(usage["outputTokens"])
	if !okTotal && !okInput && !okOutput && contextWindow == nil {
		return nil
	}
	breakdown := &TokenUsageBreakdown{
		TotalTokens:  total,
		InputTokens:  input,
		OutputTokens: output,
	}
	return &TokenUsageSnapshot{
		Last:               breakdown,
		Total:              breakdown,
		ModelContextWindow: contextWindow,
	}
}

// devinStopReasonToEvent maps a session/prompt stopReason to the terminal
// event. Live-observed: "end_turn". Quota-shaped reasons fail loudly
// (BUG-361 parity).
func devinStopReasonToEvent(stopReason string) ProviderEventType {
	switch strings.ToLower(strings.TrimSpace(stopReason)) {
	case "end_turn", "stop", "completed", "success":
		return EventTurnCompleted
	case "error", "failed", "aborted":
		return EventTurnFailed
	default:
		if devinIsQuotaStopReason(stopReason) {
			return EventTurnFailed
		}
		return EventTurnCompleted
	}
}

// devinIsQuotaStopReason reports whether a stopReason unambiguously signals
// quota/billing exhaustion (BUG-361 parity with opencodeIsQuotaStopReason).
func devinIsQuotaStopReason(stopReason string) bool {
	s := strings.ToLower(strings.TrimSpace(stopReason))
	if s == "" {
		return false
	}
	for _, tok := range []string{
		"rate_limit", "rate-limit", "rate_limited",
		"quota", "billing", "payment",
		"insufficient_credit", "insufficient credit",
		"usage_limit", "usage-limit",
	} {
		if strings.Contains(s, tok) {
			return true
		}
	}
	return false
}
