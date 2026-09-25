package runner

import (
	"io"
	"strings"
	"testing"
)

// BUG-374/375/434/436 — Devin toolCallId correlation + status mapping.
// Live wire (lt-evidence cp46/cp70): the initial `tool_call` frame carries
// title/kind/rawInput/locations/_meta.cognition.ai/toolName, but
// `tool_call_update` and `session/request_permission` frames carry ONLY the
// toolCallId (plus `_meta.cognition.ai/editableCommand` for exec approvals).

func devinToolCallNotification(toolCallID string, fields map[string]any) devinNotification {
	update := map[string]any{"sessionUpdate": "tool_call", "toolCallId": toolCallID}
	for k, v := range fields {
		update[k] = v
	}
	return devinNotification{Method: "session/update", Params: map[string]any{"sessionId": "s", "update": update}}
}

func devinToolCallUpdateNotification(toolCallID string, fields map[string]any) devinNotification {
	update := map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": toolCallID}
	for k, v := range fields {
		update[k] = v
	}
	return devinNotification{Method: "session/update", Params: map[string]any{"sessionId": "s", "update": update}}
}

// BUG-436: an in_progress tool_call_update is not a completion — the mapper
// must emit no terminal event for it.
func TestDevinToolCallUpdateInProgressEmitsNoTerminalEvent(t *testing.T) {
	for _, status := range []string{"in_progress", "pending", "running"} {
		events, _ := mapDevinNotification(devinToolCallUpdateNotification("tc1", map[string]any{"status": status}))
		for _, ev := range events {
			if ev.Type == EventToolCompleted || ev.Type == EventFileChanged {
				t.Fatalf("status=%q emitted terminal event %+v", status, ev)
			}
		}
	}
}

// BUG-375: mutation metadata lives on the tool_call frame; the terminal
// update carries only toolCallId+status. file_changed must still fire via
// the correlation cache.
func TestDevinCorrelatedToolCallUpdateEmitsFileChanged(t *testing.T) {
	cache := &devinToolCallIndex{}
	devinCorrelateToolNotification(cache, devinToolCallNotification("tc9", map[string]any{
		"title":     "edit_file",
		"kind":      "edit",
		"locations": []any{map[string]any{"path": "/w/format.go"}},
		"rawInput":  map[string]any{"file_path": "/w/format.go"},
	}))
	upd := devinCorrelateToolNotification(cache, devinToolCallUpdateNotification("tc9", map[string]any{"status": "completed"}))
	events, ok := mapDevinNotification(upd)
	if !ok {
		t.Fatal("update not mapped")
	}
	var completed, changed *ProviderEvent
	for i := range events {
		switch events[i].Type {
		case EventToolCompleted:
			completed = &events[i]
		case EventFileChanged:
			changed = &events[i]
		}
	}
	if completed == nil {
		t.Fatalf("missing tool_completed: %v", events)
	}
	if changed == nil || changed.Path != "/w/format.go" || changed.ChangeType != "edit" {
		t.Fatalf("missing file_changed from correlated tool_call: %v", events)
	}
}

// BUG-375 near-miss: a correlated READ must not emit file_changed even though
// the terminal update is otherwise bare.
func TestDevinCorrelatedReadDoesNotEmitFileChanged(t *testing.T) {
	cache := &devinToolCallIndex{}
	devinCorrelateToolNotification(cache, devinToolCallNotification("tc10", map[string]any{
		"title":    "read_file",
		"kind":     "read",
		"rawInput": map[string]any{"file_path": "/w/format.go"},
	}))
	upd := devinCorrelateToolNotification(cache, devinToolCallUpdateNotification("tc10", map[string]any{"status": "completed"}))
	events, _ := mapDevinNotification(upd)
	for _, ev := range events {
		if ev.Type == EventFileChanged {
			t.Fatalf("read emitted file_changed: %+v", ev)
		}
	}
}

// BUG-434: request_permission toolCall carries only toolCallId +
// _meta.cognition.ai/editableCommand — the command must land in Command and
// classify as exec so the approval card shows an editable shell command.
func TestDevinApprovalDetailsUsesEditableCommand(t *testing.T) {
	details := devinApprovalDetailsFromRequest(map[string]any{
		"toolCall": map[string]any{
			"toolCallId": "call_x#1",
			"_meta":      map[string]any{"cognition.ai/editableCommand": "rm -rf /tmp/x"},
		},
	}, nil)
	if details.Command != "rm -rf /tmp/x" {
		t.Fatalf("editableCommand lost: %+v", details)
	}
	if details.Kind != "exec" {
		t.Fatalf("editableCommand must classify exec, got %q", details.Kind)
	}
}

// BUG-374: a gated MCP call's permission request carries only toolCallId —
// correlation with the earlier tool_call frame must recover the tool name so
// the verdict face / ask_user matchers can classify it.
func TestDevinApprovalDetailsCorrelatesMCPToolName(t *testing.T) {
	cache := &devinToolCallIndex{}
	devinCorrelateToolNotification(cache, devinToolCallNotification("call_v#1", map[string]any{
		"rawInput": map[string]any{"status": "approved"},
		"_meta":    map[string]any{"cognition.ai/toolName": "mcp__flowpilot__submit_review_outcome"},
	}))
	details := devinApprovalDetailsFromRequest(map[string]any{
		"toolCall": map[string]any{"toolCallId": "call_v#1"},
		"options":  []any{map[string]any{"kind": "allow_once", "optionId": "allow_once"}},
	}, cache)
	if !isVerdictToolCall(details) {
		t.Fatalf("verdict tool not recognized: %+v", details)
	}
	if got := readOnlyApprovalDecision(details); got != "approve" {
		t.Fatalf("read_only posture must approve verdict tool, got %q", got)
	}
}

// BUG-374 fallback: when no tool_call correlation exists, the tool name is
// still recoverable from Devin's deterministic option labels
// ("allow calling submit_review_outcome on the flowpilot MCP server").
func TestDevinApprovalDetailsToolNameFromOptionsFallback(t *testing.T) {
	details := devinApprovalDetailsFromRequest(map[string]any{
		"toolCall": map[string]any{"toolCallId": "call_u#1"},
		"options": []any{
			map[string]any{"kind": "allow_once", "name": "Allow", "optionId": "allow_once"},
			map[string]any{"kind": "allow_always", "name": "Yes, allow calling submit_review_outcome on the flowpilot MCP server (this session)", "optionId": "allow_session"},
		},
	}, nil)
	if !isVerdictToolCall(details) {
		t.Fatalf("verdict tool not recovered from options: %+v", details)
	}
}

// BUG-374: ask_user must classify identically via correlation.
func TestDevinApprovalDetailsCorrelatesAskUser(t *testing.T) {
	cache := &devinToolCallIndex{}
	devinCorrelateToolNotification(cache, devinToolCallNotification("call_q#1", map[string]any{
		"rawInput": map[string]any{"prompt": "which?"},
		"_meta":    map[string]any{"cognition.ai/toolName": "mcp__flowpilot__ask_user"},
	}))
	details := devinApprovalDetailsFromRequest(map[string]any{
		"toolCall": map[string]any{"toolCallId": "call_q#1"},
	}, cache)
	if !isAskUserTool(details) {
		t.Fatalf("ask_user not recognized: %+v", details)
	}
}

// handleInbound end-to-end: correlated request_permission resolves with the
// recovered details and replies allow_once on approve.
func TestDevinHandleInboundCorrelatesPermissionRequest(t *testing.T) {
	a := newDevinAdapter(newDevinDispatcher(io.Discard, nil), "/tmp")
	sessionID := "working-pentagon"
	bridge := &fakeDevinBridge{decision: "approve"}
	a.mu.Lock()
	a.bridges[sessionID] = bridge
	a.mu.Unlock()

	idx := a.toolCallIndexFor(sessionID)
	devinCorrelateToolNotification(idx, devinToolCallNotification("call_v#1", map[string]any{
		"_meta": map[string]any{"cognition.ai/toolName": "mcp__flowpilot__submit_review_outcome"},
	}))

	a.handleInbound(devinInboundRequest{
		ID:     "req-1",
		Method: "session/request_permission",
		Params: map[string]any{
			"sessionId": sessionID,
			"toolCall":  map[string]any{"toolCallId": "call_v#1"},
			"options":   []any{map[string]any{"kind": "allow_once", "optionId": "allow_once"}, map[string]any{"kind": "reject_once", "optionId": "reject_once"}},
		},
	})
	if len(bridge.approvals) != 1 {
		t.Fatalf("expected 1 approval, got %d", len(bridge.approvals))
	}
	if !isVerdictToolCall(bridge.approvals[0]) {
		t.Fatalf("inbound approval not correlated: %+v", bridge.approvals[0])
	}
}

// Enrichment must not clobber fields already present on the update frame.
func TestDevinCorrelatePreservesUpdateFields(t *testing.T) {
	cache := &devinToolCallIndex{}
	devinCorrelateToolNotification(cache, devinToolCallNotification("tc11", map[string]any{
		"title": "edit_file", "kind": "edit",
		"locations": []any{map[string]any{"path": "/cached.go"}},
	}))
	upd := devinCorrelateToolNotification(cache, devinToolCallUpdateNotification("tc11", map[string]any{
		"status":    "completed",
		"kind":      "delete",
		"locations": []any{map[string]any{"path": "/live.go"}},
	}))
	update, _ := upd.Params["update"].(map[string]any)
	if update["kind"] != "delete" {
		t.Fatalf("live kind clobbered: %v", update["kind"])
	}
	locs, _ := update["locations"].([]any)
	if len(locs) == 0 || locs[0].(map[string]any)["path"] != "/live.go" {
		t.Fatalf("live locations clobbered: %v", update["locations"])
	}
	if !strings.Contains(update["title"].(string), "edit_file") {
		t.Fatalf("title should be enriched from cache when absent: %v", update["title"])
	}
}
