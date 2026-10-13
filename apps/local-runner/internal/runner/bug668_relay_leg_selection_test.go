package runner

import (
	"testing"
)

// BUG-668 (live run-752742, chat cht_3078718bf299): the closed-leg relay at
// interactive_service.go:12075 and the quota-rotate relay at :12134 pick the
// last-iterated leg with legState==active && leg.id != runID over a Go map —
// random per call. Live state had FOUR legs marked active (completed grok leg,
// cancelled leg, phantom-loop leg, idle leg); /turns on the parent relayed
// into dead legs until the coin flip landed on the live one. Selection must
// be deterministic: prefer the newest non-terminal leg (highest legSeq).
func TestBug668_RelayLegSelectionPrefersNewestNonTerminal(t *testing.T) {
	svc := bug289Service(t)
	chatID := "chat-668"

	mk := func(id string, seq int, status string) *interactiveRun {
		rs := &interactiveRun{
			id:       id,
			runKind:  "chat",
			chatID:   chatID,
			legSeq:   seq,
			legState: LegStateActive,
			status:   RunStatus(status),
			subs:     map[int64]chan ProviderEvent{},
		}
		svc.mu.Lock()
		svc.runs[id] = rs
		svc.mu.Unlock()
		return rs
	}
	mk("leg-1-dead", 1, string(RunStatusCompleted))
	mk("leg-2-dead", 2, string(RunStatusCancelled))
	live := mk("leg-3-live", 3, string(RunStatusIdle))

	// Map-order randomness: the helper must return the live leg on every
	// call, not whatever iterated last.
	for i := 0; i < 50; i++ {
		svc.mu.Lock()
		got := svc.selectRelayLegLocked(chatID, "closed-src")
		svc.mu.Unlock()
		if got != live {
			t.Fatalf("iter %d: selected %v, want newest non-terminal leg %s", i, got, live.id)
		}
	}
}

// Same selection applies when every active candidate is terminal — the relay
// must fail honestly (nil → leg_closed upstream) rather than routing a turn
// into a leg that can never accept it.
func TestBug668_RelayLegSelectionAllTerminalReturnsNil(t *testing.T) {
	svc := bug289Service(t)
	chatID := "chat-668b"
	for i, st := range []RunStatus{RunStatusCompleted, RunStatusCancelled, RunStatusFailed} {
		rs := &interactiveRun{
			id:       "dead-" + string(rune('a'+i)),
			runKind:  "chat",
			chatID:   chatID,
			legSeq:   i + 1,
			legState: LegStateActive,
			status:   st,
			subs:     map[int64]chan ProviderEvent{},
		}
		svc.mu.Lock()
		svc.runs[rs.id] = rs
		svc.mu.Unlock()
	}
	svc.mu.Lock()
	got := svc.selectRelayLegLocked(chatID, "closed-src")
	svc.mu.Unlock()
	if got != nil {
		t.Fatalf("selected terminal leg %s — must return nil so the caller fails honestly", got.id)
	}
}
