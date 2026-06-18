package runner

import "strings"

// Phase 2 (07 plan): maps Claude stream-json frames → normalized ProviderEvent. The
// runner core + clients only ever see ProviderEvent; Claude wire shapes stay here.
// Unlike the Codex mapper (one notification → one event), a single Claude `assistant`
// or `user` frame can carry several content blocks, so this returns a slice.
//
// control_request frames are NOT mapped here — the adapter routes them to the bridge
// (claude_adapter.go). Correlation (run/step/session ids) and the monotonic per-run
// `seq` are stamped by runner core at emit time (interactive_service.emitLocked); this
// mapper only sets the type-specific fields and leaves ProviderTurnID empty.
//
// Wire shapes are the ASSUMED Claude Code stream-json schema (CLI ~2.1.x); verify
// against the pinned build (07 Appendix A spike).

// mapClaudeLine converts one frame into zero or more ProviderEvents.
func mapClaudeLine(l claudeLine) []ProviderEvent {
	switch l.Type {
	case "system":
		if l.Subtype == "init" {
			return []ProviderEvent{{Type: EventTurnStarted}}
		}
		return nil // system/api_retry etc. are not client-facing turn events
	case "stream_event":
		return mapClaudeStreamEvent(l.Raw)
	case "assistant":
		return mapClaudeAssistant(l.Raw)
	case "user":
		return mapClaudeUser(l.Raw)
	case "result":
		return mapClaudeResult(l.Raw)
	}
	return nil
}

// mapClaudeStreamEvent extracts a token delta from a partial-message stream_event
// (requires --include-partial-messages). It wraps a raw Anthropic API event.
func mapClaudeStreamEvent(raw map[string]any) []ProviderEvent {
	ev, _ := raw["event"].(map[string]any)
	if ev == nil {
		return nil
	}
	if t, _ := ev["type"].(string); t != "content_block_delta" {
		return nil
	}
	delta, _ := ev["delta"].(map[string]any)
	if delta == nil {
		return nil
	}
	if dt, _ := delta["type"].(string); dt != "text_delta" {
		return nil // thinking_delta / input_json_delta are not message text
	}
	text, _ := delta["text"].(string)
	if text == "" {
		return nil
	}
	return []ProviderEvent{{Type: EventMessageDelta, Text: text}}
}

// mapClaudeAssistant turns one assistant message into message_completed (joined text)
// + tool_started for each tool_use (+ a derived file_changed for the file-editing tools).
func mapClaudeAssistant(raw map[string]any) []ProviderEvent {
	content := claudeMessageContent(raw)
	var out []ProviderEvent
	var textParts []string
	for _, c := range content {
		block, _ := c.(map[string]any)
		if block == nil {
			continue
		}
		switch bt, _ := block["type"].(string); bt {
		case "text":
			if t, _ := block["text"].(string); t != "" {
				textParts = append(textParts, t)
			}
		case "tool_use":
			name, _ := block["name"].(string)
			input := block["input"]
			out = append(out, ProviderEvent{Type: EventToolStarted, ToolName: name, Input: input})
			if fc, ok := claudeFileChangeFromToolUse(name, input); ok {
				out = append(out, fc)
			}
		}
	}
	if len(textParts) > 0 {
		out = append(out, ProviderEvent{Type: EventMessageCompleted, Text: strings.Join(textParts, "")})
	}
	if usage := claudeTokenUsage(raw["usage"]); usage != nil {
		out = append(out, ProviderEvent{Type: EventTokenUsageUpdated, TokenUsage: &TokenUsageSnapshot{Last: usage}})
	}
	return out
}

// mapClaudeUser turns tool_result blocks (carried on the following user message) into
// tool_completed. Exit code for command execution is not in the result text — the
// structured value comes from a PostToolUse(Bash) hook (--include-hook-events), wired
// later; here is_error → failed (same convention as codex_event_mapper).
func mapClaudeUser(raw map[string]any) []ProviderEvent {
	content := claudeMessageContent(raw)
	var out []ProviderEvent
	for _, c := range content {
		block, _ := c.(map[string]any)
		if block == nil {
			continue
		}
		if bt, _ := block["type"].(string); bt != "tool_result" {
			continue
		}
		status := "success"
		if isErr, _ := block["is_error"].(bool); isErr {
			status = "failed"
		}
		out = append(out, ProviderEvent{Type: EventToolCompleted, Status: status, Output: block["content"]})
	}
	return out
}

// mapClaudeResult turns the terminal result frame into turn_completed/turn_failed.
func mapClaudeResult(raw map[string]any) []ProviderEvent {
	subtype, _ := raw["subtype"].(string)
	isError, _ := raw["is_error"].(bool)
	var out []ProviderEvent
	if usage := claudeTokenUsage(raw["usage"]); usage != nil {
		out = append(out, ProviderEvent{Type: EventTokenUsageUpdated, TokenUsage: &TokenUsageSnapshot{Last: usage}})
	}
	if isError || (subtype != "" && subtype != "success") {
		// error_during_execution is transient/recoverable; max_turns/budget are not.
		recoverable := subtype == "error_during_execution"
		out = append(out, ProviderEvent{Type: EventTurnFailed, Error: claudeResultErrorMessage(raw, subtype), Recoverable: recoverable})
		return out
	}
	final, _ := raw["result"].(string)
	out = append(out, ProviderEvent{Type: EventTurnCompleted, FinalMessage: final})
	return out
}

func claudeResultErrorMessage(raw map[string]any, subtype string) string {
	if msg := claudeUsageLimitMessage(raw); msg != "" {
		return msg
	}
	if errs, ok := raw["errors"].([]any); ok && len(errs) > 0 {
		if s, ok := errs[0].(string); ok && s != "" {
			if msg := claudeUsageLimitMessage(map[string]any{"message": s}); msg != "" {
				return msg
			}
			return s
		}
		if m, ok := errs[0].(map[string]any); ok {
			if s, _ := m["message"].(string); s != "" {
				if msg := claudeUsageLimitMessage(m); msg != "" {
					return msg
				}
				return s
			}
		}
	}
	if s, _ := raw["result"].(string); s != "" {
		if msg := claudeUsageLimitMessage(raw); msg != "" {
			return msg
		}
		return s
	}
	if subtype != "" {
		return "claude turn " + subtype
	}
	return "claude turn failed"
}

func claudeUsageLimitMessage(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	flat := strings.ToLower(flattenClaudeStrings(raw))
	if flat == "" {
		return ""
	}
	hasLimitSignal := strings.Contains(flat, "out_of_credits") ||
		strings.Contains(flat, "out of credits") ||
		strings.Contains(flat, "usage limit") ||
		strings.Contains(flat, "rate limit") ||
		strings.Contains(flat, "quota") ||
		strings.Contains(flat, "credits exhausted") ||
		strings.Contains(flat, "credit balance")
	if !hasLimitSignal {
		return ""
	}
	return "Claude usage limit reached. Switch Claude account or wait for quota reset."
}

func flattenClaudeStrings(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		var parts []string
		for _, value := range t {
			if s := flattenClaudeStrings(value); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	case []any:
		var parts []string
		for _, value := range t {
			if s := flattenClaudeStrings(value); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

// claudeUserPromptText extracts the typed user prompt text from a Claude user frame
// for transcript replay. Returns "" when the content contains any tool_result block
// (those frames are tool completions replayed as tool_completed events, not prompts).
// Used only by the transcript loader — the live mapper (mapClaudeUser) is unchanged.
func claudeUserPromptText(raw map[string]any) string {
	msg, _ := raw["message"].(map[string]any)
	if msg == nil {
		return ""
	}
	// Plain string content (older Claude CLI versions).
	if s, _ := msg["content"].(string); s != "" {
		return s
	}
	content := claudeMessageContent(raw)
	var textParts []string
	for _, c := range content {
		block, _ := c.(map[string]any)
		if block == nil {
			continue
		}
		switch bt, _ := block["type"].(string); bt {
		case "tool_result":
			return "" // frame is a tool completion, not a typed prompt
		case "text":
			if t, _ := block["text"].(string); t != "" {
				textParts = append(textParts, t)
			}
		}
	}
	return strings.Join(textParts, "")
}

// claudeSessionIDFromLine returns the real Claude session_id carried on system/result
// frames (used to --resume the real session on later turns, not the synthetic id).
func claudeSessionIDFromLine(l claudeLine) string {
	if l.Type != "system" && l.Type != "result" {
		return ""
	}
	s, _ := l.Raw["session_id"].(string)
	return s
}

// claudeMessageContent returns the content blocks of an assistant/user frame's nested
// Anthropic message (frame.message.content).
func claudeMessageContent(raw map[string]any) []any {
	msg, _ := raw["message"].(map[string]any)
	if msg == nil {
		return nil
	}
	content, _ := msg["content"].([]any)
	return content
}

// claudeFileChangeFromToolUse derives a file_changed event from a file-editing tool_use
// (no native per-edit event exists; 07 capability table). Write=created, edits=modified.
func claudeFileChangeFromToolUse(name string, input any) (ProviderEvent, bool) {
	in, _ := input.(map[string]any)
	if in == nil {
		return ProviderEvent{}, false
	}
	path, _ := in["file_path"].(string)
	if path == "" {
		path, _ = in["notebook_path"].(string)
	}
	if path == "" {
		return ProviderEvent{}, false
	}
	change := ""
	switch name {
	case "Write":
		change = "created"
	case "Edit", "MultiEdit", "NotebookEdit":
		change = "modified"
	default:
		return ProviderEvent{}, false
	}
	return ProviderEvent{Type: EventFileChanged, Path: path, ChangeType: change}, true
}

func claudeTokenUsage(v any) *TokenUsageBreakdown {
	usage, _ := v.(map[string]any)
	if usage == nil {
		return nil
	}
	input, okInput := claudeUsageInt(usage["input_tokens"])
	output, okOutput := claudeUsageInt(usage["output_tokens"])
	cached, okCached := claudeUsageInt(usage["cache_read_input_tokens"])
	if !okCached {
		cached, _ = claudeUsageInt(usage["cached_input_tokens"])
	}
	reasoning, okReasoning := claudeUsageInt(usage["reasoning_output_tokens"])
	total, okTotal := claudeUsageInt(usage["total_tokens"])
	if !okTotal {
		total = input + output + cached + reasoning
		okTotal = okInput || okOutput || okCached || okReasoning
	}
	if !okInput && !okOutput && !okCached && !okReasoning && !okTotal {
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

func claudeUsageInt(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	default:
		return 0, false
	}
}
