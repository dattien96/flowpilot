package runner

import "testing"

func TestOpencodeEventMapperTextChunksToDelta(t *testing.T) {
	n := opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"sessionId": "ses_1",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": "Hello"},
			},
		},
	}
	events, ok := mapOpencodeNotification(n)
	if !ok || len(events) != 1 || events[0].Type != EventMessageDelta || events[0].Text != "Hello" {
		t.Fatalf("expected delta Hello, got %v %v", ok, events)
	}
}

func TestOpencodeEventMapperToolCallToToolStartedCompleted(t *testing.T) {
	nStart := opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"sessionId": "ses_1",
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"title":         "write",
				"toolCallId":    "call_1",
				"rawInput":      map[string]any{"filePath": "/tmp/a.txt"},
			},
		},
	}
	events, ok := mapOpencodeNotification(nStart)
	if !ok || len(events) != 1 || events[0].Type != EventToolStarted {
		t.Fatalf("expected tool_started, got %v", events)
	}
	nUpdate := opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"sessionId": "ses_1",
			"update": map[string]any{
				"sessionUpdate": "tool_call_update",
				"status":        "completed",
				"title":         "write",
				"kind":          "edit",
				"toolCallId":    "call_1",
				"locations":     []any{map[string]any{"path": "/tmp/a.txt"}},
			},
		},
	}
	events, ok = mapOpencodeNotification(nUpdate)
	if !ok {
		t.Fatalf("expected tool_completed")
	}
	foundCompleted := false
	foundFileChanged := false
	for _, ev := range events {
		if ev.Type == EventToolCompleted {
			foundCompleted = true
		}
		if ev.Type == EventFileChanged && ev.Path == "/tmp/a.txt" {
			foundFileChanged = true
		}
	}
	if !foundCompleted {
		t.Fatal("expected tool_completed")
	}
	if !foundFileChanged {
		t.Fatal("expected file_changed for edit")
	}
}

func TestOpencodeEventMapperDerivesFileChangedFromWriteDiff(t *testing.T) {
	n := opencodeNotification{
		Method: "session/update",
		Params: map[string]any{
			"update": map[string]any{
				"sessionUpdate": "tool_call_update",
				"status":        "completed",
				"title":         "write",
				"kind":          "write",
				"locations":     []any{map[string]any{"path": "/tmp/b.txt"}},
			},
		},
	}
	events, ok := mapOpencodeNotification(n)
	if !ok {
		t.Fatal("expected mapping")
	}
	hasFile := false
	for _, ev := range events {
		if ev.Type == EventFileChanged {
			hasFile = true
		}
	}
	if !hasFile {
		t.Fatal("expected file_changed")
	}
}

func TestOpencodeEventMapperTurnCompletedToFailed(t *testing.T) {
	if opencodeStopReasonToEvent("end_turn") != EventTurnCompleted {
		t.Fatal("expected turn_completed")
	}
	if opencodeStopReasonToEvent("error") != EventTurnFailed {
		t.Fatal("expected turn_failed")
	}
}

func TestOpencodeEventMapperTokenUsageToEvent(t *testing.T) {
	usage := map[string]any{
		"inputTokens":  float64(100),
		"outputTokens": float64(20),
		"totalTokens":  float64(120),
	}
	snap := opencodePromptResultTokenUsage(usage, nil)
	if snap == nil || snap.Total.TotalTokens != 120 {
		t.Fatalf("expected token usage, got %v", snap)
	}
}

func TestOpencodeCapabilitiesMatchProvenSet(t *testing.T) {
	a := &opencodeAdapter{}
	caps := a.Capabilities()
	if !caps.Streaming || !caps.Resume || !caps.FileEvents || !caps.Interrupt || !caps.SkillSelection {
		t.Fatalf("expected streaming/resume/fileEvents/interrupt/skillSelection true, got %+v", caps)
	}
	if caps.ApprovalEvents || caps.Mcp || caps.Vision {
		t.Fatalf("expected approval/mcp/vision false in MVP, got %+v", caps)
	}
}
