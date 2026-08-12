package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestChooseAssistantFinal_PrefersDeltasOverDoneStub(t *testing.T) {
	deltas := "Sure — let me work through this step.\nI reviewed the relevant files."
	stub := "Done. The change is implemented and the step is complete."
	got := chooseAssistantFinal(deltas, stub)
	if got != deltas {
		t.Fatalf("got %q want deltas", got)
	}
	if !isStepCompleteStub(stub) {
		t.Fatal("stub should be detected")
	}
}

func TestTurnFinishedMsg_DoesNotShowDoneStubWhenDeltasPresent(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.addMessage("assistant", "Real streamed answer about the bug.", "")
	m2, _ := m.Update(turnFinishedMsg{FinalMsg: "Done. The change is implemented and the step is complete."})
	am := m2.(*AppModel)
	if strings.Contains(am.lastAssistantText(), "step is complete") {
		t.Fatalf("stub leaked into transcript: %q", am.lastAssistantText())
	}
	if !strings.Contains(am.lastAssistantText(), "Real streamed answer") {
		t.Fatalf("lost deltas: %q", am.lastAssistantText())
	}
}

func TestReplayHistoryMessages_SkipsDoneStub(t *testing.T) {
	msgs := replayHistoryMessages([]client.ProviderEvent{
		{Type: "turn_started", Prompt: "hi"},
		{Type: "message_delta", Text: "Hello from history."},
		{Type: "turn_completed", FinalMessage: "Done. The change is implemented and the step is complete."},
	})
	if len(msgs) != 2 {
		t.Fatalf("msgs=%+v", msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hi" {
		t.Fatalf("user=%+v", msgs[0])
	}
	if msgs[1].Content != "Hello from history." {
		t.Fatalf("assistant=%+v", msgs[1])
	}
}

func TestResolveChatOpenTarget_IndexAndID(t *testing.T) {
	listed := []client.RunHistoryItem{
		{RunID: "run-aaa"},
		{RunID: "run-bbb"},
	}
	id, err := resolveChatOpenTarget([]string{"2"}, listed)
	if err != nil || id != "run-bbb" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	id, err = resolveChatOpenTarget([]string{"run-zzz"}, listed)
	if err != nil || id != "run-zzz" {
		t.Fatalf("id=%q err=%v", id, err)
	}
}

func TestChatListMsg_CachesForOpen(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(ChatListMsg{Items: []client.RunHistoryItem{
		{RunID: "run-1", LastPrompt: "first", Status: "completed", RunKind: "chat"},
	}})
	am := m2.(*AppModel)
	if len(am.chatList) != 1 {
		t.Fatalf("chatList=%d", len(am.chatList))
	}
	if !strings.Contains(am.View(), "first") {
		t.Fatalf("list missing title:\n%s", am.View())
	}
}

func TestChatOpenedMsg_ReplacesTranscript(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.addMessage("user", "old", "")
	m2, _ := m.Update(ChatOpenedMsg{
		Handle: client.RunHandle{RunID: "run-open", StepID: "step-1", Status: "completed"},
		Messages: []ChatMessage{
			{Role: "user", Content: "from history"},
			{Role: "assistant", Content: "reply"},
		},
	})
	am := m2.(*AppModel)
	if am.runHandle == nil || am.runHandle.RunID != "run-open" {
		t.Fatalf("handle=%+v", am.runHandle)
	}
	joined := ""
	for _, msg := range am.messages {
		joined += msg.Content + "|"
	}
	if strings.Contains(joined, "old") || !strings.Contains(joined, "from history") {
		t.Fatalf("messages=%v", am.messages)
	}
}

func TestFilterParentHistory_DropsChildren(t *testing.T) {
	got := filterParentHistory([]client.RunHistoryItem{
		{RunID: "p", ParentRunID: ""},
		{RunID: "c", ParentRunID: "p"},
	})
	if len(got) != 1 || got[0].RunID != "p" {
		t.Fatalf("got=%+v", got)
	}
}
