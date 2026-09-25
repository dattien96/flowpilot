package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Task-444 (CP-86 P-5) — TUI surface for the CP-86 events. Awareness tier and
// provider compaction are inline markers on the existing status line — never
// a modal, never a card. Ask-tier decision cards arrive through the existing
// user_question_required path and need no new rendering. Additive-only: the
// pre-existing formatContextLimits signature stays untouched; the pressure/
// compacted markers compose through the new formatContextStatus wrapper.

func TestTask444_TUI_FormatContextLimits_PressureTintAndCompacted(t *testing.T) {
	win := int64(200000)
	est := int64(5000)
	usage := &client.TokenUsageSnapshot{
		ModelContextWindow: &win,
		EstPromptTokens:    &est,
		Total:              &client.TokenUsageBreakdown{TotalTokens: 180000},
		Last:               &client.TokenUsageBreakdown{TotalTokens: 12000},
	}
	cases := []struct {
		name      string
		st        contextStatus
		wantParts []string
		wantAbsent []string
	}{
		{"clean", contextStatus{},
			[]string{"prompt ~5.0k est"}, []string{"pressure", "compacted"}},
		{"aware", contextStatus{pressureTier: "aware", pressurePct: 83},
			[]string{"ctx ~83%"}, []string{"compacted"}},
		{"ask", contextStatus{pressureTier: "ask", pressurePct: 92},
			[]string{"ctx ~92%"}, nil},
		{"compacted", contextStatus{compacted: true, compactPrev: 190000, compactCur: 18000},
			[]string{"compacted", "190.0k→18.0k"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := formatContextStatus(usage, 0, c.st)
			for _, p := range c.wantParts {
				if !strings.Contains(got, p) {
					t.Fatalf("status %q missing %q", got, p)
				}
			}
			for _, p := range c.wantAbsent {
				if strings.Contains(got, p) {
					t.Fatalf("status %q unexpectedly contains %q", got, p)
				}
			}
		})
	}
}

func TestTask444_TUI_NoUsageData_NoFakeZero(t *testing.T) {
	got := formatContextStatus(nil, 0, contextStatus{})
	if strings.Contains(got, "0") {
		t.Fatalf("missing usage must render empty/dash, not a fake zero: %q", got)
	}
}

func TestTask444_TUI_HandleEventTracksPressureAndCompaction(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false

	m2, _ := m.handleEvent(client.ProviderEvent{
		Type:              "context_pressure",
		ProviderSessionID: "leg-1",
		ContextPressure: &client.ContextPressurePayload{
			Tier: "aware", Ratio: 0.83, UsedTokens: 166000, WindowTokens: 200000, LegID: "leg-1",
		},
	})
	am := m2.(*AppModel)
	if am.ctxStatus.pressureTier != "aware" || am.ctxStatus.pressurePct != 83 {
		t.Fatalf("ctxStatus after aware = %+v", am.ctxStatus)
	}

	m3, _ := am.handleEvent(client.ProviderEvent{
		Type:              "provider_compacted",
		ProviderSessionID: "leg-1",
		ContextPressure: &client.ContextPressurePayload{
			Tier: "aware", Ratio: 0.09, UsedTokens: 18000, WindowTokens: 200000, PrevTokens: 190000, LegID: "leg-1",
		},
	})
	am = m3.(*AppModel)
	if !am.ctxStatus.compacted || am.ctxStatus.compactPrev != 190000 || am.ctxStatus.compactCur != 18000 {
		t.Fatalf("ctxStatus after compacted = %+v", am.ctxStatus)
	}
}

func TestTask444_TUI_PressureClearsWhenUsageDropsBelowTier(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.ctxStatus = contextStatus{pressureTier: "aware", pressurePct: 83}
	m.ctxStatusLegID = "leg-1"
	win := int64(200000)
	m2, _ := m.handleEvent(client.ProviderEvent{
		Type:              "token_usage_updated",
		ProviderSessionID: "leg-1",
		TokenUsage: &client.TokenUsageSnapshot{
			ModelContextWindow: &win,
			Total:              &client.TokenUsageBreakdown{TotalTokens: 100000}, // 50% — below aware
		},
	})
	am := m2.(*AppModel)
	if am.ctxStatus.pressureTier != "" {
		t.Fatalf("aware marker must clear when usage drops below tier, got %+v", am.ctxStatus)
	}
	// Compacted is a fact, not a level — it stays pinned for the leg.
	m.ctxStatus = contextStatus{compacted: true, compactPrev: 190000, compactCur: 18000}
	m3, _ := m.handleEvent(client.ProviderEvent{
		Type:              "token_usage_updated",
		ProviderSessionID: "leg-1",
		TokenUsage: &client.TokenUsageSnapshot{
			ModelContextWindow: &win,
			Total:              &client.TokenUsageBreakdown{TotalTokens: 20000},
		},
	})
	if !m3.(*AppModel).ctxStatus.compacted {
		t.Fatal("compacted marker must survive usage drop within the same leg")
	}
}

func TestTask444_TUI_LegChangeResetsContextStatus(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.ctxStatus = contextStatus{pressureTier: "aware", pressurePct: 92, compacted: true, compactPrev: 190000, compactCur: 18000}
	m.ctxStatusLegID = "leg-1"
	// rotate_leg lands the next turn on a fresh provider session — stale marks
	// from the old leg must not bleed over.
	m2, _ := m.handleEvent(client.ProviderEvent{
		Type:              "context_pressure",
		ProviderSessionID: "leg-2",
		ContextPressure: &client.ContextPressurePayload{
			Tier: "aware", Ratio: 0.81, UsedTokens: 162000, WindowTokens: 200000, LegID: "leg-2",
		},
	})
	am := m2.(*AppModel)
	if am.ctxStatus.compacted || am.ctxStatus.pressurePct != 81 {
		t.Fatalf("leg change must reset compaction/pressure marks, got %+v", am.ctxStatus)
	}
}
