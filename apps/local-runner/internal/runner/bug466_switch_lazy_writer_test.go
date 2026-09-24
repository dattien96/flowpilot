package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
)

// BUG-466: provider switch silently degrades to fresh_start when the lazy
// transcript writer has not been initialized on this process yet.
//
// Live repro: restarted runner (cht_10a27db90766) — legs restore from
// sessions.ndjson without touching the transcript stack. The first
// transcript-touching call after restart was the switch itself:
// s.chatTranscripts was nil → buildChatHandoffContext returned fresh_start
// with an empty prompt → no seed turn → the new devin leg (run-49071)
// started with ZERO conversation context despite 12 transcript records in
// the store, and the E-9 record durably logged handoffMode=fresh_start /
// includedTurnCount=0. Violates "never silently lose": the operator saw a
// committed switch that dropped all context with no signal.
func TestSwitchInitializesTranscriptWriterBeforeHandoff(t *testing.T) {
	svc, writer := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	payload := func(m map[string]any) json.RawMessage { b, _ := json.Marshal(m); return b }
	if err := writer.append(context.Background(),
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatTurnStarted, Payload: payload(map[string]any{"prompt": "remember marker-alpha-1"})},
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatMessageCompleted, Payload: payload(map[string]any{"text": "marker-alpha-1"})},
	); err != nil {
		t.Fatal(err)
	}
	// Simulate a process restart: durable records and resident legs survive,
	// but the lazily-built transcript stack is gone — exactly the state a
	// restored run leaves behind (restore does not run createRun).
	svc.chatTranscripts = nil
	svc.chatOnce = sync.Once{}

	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude, Model: "claude-sonnet-x"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp chatSwitchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Handoff.Mode == "fresh_start" || resp.Handoff.IncludedTurnCount == 0 {
		t.Fatalf("handoff silently degraded: %+v — transcript records exist but the switch never initialized the writer", resp.Handoff)
	}
}
