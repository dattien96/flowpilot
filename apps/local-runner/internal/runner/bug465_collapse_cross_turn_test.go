package runner

import (
	"encoding/json"
	"testing"
)

// BUG-465: collapseRepeatedFinals dedupes the message_completed echo pair
// (EventMessageCompleted + EventTurnCompleted carry the same final text) —
// but its predicate is "same text as the last message_completed on the leg",
// with no turn boundary. A model that answers two turns with identical text
// gets its second answer silently dropped from the rendered timeline.
//
// Live repro: chat cht_10a27db90766, leg run-49042 (codex, post-switch).
// turn-49044 and turn-49051 both ended with the identical canned text
// "Done. The change is implemented and the step is complete." — the raw
// records exist in workflow_chat_events (seqs 11/12), but the timeline read
// collapsed them against the previous turn's identical text, so the user saw
// turn_started with no answer: an apparent silent-lost-response.
func TestCollapseRepeatedFinals_DoesNotCollapseAcrossTurns(t *testing.T) {
	mk := func(leg, typ, text string) ChatTranscriptRecord {
		p, _ := json.Marshal(map[string]any{"text": text})
		return ChatTranscriptRecord{ChatID: "c", LegRunID: leg, Type: typ, Payload: p}
	}
	mkTurn := func(leg string) ChatTranscriptRecord {
		p, _ := json.Marshal(map[string]any{"prompt": "q"})
		return ChatTranscriptRecord{ChatID: "c", LegRunID: leg, Type: EventTypeChatTurnStarted, Payload: p}
	}
	recs := []ChatTranscriptRecord{
		mkTurn("leg-1"),
		mk("leg-1", EventTypeChatMessageCompleted, "Done. The change is implemented and the step is complete."),
		mk("leg-1", EventTypeChatMessageCompleted, "Done. The change is implemented and the step is complete."), // turn_completed echo — collapses
		mkTurn("leg-1"),
		mk("leg-1", EventTypeChatMessageCompleted, "Done. The change is implemented and the step is complete."), // new turn, same text — must NOT collapse
	}
	out := collapseRepeatedFinals(recs)
	got := 0
	for _, r := range out {
		if r.Type == EventTypeChatMessageCompleted {
			got++
		}
	}
	if got != 2 {
		t.Fatalf("message_completed count = %d, want 2 (one per turn); a second turn's identical answer must not be collapsed", got)
	}
}

// Same-turn echo dedup must still hold: message_completed immediately
// followed by the turn_completed echo of identical text collapses.
func TestCollapseRepeatedFinals_StillCollapsesSameTurnEcho(t *testing.T) {
	mk := func(leg, typ, text string) ChatTranscriptRecord {
		p, _ := json.Marshal(map[string]any{"text": text})
		return ChatTranscriptRecord{ChatID: "c", LegRunID: leg, Type: typ, Payload: p}
	}
	recs := []ChatTranscriptRecord{
		mk("leg-1", EventTypeChatMessageCompleted, "answer one"),
		mk("leg-1", EventTypeChatMessageCompleted, "answer one"),
	}
	out := collapseRepeatedFinals(recs)
	if len(out) != 1 {
		t.Fatalf("same-turn echo not collapsed: %d records, want 1", len(out))
	}
}
