package app

import (
	"encoding/json"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-347 (live): the post-switch seed turn streams fire-and-return on the
// attached stream; its envelope reply is an orphan assistant bubble with no
// You-box. The divider must render synchronously on switch commit and the
// seed's assistant output must be dropped until the seed turn completes.
func TestSeedTurn_SuppressesEnvelopeReplyLive(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "grok"
	m.model = "grok-4.5"
	msg := ChatSwitchedMsg{
		Resp: &client.ChatSwitchResponse{
			Handle: client.RunHandle{RunID: "run-2", RunKind: "chat", ChatID: "cht_a", ProviderKey: "grok"},
			Model:  "grok-4.5",
			Handoff: client.ChatSwitchHandoffStats{
				HandoffMode: "raw", IncludedTurnCount: 1, OmittedTurnCount: 0, Truncated: false,
			},
		},
	}
	m.applyChatSwitched(msg)

	if !m.seedTurnActive {
		t.Fatal("switch commit must arm the seed guard")
	}
	last := m.messages[len(m.messages)-1]
	if last.Role != "system" || !strings.Contains(last.Content, "⇄ switched to grok") || !strings.Contains(last.Content, "carried 1 turns (raw)") {
		t.Fatalf("switch commit must render the divider synchronously, got %q", last.Content)
	}
	if m.lastSwitchStats != nil {
		t.Fatal("divider must consume the switch stats")
	}

	// Seed assistant output is dropped.
	m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{Type: "message_delta", Text: "Chào bạn! Mình là Grok"}})
	m2, _ = m2.(*AppModel).Update(EventMsg{Ev: client.ProviderEvent{Type: "message_completed", Text: "Chào bạn! Mình là Grok 4.5, do xAI phát triển"}})
	am := m2.(*AppModel)
	if n := len(am.messages); n != 1 {
		t.Fatalf("seed reply must be dropped, got %d messages: %+v", n, am.messages)
	}

	// Seed turn completed: guard clears, no FinalMessage re-append.
	m3, _ := am.Update(EventMsg{Ev: client.ProviderEvent{Type: "turn_completed", FinalMessage: "Chào bạn! Mình là Grok 4.5, do xAI phát triển"}})
	am3 := m3.(*AppModel)
	if am3.seedTurnActive {
		t.Fatal("seed guard must clear on turn_completed")
	}
	if n := len(am3.messages); n != 1 {
		t.Fatalf("seed FinalMessage must not re-append, got %d messages", n)
	}

	// A real user turn streams normally afterwards.
	m4, _ := am3.Update(EventMsg{Ev: client.ProviderEvent{Type: "turn_started", Prompt: "bây giờ là model gì"}})
	m4, _ = m4.(*AppModel).Update(EventMsg{Ev: client.ProviderEvent{Type: "message_completed", Text: "Grok 4.5 do xAI"}})
	am4 := m4.(*AppModel)
	found := false
	for _, msgx := range am4.messages {
		if msgx.Role == "assistant" && strings.Contains(msgx.Content, "Grok 4.5 do xAI") {
			found = true
		}
	}
	if !found {
		t.Fatal("a real turn after the seed must render normally")
	}
}

// BUG-347 (live edge): a real user turn_started on the new leg disarms the
// seed guard even when turn_completed was missed (seed failed / stop raced).
func TestSeedTurn_RealTurnDisarmsGuard(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "grok"
	m.model = "grok-4.5"
	m.applyChatSwitched(ChatSwitchedMsg{
		Resp: &client.ChatSwitchResponse{
			Handle:  client.RunHandle{RunID: "run-2", RunKind: "chat", ChatID: "cht_a", ProviderKey: "grok"},
			Model:   "grok-4.5",
			Handoff: client.ChatSwitchHandoffStats{HandoffMode: "raw"},
		},
	})
	if !m.seedTurnActive {
		t.Fatal("seed guard must be armed")
	}
	m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{Type: "turn_started", Prompt: "real question"}})
	am := m2.(*AppModel)
	if am.seedTurnActive {
		t.Fatal("a real user turn must disarm the seed guard")
	}
	m3, _ := am.Update(EventMsg{Ev: client.ProviderEvent{Type: "message_completed", Text: "real answer"}})
	if !strings.Contains(m3.(*AppModel).messages[len(m3.(*AppModel).messages)-1].Content, "real answer") {
		t.Fatal("post-guard assistant output must render")
	}
}

// BUG-347 (busy message): a Tab while the switch is in flight must say
// "switch in progress", not the question/approval message.
func TestTabDuringSwitchInFlight_BusyMessageIsSwitchNotQuestion(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.chatSwitchInFlight = true
	m.chatPosturePending = "apply:code"
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{
		"code": {Provider: "opencode", Model: "opencode-go/deepseek-v4-flash"},
	}}
	cmd := m.chatPostureCmdFromPending(cfg)
	if cmd == nil {
		t.Fatal("busy Tab must still return the notice cmd")
	}
	last := m.messages[len(m.messages)-1]
	if strings.Contains(last.Content, "question or approval") {
		t.Fatalf("in-flight Tab must not use the question message, got %q", last.Content)
	}
	if !strings.Contains(last.Content, "switch in progress") {
		t.Fatalf("in-flight Tab must say switch in progress, got %q", last.Content)
	}
}

// BUG-347 follow-up (C1): a /provider or /model cross-provider while a turn is
// streaming must say "turn in progress", not "question or approval" — and must
// never change the footer provider/model.
func TestBusySwitchDuringLiveTurn_MessageIsTurnNotQuestionAndFooterStays(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark"
	m.turnLive = true
	cmd := m.routeProviderSwitch("grok", "grok-4.5")
	if cmd == nil {
		t.Fatal("busy switch must return the notice cmd")
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(detachedNoticeMsg); !ok {
			t.Fatalf("busy cmd must be detachedNoticeMsg, got %T", msg)
		}
	}
	if m.provider != "opencode" || m.model != "opencode-go/muse-spark" {
		t.Fatalf("busy switch must not change footer provider/model, got %s/%s", m.provider, m.model)
	}
	last := m.messages[len(m.messages)-1]
	if strings.Contains(last.Content, "question or approval") {
		t.Fatalf("live-turn busy must not say question/approval, got %q", last.Content)
	}
	if !strings.Contains(last.Content, "A turn is in progress") {
		t.Fatalf("live-turn busy must say turn in progress, got %q", last.Content)
	}
}

// BUG-347 (replay): replayHistoryMessages must drop the seed envelope reply
// when the seed turn_started recorded with an empty prompt.
func TestReplayHistory_DropsEmptyPromptSeedReply(t *testing.T) {
	evs := []client.ProviderEvent{
		{Type: "turn_started", Prompt: "chao ban la model gi"},
		{Type: "message_completed", Text: "Muse Spark tra loi"},
		{Type: "turn_started", Prompt: ""}, // seed turn (envelope attaches async)
		{Type: "message_completed", Text: "Chào bạn! Mình là Grok 4.5 do xAI"},
		{Type: "turn_started", Prompt: "bây giờ là model gì"},
		{Type: "message_completed", Text: "Grok 4.5 do xAI"},
	}
	out := replayHistoryMessages(evs)
	var texts []string
	for _, cm := range out {
		texts = append(texts, cm.Role+":"+cm.Content)
	}
	joined := strings.Join(texts, "\n")
	if strings.Contains(joined, "Chào bạn! Mình là Grok") {
		t.Fatalf("seed reply must be dropped in replay, got:\n%s", joined)
	}
	if !strings.Contains(joined, "user:chao ban la model gi") || !strings.Contains(joined, "user:bây giờ là model gì") {
		t.Fatalf("real turns must survive replay, got:\n%s", joined)
	}
	if !strings.Contains(joined, "assistant:Muse Spark tra loi") || !strings.Contains(joined, "assistant:Grok 4.5 do xAI") {
		t.Fatalf("real replies must survive replay, got:\n%s", joined)
	}
}

// BUG-347 (backfill): renderChatTimelineBackfill must not append the seed
// envelope reply as an assistant bubble on a prior destination leg.
func TestTimelineBackfill_DropsSeedReplyOnPriorLeg(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-3", RunKind: "chat", ChatID: "cht_a"}
	m.chatPostureCfg = client.ChatPostureConfig{}
	payload := func(v map[string]any) []byte { b, _ := json.Marshal(v); return b }
	recs := []client.ChatTranscriptRecord{
		{ChatID: "cht_a", ChatSeq: 1, LegRunID: "run-1", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": "chao"})},
		{ChatID: "cht_a", ChatSeq: 2, LegRunID: "run-1", Type: tuiRecMessageCompleted, Payload: payload(map[string]any{"text": "muse tra loi"})},
		{ChatID: "cht_a", ChatSeq: 3, LegRunID: "run-2", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": ""})}, // seed
		{ChatID: "cht_a", ChatSeq: 4, LegRunID: "run-2", Type: tuiRecProviderSwitch, Payload: payload(map[string]any{
			"toProvider": "grok", "toModel": "grok-4.5", "handoffMode": "raw",
			"includedTurnCount": 1, "omittedTurnCount": 0, "truncated": false,
		})},
		{ChatID: "cht_a", ChatSeq: 5, LegRunID: "run-2", Type: tuiRecMessageCompleted, Payload: payload(map[string]any{"text": "Chào bạn! Mình là Grok 4.5 do xAI"})},
		{ChatID: "cht_a", ChatSeq: 6, LegRunID: "run-2", Type: tuiRecTurnStarted, Payload: payload(map[string]any{"prompt": "thực sự bạn là ai"})},
		{ChatID: "cht_a", ChatSeq: 7, LegRunID: "run-2", Type: tuiRecMessageCompleted, Payload: payload(map[string]any{"text": "Grok 4.5 do xAI"})},
	}
	m.renderChatTimelineBackfill(chatTimelineBackfillMsg{Records: recs, Current: "run-3", Detached: true})
	joined := ""
	for _, cm := range m.messages {
		joined += cm.Role + ":" + cm.Content + "\n"
	}
	if strings.Contains(joined, "Chào bạn! Mình là Grok") {
		t.Fatalf("seed reply must not backfill, got:\n%s", joined)
	}
	if !strings.Contains(joined, "⇄ switched to grok") {
		t.Fatalf("E-9 divider must backfill, got:\n%s", joined)
	}
	if !strings.Contains(joined, "user:chao") || !strings.Contains(joined, "assistant:muse tra loi") {
		t.Fatalf("leg-1 turns must backfill, got:\n%s", joined)
	}
	if !strings.Contains(joined, "user:thực sự bạn là ai") || !strings.Contains(joined, "assistant:Grok 4.5 do xAI") {
		t.Fatalf("leg-2 real turn must backfill, got:\n%s", joined)
	}
}

func jsonMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}