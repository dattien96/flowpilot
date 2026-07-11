package runner

import (
	"testing"
)

func TestMapGrokToolCallUpdateUsesMetaKindForFileMutation(t *testing.T) {
	update := map[string]any{
		"sessionUpdate": "tool_call_update",
		"toolCallId":    "call-1",
		"status":        "completed",
		// No top-level kind — live Grok often only sets _meta.x.ai/tool.
		"locations": []any{map[string]any{"path": "internal/foo.go"}},
		"_meta": map[string]any{
			"x.ai/tool": map[string]any{"name": "write", "kind": "write"},
		},
	}
	events, ok := mapGrokToolCallUpdate(update)
	if !ok {
		t.Fatal("expected mapped events")
	}
	if firstEventOfType(events, EventToolCompleted) == nil {
		t.Fatalf("missing tool_completed: %+v", events)
	}
	fc := firstEventOfType(events, EventFileChanged)
	if fc == nil {
		t.Fatalf("expected file_changed from meta kind, got %+v", events)
	}
	if fc.Path != "internal/foo.go" {
		t.Fatalf("Path = %q, want internal/foo.go", fc.Path)
	}
	if fc.ChangeType != "write" {
		t.Fatalf("ChangeType = %q, want write", fc.ChangeType)
	}
}

func TestMapGrokToolCallUpdateExtractsPathFromRawInput(t *testing.T) {
	update := map[string]any{
		"sessionUpdate": "tool_call_update",
		"status":        "completed",
		"kind":          "edit",
		// No locations — path only in rawInput (search_replace style).
		"rawInput": map[string]any{"target_file": "internal/foo.go"},
		"_meta": map[string]any{
			"x.ai/tool": map[string]any{"name": "search_replace", "kind": "edit"},
		},
	}
	events, ok := mapGrokToolCallUpdate(update)
	if !ok {
		t.Fatal("expected mapped events")
	}
	fc := firstEventOfType(events, EventFileChanged)
	if fc == nil {
		t.Fatalf("expected file_changed from rawInput path, got %+v", events)
	}
	if fc.Path != "internal/foo.go" {
		t.Fatalf("Path = %q, want internal/foo.go", fc.Path)
	}
	if fc.ChangeType != "edit" {
		t.Fatalf("ChangeType = %q, want edit", fc.ChangeType)
	}
}

func TestMapGrokToolCallUpdateMutationFromToolNameWithoutKind(t *testing.T) {
	update := map[string]any{
		"sessionUpdate": "tool_call_update",
		"status":        "completed",
		"rawInput":      map[string]any{"path": "pkg/bar.go"},
		"_meta": map[string]any{
			"x.ai/tool": map[string]any{"name": "search_replace"}, // no kind field
		},
	}
	events, ok := mapGrokToolCallUpdate(update)
	if !ok {
		t.Fatal("expected mapped events")
	}
	fc := firstEventOfType(events, EventFileChanged)
	if fc == nil {
		t.Fatalf("expected file_changed from tool name search_replace, got %+v", events)
	}
	if fc.Path != "pkg/bar.go" || fc.ChangeType != "edit" {
		t.Fatalf("got path=%q changeType=%q", fc.Path, fc.ChangeType)
	}
}

func TestMapGrokToolCallUpdateReadOnlyDoesNotEmitFileChanged(t *testing.T) {
	update := map[string]any{
		"sessionUpdate": "tool_call_update",
		"status":        "completed",
		"kind":          "read",
		"locations":     []any{map[string]any{"path": "sample.txt"}},
		"_meta":         map[string]any{"x.ai/tool": map[string]any{"name": "read_file", "kind": "read"}},
	}
	events, ok := mapGrokToolCallUpdate(update)
	if !ok {
		t.Fatal("expected mapped events")
	}
	if firstEventOfType(events, EventFileChanged) != nil {
		t.Fatalf("read must not emit file_changed: %+v", events)
	}
}

func TestMapGrokToolCallUpdateRawInputJSONString(t *testing.T) {
	update := map[string]any{
		"sessionUpdate": "tool_call_update",
		"status":        "completed",
		"kind":          "write",
		"rawInput":      `{"file_path":"cmd/main.go"}`,
	}
	events, ok := mapGrokToolCallUpdate(update)
	if !ok {
		t.Fatal("expected mapped events")
	}
	fc := firstEventOfType(events, EventFileChanged)
	if fc == nil || fc.Path != "cmd/main.go" {
		t.Fatalf("expected file_changed path cmd/main.go, got %+v", events)
	}
}

// Live Grok (gate-sandbox 2026-07-11): search_replace emits
// 1) tool_call with meta.kind=edit + rawInput.file_path
// 2) tool_call_update mid-flight with kind=edit + locations (status unset)
// 3) tool_call_update status=completed with ONLY content — no kind/locations.
// Without toolCallId correlation, EventFileChanged never fires → r-ca silent.
func TestGrokToolCallCorrelationBackfillsMutationPath(t *testing.T) {
	cache := map[string]grokPendingToolCall{}
	const id = "call-f410d38f-search-replace"

	// 1) tool_call
	toolCall := grokNotification{
		Method: "session/update",
		Params: map[string]any{
			"sessionId": "s1",
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    id,
				"title":         "search_replace",
				"rawInput":      map[string]any{"file_path": "/Users/tiendat/Desktop/BE/gate-sandbox/calc.go"},
				"_meta": map[string]any{
					"x.ai/tool": map[string]any{"name": "search_replace", "kind": "edit", "namespace": "grok_build"},
				},
			},
		},
	}
	toolCall = grokCorrelateToolNotification(cache, toolCall)
	if _, ok := mapGrokNotification(toolCall); !ok {
		// tool_call still maps to tool_started
		t.Fatal("tool_call should map")
	}

	// 2) mid-flight tool_call_update (status unset) — carries kind+locations
	mid := grokNotification{
		Method: "session/update",
		Params: map[string]any{
			"sessionId": "s1",
			"update": map[string]any{
				"sessionUpdate": "tool_call_update",
				"toolCallId":    id,
				"kind":          "edit",
				"title":         "Edit `calc.go`",
				"locations":     []any{map[string]any{"path": "/Users/tiendat/Desktop/BE/gate-sandbox/calc.go"}},
				"rawInput":      map[string]any{"variant": "SearchReplace", "file_path": "/Users/tiendat/Desktop/BE/gate-sandbox/calc.go"},
				"_meta": map[string]any{
					"x.ai/tool": map[string]any{"name": "search_replace", "kind": "edit"},
				},
			},
		},
	}
	mid = grokCorrelateToolNotification(cache, mid)
	if events, ok := mapGrokNotification(mid); ok {
		// status empty → no client event yet (current contract)
		if firstEventOfType(events, EventFileChanged) != nil {
			// if mid-flight ever maps, still OK — but today it should not
			t.Log("mid-flight unexpectedly mapped; correlation still required for completed")
		}
	}

	// 3) completed frame stripped of kind/locations (live shape)
	completed := grokNotification{
		Method: "session/update",
		Params: map[string]any{
			"sessionId": "s1",
			"update": map[string]any{
				"sessionUpdate": "tool_call_update",
				"toolCallId":    id,
				"status":        "completed",
				"content":       []any{map[string]any{"type": "content", "content": map[string]any{"type": "text", "text": "ok"}}},
			},
		},
	}
	// Without correlation: no file_changed
	bare, ok := mapGrokNotification(completed)
	if !ok {
		t.Fatal("completed should map to tool_completed")
	}
	if firstEventOfType(bare, EventFileChanged) != nil {
		t.Fatal("bare completed frame must not already have file_changed (would mean test fixture wrong)")
	}

	// With correlation: file_changed restored
	enriched := grokCorrelateToolNotification(cache, completed)
	events, ok := mapGrokNotification(enriched)
	if !ok {
		t.Fatal("enriched completed should map")
	}
	fc := firstEventOfType(events, EventFileChanged)
	if fc == nil {
		t.Fatalf("expected file_changed after correlation, got %+v", events)
	}
	if fc.Path != "/Users/tiendat/Desktop/BE/gate-sandbox/calc.go" {
		t.Fatalf("Path = %q", fc.Path)
	}
	if fc.ChangeType != "edit" {
		t.Fatalf("ChangeType = %q, want edit", fc.ChangeType)
	}
}

func firstEventOfType(events []ProviderEvent, typ ProviderEventType) *ProviderEvent {
	for i := range events {
		if events[i].Type == typ {
			return &events[i]
		}
	}
	return nil
}
