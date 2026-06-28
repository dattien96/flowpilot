package runner

import "testing"

func TestMapGeminiACPToolEvents(t *testing.T) {
	start := mapGeminiACPUpdate(map[string]interface{}{
		"method": "session/update",
		"params": map[string]interface{}{
			"update": map[string]interface{}{
				"sessionUpdate": "tool_call",
				"toolCallId":    "call-1",
				"title":         "Read file",
				"kind":          "read",
				"status":        "in_progress",
				"rawInput":      map[string]interface{}{"path": "README.md"},
			},
		},
	})
	if len(start) != 1 || start[0].Type != EventToolStarted {
		t.Fatalf("start events = %+v, want one tool_started", start)
	}
	if start[0].ToolName != "Read file" || start[0].Status != "in_progress" {
		t.Fatalf("start event = %+v, want name/status", start[0])
	}

	done := mapGeminiACPUpdate(map[string]interface{}{
		"method": "session/update",
		"params": map[string]interface{}{
			"update": map[string]interface{}{
				"sessionUpdate": "tool_call_update",
				"toolCallId":    "call-1",
				"title":         "Read file",
				"kind":          "read",
				"status":        "completed",
				"rawOutput":     "ok",
			},
		},
	})
	if len(done) != 1 || done[0].Type != EventToolCompleted {
		t.Fatalf("done events = %+v, want one tool_completed", done)
	}
	if done[0].ToolName != "Read file" || done[0].Output != "ok" || done[0].Status != "completed" {
		t.Fatalf("done event = %+v, want name/output/status", done[0])
	}
}

func TestGeminiACPApprovalDetailsAndResponse(t *testing.T) {
	params := map[string]interface{}{
		"sessionId": "s1",
		"toolCall": map[string]interface{}{
			"toolCallId": "call-1",
			"title":      "Write file",
			"kind":       "edit",
			"status":     "pending",
			"locations": []interface{}{
				map[string]interface{}{"path": "/tmp/file.txt", "line": float64(1)},
			},
		},
		"options": []interface{}{
			map[string]interface{}{"optionId": "allow-once", "name": "Allow once", "kind": "allow_once"},
			map[string]interface{}{"optionId": "reject-once", "name": "Reject once", "kind": "reject_once"},
		},
	}
	details := geminiACPApprovalDetails(params)
	if details.Command != "Write file" || details.Cwd != "/tmp/file.txt" {
		t.Fatalf("approval details = %+v, want command/cwd", details)
	}
	if len(details.Decisions) != 2 || details.Decisions[0].Value != "allow-once" || details.Decisions[1].Value != "reject-once" {
		t.Fatalf("approval decisions = %+v, want ACP options", details.Decisions)
	}

	deny := geminiACPPermissionResponse(params, "deny", nil)
	outcome, _ := deny["outcome"].(map[string]interface{})
	if outcome["outcome"] != "selected" || outcome["optionId"] != "reject-once" {
		t.Fatalf("deny response = %+v, want selected reject-once", deny)
	}
	allow := geminiACPPermissionResponse(params, "allow-once", nil)
	outcome, _ = allow["outcome"].(map[string]interface{})
	if outcome["optionId"] != "allow-once" {
		t.Fatalf("explicit response = %+v, want allow-once", allow)
	}
}
