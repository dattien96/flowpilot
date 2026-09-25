package app

import (
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-450: an orchStreamOpenedMsg produced by a cmdStartOrchestrationStream
// cmd issued BEFORE a provider switch must not attach when it lands AFTER the
// switch — it would cancel the new leg's stream (stopOrchestrationStream on
// whatever is current) and pin the dead leg's channels. The message must
// carry the run it was opened for and Update must drop+cancel mismatches.
func TestBug450_StaleOrchOpenDoesNotDisplaceNewLegStream(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-old", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "grok"
	m.model = "grok-4.5"

	// Issue the open cmd while the OLD leg is current — this is the in-flight
	// window the bug lives in.
	cmd := m.cmdStartOrchestrationStream()
	if cmd == nil {
		t.Fatal("cmdStartOrchestrationStream must return a cmd while no stream is attached")
	}
	msg := cmd() // stream for run-old now exists in flight
	opened, ok := msg.(orchStreamOpenedMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want orchStreamOpenedMsg", msg)
	}

	// Provider switch lands BEFORE the open message is delivered.
	m.applyChatSwitched(ChatSwitchedMsg{Resp: &client.ChatSwitchResponse{
		Handle: client.RunHandle{RunID: "run-new", RunKind: "chat", ChatID: "cht_a", ProviderKey: "codex"},
		Model:  "gpt-5.4-mini",
	}})
	if m.runHandle == nil || m.runHandle.RunID != "run-new" {
		t.Fatalf("switch did not adopt run-new: %+v", m.runHandle)
	}

	// The new leg's stream is now attached and must survive.
	newCancelled := false
	newCh := make(chan client.ProviderEvent)
	newSt := &orchStreamState{evCh: newCh, cancel: func() { newCancelled = true }}
	m.orchStream = newSt

	// Deliver the STALE open (run-old). It must be dropped + cancelled, not
	// tear down the new leg's stream and attach the dead one.
	m.Update(opened)

	if newCancelled {
		t.Fatal("stale orchStreamOpenedMsg cancelled the NEW leg's live stream")
	}
	if m.orchStream != newSt {
		t.Fatalf("stale open displaced the current stream: orchStream=%p want %p", m.orchStream, newSt)
	}
	if m.orchStream.evCh == opened.EvCh {
		t.Fatal("stale run-old channel attached as the live orchestration stream")
	}
}

// Positive control: an open issued for the CURRENT run must still attach.
func TestBug450_CurrentRunOrchOpenAttaches(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-cur", RunKind: "chat", ChatID: "cht_a"}

	cmd := m.cmdStartOrchestrationStream()
	if cmd == nil {
		t.Fatal("cmd nil")
	}
	msg := cmd()
	m.Update(msg)

	if m.orchStream == nil {
		t.Fatal("current-run open must attach the orchestration stream")
	}
}

// BUG-452: when the switch response carries no model (legacy/edge runner),
// the requested target model is a stronger claim than catalog index 0 —
// catalog order is not the session's resolved model.
func TestBug452_EmptyResponsePrefersRequestedOverCatalogOrder(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.model = "opencode/muse-spark-1.2-contributor-free"
	m.providers = []client.Provider{
		{Key: "devin", Models: []client.ProviderModel{
			{ID: "devin/swe-2-high"}, // catalog[0] — must NOT win over the request
			{ID: "devin/swe-2"},
		}},
	}

	m.applyChatSwitched(ChatSwitchedMsg{
		Resp: &client.ChatSwitchResponse{
			Handle: client.RunHandle{RunID: "run-2", RunKind: "chat", ChatID: "cht_a", ProviderKey: "devin"},
			Model:  "", // legacy empty response
		},
		TargetProvider: "devin",
		TargetModel:    "devin/swe-2",
	})

	if m.model != "devin/swe-2" {
		t.Fatalf("requested model must win over catalog[0], got %q", m.model)
	}
}
