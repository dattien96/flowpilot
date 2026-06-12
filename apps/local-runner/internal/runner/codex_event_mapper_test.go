package runner

import "testing"

func TestMapCodexNotification(t *testing.T) {
	cases := []struct {
		name   string
		method string
		params map[string]any
		want   ProviderEventType
		ok     bool
		check  func(ProviderEvent) bool
	}{
		{"delta", "turn.delta", map[string]any{"text": "hello"}, EventMessageDelta, true,
			func(e ProviderEvent) bool { return e.Text == "hello" }},
		{"generated delta", "item/agentMessage/delta", map[string]any{"turnId": "t1", "delta": "hello"}, EventMessageDelta, true,
			func(e ProviderEvent) bool { return e.ProviderTurnID == "t1" && e.Text == "hello" }},
		{"message", "turn.message", map[string]any{"text": "final"}, EventMessageCompleted, true,
			func(e ProviderEvent) bool { return e.Text == "final" }},
		{"generated agent message completed", "item/completed", map[string]any{"turnId": "t1", "item": map[string]any{"type": "agentMessage", "text": "final"}}, EventMessageCompleted, true,
			func(e ProviderEvent) bool { return e.ProviderTurnID == "t1" && e.Text == "final" }},
		{"tool started", "tool.started", map[string]any{"name": "grep"}, EventToolStarted, true,
			func(e ProviderEvent) bool { return e.ToolName == "grep" }},
		{"tool completed", "tool.completed", map[string]any{"name": "grep", "status": "success"}, EventToolCompleted, true,
			func(e ProviderEvent) bool { return e.ToolName == "grep" && e.Status == "success" }},
		{"command failed", "command.completed", map[string]any{"command": "build", "exitCode": float64(1)}, EventToolCompleted, true,
			func(e ProviderEvent) bool { return e.Status == "failed" }},
		{"command ok", "command.completed", map[string]any{"command": "build", "exitCode": float64(0)}, EventToolCompleted, true,
			func(e ProviderEvent) bool { return e.Status == "success" }},
		{"generated command completed", "item/completed", map[string]any{"item": map[string]any{"type": "commandExecution", "command": "build", "status": "completed", "exitCode": float64(0), "aggregatedOutput": "ok"}}, EventToolCompleted, true,
			func(e ProviderEvent) bool { return e.ToolName == "build" && e.Status == "success" && e.Output == "ok" }},
		{"file", "file.changed", map[string]any{"path": "a.go", "changeType": "modified"}, EventFileChanged, true,
			func(e ProviderEvent) bool { return e.Path == "a.go" && e.ChangeType == "modified" }},
		{"generated file", "item/completed", map[string]any{"item": map[string]any{"type": "fileChange", "status": "completed", "changes": []any{map[string]any{"path": "a.go"}}}}, EventFileChanged, true,
			func(e ProviderEvent) bool { return e.Path == "a.go" && e.ChangeType == "completed" }},
		{"failed", "turn.failed", map[string]any{"error": "boom", "recoverable": true}, EventTurnFailed, true,
			func(e ProviderEvent) bool { return e.Error == "boom" && e.Recoverable }},
		{"generated error", "error", map[string]any{"turnId": "t1", "error": map[string]any{"message": "usage limit"}}, EventTurnFailed, true,
			func(e ProviderEvent) bool {
				return e.ProviderTurnID == "t1" && e.Error == "usage limit" && !e.Recoverable
			}},
		{"completed", "turn.completed", map[string]any{"finalMessage": "done"}, EventTurnCompleted, true,
			func(e ProviderEvent) bool { return e.FinalMessage == "done" }},
		{"generated completed", "turn/completed", map[string]any{"turn": map[string]any{"items": []any{map[string]any{"type": "agentMessage", "text": "done"}}}}, EventTurnCompleted, true,
			func(e ProviderEvent) bool { return e.FinalMessage == "done" }},
		{"generated failed completed", "turn/completed", map[string]any{"turnId": "t1", "turn": map[string]any{"status": "failed", "error": map[string]any{"message": "usage limit"}}}, EventTurnFailed, true,
			func(e ProviderEvent) bool { return e.ProviderTurnID == "t1" && e.Error == "usage limit" }},
		{"unknown", "some.unmapped.event", map[string]any{}, "", false, nil},
		{"empty delta dropped", "turn.delta", map[string]any{}, "", false, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev, ok := mapCodexNotification(codexNotification{Method: c.method, Params: c.params})
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !c.ok {
				return
			}
			if ev.Type != c.want {
				t.Fatalf("type = %s, want %s", ev.Type, c.want)
			}
			if c.check != nil && !c.check(ev) {
				t.Fatalf("field check failed for %+v", ev)
			}
		})
	}
}
