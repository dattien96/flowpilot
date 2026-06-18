package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadCodexTranscriptEvents verifies the on-disk Codex rollout JSONL is mapped
// to the same transcript-replay shape as Claude: assistant messages + tool
// started/completed events, with user/developer prompts, reasoning, and the
// parallel event_msg stream all ignored (so each turn is rendered once).
func TestLoadCodexTranscriptEvents(t *testing.T) {
	lines := []string{
		`{"type":"session_meta","payload":{"id":"sess-1","cwd":"/repo"}}`,
		`{"type":"event_msg","payload":{"type":"task_started"}}`,
		`{"type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"system instructions"}]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello there"}]}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"hello there"}}`,
		`{"type":"response_item","payload":{"type":"reasoning","summary":[]}}`,
		`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi! "},{"type":"output_text","text":"How can I help?"}]}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"Hi! How can I help?"}}`,
		`{"type":"response_item","payload":{"type":"function_call","call_id":"c1","name":"shell","arguments":"{\"command\":[\"ls\"]}"}}`,
		`{"type":"response_item","payload":{"type":"function_call_output","call_id":"c1","output":"file.txt"}}`,
		`{"type":"response_item","payload":{"type":"web_search_call","action":{"query":"golang"}}}`,
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	events := loadCodexTranscriptEvents(path)

	type want struct {
		typ  ProviderEventType
		text string
		tool string
	}
	expected := []want{
		{typ: EventMessageCompleted, text: "Hi! How can I help?"},
		{typ: EventToolStarted, tool: "shell"},
		{typ: EventToolCompleted},
		{typ: EventToolStarted, tool: "web_search"},
		{typ: EventToolCompleted, tool: "web_search"},
	}
	if len(events) != len(expected) {
		t.Fatalf("got %d events, want %d: %+v", len(events), len(expected), events)
	}
	for i, w := range expected {
		if events[i].Type != w.typ {
			t.Fatalf("event[%d].Type = %q, want %q", i, events[i].Type, w.typ)
		}
		if w.text != "" && events[i].Text != w.text {
			t.Fatalf("event[%d].Text = %q, want %q", i, events[i].Text, w.text)
		}
		if w.tool != "" && events[i].ToolName != w.tool {
			t.Fatalf("event[%d].ToolName = %q, want %q", i, events[i].ToolName, w.tool)
		}
	}
}

// TestLoadCodexTranscriptEventsMissingFile returns nil (best-effort) for an absent
// rollout file so resume is never blocked by a missing transcript.
func TestLoadCodexTranscriptEventsMissingFile(t *testing.T) {
	if events := loadCodexTranscriptEvents(filepath.Join(t.TempDir(), "nope.jsonl")); events != nil {
		t.Fatalf("expected nil for missing file, got %+v", events)
	}
}
