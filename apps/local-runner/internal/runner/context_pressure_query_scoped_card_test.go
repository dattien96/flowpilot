package runner

// Review finding (round 5): the pressure ladder still fed a query-scoped
// leg's `Last` — which is the single query's token aggregate, not session
// occupancy — straight into the ≥80/≥90 tiers, so one long Claude query
// opened a `context_pressure_90` card claiming "this session is at 90% of
// its context window" and offered `rotate_leg` — a meaningless option on a
// provider that already respawns a fresh session per query. Query-scoped
// legs have no session-fullness signal at all: the honest posture is the
// same as round-4 compaction — blind, not a mislabeled guess. Cap
// accounting on accumulated Total is untouched.

import (
	"context"
	"testing"
	"time"
)

func bug513PressureEvents(rs *interactiveRun) (pressure, cards int) {
	for _, e := range rs.events {
		if e.Type == EventContextPressure {
			pressure++
		}
		if e.Type == EventUserQuestionRequired {
			cards++
		}
	}
	return pressure, cards
}

func TestQueryScopedLeg_PressureLadderStaysBlind(t *testing.T) {
	t.Setenv("FLOWPILOT_CONTEXT_PRESSURE", "1")
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-cl", providerKey: ProviderKeyClaude, providerSessionID: "sess-1"}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.mu.Lock()
	// A single query at 95% of the window: on a query-scoped leg this says
	// "the query was huge", NOT "the session is nearly full" — the session
	// starts fresh next turn. No pressure event, no card.
	svc.emitLocked(rs, bug513UsageWindow(600, 350, true, 1000))
	svc.mu.Unlock()

	pressure, cards := bug513PressureEvents(rs)
	if pressure != 0 {
		t.Fatalf("query-scoped leg must not emit context_pressure events (Last is per-query), got %d", pressure)
	}
	if cards != 0 {
		t.Fatalf("query-scoped leg must not open a context-pressure card, got %d", cards)
	}
	if len(rs.pressureTierFired) != 0 {
		t.Fatalf("no tier may latch for a query-scoped leg, got %v", rs.pressureTierFired)
	}
}

func TestQueryScopedLeg_CapAccountingStillAccumulates(t *testing.T) {
	t.Setenv("FLOWPILOT_CONTEXT_PRESSURE", "1")
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-cl", providerKey: ProviderKeyClaude, providerSessionID: "sess-1"}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.mu.Lock()
	svc.emitLocked(rs, bug513UsageWindow(600, 350, true, 1000))
	svc.emitLocked(rs, bug513UsageWindow(100, 50, true, 1000))
	svc.mu.Unlock()

	// Ladder blind does not mean usage-blind: Total still accumulates
	// session-wide for caps (950 + 150 = 1100).
	svc.mu.Lock()
	last := rs.events[len(rs.events)-1].TokenUsage.Total.TotalTokens
	svc.mu.Unlock()
	if last != 1100 {
		t.Fatalf("query-scoped Total must accumulate for caps, got %d want 1100", last)
	}
}

// Live entry: a real turn on a Claude leg whose adapter reports a single
// huge query (95% of window, UsageScopeQuery) must not open the pressure
// card — startTurn → provider bridge → emitLocked → eval seam.
func TestQueryScopedLeg_RealTurnOpensNoCard(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyClaude, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				window := int64(1000)
				b.Emit(ProviderEvent{
					Type: EventTokenUsageUpdated,
					TokenUsage: &TokenUsageSnapshot{
						Last:               &TokenUsageBreakdown{TotalTokens: 950},
						Total:              &TokenUsageBreakdown{TotalTokens: 950},
						UsageScopeQuery:    true,
						ModelContextWindow: &window,
					},
				})
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	h, aerr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude, Model: "claude-sonnet",
	})
	if aerr != nil {
		t.Fatalf("createRun: %v", aerr)
	}
	if _, aerr := svc.startTurn(h.RunID, TurnInput{StepID: "s1", Prompt: "big query"}, "", ""); aerr != nil {
		t.Fatalf("startTurn: %v", aerr)
	}
	waitLoop(t, "turn completed", 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[h.RunID]
		return rs != nil && rs.status == RunStatusCompleted
	})
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	pressure, cards := bug513PressureEvents(rs)
	svc.mu.Unlock()
	if pressure != 0 || cards != 0 {
		t.Fatalf("a query-scoped 95%% turn must open no pressure card, events=%d cards=%d", pressure, cards)
	}
}

func TestCumulativeLeg_PressureLadderUnchanged(t *testing.T) {
	t.Setenv("FLOWPILOT_CONTEXT_PRESSURE", "1")
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-dv", providerKey: ProviderKeyDevin, providerSessionID: "sess-d", chatID: "chat-1", legState: LegStateActive}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	svc.mu.Lock()
	// Cumulative provider: Last tracks real session occupancy — the ask
	// tier card must still open.
	svc.emitLocked(rs, bug513UsageWindow(600, 350, false, 1000))
	svc.mu.Unlock()

	pressure, cards := bug513PressureEvents(rs)
	if pressure != 1 {
		t.Fatalf("cumulative leg at 95%% must emit context_pressure, got %d", pressure)
	}
	if cards != 1 {
		t.Fatalf("cumulative leg at 95%% must open the ask card, got %d", cards)
	}
}
