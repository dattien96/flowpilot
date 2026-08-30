package app

// Chat switch surface tests (Task-315 slice 1): routing rules, prefix parity
// with the runner constant, the addMessage divider collapse, and the client
// switch round-trip. New file — additive only.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestHandoffPromptPrefixParity(t *testing.T) {
	if client.HandoffPromptPrefix != "[FlowPilot cross-provider chat handoff]" {
		t.Fatalf("TUI prefix drifted: %q", client.HandoffPromptPrefix)
	}
}

func TestRouteProviderSwitchRules(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	// No run → legacy path (nil command).
	if cmd := m.routeProviderSwitch("grok", ""); cmd != nil {
		t.Fatal("no-run must not route")
	}
	// Workflow run → legacy path.
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "workflow", ChatID: ""}
	if cmd := m.routeProviderSwitch("grok", ""); cmd != nil {
		t.Fatal("workflow run must not route")
	}
	// Chat run + foreign provider → routed.
	m.runHandle = &client.RunHandle{RunID: "run-2", RunKind: "chat", ChatID: "cht_a"}
	if cmd := m.routeProviderSwitch("grok", "grok-4.5"); cmd == nil {
		t.Fatal("chat foreign-provider must route")
	}
	if !m.chatSwitchInFlight {
		t.Fatal("in-flight guard not raised by routing")
	}
	// Same provider → in-place (no routing).
	m.chatSwitchInFlight = false
	m.provider = "codex"
	if cmd := m.routeProviderSwitch("codex", ""); cmd != nil {
		t.Fatal("same provider must stay in-place")
	}
	// In-flight → dropped (single-leg guarantee, CS-05).
	m.chatSwitchInFlight = true
	if cmd := m.routeProviderSwitch("grok", ""); cmd != nil {
		t.Fatal("in-flight switch must drop re-routes")
	}
}

func TestAdoptKeepsTranscriptAndResetsStreamState(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.addMessage("user", "hello ban la model gi", "")
	m.addMessage("assistant", "toi la Muse Spark", "")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a", ProviderKey: "codex", LastEventSeq: 4}
	m.provider = "codex"
	before := len(m.messages)

	m.applyChatSwitched(ChatSwitchedMsg{Resp: &client.ChatSwitchResponse{
		Handle: client.RunHandle{RunID: "run-2", RunKind: "chat", ChatID: "cht_a", ProviderKey: "grok", LastEventSeq: 0},
		ChatID: "cht_a", LegSeq: 1, Model: "grok-4.5",
		Handoff: client.ChatSwitchHandoffStats{HandoffMode: "raw", IncludedTurnCount: 1},
	}})
	// Success keeps the transcript byte-identical: no client-synthesized
	// divider (SD-26 D-7 single-source — the seed turn carries it).
	if len(m.messages) != before {
		t.Fatalf("success added a client divider: %d → %d", before, len(m.messages))
	}
	if m.runHandle.RunID != "run-2" || m.provider != "grok" || m.model != "grok-4.5" {
		t.Fatalf("adopt state = run=%s provider=%s model=%s", m.runHandle.RunID, m.provider, m.model)
	}
	if m.chatSwitchInFlight {
		t.Fatal("in-flight guard not cleared")
	}

	// Failure keeps the chat on the source leg and appends one error line.
	failed := len(m.messages)
	m.applyChatSwitched(ChatSwitchedMsg{Err: context.DeadlineExceeded})
	if m.runHandle.RunID != "run-2" || m.provider != "grok" {
		t.Fatalf("failure mutated adoption state: %s/%s", m.runHandle.RunID, m.provider)
	}
	if len(m.messages) != failed+1 || !strings.Contains(m.messages[len(m.messages)-1].Content, "Provider switch failed") {
		t.Fatalf("no failure line: %+v", m.messages[len(m.messages)-1])
	}
}

func TestAddMessageCollapsesHandoffEnvelope(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.addMessage("user", client.HandoffPromptPrefix+"\n\n<previous_conversation>huge blob</previous_conversation>", "")
	last := m.messages[len(m.messages)-1]
	if last.Role != "system" || strings.Contains(last.Content, "previous_conversation") {
		t.Fatalf("seed envelope rendered raw: %+v", last)
	}
}

func TestClientSwitchChatProviderRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/chats/cht_a/switch-provider" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var in client.ChatSwitchInput
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.TargetProviderKey != "grok" || in.Model != "grok-4.5" {
			t.Errorf("input = %+v", in)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"handle": map[string]any{"runId": "run-9", "providerKey": "grok", "chatId": "cht_a", "legSeq": 1},
			"chatId": "cht_a", "legSeq": 1, "model": "grok-4.5",
			"handoff": map[string]any{"handoffMode": "raw", "includedTurnCount": 2},
		})
	}))
	defer srv.Close()
	cl := client.New(srv.URL)
	resp, err := cl.SwitchChatProvider(context.Background(), "cht_a", client.ChatSwitchInput{TargetProviderKey: "grok", Model: "grok-4.5"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Handle.RunID != "run-9" || resp.Handle.ChatID != "cht_a" || resp.LegSeq != 1 || resp.Model != "grok-4.5" || resp.Handoff.HandoffMode != "raw" {
		t.Fatalf("round-trip = %+v", resp)
	}
}

func TestTabCrossProviderRoutesToSwitch(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{
		"plan": {Provider: "grok", Model: "grok-4.5"},
	}}
	if cmd := m.routePostureSwitch(cfg, "plan"); cmd == nil {
		t.Fatal("cross-provider Tab must route to the switch endpoint")
	}
	if !m.chatSwitchInFlight || m.chatSwitchQueuedPosture != "plan" {
		t.Fatalf("guard/queue = %v/%q", m.chatSwitchInFlight, m.chatSwitchQueuedPosture)
	}
	// Same-provider pin stays in-place.
	m.chatSwitchInFlight = false
	m.chatSwitchQueuedPosture = ""
	m.provider = "opencode"
	cfg.Profiles["plan"] = client.ChatPostureProfile{Provider: "opencode", Model: "opencode-go/deepseek-v4-flash"}
	if cmd := m.routePostureSwitch(cfg, "plan"); cmd != nil {
		t.Fatal("same-provider Tab must apply in place")
	}
}

func TestBareModelPinDerivesProviderAndPersists(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.providers = []client.Provider{{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.5"}}}}
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{
		"plan": {Model: "grok-4.5"}, // BUG-330 pin: bare model, no provider
	}}
	if m.pinnedProviderFor(cfg.Profiles["plan"]) != "grok" {
		t.Fatalf("pinnedProviderFor = %q, want grok", m.pinnedProviderFor(cfg.Profiles["plan"]))
	}
	if cmd := m.routePostureSwitch(cfg, "plan"); cmd == nil {
		t.Fatal("bare-model Tab must route")
	}
	if cfg.Profiles["plan"].Provider != "grok" || !m.chatPostureDirty {
		t.Fatalf("derive-once not persisted: provider=%q dirty=%v", cfg.Profiles["plan"].Provider, m.chatPostureDirty)
	}
}
