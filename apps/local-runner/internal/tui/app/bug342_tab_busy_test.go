package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-342: Tab scan( plan → code cycle ) leaked to in-place apply while
// turnLive/inFlight, causing 409 + session desync (417944→417970). C2 must
// block Tab while busy and never apply in-place during inFlight.

// While a turn is live, cross-provider Tab must be blocked (C2), not routed.
func TestTabWhileTurnLiveBlocked(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-417944", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark-1.2-contributor"
	m.connStatus = ConnRunning
	m.turnLive = true
	m.providers = []client.Provider{
		{Key: "opencode", Models: []client.ProviderModel{{ID: "opencode-go/muse-spark-1.2-contributor"}, {ID: "opencode-go/deepseek"}}},
		{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.5"}}},
	}
	cfg := client.ChatPostureConfig{
		Active: "scan",
		Profiles: map[string]client.ChatPostureProfile{
			"scan": {Provider: "opencode", Model: "opencode-go/muse-spark-1.2-contributor"},
			"plan": {Provider: "grok", Model: "grok-4.5"},
		},
	}
	cmd := m.routePostureSwitch(cfg, "plan")
	if cmd == nil {
		t.Fatal("C2 busy must return a notice cmd, not nil")
	}
	if m.chatSwitchInFlight {
		t.Fatal("blocked Tab must not set inFlight")
	}
	if m.provider != "opencode" {
		t.Fatalf("provider must not change while busy, got %q", m.provider)
	}
	// The cmd must produce a C2 notice, not a switch.
	msg := cmd()
	if _, ok := msg.(detachedNoticeMsg); !ok {
		t.Fatalf("C2 cmd must be detachedNoticeMsg, got %T", msg)
	}
	found := false
	for _, mm := range m.messages {
		if strings.Contains(mm.Content, "Cannot switch provider/model while a question or approval is pending") {
			found = true
		}
	}
	if !found {
		t.Fatalf("C2 notice missing, messages=%+v", m.messages)
	}
}

// While a switch is in flight, a pending Tab apply must not leak to in-place.
func TestTabPendingWhileInFlightDoesNotApplyInPlace(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark"
	m.chatSwitchInFlight = true
	m.chatPosturePending = "apply:plan"
	cfg := client.ChatPostureConfig{
		Active: "scan",
		Profiles: map[string]client.ChatPostureProfile{
			"plan": {Provider: "grok", Model: "grok-4.5"},
			"code": {Provider: "opencode", Model: "opencode-go/deepseek"},
		},
	}
	// Simulate the Update path that loads posture then calls chatPostureCmdFromPending
	cmd := m.chatPostureCmdFromPending(cfg)
	if cmd == nil {
		t.Fatal("inFlight pending apply must return C2 notice, not nil")
	}
	msg := cmd()
	if _, ok := msg.(detachedNoticeMsg); !ok {
		t.Fatalf("must be C2 notice, got %T", msg)
	}
	if m.provider != "opencode" || m.model != "opencode-go/muse-spark" {
		t.Fatalf("inFlight must not apply in-place, got %s/%s", m.provider, m.model)
	}
	if m.chatPosture == "plan" {
		t.Fatal("posture must not change while inFlight")
	}
}

// Double Tab while inFlight must not mint a second leg.
func TestDoubleTabWhileInFlightOnlyOneLeg(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.providers = []client.Provider{
		{Key: "opencode", Models: []client.ProviderModel{{ID: "opencode-go/muse-spark"}, {ID: "opencode-go/deepseek"}}},
		{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.5"}}},
		{Key: "codex", Models: []client.ProviderModel{{ID: "codex-model"}}},
	}
	cfg := client.ChatPostureConfig{
		Active: "scan",
		Profiles: map[string]client.ChatPostureProfile{
			"plan": {Provider: "grok", Model: "grok-4.5"},
			"code": {Provider: "codex", Model: "codex-model"},
		},
	}
	// First Tab: scan -> plan should route
	if cmd := m.routePostureSwitch(cfg, "plan"); cmd == nil {
		t.Fatal("first Tab must route")
	}
	if !m.chatSwitchInFlight {
		t.Fatal("first Tab must set inFlight")
	}
	// Second Tab while inFlight: must not route (would mint second leg)
	if cmd2 := m.routePostureSwitch(cfg, "code"); cmd2 != nil {
		// The early guard returns nil for inFlight, so second must be nil.
		// If it returned a C2 notice, that's also acceptable as blocked, but
		// it must not be a switch.
		t.Fatalf("second Tab while inFlight must not route a switch, got %v", cmd2)
	}
}

// Same-provider Tab while busy must also be blocked (no in-place leak).
func TestSameProviderTabWhileTurnLiveBlocked(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark"
	m.turnLive = true
	m.chatPosturePending = "apply:code"
	cfg := client.ChatPostureConfig{
		Active: "scan",
		Profiles: map[string]client.ChatPostureProfile{
			"code": {Provider: "opencode", Model: "opencode-go/deepseek"},
		},
	}
	cmd := m.chatPostureCmdFromPending(cfg)
	if cmd == nil {
		t.Fatal("same-provider while live must be blocked with C2 notice")
	}
	if m.model != "opencode-go/muse-spark" {
		t.Fatalf("same-provider blocked must not change model, got %q", m.model)
	}
}

// Provider-agnostic: C2 busy guard works for any target provider.
func TestTabBusyIsProviderAgnostic(t *testing.T) {
	for _, target := range []string{"grok", "codex", "claude", "opencode"} {
		t.Run(target, func(t *testing.T) {
			m := New(config.ChatConfig{}, "http://127.0.0.1:1")
			m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
			// Ensure source provider differs from target so it would route if not busy.
			m.provider = "opencode"
			if target == "opencode" {
				m.provider = "codex"
			}
			m.turnLive = true
			m.providers = []client.Provider{
				{Key: target, Models: []client.ProviderModel{{ID: target + "-model"}}},
				{Key: m.provider, Models: []client.ProviderModel{{ID: "src-model"}}},
			}
			cfg := client.ChatPostureConfig{
				Active: "scan",
				Profiles: map[string]client.ChatPostureProfile{
					"plan": {Provider: target, Model: target + "-model"},
				},
			}
			cmd := m.routePostureSwitch(cfg, "plan")
			if cmd == nil {
				t.Fatalf("%s: busy must block with C2 notice", target)
			}
			if m.chatSwitchInFlight {
				t.Fatalf("%s: must not set inFlight while busy", target)
			}
		})
	}
}

// After the turn completes, Tab must work again.
func TestTabAfterTurnCompletedWorks(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "opencode"
	m.turnLive = true
	m.providers = []client.Provider{
		{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.5"}}},
	}
	cfg := client.ChatPostureConfig{
		Active: "scan",
		Profiles: map[string]client.ChatPostureProfile{
			"plan": {Provider: "grok", Model: "grok-4.5"},
		},
	}
	if cmd := m.routePostureSwitch(cfg, "plan"); cmd == nil {
		t.Fatal("while live must block")
	}
	// Simulate terminal via orch.
	m.turnLive = false
	m.connStatus = ConnIdle
	m.statusMsg = "done"
	if cmd := m.routePostureSwitch(cfg, "plan"); cmd == nil {
		t.Fatal("after done, Tab must route again")
	}
}
