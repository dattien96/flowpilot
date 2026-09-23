package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

// BUG-452 (runner half): switchChatProvider must return the new leg's
// RESOLVED model, not echo the request's (possibly empty) model. The TUI
// displays resp.Model — echoing "" pushed it onto a catalog-index-0 guess
// that can differ from the session's actual model.
func TestBug452_SwitchResponseReturnsResolvedLegModel(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	resp, apiErr := svc.switchChatProvider(context.Background(), handle.ChatID, chatSwitchRequest{
		TargetProviderKey: ProviderKeyClaude,
		// Model intentionally empty — the leg resolves the provider default.
	})
	if apiErr != nil {
		t.Fatalf("switch: %v", apiErr)
	}
	want := defaultModelForProvider(ProviderKeyClaude)
	if want == "" {
		t.Fatal("no default model for claude — fixture broken")
	}
	if resp.Model != want {
		t.Fatalf("switch response Model = %q, want the leg's resolved model %q (echoing the empty request forces the client to guess)", resp.Model, want)
	}
}

// Positive control: an explicitly requested model still round-trips.
func TestBug452_SwitchResponseExplicitModelRoundTrips(t *testing.T) {
	svc, _ := newSwitchTestService(t)
	handle, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	resp, apiErr := svc.switchChatProvider(context.Background(), handle.ChatID, chatSwitchRequest{
		TargetProviderKey: ProviderKeyClaude,
		Model:             "sonnet-explicit",
	})
	if apiErr != nil {
		t.Fatalf("switch: %v", apiErr)
	}
	if resp.Model != "sonnet-explicit" {
		t.Fatalf("explicit request Model must echo, got %q", resp.Model)
	}
}

// BUG-451: a resumed Devin session whose session/load result carries no
// model currentValue, followed by a REJECTED model set_config_option, must
// not persist the requested id as if it ran. The applied model is genuinely
// unknown → the record carries a typed unknown marker, not a lie.
func TestBug451_ResumedSessionRejectedModelRecordsUnknown(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/load":
			// Config-only result: sessionId echoed, configOptions carry NO
			// model currentValue — the true session model is unobservable.
			fd.reply(m["id"], map[string]any{
				"sessionId":     "s-resume-1",
				"configOptions": []any{map[string]any{"id": "mode", "currentValue": "smart"}},
			})
		case "session/set_config_option":
			params, _ := m["params"].(map[string]any)
			if params["configId"] == "model" {
				fd.send(map[string]any{"jsonrpc": "2.0", "id": m["id"], "error": map[string]any{
					"code": -32602, "message": "Invalid value 'swe-2-max' for config option 'model'",
				}})
			} else {
				fd.reply(m["id"], map[string]any{"configOptions": []any{}})
			}
		case "session/prompt":
			go func() {
				fd.notify("session/update", map[string]any{"sessionId": "s-resume-1", "update": map[string]any{
					"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "ok"},
				}})
				time.Sleep(10 * time.Millisecond)
				fd.reply(m["id"], map[string]any{"stopReason": "end_turn"})
			}()
		}
	})

	a := newDevinAdapter(d, "/tmp")
	store := &recordingSessionStore{}
	a.sessionStore = store
	bridge := &fakeDevinBridge{}
	req := TurnRequest{
		RunID: "run-r1", Prompt: "hi", Cwd: "/tmp",
		ModelName:         "devin/swe-2-max",
		ProviderSessionID: "s-resume-1", // resume path → session/load
	}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	var lastModel string
	for _, rec := range store.records {
		if rec.ProviderSessionID == "s-resume-1" {
			lastModel = rec.ModelName
		}
	}
	if lastModel == "devin/swe-2-max" {
		t.Fatalf("record persisted the REJECTED requested model %q — applied model was never observed", lastModel)
	}
	if !strings.Contains(lastModel, "unknown") {
		t.Fatalf("record must carry a typed unknown marker when the applied model is unobservable, got %q", lastModel)
	}
}
