package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

// BUG-379/433: a requested devin model id absent from the live catalog gets
// `session/set_config_option` → Invalid params. The rejection must surface as
// a user-visible note and the session record must carry the APPLIED model,
// not the rejected id.

func TestDevinInvalidModelEmitsWarningAndRecordsAppliedModel(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fd.reply(m["id"], map[string]any{
				"sessionId": "s-model-fallback",
				"configOptions": []any{map[string]any{
					"id": "model", "currentValue": "swe-2-high",
					"options": []any{map[string]any{"value": "swe-2-high"}},
				}},
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
				fd.notify("session/update", map[string]any{"sessionId": "s-model-fallback", "update": map[string]any{
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
	req := TurnRequest{RunID: "run-m1", Prompt: "hi", Cwd: "/tmp", ModelName: "devin/swe-2-max"}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	var warned bool
	for _, ev := range bridge.events {
		if ev.Type == EventMessageDelta && strings.Contains(ev.Text, "swe-2-max") && strings.Contains(ev.Text, "[model]") {
			warned = true
		}
	}
	if !warned {
		t.Fatal("rejected model id must surface a [model] warning event")
	}

	var lastModel string
	for _, rec := range store.records {
		if rec.ProviderSessionID == "s-model-fallback" {
			lastModel = rec.ModelName
		}
	}
	if lastModel != "devin/swe-2-high" {
		t.Fatalf("session record must carry the applied model, got %q", lastModel)
	}
}

// Near-miss: an accepted set_config_option records the requested id (prefixed)
// and emits no warning.
func TestDevinAcceptedModelRecordsRequestedNoWarning(t *testing.T) {
	d, fd := startFakeDevin(t, nil)
	fd.serve(func(fd *fakeDevin, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fd.reply(m["id"], map[string]any{
				"sessionId": "s-model-ok",
				"configOptions": []any{map[string]any{
					"id": "model", "currentValue": "swe-2-high",
					"options": []any{map[string]any{"value": "swe-2-high"}, map[string]any{"value": "swe-2-low"}},
				}},
			})
		case "session/set_config_option":
			params, _ := m["params"].(map[string]any)
			if params["configId"] == "model" {
				fd.reply(m["id"], map[string]any{"configOptions": []any{map[string]any{"id": "model", "currentValue": "swe-2-low"}}})
			} else {
				fd.reply(m["id"], map[string]any{"configOptions": []any{}})
			}
		case "session/prompt":
			go func() {
				fd.notify("session/update", map[string]any{"sessionId": "s-model-ok", "update": map[string]any{
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
	req := TurnRequest{RunID: "run-m2", Prompt: "hi", Cwd: "/tmp", ModelName: "devin/swe-2-low"}
	if err := a.SendTurn(context.Background(), req, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	for _, ev := range bridge.events {
		if ev.Type == EventMessageDelta && strings.Contains(ev.Text, "[model]") {
			t.Fatalf("accepted model must not warn: %q", ev.Text)
		}
	}
	var lastModel string
	for _, rec := range store.records {
		if rec.ProviderSessionID == "s-model-ok" {
			lastModel = rec.ModelName
		}
	}
	if lastModel != "devin/swe-2-low" {
		t.Fatalf("session record must carry the applied model, got %q", lastModel)
	}
}
