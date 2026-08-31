package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Regression for run-197604 (TUI): createRun now echoes RunKind+ChatID.
// Legacy handle with ChatID but no RunKind must still route (flag flag).
func TestModelForeignEmptyRunKindStillRoutes(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.providers = []client.Provider{
		{Key: "opencode", Models: []client.ProviderModel{{ID: "opencode-go/muse-spark-1.2-contributor"}}},
		{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.6"}}},
	}
	// Simulate legacy handle: ChatID present, RunKind empty (createRun before fix).
	m.runHandle = &client.RunHandle{RunID: "run-197604", RunKind: "", ChatID: "cht_test", ProviderKey: "opencode"}
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark-1.2-contributor"

	_, cmd := m.handleSlashCommand("/model grok-4.6")
	if cmd == nil || !m.chatSwitchInFlight {
		t.Fatalf("/model grok-4.6 must route even when RunKind empty but ChatID present, cmd=%v inFlight=%v", cmd, m.chatSwitchInFlight)
	}
	if m.provider != "opencode" {
		t.Fatalf("provider must stay opencode until switch completes, got %q", m.provider)
	}
	// Must not have emitted "next prompt uses this model" (in-place fallback).
	for _, msg := range m.messages {
		if strings.Contains(msg.Content, "next prompt uses this model") {
			t.Fatalf("must not emit in-place message when routed, got %q", msg.Content)
		}
	}
}

// Gap: handle with ChatID but RunKind empty for /provider must also route.
func TestProviderEmptyRunKindStillRoutes(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.providers = []client.Provider{
		{Key: "opencode", Models: []client.ProviderModel{{ID: "opencode-go/muse-spark"}}},
		{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.6"}}},
	}
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "", ChatID: "cht_a", ProviderKey: "opencode"}
	m.provider = "opencode"
	_, cmd := m.handleSlashCommand("/provider grok")
	if cmd == nil || !m.chatSwitchInFlight {
		t.Fatalf("/provider grok must route with ChatID even when RunKind empty")
	}
}

// Unroutable live chat (missing ChatID and RunKind) must NOT inject foreign provider.
func TestModelForeignLiveChatWithoutIdentityDoesNotInject(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.providers = []client.Provider{
		{Key: "opencode", Models: []client.ProviderModel{{ID: "opencode-go/muse-spark"}}},
		{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.6"}}},
		{Key: "claude", Models: []client.ProviderModel{{ID: "claude-sonnet"}}},
		{Key: "codex", Models: []client.ProviderModel{{ID: "codex-5.1"}}},
	}
	// Live run but neither ChatID nor RunKind — simulates a handle before the echo fix.
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "", ChatID: "", ProviderKey: "opencode"}
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark"

	_, cmd := m.handleSlashCommand("/model grok-4.6")
	if cmd != nil {
		t.Fatalf("unroutable live chat must not route, cmd=%v", cmd)
	}
	if m.provider != "opencode" {
		t.Fatalf("must not inject foreign provider on unroutable live chat, got %q", m.provider)
	}
	if !strings.Contains(m.messages[len(m.messages)-1].Content, "Cannot switch") {
		t.Fatalf("must show Cannot switch error, got %q", m.messages[len(m.messages)-1].Content)
	}
	// Same for claude and codex targets (R2 parity).
	for _, target := range []string{"claude-sonnet", "codex-5.1"} {
		m2 := New(config.ChatConfig{}, "http://127.0.0.1:1")
		m2.providers = m.providers
		m2.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "", ChatID: "", ProviderKey: "opencode"}
		m2.provider = "opencode"
		m2.model = "opencode-go/muse-spark"
		_, cmd2 := m2.handleSlashCommand("/model " + target)
		if cmd2 != nil || m2.provider != "opencode" {
			t.Fatalf("target %s: must not inject, cmd=%v provider=%q", target, cmd2, m2.provider)
		}
	}
}

// No live run: foreign /model may set provider locally for the next chat.
func TestModelForeignNoRunSetsProviderLocally(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.providers = []client.Provider{
		{Key: "opencode", Models: []client.ProviderModel{{ID: "opencode-go/muse-spark"}}},
		{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.6"}}},
	}
	m.runHandle = nil
	m.provider = "opencode"
	_, _ = m.handleSlashCommand("/model grok-4.6")
	if m.chatSwitchInFlight {
		t.Fatal("no-run must not set in-flight")
	}
	if m.provider != "grok" || m.model != "grok-4.6" {
		t.Fatalf("no-run foreign /model must set provider/model locally, got %q/%q", m.provider, m.model)
	}
}

// Helper parity: isChatHandle.
func TestIsChatHandle(t *testing.T) {
	if !isChatHandle(&client.RunHandle{RunKind: "chat"}) {
		t.Fatal("RunKind chat must be chat")
	}
	if !isChatHandle(&client.RunHandle{RunKind: "", ChatID: "cht_a"}) {
		t.Fatal("ChatID present must be chat even when RunKind empty")
	}
	if isChatHandle(&client.RunHandle{RunKind: "workflow"}) {
		t.Fatal("workflow must not be chat")
	}
	if isChatHandle(&client.RunHandle{RunKind: "", ChatID: ""}) {
		t.Fatal("empty must not be chat")
	}
	if isChatHandle(nil) {
		t.Fatal("nil must not be chat")
	}
}
