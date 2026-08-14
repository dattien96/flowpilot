package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

func TestReplayHistoryMessages_KeepsPostToolAssistantFrame(t *testing.T) {
	// Grok chat_history.jsonl: assistant (pre-write) + tool + assistant (Đã tạo file).
	msgs := replayHistoryMessages([]client.ProviderEvent{
		{Type: "turn_started", Prompt: "ghi file tesssst.txt với nội dung test hihi"},
		{Type: "message_completed", Text: "Creating `tesssst.txt` with the requested content."},
		{Type: "tool_started", ToolName: "write"},
		{Type: "message_completed", Text: "Đã tạo file tesssst.txt với nội dung:\n\n```\ntest hihi\n```"},
		{Type: "turn_completed", FinalMessage: "Creating `tesssst.txt` with the requested content.Đã tạo file tesssst.txt với nội dung:\n\n```\ntest hihi\n```"},
	})
	if len(msgs) != 2 {
		t.Fatalf("msgs=%d want user+assistant", len(msgs))
	}
	got := msgs[1].Content
	if !strings.Contains(got, "Creating") {
		t.Fatalf("missing pre-tool frame: %q", got)
	}
	if !strings.Contains(got, "Đã tạo file") {
		t.Fatalf("missing post-tool answer (run-97624 /open drop): %q", got)
	}
	if !strings.Contains(got, "test hihi") {
		t.Fatalf("missing file body: %q", got)
	}
}

func TestReplayHistoryMessages_TurnCompletedSupersetFillsPrefix(t *testing.T) {
	msgs := replayHistoryMessages([]client.ProviderEvent{
		{Type: "turn_started", Prompt: "hi"},
		{Type: "message_completed", Text: "Creating file."},
		{Type: "turn_completed", FinalMessage: "Creating file.\nĐã tạo file xx với nội dung"},
	})
	if !strings.Contains(msgs[1].Content, "Đã tạo file xx") {
		t.Fatalf("superset finalMessage not merged: %q", msgs[1].Content)
	}
}

func TestReplayHistoryMessages_DoesNotReplaceWithStepStub(t *testing.T) {
	msgs := replayHistoryMessages([]client.ProviderEvent{
		{Type: "message_completed", Text: "real answer"},
		{Type: "turn_completed", FinalMessage: "The change is implemented. The step is complete."},
	})
	if msgs[0].Content != "real answer" {
		t.Fatalf("stub replaced answer: %q", msgs[0].Content)
	}
}

func TestReplayHistoryMessages_ClaudeCodexSameMerge(t *testing.T) {
	for _, name := range []string{"claude", "codex", "grok"} {
		t.Run(name, func(t *testing.T) {
			msgs := replayHistoryMessages([]client.ProviderEvent{
				{Type: "turn_started", Prompt: name + " write"},
				{Type: "message_completed", Text: "Creating file."},
				{Type: "message_completed", Text: "Wrote it."},
			})
			if !strings.Contains(msgs[1].Content, "Creating file.") || !strings.Contains(msgs[1].Content, "Wrote it.") {
				t.Fatalf("%s merge=%q", name, msgs[1].Content)
			}
		})
	}
}
