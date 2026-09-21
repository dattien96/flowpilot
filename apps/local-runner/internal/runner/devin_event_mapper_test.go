package runner

import (
	"encoding/json"
	"testing"
)

// Task-401 mapper tests over the live-captured fixtures.

func TestMapDevinNotificationAgentMessageChunk(t *testing.T) {
	var msg map[string]any
	if err := json.Unmarshal(devinFixture(t, "update_agent_message_chunk.json"), &msg); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	params, _ := msg["params"].(map[string]any)
	events, ok := mapDevinNotification(devinNotification{Method: "session/update", Params: params})
	if !ok || len(events) != 1 {
		t.Fatalf("expected 1 event, got %v", events)
	}
	if events[0].Type != EventMessageDelta || events[0].Text != "PROBE" {
		t.Fatalf("event: %+v", events[0])
	}
}

func TestMapDevinNotificationThoughtChunkDropped(t *testing.T) {
	var msg map[string]any
	if err := json.Unmarshal(devinFixture(t, "update_agent_thought_chunk.json"), &msg); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	params, _ := msg["params"].(map[string]any)
	events, ok := mapDevinNotification(devinNotification{Method: "session/update", Params: params})
	if ok || len(events) != 0 {
		t.Fatalf("thought chunks must not emit events, got %v", events)
	}
}

func TestMapDevinNotificationUsageUpdate(t *testing.T) {
	var msg map[string]any
	if err := json.Unmarshal(devinFixture(t, "update_usage_update.json"), &msg); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	params, _ := msg["params"].(map[string]any)
	events, ok := mapDevinNotification(devinNotification{Method: "session/update", Params: params})
	if !ok || len(events) != 1 {
		t.Fatalf("expected usage event, got %v", events)
	}
	tok := events[0].TokenUsage
	if tok == nil || tok.Last == nil {
		t.Fatal("expected token snapshot")
	}
	if tok.Last.InputTokens != 13133 || tok.Last.OutputTokens != 29 {
		t.Fatalf("_meta token split lost: %+v", tok.Last)
	}
	if tok.ModelContextWindow == nil || *tok.ModelContextWindow != 262000 {
		t.Fatalf("context window: %v", tok.ModelContextWindow)
	}
}

func TestMapDevinNotificationToolCallLifecycle(t *testing.T) {
	started, ok := mapDevinNotification(devinNotification{Method: "session/update", Params: map[string]any{
		"sessionId": "s",
		"update": map[string]any{
			"sessionUpdate": "tool_call",
			"title":         "bash",
			"toolCallId":    "tc1",
			"rawInput":      map[string]any{"command": "ls"},
		},
	}})
	if !ok || started[0].Type != EventToolStarted || started[0].ToolName != "bash" {
		t.Fatalf("tool_call: %v", started)
	}

	completed, ok := mapDevinNotification(devinNotification{Method: "session/update", Params: map[string]any{
		"sessionId": "s",
		"update": map[string]any{
			"sessionUpdate": "tool_call_update",
			"status":        "completed",
			"title":         "edit_file",
			"kind":          "edit",
			"toolCallId":    "tc1",
			"locations":     []any{map[string]any{"path": "/x/y.go"}},
			"rawOutput":     map[string]any{"ok": true},
		},
	}})
	if !ok || len(completed) != 2 {
		t.Fatalf("tool_call_update: %v", completed)
	}
	if completed[0].Type != EventToolCompleted || completed[0].Status != "success" {
		t.Fatalf("completed event: %+v", completed[0])
	}
	if completed[1].Type != EventFileChanged || completed[1].Path != "/x/y.go" || completed[1].ChangeType != "edit" {
		t.Fatalf("file_changed event: %+v", completed[1])
	}
}

// Extensions and non-update kinds are tolerated without events.
func TestMapDevinNotificationToleratesExtensionsAndInfoKinds(t *testing.T) {
	for _, method := range []string{"_cognition.ai/output", "_cognition.ai/turn_stats", "_cognition.ai/agent_stopped", "_cognition.ai/mcp/serversChanged"} {
		if evs, ok := mapDevinNotification(devinNotification{Method: method, Params: map[string]any{"sessionId": "s"}}); ok || len(evs) != 0 {
			t.Fatalf("%s must not emit events", method)
		}
	}
	for _, kind := range []string{"session_info_update", "current_mode_update", "available_commands_update", "config_option_update", "user_message_chunk", "plan"} {
		if evs, ok := mapDevinNotification(devinNotification{Method: "session/update", Params: map[string]any{
			"sessionId": "s",
			"update":    map[string]any{"sessionUpdate": kind},
		}}); ok || len(evs) != 0 {
			t.Fatalf("%s must not emit events", kind)
		}
	}
}

func TestDevinStopReasonMapping(t *testing.T) {
	if devinStopReasonToEvent("end_turn") != EventTurnCompleted {
		t.Fatal("end_turn -> completed")
	}
	if devinStopReasonToEvent("rate_limit") != EventTurnFailed {
		t.Fatal("rate_limit -> failed (quota)")
	}
	if devinStopReasonToEvent("error") != EventTurnFailed {
		t.Fatal("error -> failed")
	}
	if devinStopReasonToEvent("") != EventTurnCompleted {
		t.Fatal("empty -> completed")
	}
}

func TestDevinTextContentShapes(t *testing.T) {
	if got := devinTextContent("plain"); got != "plain" {
		t.Fatalf("string: %q", got)
	}
	if got := devinTextContent(map[string]any{"type": "text", "text": "t"}); got != "t" {
		t.Fatalf("map: %q", got)
	}
	if got := devinTextContent([]any{map[string]any{"text": "a"}, "b"}); got != "ab" {
		t.Fatalf("array: %q", got)
	}
}
