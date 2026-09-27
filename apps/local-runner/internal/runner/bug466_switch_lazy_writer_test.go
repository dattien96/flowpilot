package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
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

// BUG-466 coverage gap (CP-58 review 2026-09-26): the lazy-writer fix only
// proved Handoff.Mode/IncludedTurnCount — nothing asserted the seed prompt
// actually reaches the TARGET provider's SendTurn, or that it is the new
// leg's first turn. A healthy-mode response with a dropped seed prompt is
// indistinguishable from a real handoff at the HTTP layer. Pin both: the
// seed TurnRequest must be the target adapter's first request and must carry
// the <previous_conversation> envelope with the transcript marker bytes.
func TestSwitchSeedPromptReachesTargetAdapterAsFirstTurn(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())

	capture := make(chan TurnRequest, 2)
	reg := DefaultProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true, Interrupt: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return &reattachCaptureAdapterWithKey{key: ProviderKeyClaude, ch: capture}
		},
	})
	svc, _ := newTestServerWith(t, reg, nil, nil)
	writer := svc.ensureChatTranscriptWriter()
	if writer == nil {
		t.Fatal("chat transcript writer nil with flag on")
	}

	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	payload := func(m map[string]any) json.RawMessage { b, _ := json.Marshal(m); return b }
	if err := writer.append(context.Background(),
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatTurnStarted, Payload: payload(map[string]any{"prompt": "seed-marker-delta-7 user question"})},
		ChatTranscriptRecord{ChatID: handle.ChatID, LegRunID: handle.RunID, Type: EventTypeChatMessageCompleted, Payload: payload(map[string]any{"text": "seed-marker-delta-7 assistant reply"})},
	); err != nil {
		t.Fatal(err)
	}

	rec := postSwitch(t, svc, handle.ChatID, chatSwitchRequest{TargetProviderKey: ProviderKeyClaude, Model: "claude-sonnet-x"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	select {
	case req := <-capture:
		if !strings.Contains(req.Prompt, "<previous_conversation>") {
			t.Fatalf("seed turn prompt missing <previous_conversation> envelope, got %q", req.Prompt)
		}
		if !strings.Contains(req.Prompt, "seed-marker-delta-7") {
			t.Fatalf("seed turn prompt missing transcript marker bytes, got %q", req.Prompt)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no TurnRequest on the target provider — switch seed never reached SendTurn")
	}
}
