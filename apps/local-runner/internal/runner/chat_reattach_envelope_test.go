package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

type reattachCaptureAdapter struct {
	ch chan TurnRequest
}

func (a *reattachCaptureAdapter) Key() ProviderKey { return ProviderKeyOpencode }
func (a *reattachCaptureAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *reattachCaptureAdapter) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	select {
	case a.ch <- req:
	default:
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
	return nil
}

// TestReattachFirstTurnIncludesPriorHistory verifies the reattach fix for
// run-405014 -> 401898: after /open, the first turn on the reattached leg
// must carry the prior chat history (Nam), not just workspace context.
// Provider-agnostic: envelope is built from transcript (chatId only), not per-provider.
func TestReattachFirstTurnIncludesPriorHistory(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	svc, _ := newTestServer(t)
	// Capture adapter for opencode
	capture := &reattachCaptureAdapter{ch: make(chan TurnRequest, 2)}
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:   ProviderKeyOpencode,
		Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter { return capture },
	})
	svc.registry = reg

	// Create initial chat leg
	h1, err := svc.createRun(StartRunInput{
		ProjectID: "proj-reattach", ChatMode: "normal_chat",
		ProviderKey: ProviderKeyOpencode, Model: "opencode-go/longcat-2.0",
	})
	if err != nil {
		t.Fatalf("createRun h1: %v", err)
	}
	// Seed transcript with history that should be remembered after reattach
	writer := svc.ensureChatTranscriptWriter()
	if writer == nil {
		t.Fatal("no transcript writer")
	}
	records := []ChatTranscriptRecord{
		{ChatID: h1.ChatID, ChatSeq: 1, LegRunID: h1.RunID, Type: EventTypeChatTurnStarted, Payload: jsonRaw(`{"prompt":"Toi la Nam, 15 tuoi, nho nhe"}`)},
		{ChatID: h1.ChatID, ChatSeq: 2, LegRunID: h1.RunID, Type: EventTypeChatMessageCompleted, Payload: jsonRaw(`{"text":"Ok Nam, nho roi nhe"}`)},
		{ChatID: h1.ChatID, ChatSeq: 3, LegRunID: h1.RunID, Type: EventTypeChatTurnStarted, Payload: jsonRaw(`{"prompt":"The bay gio ban la model gi"}`)},
		{ChatID: h1.ChatID, ChatSeq: 4, LegRunID: h1.RunID, Type: EventTypeChatMessageCompleted, Payload: jsonRaw(`{"text":"Minh la longcat-2.0"}`)},
	}
	for _, r := range records {
		if err := writer.append(context.Background(), r); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	// Simulate restart: new leg via reattach (ChatID+SwitchFromRunID)
	h2, err := svc.createRun(StartRunInput{
		ProjectID: "proj-reattach", ChatMode: "normal_chat",
		ProviderKey: ProviderKeyOpencode, ChatID: h1.ChatID, SwitchFromRunID: h1.RunID,
	})
	if err != nil {
		t.Fatalf("createRun h2 reattach: %v", err)
	}
	if h2.ChatID != h1.ChatID || h2.LegSeq != 1 {
		t.Fatalf("reattach identity: chatId=%q legSeq=%d want %q 1", h2.ChatID, h2.LegSeq, h1.ChatID)
	}
	// First turn on reattached leg should carry history
	turnID := "turn-reattach-1"
	in := TurnInput{StepID: h2.StepID, Prompt: "ban biet gi ve toi"}
	// startTurn will go through runTurn and prepend envelope
	if _, apiErr := svc.startTurn(h2.RunID, in, "", turnID); apiErr != nil {
		t.Fatalf("startTurn reattach first: %v", apiErr)
	}
	select {
	case req := <-capture.ch:
		// The adapter should receive a prompt that contains the prior history (Nam)
		// and the current prompt, not just workspace context.
		// runTurn builds providerPrompt = envelope + "\n\n" + userPrompt
		if !strings.Contains(req.Prompt, "Nam") {
			t.Fatalf("first reattach turn must contain prior history Nam, got prompt=%q", req.Prompt)
		}
		if !strings.Contains(req.Prompt, "ban biet gi ve toi") {
			t.Fatalf("must contain current prompt, got %q", req.Prompt)
		}
		if !strings.Contains(req.Prompt, "<previous_conversation>") {
			t.Fatalf("must contain handoff envelope, got %q", req.Prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no TurnRequest captured for reattach first turn (timeout)")
	}
	// Wait for first turn to fully complete (turnInFlight cleared) before second
	time.Sleep(400 * time.Millisecond)
	// Second turn on same reattached leg must NOT re-add history (only first)
	turnID2 := "turn-reattach-2"
	in2 := TurnInput{StepID: h2.StepID, Prompt: "cau thu 2"}
	if _, apiErr := svc.startTurn(h2.RunID, in2, "", turnID2); apiErr != nil {
		t.Fatalf("startTurn second: %v", apiErr)
	}
	select {
	case req2 := <-capture.ch:
		if strings.Contains(req2.Prompt, "<previous_conversation>") {
			t.Fatalf("second turn must not re-add envelope, got %q", req2.Prompt)
		}
		if !strings.Contains(req2.Prompt, "cau thu 2") {
			t.Fatalf("second prompt missing, got %q", req2.Prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no second TurnRequest (timeout)")
	}
}

func jsonRaw(s string) []byte { return []byte(s) }

// Table for provider-agnostic proof: same logic for grok/codex/claude
func TestReattachEnvelopeProviderAgnostic(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyOpencode, ProviderKeyGrok, ProviderKeyCodex, ProviderKeyClaude} {
		t.Run(string(pk), func(t *testing.T) {
			t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
			svc, _ := newTestServer(t)
			capture := &reattachCaptureAdapter{ch: make(chan TurnRequest, 1)}
			// Override to use captured provider key
			captureAdapter := &reattachCaptureAdapter{ch: capture.ch}
			// Hack: override Key() via embedding
			reg := newProviderRegistry()
			reg.register(ProviderRegistration{
				Key: pk, Status: ProviderStatusAvailable,
				Capabilities: ProviderCapabilities{Streaming: true},
				newAdapter: func() ProviderRuntimeAdapter {
					return &reattachCaptureAdapterWithKey{key: pk, ch: capture.ch}
				},
			})
			_ = captureAdapter
			svc.registry = reg
			h1, err := svc.createRun(StartRunInput{ProjectID: "proj-" + string(pk), ChatMode: "normal_chat", ProviderKey: pk})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			writer := svc.ensureChatTranscriptWriter()
			if writer == nil {
				t.Fatal("no writer")
			}
			_ = writer.append(context.Background(), ChatTranscriptRecord{ChatID: h1.ChatID, ChatSeq: 1, LegRunID: h1.RunID, Type: EventTypeChatTurnStarted, Payload: jsonRaw(`{"prompt":"history turn"}`)})
			_ = writer.append(context.Background(), ChatTranscriptRecord{ChatID: h1.ChatID, ChatSeq: 2, LegRunID: h1.RunID, Type: EventTypeChatMessageCompleted, Payload: jsonRaw(`{"text":"reply"}`)})
			h2, err := svc.createRun(StartRunInput{ProjectID: "proj-" + string(pk), ChatMode: "normal_chat", ProviderKey: pk, ChatID: h1.ChatID, SwitchFromRunID: h1.RunID})
			if err != nil {
				t.Fatalf("reattach: %v", err)
			}
			if h2.ChatID != h1.ChatID {
				t.Fatalf("chatId mismatch")
			}
			// First turn should get envelope
			if _, apiErr := svc.startTurn(h2.RunID, TurnInput{StepID: h2.StepID, Prompt: "next"}, "", "turn-next"); apiErr != nil {
				t.Fatalf("startTurn: %v", apiErr)
			}
			select {
			case req := <-capture.ch:
				if !strings.Contains(req.Prompt, "history turn") {
					t.Fatalf("%s: expected history in envelope, got %q", pk, req.Prompt)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("no capture (timeout)")
			}
		})
	}
}

type reattachCaptureAdapterWithKey struct {
	key ProviderKey
	ch  chan TurnRequest
}

func (a *reattachCaptureAdapterWithKey) Key() ProviderKey { return a.key }
func (a *reattachCaptureAdapterWithKey) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *reattachCaptureAdapterWithKey) SendTurn(_ context.Context, req TurnRequest, bridge TurnBridge) error {
	select {
	case a.ch <- req:
	default:
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
	return nil
}
