package runner

import "testing"

// BUG-435 (cp46 live evidence): a resumed devin chat replayed one completed
// turn as turn_started → turn_completed → message_completed → turn_completed.
// mergeTurnLogAssistantsIntoTranscript inserted the recovered assistant frame
// AFTER the turn's own terminal (lastOwn+1), and the trailing synthetic-close
// in appendTranscriptReplayEventsOpt then appended a second turn_completed.
// The assistant frame must land BEFORE the turn's terminal event.

func TestBug435_AssistantMergeInsertsBeforeTerminal(t *testing.T) {
	entries := []turnLogLine{
		{Kind: turnLogKindTranscriptTurn, Prompt: "Reply with exactly: ok", Assistant: "ok", TurnID: "turn-40"},
	}
	historical := promptOnlyTurnLogEvents(entries)
	if len(historical) != 2 || historical[0].Type != EventTurnStarted || historical[1].Type != EventTurnCompleted {
		t.Fatalf("prompt-only seed: %v", historical)
	}
	merged := mergeTurnLogAssistantsIntoTranscript(historical, entries)
	want := []ProviderEventType{EventTurnStarted, EventMessageCompleted, EventTurnCompleted}
	if len(merged) != len(want) {
		t.Fatalf("merged = %v events, want %v", merged, want)
	}
	for i, typ := range want {
		if merged[i].Type != typ {
			t.Fatalf("merged[%d] = %s, want %s (all: %+v)", i, merged[i].Type, typ, merged)
		}
	}
	if merged[1].Text != "ok" {
		t.Fatalf("assistant text lost: %+v", merged[1])
	}
}

// Multi-turn: each turn's recovered assistant lands inside ITS OWN turn
// bounds, before its terminal — never after the whole stream.
func TestBug435_AssistantMergeMultiTurnOrdering(t *testing.T) {
	entries := []turnLogLine{
		{Kind: turnLogKindTranscriptTurn, Prompt: "p1", Assistant: "a1", TurnID: "turn-1"},
		{Kind: turnLogKindTranscriptTurn, Prompt: "p2", Assistant: "a2", TurnID: "turn-2"},
	}
	historical := promptOnlyTurnLogEvents(entries)
	merged := mergeTurnLogAssistantsIntoTranscript(historical, entries)
	want := []ProviderEventType{
		EventTurnStarted, EventMessageCompleted, EventTurnCompleted,
		EventTurnStarted, EventMessageCompleted, EventTurnCompleted,
	}
	if len(merged) != len(want) {
		t.Fatalf("merged = %v events, want %v", merged, want)
	}
	for i, typ := range want {
		if merged[i].Type != typ {
			t.Fatalf("merged[%d] = %s, want %s", i, merged[i].Type, typ)
		}
	}
	if merged[1].Text != "a1" || merged[4].Text != "a2" {
		t.Fatalf("assistant texts misplaced: %+v", merged)
	}
	terminals := 0
	for _, e := range merged {
		if e.Type == EventTurnCompleted {
			terminals++
		}
	}
	if terminals != 2 {
		t.Fatalf("expected exactly 2 turn_completed, got %d", terminals)
	}
}
