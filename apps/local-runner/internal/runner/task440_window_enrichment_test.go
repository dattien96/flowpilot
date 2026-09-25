package runner

import "testing"

// Task-440 (CP-86 P-1): every token_usage_updated event must carry
// ModelContextWindow when the catalog knows the model. Providers that never
// self-report a window (Claude CLI) get the catalog value filled at
// emitLocked; self-reported values are never overridden; unknown models stay
// nil (deterministic degradation — never a fake number).

func emitUsageEvent(t *testing.T, svc *InteractiveService, rs *interactiveRun, usage *TokenUsageSnapshot) ProviderEvent {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return svc.emitLocked(rs, ProviderEvent{Type: EventTokenUsageUpdated, TokenUsage: usage})
}

func newTask440Run(svc *InteractiveService, id string, key ProviderKey, model string) *interactiveRun {
	rs := &interactiveRun{id: id, providerKey: key, modelName: model, workspaceCwd: ""}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()
	return rs
}

func TestTask440_UsageEventWithoutWindow_EnrichedFromCatalog(t *testing.T) {
	svc := NewInteractiveService()
	rs := newTask440Run(svc, "run-440a", ProviderKeyClaude, "claude-sonnet-4-5")

	ev := emitUsageEvent(t, svc, rs, &TokenUsageSnapshot{
		Total: &TokenUsageBreakdown{TotalTokens: 12000},
	})
	if ev.TokenUsage == nil || ev.TokenUsage.ModelContextWindow == nil {
		t.Fatal("ModelContextWindow = nil, want catalog value for claude-sonnet-4-5")
	}
	if got := *ev.TokenUsage.ModelContextWindow; got != 200000 {
		t.Fatalf("ModelContextWindow = %d, want 200000", got)
	}
}

func TestTask440_SelfReportedWindow_NeverOverridden(t *testing.T) {
	svc := NewInteractiveService()
	self := int64(262144)
	rs := newTask440Run(svc, "run-440b", ProviderKeyClaude, "claude-opus")

	ev := emitUsageEvent(t, svc, rs, &TokenUsageSnapshot{
		Total:              &TokenUsageBreakdown{TotalTokens: 9000},
		ModelContextWindow: &self,
	})
	if ev.TokenUsage.ModelContextWindow == nil || *ev.TokenUsage.ModelContextWindow != 262144 {
		t.Fatalf("self-reported window = %v, want preserved 262144", ev.TokenUsage.ModelContextWindow)
	}
}

func TestTask440_UnknownModel_WindowStaysNil(t *testing.T) {
	svc := NewInteractiveService()
	rs := newTask440Run(svc, "run-440c", ProviderKeyClaude, "totally-unknown-model")

	ev := emitUsageEvent(t, svc, rs, &TokenUsageSnapshot{
		Total: &TokenUsageBreakdown{TotalTokens: 5000},
	})
	if ev.TokenUsage.ModelContextWindow != nil {
		t.Fatalf("unknown model window = %d, want nil (never a fake number)", *ev.TokenUsage.ModelContextWindow)
	}
}

func TestTask440_EnrichmentIsPureInMemory_NoIOUnderLock(t *testing.T) {
	// emitLocked runs under s.mu — enrichment must work with zero provider
	// binaries, no catalog store hits, and no network. A bare service +
	// registered run proving the fill is the no-I/O evidence.
	svc := NewInteractiveService()
	rs := newTask440Run(svc, "run-440d", ProviderKeyClaude, "claude-haiku")

	ev := emitUsageEvent(t, svc, rs, &TokenUsageSnapshot{
		Last: &TokenUsageBreakdown{TotalTokens: 3000},
	})
	if ev.TokenUsage.ModelContextWindow == nil || *ev.TokenUsage.ModelContextWindow != 200000 {
		t.Fatalf("ModelContextWindow = %v, want 200000 from pure catalog lookup", ev.TokenUsage.ModelContextWindow)
	}
}

func TestTask440_ClaudeCodexGrok_Parity(t *testing.T) {
	svc := NewInteractiveService()
	self := int64(131072)

	cases := []struct {
		name       string
		provider   ProviderKey
		model      string
		selfReport *int64
		want       int64 // 0 = expect nil
	}{
		{"claude catalog fill", ProviderKeyClaude, "claude-opus", nil, 200000},
		{"codex self-report preserved", ProviderKeyCodex, "gpt-5.5", &self, 131072},
		{"grok self-report preserved", ProviderKeyGrok, "grok-4.5", &self, 131072},
		{"unknown provider-model stays nil", ProviderKeyGrok, "grok-nonexistent-9", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := newTask440Run(svc, "run-440p-"+tc.name, tc.provider, tc.model)
			ev := emitUsageEvent(t, svc, rs, &TokenUsageSnapshot{
				Total:              &TokenUsageBreakdown{TotalTokens: 1000},
				ModelContextWindow: tc.selfReport,
			})
			if tc.want == 0 {
				if ev.TokenUsage.ModelContextWindow != nil {
					t.Fatalf("window = %d, want nil", *ev.TokenUsage.ModelContextWindow)
				}
				return
			}
			if ev.TokenUsage.ModelContextWindow == nil || *ev.TokenUsage.ModelContextWindow != tc.want {
				t.Fatalf("window = %v, want %d", ev.TokenUsage.ModelContextWindow, tc.want)
			}
		})
	}
}

// TestTask440_LegWindowCarryForward covers the live-observed gap: Devin's
// usage_update events carry `size`, but the turn-terminal usage event does
// not — and devin models have no catalog window, so the last event landed
// with ModelContextWindow=nil. The context window is a leg property, not a
// per-event property: once seen on a leg it carries forward to later events
// of the SAME leg so the pressure ladder never silently skips the deciding
// event. A different leg (new providerSessionID) does NOT inherit it.
func TestTask440_LegWindowCarryForward(t *testing.T) {
	svc := NewInteractiveService()
	win := int64(262000)
	rs := newTask440Run(svc, "run-440c2", ProviderKeyDevin, "devin/swe-2-high")
	rs.providerSessionID = "leg-a"

	// Mid-turn usage_update self-reports the window (devin `size`).
	ev1 := emitUsageEvent(t, svc, rs, &TokenUsageSnapshot{
		Total:              &TokenUsageBreakdown{TotalTokens: 11000},
		ModelContextWindow: &win,
	})
	if ev1.TokenUsage.ModelContextWindow == nil || *ev1.TokenUsage.ModelContextWindow != 262000 {
		t.Fatalf("self-reported window = %v, want 262000", ev1.TokenUsage.ModelContextWindow)
	}

	// Turn-terminal usage event lacks `size` and devin has no catalog value —
	// the leg's last-known window must carry forward.
	ev2 := emitUsageEvent(t, svc, rs, &TokenUsageSnapshot{
		Total: &TokenUsageBreakdown{TotalTokens: 12410},
	})
	if ev2.TokenUsage.ModelContextWindow == nil || *ev2.TokenUsage.ModelContextWindow != 262000 {
		t.Fatalf("same-leg window = %v, want carry-forward 262000", ev2.TokenUsage.ModelContextWindow)
	}

	// A different leg must NOT inherit — leg windows are session-scoped.
	rs.providerSessionID = "leg-b"
	ev3 := emitUsageEvent(t, svc, rs, &TokenUsageSnapshot{
		Total: &TokenUsageBreakdown{TotalTokens: 500},
	})
	if ev3.TokenUsage.ModelContextWindow != nil {
		t.Fatalf("new-leg window = %d, want nil (no cross-leg carry)", *ev3.TokenUsage.ModelContextWindow)
	}
}
