package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadClaudeTranscriptEvents verifies the Claude JSONL transcript loader
// replays both assistant output AND typed user prompts in correct order.
// Tool-result frames must continue mapping to tool_completed (not prompts).
func TestLoadClaudeTranscriptEvents(t *testing.T) {
	lines := []string{
		// system/init → turn_started (no prompt; from mapClaudeLine)
		`{"type":"system","subtype":"init","session_id":"s1","cwd":"/repo"}`,
		// typed user prompt → prepend turn_started{prompt}; mapClaudeUser emits nothing (no tool_result)
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello there"}]}}`,
		// assistant response → message_completed
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hi! How can I help?"}]}}`,
		// tool_result frame → claudeUserPromptText returns ""; mapClaudeUser → tool_completed
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"result text"}]}}`,
		// assistant tool_use → tool_started
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}`,
		// second typed user prompt → turn_started{prompt} with a distinct ProviderTurnID
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"what next?"}]}}`,
		// result/success → turn_completed
		`{"type":"result","subtype":"success","result":"Done"}`,
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	events, _ := loadClaudeTranscriptEvents(path)

	type want struct {
		typ    ProviderEventType
		text   string
		prompt string
		tool   string
	}
	expected := []want{
		// from system/init via mapClaudeLine (no prompt field)
		{typ: EventTurnStarted},
		// from user "hello there" (new: prepended by loader)
		{typ: EventTurnStarted, prompt: "hello there"},
		// from assistant text
		{typ: EventMessageCompleted, text: "Hi! How can I help?"},
		// from user tool_result via mapClaudeUser
		{typ: EventToolCompleted},
		// from assistant tool_use via mapClaudeAssistant
		{typ: EventToolStarted, tool: "Bash"},
		// from user "what next?" (new: prepended by loader)
		{typ: EventTurnStarted, prompt: "what next?"},
		// from result/success via mapClaudeResult
		{typ: EventTurnCompleted},
	}
	if len(events) != len(expected) {
		t.Fatalf("got %d events, want %d: %+v", len(events), len(expected), events)
	}
	seenTurnIDs := map[string]bool{}
	for i, w := range expected {
		if events[i].Type != w.typ {
			t.Fatalf("event[%d].Type = %q, want %q", i, events[i].Type, w.typ)
		}
		if w.text != "" && events[i].Text != w.text {
			t.Fatalf("event[%d].Text = %q, want %q", i, events[i].Text, w.text)
		}
		if w.prompt != "" && events[i].Prompt != w.prompt {
			t.Fatalf("event[%d].Prompt = %q, want %q", i, events[i].Prompt, w.prompt)
		}
		if w.tool != "" && events[i].ToolName != w.tool {
			t.Fatalf("event[%d].ToolName = %q, want %q", i, events[i].ToolName, w.tool)
		}
		// Replayed prompt turn_started events must each have a non-empty unique ProviderTurnID.
		if events[i].Type == EventTurnStarted && events[i].Prompt != "" {
			id := events[i].ProviderTurnID
			if id == "" {
				t.Fatalf("event[%d] turn_started{prompt} has empty ProviderTurnID", i)
			}
			if seenTurnIDs[id] {
				t.Fatalf("event[%d] turn_started{prompt} has duplicate ProviderTurnID %q", i, id)
			}
			seenTurnIDs[id] = true
		}
	}
}

// TestLoadClaudeTranscriptEventsToolResultNotPrompt verifies that a user frame
// whose entire content is tool_result blocks does NOT produce a prompt event —
// only the tool_completed from mapClaudeUser.
func TestLoadClaudeTranscriptEventsToolResultNotPrompt(t *testing.T) {
	lines := []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/a.go"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"file contents"}]}}`,
		`{"type":"result","subtype":"success","result":""}`,
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	events, _ := loadClaudeTranscriptEvents(path)

	for _, e := range events {
		if e.Type == EventTurnStarted && e.Prompt != "" {
			t.Fatalf("tool_result frame must not produce a prompt turn_started; got %+v", e)
		}
	}
	hasToolCompleted := false
	for _, e := range events {
		if e.Type == EventToolCompleted {
			hasToolCompleted = true
		}
	}
	if !hasToolCompleted {
		t.Fatal("expected tool_completed from tool_result frame, got none")
	}
}

// TestLoadClaudeTranscriptEventsMissingFile returns nil for an absent file so
// resume is never blocked by a missing transcript (parity with Codex loader).
func TestLoadClaudeTranscriptEventsMissingFile(t *testing.T) {
	if events, _ := loadClaudeTranscriptEvents(filepath.Join(t.TempDir(), "nope.jsonl")); events != nil {
		t.Fatalf("expected nil for missing file, got %+v", events)
	}
}

// TestLoadClaudeTranscriptEventsPlainStringContent verifies the plain-string
// content path (older Claude CLI versions where content is a bare string, not
// a []block).
func TestLoadClaudeTranscriptEventsPlainStringContent(t *testing.T) {
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"plain prompt text"}}`,
		`{"type":"result","subtype":"success","result":""}`,
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude.jsonl")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	events, _ := loadClaudeTranscriptEvents(path)

	var promptEvent *ProviderEvent
	for i := range events {
		if events[i].Type == EventTurnStarted && events[i].Prompt != "" {
			promptEvent = &events[i]
			break
		}
	}
	if promptEvent == nil {
		t.Fatal("expected turn_started{prompt} for plain-string content, got none")
	}
	if promptEvent.Prompt != "plain prompt text" {
		t.Fatalf("Prompt = %q, want %q", promptEvent.Prompt, "plain prompt text")
	}
	if promptEvent.ProviderTurnID == "" {
		t.Fatal("ProviderTurnID must be set on replayed prompt event")
	}
}
