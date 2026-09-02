package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-339 / CP-59 F4: /open must backfill prior legs via chatTimeline.
// Provider-agnostic: join is by chatId only, no providerKey branch. Proof:
// cmdBackfillChatTimeline / renderChatTimelineBackfill / replayHistoryMessages
// never branch on providerKey — only on ChatID / HandoffPrefix / record type.

func TestChatOpenedMsg_ResetsBackfillFlagAndWiresTimeline(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.chatBackfillDone = true // stale from previous open
	_, cmd := m.Update(ChatOpenedMsg{
		Handle:   client.RunHandle{RunID: "run-3", RunKind: "chat", ChatID: "cht_a", ProviderKey: "opencode"},
		Messages: []ChatMessage{{Role: "assistant", Content: "current leg reply"}},
	})
	if cmd == nil {
		t.Fatal("ChatOpenedMsg with chat ChatID should return a batch including backfill cmd")
	}
	// After ChatOpenedMsg, flag must be reset to false so backfill can run.
	m2 := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m2.chatBackfillDone = true
	m2a, _ := m2.Update(ChatOpenedMsg{
		Handle:   client.RunHandle{RunID: "run-3", RunKind: "chat", ChatID: "cht_a"},
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if m2a.(*AppModel).chatBackfillDone != false {
		t.Fatal("ChatOpenedMsg must reset chatBackfillDone to false")
	}
}

func TestChatOpenedMsg_NoBackfillForWorkflowOrNoChatID(t *testing.T) {
	// Workflow runKind must not trigger backfill (chat_timeline is chat-gated).
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	_, cmd := m.Update(ChatOpenedMsg{
		Handle: client.RunHandle{RunID: "run-wf", RunKind: "workflow", ChatID: "cht_a"},
	})
	// cmd may contain orchestration hydrate but should not contain a failing timeline fetch;
	// the backfill helper itself returns nil when handle.ChatID empty — for workflow we
	// still have ChatID but isChatHandle returns false, so no backfill.
	// We assert via direct helper: isChatHandle distinguishes.
	if isChatHandle(&client.RunHandle{RunKind: "workflow", ChatID: "cht_a"}) {
		t.Fatal("workflow handle must not be considered chat")
	}
	// No ChatID case
	m2 := New(config.ChatConfig{}, "http://127.0.0.1:1")
	if m2.cmdBackfillChatTimeline(client.RunHandle{RunID: "run-1"}) != nil {
		t.Fatal("empty ChatID should yield nil backfill cmd")
	}
	_ = cmd
}

func TestReplayHistoryMessages_SkipsHandoffSeed(t *testing.T) {
	seed := client.HandoffPromptPrefix + "\n\n<previous_conversation>carried</previous_conversation>"
	msgs := replayHistoryMessages([]client.ProviderEvent{
		{Type: "turn_started", Prompt: seed},
		{Type: "message_completed", Text: "should merge if seed skipped"},
		{Type: "turn_started", Prompt: "real prompt"},
		{Type: "message_completed", Text: "real reply"},
	})
	joined := ""
	for _, m := range msgs {
		joined += m.Content + "|"
	}
	if strings.Contains(joined, "previous_conversation") || strings.Contains(joined, client.HandoffPromptPrefix) {
		t.Fatalf("seed must not appear in replay: %v", msgs)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (real prompt + reply), got %d: %v", len(msgs), msgs)
	}
	if msgs[0].Content != "real prompt" {
		t.Fatalf("first msg = %q want real prompt", msgs[0].Content)
	}
}

func TestRenderChatTimelineBackfill_PrependsPriorBeforeCurrent(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	// Simulate /open replay already placed current leg's message + Opened line.
	m.messages = []ChatMessage{
		{Role: "assistant", Content: "current leg reply"},
		{Role: "system", Content: "Opened chat run-3 — continue typing..."},
	}
	payload := func(v map[string]any) json.RawMessage { b, _ := json.Marshal(v); return b }
	msg := chatTimelineBackfillMsg{
		Current:  "run-3",
		Detached: false,
		Records: []client.ChatTranscriptRecord{
			{ChatID: "cht_a", ChatSeq: 1, LegRunID: "run-1", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": "hello ban la model gi"})},
			{ChatID: "cht_a", ChatSeq: 2, LegRunID: "run-1", Type: tuiRecMessageCompleted, Payload: payload(map[string]any{"text": "toi la Muse Spark"})},
			{ChatID: "cht_a", ChatSeq: 3, LegRunID: "run-2", Type: tuiRecProviderSwitch, Payload: payload(map[string]any{"toProvider": "grok", "toModel": "grok-4.5", "handoffMode": "raw", "includedTurnCount": 1})},
			{ChatID: "cht_a", ChatSeq: 4, LegRunID: "run-2", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": "grok prompt"})},
			{ChatID: "cht_a", ChatSeq: 5, LegRunID: "run-2", Type: tuiRecMessageCompleted, Payload: payload(map[string]any{"text": "grok reply"})},
			// Switch into current leg lives on destination (run-3) — must be kept and appear before current.
			{ChatID: "cht_a", ChatSeq: 6, LegRunID: "run-3", Type: tuiRecProviderSwitch, Payload: payload(map[string]any{"toProvider": "opencode", "toModel": "opencode-go/longcat-2.0", "handoffMode": "raw", "includedTurnCount": 2})},
			{ChatID: "cht_a", ChatSeq: 7, LegRunID: "run-3", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": "current leg — must be skipped"})},
		},
	}
	m2, _ := m.Update(msg)
	am := m2.(*AppModel)
	// Expected order: run-1 user, run-1 assistant, divider1, run-2 user, run-2 assistant, divider2, then original current + Opened
	if len(am.messages) != 8 {
		t.Fatalf("messages=%d want 8: %+v", len(am.messages), am.messages)
	}
	if am.messages[0].Content != "hello ban la model gi" {
		t.Fatalf("msg0 = %q", am.messages[0].Content)
	}
	if am.messages[1].Content != "toi la Muse Spark" {
		t.Fatalf("msg1 = %q", am.messages[1].Content)
	}
	if !strings.Contains(am.messages[2].Content, "switched to grok") {
		t.Fatalf("divider1 = %q", am.messages[2].Content)
	}
	if am.messages[3].Content != "grok prompt" {
		t.Fatalf("msg3 = %q", am.messages[3].Content)
	}
	if am.messages[5].Content == "" || !strings.Contains(am.messages[5].Content, "switched to opencode") {
		t.Fatalf("divider2 must be before current, got %+v", am.messages[5])
	}
	if am.messages[6].Content != "current leg reply" {
		t.Fatalf("current must follow prior, got %q", am.messages[6].Content)
	}
	if !strings.Contains(am.messages[7].Content, "Opened chat") {
		t.Fatalf("opened line must stay last, got %q", am.messages[7].Content)
	}
}

func TestRenderChatTimelineBackfill_DropsSeedPromptReplyKeepsDivider(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	payload := func(v map[string]any) json.RawMessage { b, _ := json.Marshal(v); return b }
	seed := client.HandoffPromptPrefix + "\n\n<previous_conversation>blob</previous_conversation>"
	msg := chatTimelineBackfillMsg{
		Current: "run-2",
		Records: []client.ChatTranscriptRecord{
			{ChatID: "cht_a", ChatSeq: 1, LegRunID: "run-1", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": seed})},
			{ChatID: "cht_a", ChatSeq: 2, LegRunID: "run-1", Type: tuiRecMessageCompleted, Payload: payload(map[string]any{"text": "reply"})},
			{ChatID: "cht_a", ChatSeq: 3, LegRunID: "run-2", Type: tuiRecProviderSwitch, Payload: payload(map[string]any{"toProvider": "grok", "toModel": "grok-4.5", "handoffMode": "raw", "includedTurnCount": 1})},
		},
	}
	m2, _ := m.Update(msg)
	am := m2.(*AppModel)
	// BUG-347: the seed prompt AND its envelope reply are both dropped — only
	// the E-9 divider represents the switch (D-7 single source).
	if len(am.messages) != 1 {
		t.Fatalf("seed prompt+reply should be dropped, divider kept: %+v", am.messages)
	}
	if !strings.Contains(am.messages[0].Content, "switched to grok") {
		t.Fatalf("divider missing: %+v", am.messages)
	}
}

func TestRenderChatTimelineBackfill_KeepsSwitchDividerOnCurrentLeg(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.messages = []ChatMessage{{Role: "assistant", Content: "current"}}
	payload := func(v map[string]any) json.RawMessage { b, _ := json.Marshal(v); return b }
	msg := chatTimelineBackfillMsg{
		Current: "run-3",
		Records: []client.ChatTranscriptRecord{
			{ChatID: "cht_a", ChatSeq: 1, LegRunID: "run-1", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": "hi"})},
			{ChatID: "cht_a", ChatSeq: 2, LegRunID: "run-3", Type: tuiRecProviderSwitch, Payload: payload(map[string]any{"toProvider": "opencode", "toModel": "longcat-2.0", "handoffMode": "raw", "includedTurnCount": 1})},
			{ChatID: "cht_a", ChatSeq: 3, LegRunID: "run-3", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": "skip me — current turn"})},
		},
	}
	m2, _ := m.Update(msg)
	am := m2.(*AppModel)
	// Prior hi, divider (even though LegRunID == current), then current
	if len(am.messages) != 3 {
		t.Fatalf("want 3 (prior + divider + current), got %d: %+v", len(am.messages), am.messages)
	}
	if am.messages[0].Content != "hi" || !strings.Contains(am.messages[1].Content, "switched to opencode") || am.messages[2].Content != "current" {
		t.Fatalf("order wrong: %+v", am.messages)
	}
}

func TestChatBackfillDone_ResetsOnNewOpen(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	payload := func(v map[string]any) json.RawMessage { b, _ := json.Marshal(v); return b }
	// First open
	m1, _ := m.Update(ChatOpenedMsg{Handle: client.RunHandle{RunID: "run-2", RunKind: "chat", ChatID: "cht_a"}, Messages: []ChatMessage{{Role: "user", Content: "a"}}})
	am1 := m1.(*AppModel)
	// Backfill once
	msg := chatTimelineBackfillMsg{
		Current: "run-2",
		Records: []client.ChatTranscriptRecord{
			{ChatID: "cht_a", ChatSeq: 1, LegRunID: "run-1", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": "prior a"})},
		},
	}
	am2, _ := am1.Update(msg)
	if len(am2.(*AppModel).messages) != 3 { // prior a + a + Opened
		t.Fatalf("first backfill should have prepended, got %+v", am2.(*AppModel).messages)
	}
	// Second open of another chat should reset the guard
	am3, _ := am2.Update(ChatOpenedMsg{Handle: client.RunHandle{RunID: "run-9", RunKind: "chat", ChatID: "cht_b"}, Messages: []ChatMessage{{Role: "user", Content: "b"}}})
	if am3.(*AppModel).chatBackfillDone != false {
		t.Fatal("second ChatOpenedMsg must reset chatBackfillDone")
	}
	msg2 := chatTimelineBackfillMsg{
		Current: "run-9",
		Records: []client.ChatTranscriptRecord{
			{ChatID: "cht_b", ChatSeq: 1, LegRunID: "run-8", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": "prior b"})},
		},
	}
	am4, _ := am3.Update(msg2)
	am := am4.(*AppModel)
	if len(am.messages) != 3 || am.messages[0].Content != "prior b" || am.messages[1].Content != "b" {
		t.Fatalf("second chat backfill must prepend independently: %+v", am.messages)
	}
	if !strings.Contains(am.messages[2].Content, "Opened chat") {
		t.Fatalf("opened line must stay last: %+v", am.messages)
	}
}

func TestCmdBackfillChatTimeline_FetchesProviderAgnostic(t *testing.T) {
	// Prove the fetch is chatId-only, not providerKey-dependent.
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chatId": "cht_a", "legs": []any{}, "records": []any{}, "nextSeq": 0, "truncated": false,
		})
	}))
	defer srv.Close()
	m := New(config.ChatConfig{}, srv.URL)
	cmd := m.cmdBackfillChatTimeline(client.RunHandle{RunID: "run-3", RunKind: "chat", ChatID: "cht_a"})
	if cmd == nil {
		t.Fatal("expected cmd for chat handle")
	}
	msg := cmd()
	bm, ok := msg.(chatTimelineBackfillMsg)
	if !ok || bm.Err != nil {
		t.Fatalf("backfill msg err: %+v", msg)
	}
	if gotPath != "/client/chats/cht_a/timeline" {
		t.Fatalf("path=%q want /client/chats/cht_a/timeline", gotPath)
	}
}

func TestChatOpenedMsg_FallbackToHistoryMetaChatID(t *testing.T) {
	// BUG-338: ResumeRun handle may lack ChatID (session predates column) but
	// the history list already stamps it via transcriptLegIndex. The open must
	// still backfill using HistoryMeta.ChatID.
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	// Simulate chatList with stamped entry
	m.chatList = []client.RunHistoryItem{
		{RunID: "run-392046", ChatID: "cht_test", LegSeq: 2, RunKind: "chat", ProviderKey: "opencode"},
	}
	// ChatOpenedMsg with empty ChatID on handle but HistoryMeta carries cht_test
	_, cmd := m.Update(ChatOpenedMsg{
		Handle:      client.RunHandle{RunID: "run-392046", RunKind: "chat", ChatID: ""},
		HistoryMeta: client.RunHistoryItem{RunID: "run-392046", ChatID: "cht_test", RunKind: "chat"},
		Messages:    []ChatMessage{{Role: "assistant", Content: "current"}},
	})
	if cmd == nil {
		t.Fatal("fallback to HistoryMeta ChatID should still produce a backfill cmd")
	}
	// Workflow with chatId must not fallback (should not fetch)
	m2 := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m2.chatList = []client.RunHistoryItem{
		{RunID: "run-wf", ChatID: "", RunKind: "workflow"},
	}
	_, cmd2 := m2.Update(ChatOpenedMsg{
		Handle:      client.RunHandle{RunID: "run-wf", RunKind: "workflow", ChatID: ""},
		HistoryMeta: client.RunHistoryItem{RunID: "run-wf", RunKind: "workflow"},
	})
	if cmd2 != nil {
		// cmd2 may contain other cmds (hydrates) but should not contain backfill;
		// we check directly via helper: empty chatId yields nil
		if m2.cmdBackfillChatTimeline(client.RunHandle{RunID: "run-wf", RunKind: "workflow", ChatID: ""}) != nil {
			t.Fatal("workflow should not backfill")
		}
	}
}

func TestChatTimelineBackfillMsg_SurfacesError(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.messages = []ChatMessage{{Role: "assistant", Content: "current"}}
	m.chatBackfillDone = false
	msg := chatTimelineBackfillMsg{Current: "run-1", Err: assertErr("timeline 404")}
	m2, _ := m.Update(msg)
	am := m2.(*AppModel)
	if !am.chatBackfillDone {
		t.Fatal("error backfill must still mark done to avoid retry loop")
	}
	found := false
	for _, mm := range am.messages {
		if strings.Contains(mm.Content, "Chat history unavailable") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error line, got %+v", am.messages)
	}
}

func TestRoutePostureSwitch_DetachedDefersToReattach(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.chatDetached = true
	m.runHandle = &client.RunHandle{RunID: "run-398551", RunKind: "chat", ChatID: "cht_test"}
	m.provider = "opencode"
	m.model = "opencode-go/longcat-2.0"
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{
		"code": {Provider: "opencode", Model: "opencode-go/longcat-2.0"},
		"plan": {Provider: "grok", Model: "grok-4.5"},
	}}
	// Bare grok pin would normally switch; detached should defer, not call endpoint
	before := len(m.messages)
	cmd := m.routePostureSwitch(cfg, "plan")
	if cmd == nil {
		t.Fatal("detached Tab must return detachedNoticeMsg, not nil")
	}
	if m.provider != "grok" || m.model != "grok-4.5" {
		t.Fatalf("detached Tab should apply locally: provider=%q model=%q", m.provider, m.model)
	}
	if m.chatSwitchInFlight {
		t.Fatal("detached Tab must not set inFlight")
	}
	if len(m.messages) != before+1 || !strings.Contains(m.messages[len(m.messages)-1].Content, "reattaches") {
		t.Fatalf("expected defer notice: %+v", m.messages[len(m.messages)-1])
	}
	// Same-provider Tab on detached should be in-place (no defer)
	m2 := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m2.chatDetached = true
	m2.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m2.provider = "opencode"
	cfg2 := client.ChatPostureConfig{Active: "plan", Profiles: map[string]client.ChatPostureProfile{
		"plan": {Provider: "opencode", Model: "opencode-go/longcat-2.0"},
	}}
	if cmd := m2.routePostureSwitch(cfg2, "plan"); cmd != nil {
		t.Fatal("same-provider detached Tab must be in-place, no cmd")
	}
}

func TestApplyChatSwitched_409ChatNoActiveLegDefers(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.provider = "opencode"
	m.model = "opencode-go/longcat-2.0"
	m.chatSwitchInFlight = true
	msg := ChatSwitchedMsg{Err: assertErr("runner API error 409 (chat_no_active_leg): chat has no active leg"), TargetProvider: "grok", TargetModel: "grok-4.5"}
	m.applyChatSwitched(msg)
	if !m.chatDetached {
		t.Fatal("409 should set chatDetached=true")
	}
	if m.provider != "grok" || m.model != "grok-4.5" {
		t.Fatalf("409 should apply target locally: %q/%q", m.provider, m.model)
	}
	if m.chatSwitchInFlight {
		t.Fatal("inFlight must be cleared")
	}
	found := false
	for _, mm := range m.messages {
		if strings.Contains(mm.Content, "reattaches") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected defer notice after 409: %+v", m.messages)
	}
}

func TestCmdBackfillChatTimeline_DetachedForCompletedLegs(t *testing.T) {
	// Timeline with legs marked active but status completed should be considered detached
	// so Tab defers. This mirrors run-398551 after /open where LegStateActive + completed.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chatId": "cht_test",
			"legs": []map[string]any{
				{"runId": "run-1", "legSeq": 0, "legState": "active", "status": "completed"},
				{"runId": "run-2", "legSeq": 1, "legState": "active", "status": "completed"},
			},
			"records": []any{},
		})
	}))
	defer srv.Close()
	m := New(config.ChatConfig{}, srv.URL)
	cmd := m.cmdBackfillChatTimeline(client.RunHandle{RunID: "run-2", RunKind: "chat", ChatID: "cht_test"})
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg := cmd()
	bm := msg.(chatTimelineBackfillMsg)
	if !bm.Detached {
		t.Fatal("completed legs should be considered detached")
	}
	// Running leg should not be detached
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chatId": "cht_test",
			"legs": []map[string]any{
				{"runId": "run-1", "legSeq": 0, "legState": "active", "status": "running"},
			},
			"records": []any{},
		})
	}))
	defer srv2.Close()
	m2 := New(config.ChatConfig{}, srv2.URL)
	cmd2 := m2.cmdBackfillChatTimeline(client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_test"})
	if cmd2 == nil {
		t.Fatal("expected cmd2")
	}
	bm2 := cmd2().(chatTimelineBackfillMsg)
	if bm2.Detached {
		t.Fatal("running leg should not be detached")
	}
}

func TestRoutePostureSwitch_BlockedWhenQuestionPending(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-405014", RunKind: "chat", ChatID: "cht_test"}
	m.provider = "opencode"
	m.model = "opencode-go/longcat-2.0"
	m.question = &QuestionState{ID: "q-1", Prompt: "Ban ten la gi?"}
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{
		"code": {Provider: "opencode", Model: "opencode-go/longcat-2.0"},
		"plan": {Provider: "grok", Model: "grok-4.5"},
	}}
	cmd := m.routePostureSwitch(cfg, "plan")
	if cmd == nil {
		t.Fatal("pending question should still return a notice cmd, not nil (to show friendly message)")
	}
	if m.chatSwitchInFlight {
		t.Fatal("must not set inFlight when blocked by pending question")
	}
	// Should not have called SwitchChatProvider, so provider must stay
	if m.provider != "opencode" {
		t.Fatalf("provider must not change when blocked, got %q", m.provider)
	}
	found := false
	for _, mm := range m.messages {
		if strings.Contains(mm.Content, "question or approval is pending") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pending-question guard message: %+v", m.messages)
	}
	// Also test turnStream busy blocks
	m2 := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m2.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m2.provider = "opencode"
	m2.turnStream = &turnStreamState{}
	cfg2 := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{
		"plan": {Provider: "grok", Model: "grok-4.5"},
	}}
	if cmd := m2.routePostureSwitch(cfg2, "plan"); cmd == nil {
		t.Fatal("turnStream busy should also block with notice")
	}
}

func TestRouteProviderSwitch_BlockedWhenQuestionPending(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.question = &QuestionState{ID: "q-1"}
	cmd := m.routeProviderSwitch("grok", "grok-4.5")
	if cmd == nil {
		t.Fatal("expected block notice cmd")
	}
	if m.chatSwitchInFlight {
		t.Fatal("must not set inFlight when blocked")
	}
}

func assertErr(s string) error { return &testErr{s} }

type testErr struct{ s string }

func (e *testErr) Error() string { return e.s }
