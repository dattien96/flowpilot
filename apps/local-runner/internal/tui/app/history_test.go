package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

func TestFilterHistorySuggestions_FiltersAsYouType(t *testing.T) {
	items := []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "fix login bug", Status: "completed", RunKind: "chat"},
		{RunID: "run-bbb", LastPrompt: "ship android", Status: "running", RunKind: "workflow"},
	}
	all := filterHistorySuggestions("/history ", items)
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d: %+v", len(all), all)
	}
	filtered := filterHistorySuggestions("/history android", items)
	if len(filtered) != 1 || filtered[0].value != "run-bbb" {
		t.Fatalf("filtered=%+v", filtered)
	}
	if filterHistorySuggestions("/history", items) != nil {
		t.Fatal("bare /history should not open picker")
	}
	openPick := filterHistorySuggestions("/open and", items)
	if len(openPick) != 1 || openPick[0].value != "run-bbb" || openPick[0].slash != "/open" {
		t.Fatalf("openPick=%+v", openPick)
	}
	resumePick := filterHistorySuggestions("/resume login", items)
	if len(resumePick) != 1 || resumePick[0].value != "run-aaa" || resumePick[0].slash != "/resume" {
		t.Fatalf("resumePick=%+v", resumePick)
	}
}

func TestFormatOpenChatErr_SessionUnavailableIsRunner(t *testing.T) {
	err := &client.APIError{Status: 409, Code: "session_unavailable", Message: "session data not found on this machine"}
	got := formatOpenChatErr(err)
	if !strings.Contains(got, "runner") || !strings.Contains(got, "not a TUI bug") {
		t.Fatalf("got=%q", got)
	}
}

func TestChatListMsg_SilentDoesNotDump(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(ChatListMsg{
		Items:  []client.RunHistoryItem{{RunID: "run-silent", LastPrompt: "quiet"}},
		Silent: true,
	})
	am := m2.(*AppModel)
	if len(am.chatList) != 1 || am.chatList[0].RunID != "run-silent" {
		t.Fatalf("chatList=%+v", am.chatList)
	}
	if strings.Contains(am.View(), "quiet") {
		t.Fatalf("silent prefetch should not dump list:\n%s", am.View())
	}
}

func TestHistoryTabCompletesSelection(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha", Status: "completed"},
		{RunID: "run-bbb", LastPrompt: "beta", Status: "completed"},
	}
	m.inputValue = "/history be"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if got := m2.(*AppModel).inputValue; got != "/history run-bbb" {
		t.Fatalf("inputValue=%q", got)
	}
}

func TestEnter_AcceptsHighlightedHistorySuggestion(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha", Status: "completed"},
		{RunID: "run-bbb", LastPrompt: "beta", Status: "completed"},
	}
	m.inputValue = "/history "
	m.suggIdx = 1
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.inputValue != "" {
		t.Fatalf("input should clear after Enter accept, got %q", am.inputValue)
	}
	if cmd == nil {
		t.Fatal("expected open-chat command")
	}
	if !strings.Contains(am.View(), "Opening chat run-bbb") {
		t.Fatalf("expected open message:\n%s", am.View())
	}
}

func TestEnter_AcceptsHighlightedOpenSuggestion(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha", Status: "completed"},
		{RunID: "run-bbb", LastPrompt: "beta", Status: "completed"},
	}
	m.inputValue = "/open "
	m.suggIdx = 0
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("expected open-chat command")
	}
	if !strings.Contains(am.View(), "Opening chat run-aaa") {
		t.Fatalf("expected open message:\n%s", am.View())
	}
}

func TestRenderFlowpilotLoader_ContainsWordmark(t *testing.T) {
	got := renderFlowpilotLoader(3, false)
	if !strings.Contains(got, "loading session") {
		t.Fatalf("missing status line:\n%s", got)
	}
	if !strings.Contains(got, "_") && !strings.Contains(got, "|") {
		t.Fatalf("missing ascii art:\n%s", got)
	}
}
