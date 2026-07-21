package runner

// BUG-298: a chat-mode run using the built-in Review Loop orchestration (which IS
// flowEngineDriven=true, per the BUG-288 fix) got permanently stuck at
// status="running" after its hub called submit_review_outcome a second time. The
// session-level gate flag (pendingFlowGateSettle, armed by
// markPendingFlowGateSettleLocked whenever flowEngineDriven is true) said a gate
// check was owed, but requiresGateSettlement (dispatch_record.go) never recognized
// runMode "chat" as needing settlement, so SettleOwed was persisted false for every
// chat-mode turn. That silently starved the live settle-drive scheduler
// (maybeScheduleSettleAfterTerminal) of the trigger it needs to re-run
// resumePendingFlowGate, leaving the run stuck until a server restart's boot-time
// recovery (which reads pendingFlowGateSettle directly, bypassing SettleOwed)
// finally resolved it.
//
// Confirmed with real production data (sessions.ndjson + dispatch.ndjson for the
// actual affected run): status flipped to "running"+pending_flow_gate_settle=true
// right after the hub's submit_review_outcome turn, and the matching dispatch
// record showed terminal_completed with settle_owed:false, with no further
// activity ever recorded for that run until restart.
//
// The fix adds "chat" to requiresGateSettlement's recognized runMode set.
// Confirmed provider-agnostic: neither requiresGateSettlement nor newDispatchRecord
// take a providerKey parameter or branch on one — the gap depends only on
// rs.runKind, so it reproduces (and the fix applies) identically for Claude, Codex,
// and Grok. The provider-parameterized test below locks that in directly.
//
// additive-tests-only: no existing test is modified.

import "testing"

func TestBug298RequiresGateSettlementRecognizesChatMode(t *testing.T) {
	if !requiresGateSettlement("chat", "some-step") {
		t.Fatal("BUG-298: runMode=\"chat\" with a non-empty stepID must require gate settlement")
	}
}

func TestBug298RequiresGateSettlementNonRegression(t *testing.T) {
	cases := []struct {
		runMode string
		stepID  string
		want    bool
	}{
		{"flow", "step-1", true},
		{"workflow", "step-1", true},
		{"", "step-1", true},
		{"chat", "", false}, // empty stepID never owes settlement, regardless of runMode
	}
	for _, c := range cases {
		if got := requiresGateSettlement(c.runMode, c.stepID); got != c.want {
			t.Fatalf("requiresGateSettlement(%q, %q) = %v, want %v", c.runMode, c.stepID, got, c.want)
		}
	}
}

// TestBug298NewDispatchRecordSetsSettleOwedForChatMode drives the real
// newDispatchRecord constructor (not just the predicate in isolation) with
// runKind="chat" for each provider, confirming the composed path — the one that
// actually persists SettleOwed to dispatch.ndjson — produces true, and that this
// holds identically regardless of which provider is running the hub.
func TestBug298NewDispatchRecordSetsSettleOwedForChatMode(t *testing.T) {
	for _, providerKey := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(providerKey), func(t *testing.T) {
			rs := &interactiveRun{id: "run-298", runKind: "chat", flowEngineDriven: true, providerKey: providerKey}
			env := DispatchEnvelope{TurnID: "turn-298", RunID: rs.id, StepID: "chat-run-298", ProviderKey: providerKey}
			rec := newDispatchRecord(rs, "turn-298", env, "", 0, "")
			if !rec.SettleOwed {
				t.Fatalf("BUG-298 (%s): chat-mode dispatch record SettleOwed = false, want true", providerKey)
			}
		})
	}
}
