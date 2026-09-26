package runner

// BUG-513 (deep review B-3, 2026-09-26): the Claude event mapper emits
// TokenUsageSnapshot{Last: usage} only — TokenUsage.Total stays nil. The
// usage-cap ledger (context_usage.go: latest Total.TotalTokens per provider
// session) and same-leg compaction detection (context_pressure.go:
// snap.Total drop) therefore see NOTHING for Claude runs while Codex,
// Devin, Grok and OpenCode all report cumulative Total. The parity test
// TestTask442_ClaudeCodexGrok_Parity injects Total directly and never
// exercises the mapper, so the blind spot was invisible to unit tests.
//
// Claude respawns `claude -p` per turn: a result event's usage is the
// aggregate for that query, not session-cumulative like Devin's totalTokens.
// The mapper marks it query-scoped; the emit seam accumulates query-scoped
// totals into the session-cumulative Total the ledger semantics expect,
// seeding from the last accumulated event on the same session (survives
// restart since the seam mutates the event before it lands durably).
// Adapter-reported session totals are never touched.

import (
	"testing"
)

// Round 4 (review): the round-2/3 attempts inferred Claude compaction from
// a snap.Last drop (gated at ≥80% of window in round 3). But Last is the
// single query's token aggregate, not session occupancy — a long query
// followed by a short one satisfies even the near-window gate without the
// provider compacting anything. Query-scoped legs therefore stay blind to
// provider_compacted; only cumulative legs (Devin/Grok/Codex), whose Total
// genuinely tracks the session, can flag a drop. Query-scoped Total still
// accumulates for cap accounting.

func bug513Usage(in, out int64, queryScoped bool) ProviderEvent {
	return bug513UsageWindow(in, out, queryScoped, 0)
}

func bug513UsageWindow(in, out int64, queryScoped bool, window int64) ProviderEvent {
	snap := &TokenUsageSnapshot{
		Last:            &TokenUsageBreakdown{TotalTokens: in + out, InputTokens: in, OutputTokens: out},
		Total:           &TokenUsageBreakdown{TotalTokens: in + out, InputTokens: in, OutputTokens: out},
		UsageScopeQuery: queryScoped,
	}
	if window > 0 {
		snap.ModelContextWindow = &window
	}
	return ProviderEvent{Type: EventTokenUsageUpdated, TokenUsage: snap}
}

func bug513Compacted(rs *interactiveRun) bool {
	for _, e := range rs.events {
		if e.Type == EventProviderCompacted {
			return true
		}
	}
	return false
}

func TestBug513_QueryScopedLegStaysBlindToPerQueryDrop(t *testing.T) {
	t.Setenv("FLOWPILOT_CONTEXT_PRESSURE", "1")
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-cl", providerKey: ProviderKeyClaude, providerSessionID: "sess-1"}

	svc.mu.Lock()
	// Grok's false-positive shape: a long query (900 of a 1000-token
	// window — even the round-3 near-window gate would pass) followed by a
	// short one (400 — a 56% Last drop). Neither figure is session
	// occupancy, so this CANNOT prove the provider compacted — the honest
	// answer for a query-scoped leg is blind, not a guess.
	svc.emitLocked(rs, bug513UsageWindow(600, 300, true, 1000))
	svc.emitLocked(rs, bug513UsageWindow(100, 300, true, 1000))
	svc.mu.Unlock()

	if bug513Compacted(rs) {
		t.Fatal("query-scoped leg: a per-query Last drop must NOT flag provider_compacted — Last is not session fullness")
	}
	svc.mu.Lock()
	degraded := rs.contextDegradedLegs["sess-1"]
	total := rs.events[len(rs.events)-1].TokenUsage.Total.TotalTokens
	svc.mu.Unlock()
	if degraded {
		t.Fatal("a blind leg must not be marked context_degraded")
	}
	// Cap accounting is unaffected: query-scoped Total still accumulates
	// session-wide (900 + 400 = 1300).
	if total != 1300 {
		t.Fatalf("query-scoped Total must still accumulate for caps, got %d want 1300", total)
	}
}

func TestBug513_QueryScopedLegNoFalsePositiveOnGrowth(t *testing.T) {
	t.Setenv("FLOWPILOT_CONTEXT_PRESSURE", "1")
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-cl", providerKey: ProviderKeyClaude, providerSessionID: "sess-1"}

	svc.mu.Lock()
	svc.emitLocked(rs, bug513Usage(100, 50, true))
	svc.emitLocked(rs, bug513Usage(500, 200, true)) // larger query — growth is normal
	svc.mu.Unlock()
	if bug513Compacted(rs) {
		t.Fatal("growing per-query context must not be flagged as compaction")
	}
}

// Round 3/4 (review): a per-query figure drops on ANY shorter turn — no
// window-based gate can tell "provider compacted" from "operator sent less
// context" because Last is not session fullness. Query-scoped legs stay
// blind; only cumulative legs can flag a drop.

func TestBug513_QueryScopedLegShortTurnIsNotCompaction(t *testing.T) {
	t.Setenv("FLOWPILOT_CONTEXT_PRESSURE", "1")
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-cl", providerKey: ProviderKeyClaude, providerSessionID: "sess-1"}

	svc.mu.Lock()
	// 500 of a 1000-token window — HALF full, nowhere near compaction
	// territory. A 40% drop to 300 is a normal shorter turn, not a
	// provider compaction (Grok review: this exact shape false-positived
	// under the bare Last-drop detector).
	svc.emitLocked(rs, bug513UsageWindow(400, 100, true, 1000))
	svc.emitLocked(rs, bug513UsageWindow(200, 100, true, 1000))
	svc.mu.Unlock()

	if bug513Compacted(rs) {
		t.Fatal("a shorter turn from a half-full window must NOT flag provider compaction")
	}
	svc.mu.Lock()
	degraded := rs.contextDegradedLegs["sess-1"]
	svc.mu.Unlock()
	if degraded {
		t.Fatal("a non-compacted leg must not be marked context_degraded")
	}
}

func TestBug513_QueryScopedLegUnknownWindowStaysBlind(t *testing.T) {
	t.Setenv("FLOWPILOT_CONTEXT_PRESSURE", "1")
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-cl", providerKey: ProviderKeyClaude, providerSessionID: "sess-1"}

	svc.mu.Lock()
	// No window anywhere (modelName empty → catalog nil, no leg carry) —
	// the detector cannot distinguish compaction from variance → blind.
	svc.emitLocked(rs, bug513Usage(700, 300, true))
	svc.emitLocked(rs, bug513Usage(100, 300, true))
	svc.mu.Unlock()

	if bug513Compacted(rs) {
		t.Fatal("unknown window: a per-query drop must stay blind rather than guess compaction")
	}
}

func TestBug513_CumulativeLegStillTracksTotalDrop(t *testing.T) {
	t.Setenv("FLOWPILOT_CONTEXT_PRESSURE", "1")
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-dv", providerKey: ProviderKeyDevin, providerSessionID: "sess-d"}

	svc.mu.Lock()
	svc.emitLocked(rs, bug513Usage(800, 200, false))
	svc.emitLocked(rs, bug513Usage(300, 100, false)) // cumulative Total drops 60%
	svc.mu.Unlock()
	if !bug513Compacted(rs) {
		t.Fatal("cumulative leg: a sharp Total drop must still flag provider compaction")
	}
}

func TestBug513_ClaudeResultUsageCarriesQueryScopedTotal(t *testing.T) {
	evs := mapClaudeLine(claudeLine{Type: "result", Subtype: "success", Raw: map[string]any{
		"subtype":  "success",
		"is_error": false,
		"result":   "done",
		"usage": map[string]any{
			"input_tokens":  float64(150),
			"output_tokens": float64(50),
		},
	}})
	var snap *TokenUsageSnapshot
	for _, e := range evs {
		if e.Type == EventTokenUsageUpdated {
			snap = e.TokenUsage
		}
	}
	if snap == nil || snap.Total == nil || snap.Total.TotalTokens != 200 {
		t.Fatalf("result usage must populate Total so cap/compaction see Claude usage, got %+v", snap)
	}
	if !snap.UsageScopeQuery {
		t.Fatal("Claude query-aggregate usage must be marked query-scoped so the seam accumulates, not replaces, the session total")
	}
}

func TestBug513_SeamAccumulatesQueryScopedUsagePerSession(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-cl", providerKey: ProviderKeyClaude, providerSessionID: "sess-1"}

	emit := func(in, out int64) ProviderEvent {
		return ProviderEvent{
			Type: EventTokenUsageUpdated,
			TokenUsage: &TokenUsageSnapshot{
				Last:            &TokenUsageBreakdown{TotalTokens: in + out, InputTokens: in, OutputTokens: out},
				Total:           &TokenUsageBreakdown{TotalTokens: in + out, InputTokens: in, OutputTokens: out},
				UsageScopeQuery: true,
			},
		}
	}

	svc.mu.Lock()
	svc.emitLocked(rs, emit(100, 50))
	svc.mu.Unlock()
	got := rs.events[len(rs.events)-1].TokenUsage.Total.TotalTokens
	if got != 150 {
		t.Fatalf("first turn total = %d, want 150", got)
	}

	svc.mu.Lock()
	svc.emitLocked(rs, emit(200, 40))
	svc.mu.Unlock()
	got = rs.events[len(rs.events)-1].TokenUsage.Total.TotalTokens
	if got != 390 {
		t.Fatalf("second turn total must accumulate session-wide = %d, want 390", got)
	}

	// A new leg (provider switch re-pins providerSessionID) restarts
	// accumulation — emitLocked stamps the run's current session onto
	// every event, so the leg boundary is the run field.
	svc.mu.Lock()
	rs.providerSessionID = "sess-2"
	svc.emitLocked(rs, emit(10, 5))
	svc.mu.Unlock()
	got = rs.events[len(rs.events)-1].TokenUsage.Total.TotalTokens
	if got != 15 {
		t.Fatalf("new leg must restart accumulation = %d, want 15", got)
	}
}

func TestBug513_AdapterReportedTotalIsUntouched(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-dv", providerKey: ProviderKeyDevin, providerSessionID: "sess-d"}

	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{
		Type:              EventTokenUsageUpdated,
		ProviderSessionID: "sess-d",
		TokenUsage: &TokenUsageSnapshot{
			Last:  &TokenUsageBreakdown{TotalTokens: 500},
			Total: &TokenUsageBreakdown{TotalTokens: 500}, // already session-cumulative
		},
	})
	svc.emitLocked(rs, ProviderEvent{
		Type:              EventTokenUsageUpdated,
		ProviderSessionID: "sess-d",
		TokenUsage: &TokenUsageSnapshot{
			Last:  &TokenUsageBreakdown{TotalTokens: 900},
			Total: &TokenUsageBreakdown{TotalTokens: 900},
		},
	})
	svc.mu.Unlock()
	if got := rs.events[len(rs.events)-1].TokenUsage.Total.TotalTokens; got != 900 {
		t.Fatalf("adapter-reported cumulative Total must pass through, got %d", got)
	}
}
