package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestKnownSlashCommands_IncludesDelete(t *testing.T) {
	found := false
	for _, sc := range knownSlashCommands {
		if sc.name == "/delete" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("knownSlashCommands missing /delete")
	}
}

func TestFilterDeleteSuggestions_IncludesAllAndTicks(t *testing.T) {
	items := []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "fix login bug", Status: "completed", RunKind: "chat"},
		{RunID: "run-bbb", LastPrompt: "ship android", Status: "running", RunKind: "workflow"},
	}
	del := filterDeleteSuggestions("/delete ", items)
	if len(del) != 3 {
		t.Fatalf("want 3 (all+2), got %d: %+v", len(del), del)
	}
	if del[0].value != "all" || del[0].kind != "delete" || del[0].slash != "/delete" {
		t.Fatalf("first row should be all delete, got %+v", del[0])
	}
	if !strings.Contains(del[0].detail, "[ ] all") {
		t.Fatalf("all detail should show [ ] when none ticked, got %q", del[0].detail)
	}
	if del[1].value != "run-aaa" || del[1].kind != "delete" {
		t.Fatalf("second row %+v", del[1])
	}
	if !strings.Contains(del[1].detail, "[ ] #1") {
		t.Fatalf("tick should be [ ] when not selected, got %q", del[1].detail)
	}
	// selected tick
	sel := map[string]bool{"run-aaa": true}
	delSel := filterDeleteSuggestionsWithSelected("/delete ", items, nil, sel)
	if !strings.Contains(delSel[1].detail, "[*] #1") {
		t.Fatalf("selected should show [*], got %q", delSel[1].detail)
	}
	// all ticked when every run selected
	selAll := map[string]bool{"run-aaa": true, "run-bbb": true}
	delAllSel := filterDeleteSuggestionsWithSelected("/delete ", items, nil, selAll)
	if !strings.Contains(delAllSel[0].detail, "[*] all") {
		t.Fatalf("all should show [*] when all ticked, got %q", delAllSel[0].detail)
	}
	// Bare should not open picker
	if filterDeleteSuggestions("/delete", items) != nil {
		t.Fatal("bare /delete should not open picker")
	}
	// Query filter: /delete android should show all + 1 filtered (not 0)
	f := filterDeleteSuggestions("/delete android", items)
	if len(f) != 2 {
		t.Fatalf("filtered with all+1 got %d: %+v", len(f), f)
	}
	if f[1].value != "run-bbb" {
		t.Fatalf("filtered value %q", f[1].value)
	}
}

func TestDelete_TabTogglesSingleAndAll(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha", Status: "completed"},
		{RunID: "run-bbb", LastPrompt: "beta", Status: "completed"},
	}
	m.inputValue = "/delete "
	// Tab on second row (run-aaa) -> tick it
	m.suggIdx = 1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if !am.deleteSelected["run-aaa"] {
		t.Fatalf("run-aaa should be ticked after Tab, got %+v", am.deleteSelected)
	}
	if len(am.deleteSelected) != 1 {
		t.Fatalf("only one ticked, got %+v", am.deleteSelected)
	}
	// Tab again on same row -> untick
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if m3.(*AppModel).deleteSelected != nil && m3.(*AppModel).deleteSelected["run-aaa"] {
		t.Fatalf("should untick after second Tab")
	}
	// Tab on all row -> tick all
	m4 := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m4.chatList = m.chatList
	m4.inputValue = "/delete "
	m4.suggIdx = 0
	m5, _ := m4.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am5 := m5.(*AppModel)
	if len(am5.deleteSelected) != 2 || !am5.deleteSelected["run-aaa"] || !am5.deleteSelected["run-bbb"] {
		t.Fatalf("all should tick both, got %+v", am5.deleteSelected)
	}
	// Tab on all again -> untick all
	m6, _ := am5.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if m6.(*AppModel).deleteSelected != nil {
		t.Fatalf("all second Tab should clear, got %+v", m6.(*AppModel).deleteSelected)
	}
}

func TestDelete_EnterWithNoSelectionDeletesHighlighted(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha", Status: "completed"},
		{RunID: "run-bbb", LastPrompt: "beta", Status: "completed"},
	}
	m.inputValue = "/delete "
	m.suggIdx = 1 // run-aaa
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if len(am.deletePendingIDs) != 1 || am.deletePendingIDs[0] != "run-aaa" {
		t.Fatalf("pending %+v want [run-aaa]", am.deletePendingIDs)
	}
	if !strings.Contains(strings.ToLower(am.lastMessageText()), "delete") {
		t.Fatalf("prompt %q", am.lastMessageText())
	}
}

func TestDelete_EnterWithSelectionDeletesBatch(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha"},
		{RunID: "run-bbb", LastPrompt: "beta"},
		{RunID: "run-ccc", LastPrompt: "gamma"},
	}
	m.inputValue = "/delete "
	// Tick aaa and bbb via Tab
	m.suggIdx = 1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	m3, _ := m2.(*AppModel).handleKey(tea.KeyMsg{Type: tea.KeyTab}) // this second Tab will toggle? Actually after first Tab, suggIdx stays at 1 retargeted, second Tab on same row unticks — need to move to next row
	// Instead manually set selection
	am := m3.(*AppModel)
	// Reset and directly set selection to mimic two ticks
	am.deleteSelected = map[string]bool{"run-aaa": true, "run-bbb": true}
	am.inputValue = "/delete "
	am.suggIdx = 0
	m4, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am4 := m4.(*AppModel)
	if len(am4.deletePendingIDs) != 2 {
		t.Fatalf("batch pending %v", am4.deletePendingIDs)
	}
	if am4.deletePendingIDs[0] != "run-aaa" || am4.deletePendingIDs[1] != "run-bbb" {
		t.Fatalf("batch order %v", am4.deletePendingIDs)
	}
	if !strings.Contains(am4.lastMessageText(), "2 chats") {
		t.Fatalf("batch prompt %q", am4.lastMessageText())
	}
}

func TestDelete_EnterOnAllRowDeletesAll(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha"},
		{RunID: "run-bbb", LastPrompt: "beta"},
	}
	m.inputValue = "/delete "
	m.suggIdx = 0 // all
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if len(am.deletePendingIDs) != 2 {
		t.Fatalf("all pending %v", am.deletePendingIDs)
	}
	if !strings.Contains(strings.ToLower(am.lastMessageText()), "delete") {
		t.Fatalf("prompt %q", am.lastMessageText())
	}
}

func TestHandleSlash_DeleteArmsPendingNotDirectDelete(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha", Status: "completed"},
		{RunID: "run-bbb", LastPrompt: "beta", Status: "completed"},
	}
	m2, cmd := m.handleSlashCommand("/delete 2")
	am := m2.(*AppModel)
	if len(am.deletePendingIDs) != 1 || am.deletePendingIDs[0] != "run-bbb" {
		t.Fatalf("pending %v want [run-bbb]", am.deletePendingIDs)
	}
	if cmd != nil {
		if _, ok := cmd().(ChatDeletedMsg); ok {
			t.Fatal("should not dispatch ChatDeletedMsg directly from slash")
		}
	}
	if len(am.chatList) != 2 {
		t.Fatalf("chatList mutated prematurely")
	}
}

func TestHandleSlash_DeleteAllArmsPending(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha"},
		{RunID: "run-bbb", LastPrompt: "beta"},
	}
	m2, _ := m.handleSlashCommand("/delete all")
	am := m2.(*AppModel)
	if len(am.deletePendingIDs) != 2 {
		t.Fatalf("all pending %v", am.deletePendingIDs)
	}
	if !strings.Contains(strings.ToLower(am.lastMessageText()), "all") {
		t.Fatalf("prompt %q", am.lastMessageText())
	}
}

func TestHandleSlash_DeleteNoArgWithOpenArmsCurrent(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{{RunID: "run-aaa", LastPrompt: "alpha"}}
	m.runHandle = &client.RunHandle{RunID: "run-aaa"}
	m2, _ := m.handleSlashCommand("/delete")
	am := m2.(*AppModel)
	if len(am.deletePendingIDs) != 1 || am.deletePendingIDs[0] != "run-aaa" {
		t.Fatalf("pending %v", am.deletePendingIDs)
	}
}

func TestHandleSlash_DeleteNoArgNoOpenShowsUsage(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/delete")
	am := m2.(*AppModel)
	if len(am.deletePendingIDs) != 0 || am.deletePendingRunID != "" {
		t.Fatalf("should not arm pending, got %+v %q", am.deletePendingIDs, am.deletePendingRunID)
	}
	if !strings.Contains(strings.ToLower(am.lastMessageText()), "usage") {
		t.Fatalf("expected usage, got %q", am.lastMessageText())
	}
}

func TestEnter_AcceptsDeleteSuggestionArmsPending(t *testing.T) {
	// Previous test now expects all row at idx 0, so single is idx 1
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha", Status: "completed"},
		{RunID: "run-bbb", LastPrompt: "beta", Status: "completed"},
	}
	m.inputValue = "/delete "
	m.suggIdx = 1 // run-aaa (0 is all)
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if len(am.deletePendingIDs) != 1 || am.deletePendingIDs[0] != "run-aaa" {
		t.Fatalf("pending %v want [run-aaa]", am.deletePendingIDs)
	}
	if am.inputValue != "" {
		t.Fatalf("input should clear after arming, got %q", am.inputValue)
	}
	if !strings.Contains(strings.ToLower(am.lastMessageText()), "delete") {
		t.Fatalf("expected delete prompt, got %q", am.lastMessageText())
	}
	if len(am.chatList) != 2 {
		t.Fatalf("chatList should not drop until y")
	}
}

func TestHandleKey_PendingDeleteYDispatchesDelete(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.deletePendingIDs = []string{"run-aaa"}
	m.deletePendingRunID = "run-aaa"
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("y should dispatch delete cmd")
	}
	msg := cmd()
	del, ok := msg.(ChatDeletedMsg)
	if !ok {
		t.Fatalf("msg %T not ChatDeletedMsg", msg)
	}
	if del.RunID != "run-aaa" {
		t.Fatalf("runID=%q", del.RunID)
	}
	if len(m2.(*AppModel).deletePendingIDs) != 0 && m2.(*AppModel).deletePendingRunID != "" {
		t.Fatalf("pending should clear after y")
	}
	// batch queue should have been set
	if m2.(*AppModel).deleteBatchTotal != 1 {
		t.Fatalf("batchTotal should be 1")
	}
}

func TestHandleKey_PendingBatchYDispatchesFirst(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.deletePendingIDs = []string{"run-aaa", "run-bbb"}
	m.deletePendingRunID = "run-aaa"
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("y should dispatch")
	}
	am := m2.(*AppModel)
	if am.deleteBatchTotal != 2 {
		t.Fatalf("total %d", am.deleteBatchTotal)
	}
	if len(am.deleteBatchQueue) != 1 || am.deleteBatchQueue[0] != "run-bbb" {
		t.Fatalf("queue %v", am.deleteBatchQueue)
	}
	msg := cmd()
	if msg.(ChatDeletedMsg).RunID != "run-aaa" {
		t.Fatalf("first id")
	}
}

func TestHandleKey_PendingDeleteEnterDispatchesDelete(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.deletePendingIDs = []string{"run-aaa"}
	m.deletePendingRunID = "run-aaa"
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should dispatch delete")
	}
	if _, ok := cmd().(ChatDeletedMsg); !ok {
		t.Fatalf("not ChatDeletedMsg")
	}
	_ = m2
}

func TestHandleKey_PendingDeleteNCancels(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.deletePendingIDs = []string{"run-aaa"}
	m.deletePendingRunID = "run-aaa"
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if cmd != nil {
		if _, ok := cmd().(ChatDeletedMsg); ok {
			t.Fatal("n should not dispatch delete")
		}
	}
	am := m2.(*AppModel)
	if len(am.deletePendingIDs) != 0 || am.deletePendingRunID != "" {
		t.Fatalf("pending not cleared")
	}
	if !strings.Contains(strings.ToLower(am.lastMessageText()), "cancel") {
		t.Fatalf("expected cancel msg, got %q", am.lastMessageText())
	}
}

func TestHandleKey_PendingDeleteEscCancels(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.deletePendingIDs = []string{"run-aaa"}
	m.deletePendingRunID = "run-aaa"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	am := m2.(*AppModel)
	if len(am.deletePendingIDs) != 0 || am.deletePendingRunID != "" {
		t.Fatal("esc should cancel pending")
	}
}

func TestHandleKey_PendingSwallowsOtherKeys(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.deletePendingIDs = []string{"run-aaa"}
	m.deletePendingRunID = "run-aaa"
	m.inputValue = ""
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m2.(*AppModel).inputValue != "" {
		t.Fatalf("should swallow typed x while pending")
	}
}

func TestHandleChatDeleted_SuccessDropsRowAndResetsCurrentChat(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha"},
		{RunID: "run-bbb", LastPrompt: "beta"},
	}
	m.runHandle = &client.RunHandle{RunID: "run-aaa"}
	m.messages = []ChatMessage{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}}
	m.deletePendingIDs = []string{"run-aaa"}
	m.deletePendingRunID = "run-aaa"
	m2, _ := m.handleChatDeleted(ChatDeletedMsg{RunID: "run-aaa"})
	am := m2.(*AppModel)
	if len(am.chatList) != 1 || am.chatList[0].RunID != "run-bbb" {
		t.Fatalf("chatList %+v", am.chatList)
	}
	if am.runHandle != nil {
		t.Fatalf("current handle should clear")
	}
	if len(am.messages) != 1 || am.messages[0].Role != "system" {
		t.Fatalf("messages should be single system line, got %+v", am.messages)
	}
	if !strings.Contains(strings.ToLower(am.lastMessageText()), "deleted current chat") {
		t.Fatalf("msg %q", am.lastMessageText())
	}
}

func TestHandleChatDeleted_SuccessKeepsTranscriptForOtherChat(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "alpha"},
		{RunID: "run-bbb", LastPrompt: "beta"},
	}
	m.runHandle = &client.RunHandle{RunID: "run-aaa"}
	m.messages = []ChatMessage{{Role: "user", Content: "keep me"}}
	m2, _ := m.handleChatDeleted(ChatDeletedMsg{RunID: "run-bbb"})
	am := m2.(*AppModel)
	if len(am.chatList) != 1 || am.chatList[0].RunID != "run-aaa" {
		t.Fatalf("chatList %+v", am.chatList)
	}
	if am.runHandle == nil || am.runHandle.RunID != "run-aaa" {
		t.Fatal("current should stay")
	}
	if len(am.messages) != 2 || am.messages[0].Content != "keep me" {
		t.Fatalf("messages lost %+v", am.messages)
	}
	if !strings.Contains(strings.ToLower(am.lastMessageText()), "deleted chat") {
		t.Fatalf("msg %q", am.lastMessageText())
	}
}

func TestHandleChatDeleted_BatchQueuesNext(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "a"},
		{RunID: "run-bbb", LastPrompt: "b"},
		{RunID: "run-ccc", LastPrompt: "c"},
	}
	m.deleteBatchQueue = []string{"run-bbb", "run-ccc"}
	m.deleteBatchTotal = 3
	m2, cmd := m.handleChatDeleted(ChatDeletedMsg{RunID: "run-aaa"})
	if cmd == nil {
		t.Fatal("should queue next delete")
	}
	am := m2.(*AppModel)
	if len(am.chatList) != 2 {
		t.Fatalf("chatList %v", am.chatList)
	}
	// Simulate next delete success without hitting network
	m3, cmd2 := am.handleChatDeleted(ChatDeletedMsg{RunID: "run-bbb"})
	if cmd2 == nil {
		t.Fatal("should queue third")
	}
	m4, cmd3 := m3.(*AppModel).handleChatDeleted(ChatDeletedMsg{RunID: "run-ccc"})
	if cmd3 != nil {
		t.Fatalf("no more queue, should be nil got %v", cmd3)
	}
	if m4.(*AppModel).deleteBatchTotal != 0 {
		t.Fatalf("batch total should clear")
	}
	if !strings.Contains(strings.ToLower(m4.(*AppModel).lastMessageText()), "deleted 3 chats") {
		t.Fatalf("final batch msg %q", m4.(*AppModel).lastMessageText())
	}
}

func TestHandleChatDeleted_ErrorKeepsRow(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{{RunID: "run-aaa", LastPrompt: "alpha"}}
	m2, _ := m.handleChatDeleted(ChatDeletedMsg{RunID: "run-aaa", Err: "run_not_found"})
	am := m2.(*AppModel)
	if len(am.chatList) != 1 {
		t.Fatalf("should keep row on error")
	}
	if !strings.Contains(strings.ToLower(am.lastMessageText()), "delete failed") {
		t.Fatalf("msg %q", am.lastMessageText())
	}
}

// lastMessageText is a helper for assertions.
func (m *AppModel) lastMessageText() string {
	if len(m.messages) == 0 {
		return ""
	}
	return m.messages[len(m.messages)-1].Content
}
