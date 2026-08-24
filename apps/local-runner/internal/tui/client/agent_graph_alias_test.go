package client_test

import (
	"encoding/json"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

func TestProviderEvent_AgentGraphAliasSnapshot(t *testing.T) {
	// Runner emits agentGraphSnapshot (desktop-aligned), TUI must decode it even though old payloads used agentGraph.
	payloadSnapshot := `{"id":"e1","seq":1,"type":"agent_graph_updated","agentGraphSnapshot":{"parentRunId":"run-127174","loopState":{"status":"blocked","blockReason":"escalate"}}}`
	var ev client.ProviderEvent
	if err := json.Unmarshal([]byte(payloadSnapshot), &ev); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if g := ev.EffectiveAgentGraph(); g == nil {
		t.Fatalf("EffectiveAgentGraph nil for agentGraphSnapshot payload")
	} else if g.ParentRunID != "run-127174" || g.LoopState.Status != "blocked" {
		t.Fatalf("decoded snapshot = %+v", g)
	}

	// Legacy runner still emits agentGraph — must still work.
	payloadLegacy := `{"id":"e2","seq":2,"type":"agent_graph_updated","agentGraph":{"parentRunId":"run-legacy","loopState":{"status":"running"}}}`
	var ev2 client.ProviderEvent
	if err := json.Unmarshal([]byte(payloadLegacy), &ev2); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if g := ev2.EffectiveAgentGraph(); g == nil {
		t.Fatalf("EffectiveAgentGraph nil for legacy agentGraph payload")
	} else if g.ParentRunID != "run-legacy" || g.LoopState.Status != "running" {
		t.Fatalf("decoded legacy = %+v", g)
	}

	// Both present — legacy wins (backward compat)
	payloadBoth := `{"id":"e3","seq":3,"type":"agent_graph_updated","agentGraph":{"parentRunId":"run-legacy","loopState":{"status":"running"}},"agentGraphSnapshot":{"parentRunId":"run-snapshot","loopState":{"status":"blocked"}}}`
	var ev3 client.ProviderEvent
	if err := json.Unmarshal([]byte(payloadBoth), &ev3); err != nil {
		t.Fatalf("unmarshal both: %v", err)
	}
	if g := ev3.EffectiveAgentGraph(); g.ParentRunID != "run-legacy" {
		t.Fatalf("both present: got %q, want run-legacy (legacy preferred)", g.ParentRunID)
	}
}
