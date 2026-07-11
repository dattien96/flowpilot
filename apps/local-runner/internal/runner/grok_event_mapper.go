package runner

import (
	"encoding/json"
	"strings"
)

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
//
// Task-212 DOD-2: Grok often puts mutation kind under `_meta.x.ai/tool.kind`
// (or only the tool name) and the path under `rawInput` rather than top-level
// `kind` + `locations`. Post-turn flowgate r-ca only sees WrittenPaths from
// EventFileChanged, so those mutations must still map here.
func mapGrokToolCallUpdate(update map[string]any) ([]ProviderEvent, bool) {
	status, _ := update["status"].(string)
	if status == "" {
		// A mid-flight update with no status (still running) carries no
		// client-facing event yet.
		return nil, false
	}
	title, _ := update["title"].(string)
	mutationKind := grokToolMutationKind(update)

	events := []ProviderEvent{{
		Type:     EventToolCompleted,
		ToolName: grokToolDisplayName(update, title),
		Status:   grokToolStatus(status),
		Output:   grokToolOutput(update),
	}}

	if mutationKind != "" {
		for _, path := range grokMutationPaths(update) {
			events = append(events, ProviderEvent{
				Type:       EventFileChanged,
				Path:       path,
				ChangeType: mutationKind,
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

// grokToolMutationKind resolves the file-mutation kind for a tool_call/_update.
// Order: top-level kind → _meta.x.ai/tool.kind → known tool name (meta/title).
// Returns "" when the call is not a file mutation.
func grokToolMutationKind(update map[string]any) string {
	if update == nil {
		return ""
	}
	if kind, _ := update["kind"].(string); grokIsFileMutationKind(kind) {
		return strings.ToLower(strings.TrimSpace(kind))
	}
	if toolMeta := grokXAIToolMeta(update); toolMeta != nil {
		if kind, _ := toolMeta["kind"].(string); grokIsFileMutationKind(kind) {
			return strings.ToLower(strings.TrimSpace(kind))
		}
		if name, _ := toolMeta["name"].(string); name != "" {
			if mapped := grokMutationKindFromToolName(name); mapped != "" {
				return mapped
			}
		}
	}
	if title, _ := update["title"].(string); title != "" {
		if mapped := grokMutationKindFromToolName(title); mapped != "" {
			return mapped
		}
	}
	return ""
}

// grokMutationKindFromToolName maps Grok native tool names onto the narrow
// mutation allowlist used for EventFileChanged / r-ca WrittenPaths.
func grokMutationKindFromToolName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "write", "write_file", "create", "create_file":
		return "write"
	case "delete", "delete_file":
		return "delete"
	case "edit", "search_replace", "strreplace", "str_replace", "apply_patch":
		return "edit"
	default:
		return ""
	}
}

func grokXAIToolMeta(update map[string]any) map[string]any {
	if update == nil {
		return nil
	}
	meta, _ := update["_meta"].(map[string]any)
	if meta == nil {
		return nil
	}
	toolMeta, _ := meta["x.ai/tool"].(map[string]any)
	return toolMeta
}

// grokMutationPaths returns de-duplicated file paths for a mutation tool call.
// Prefer locations[].path; fall back to structured rawInput path fields and
// `_meta.x.ai/tool.input` (live Grok search_replace).
func grokMutationPaths(update map[string]any) []string {
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
	for _, p := range grokLocationPaths(update["locations"]) {
		add(p)
	}
	if len(out) == 0 {
		for _, p := range grokPathsFromRawInput(update["rawInput"]) {
			add(p)
		}
	}
	if len(out) == 0 {
		if toolMeta := grokXAIToolMeta(update); toolMeta != nil {
			if input, ok := toolMeta["input"].(map[string]any); ok {
				for _, p := range grokPathsFromRawInputMap(input) {
					add(p)
				}
			}
		}
	}
	return out
}

// grokPendingToolCall caches mutation kind/path from earlier tool_call /
// mid-flight tool_call_update frames. Live Grok (0.2.x) puts kind+locations on
// the first tool_call_update (status unset) but the final
// status=completed frame only has content/rawOutput — so without this cache
// EventFileChanged never fires and flowgate r-ca WrittenPaths stays empty
// (Task-212 DOD-2, live gate-sandbox 2026-07-11).
type grokPendingToolCall struct {
	mutationKind string
	paths        []string
}

// grokRememberToolCall stores mutation metadata from a tool_call or
// tool_call_update update map into cache (keyed by toolCallId).
func grokRememberToolCall(cache map[string]grokPendingToolCall, update map[string]any) {
	if cache == nil || update == nil {
		return
	}
	id, _ := update["toolCallId"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	kind := grokToolMutationKind(update)
	paths := grokMutationPaths(update)
	if kind == "" && len(paths) == 0 {
		return
	}
	prev := cache[id]
	if kind != "" {
		prev.mutationKind = kind
	}
	if len(paths) > 0 {
		prev.paths = paths
	}
	cache[id] = prev
}

// grokEnrichToolCallUpdate merges cached kind/paths into a completed
// tool_call_update that no longer carries them. Returns a shallow-copied update.
func grokEnrichToolCallUpdate(cache map[string]grokPendingToolCall, update map[string]any) map[string]any {
	if update == nil {
		return update
	}
	id, _ := update["toolCallId"].(string)
	prev, ok := cache[strings.TrimSpace(id)]
	if !ok {
		return update
	}
	out := make(map[string]any, len(update)+4)
	for k, v := range update {
		out[k] = v
	}
	kind, _ := out["kind"].(string)
	if !grokIsFileMutationKind(kind) && prev.mutationKind != "" {
		out["kind"] = prev.mutationKind
	}
	if len(grokMutationPaths(out)) == 0 && len(prev.paths) > 0 {
		locs := make([]any, 0, len(prev.paths))
		for _, p := range prev.paths {
			locs = append(locs, map[string]any{"path": p})
		}
		out["locations"] = locs
	}
	return out
}

// grokCorrelateToolNotification remembers tool mutation metadata and enriches
// completed tool_call_update frames before mapping. Safe no-op for non-tool notifications.
func grokCorrelateToolNotification(cache map[string]grokPendingToolCall, n grokNotification) grokNotification {
	if cache == nil || n.Method != "session/update" || n.Params == nil {
		return n
	}
	update, _ := n.Params["update"].(map[string]any)
	if update == nil {
		return n
	}
	su, _ := update["sessionUpdate"].(string)
	if su != "tool_call" && su != "tool_call_update" {
		return n
	}
	grokRememberToolCall(cache, update)
	if su != "tool_call_update" {
		return n
	}
	status, _ := update["status"].(string)
	if status == "" {
		return n
	}
	// completed / failed / in_progress with stripped fields — enrich from cache
	enriched := grokEnrichToolCallUpdate(cache, update)
	params := make(map[string]any, len(n.Params))
	for k, v := range n.Params {
		params[k] = v
	}
	params["update"] = enriched
	n.Params = params
	return n
}

func grokLocationPaths(v any) []string {
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

func grokPathsFromRawInput(v any) []string {
	switch raw := v.(type) {
	case map[string]any:
		return grokPathsFromRawInputMap(raw)
	case string:
		s := strings.TrimSpace(raw)
		if s == "" {
			return nil
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			return nil
		}
		return grokPathsFromRawInputMap(m)
	default:
		return nil
	}
}

func grokPathsFromRawInputMap(m map[string]any) []string {
	if m == nil {
		return nil
	}
	keys := []string{"path", "file_path", "target_file", "target_path", "filename"}
	var out []string
	for _, k := range keys {
		if s, _ := m[k].(string); strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
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
	if name := grokRawToolName(update, fallbackTitle); name != "" {
		return grokNormalizedToolDisplayName(name)
	}
	return "tool"
}

// grokRawToolName returns the machine tool name from a tool_call/update frame
// without UI-boundary normalization (used by native-tool shims).
func grokRawToolName(update map[string]any, fallbackTitle string) string {
	if toolMeta := grokXAIToolMeta(update); toolMeta != nil {
		if name, _ := toolMeta["name"].(string); name != "" {
			return name
		}
	}
	if fallbackTitle != "" {
		return fallbackTitle
	}
	return ""
}

// grokNormalizedToolDisplayName maps Grok native tool names to FlowPilot's public tool
// names at the UI/event boundary (Task-209 DOD-8 / GR-07, BUG-124). FlowPilot MCP
// tools keep their own names; native Grok-only tools are aliased for desktop grouping.
func grokNormalizedToolDisplayName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "spawn_subagent":
		return "spawn_agent"
	case "ask_user_question":
		return "ask_user"
	default:
		return name
	}
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
