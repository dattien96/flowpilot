package app

import (
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/client"
)

// Task-450 (CP-87 P-6): the TUI quota candidate table renders runner order
// verbatim with the contractual columns, and the cooldown bar counts down
// from server timestamps — never client-relative.

func TestTask450_TUICandidateTableAndSettings(t *testing.T) {
	pct := 90
	decision := client.QuotaRouteDecision{
		RunID: "run-1", Trigger: "quota_exhausted", Reason: "quota_exhausted",
		ProviderKey: "codex", Model: "gpt-5.4", AccountID: "cx-0",
		PolicyVersion: 1,
		Candidates: []client.QuotaRouteCandidate{
			{
				ProviderKey: "codex", Model: "gpt-5.4", WorkloadClass: "coding",
				AccountID: "cx-1",
				Headroom:  client.AccountHeadroomInfo{State: "healthy", RemainingPercent: &pct, Confidence: "exact"},
			},
			{
				ProviderKey: "claude", Model: "claude-sonnet-x", WorkloadClass: "coding",
				AccountID: "cl-1", RejectionReasons: []string{"unknown_quota"},
			},
			{
				ProviderKey: "codex", Model: "gpt-5.4", WorkloadClass: "coding",
				AccountID: "cx-2", RejectionReasons: []string{"exhausted_quota"},
			},
		},
	}
	lines := renderQuotaCandidateTable(decision, 120)
	joined := strings.Join(lines, "\n")
	for _, col := range []string{"provider", "model", "workload", "account", "headroom", "reset", "confidence", "reason"} {
		if !strings.Contains(joined, col) {
			t.Fatalf("table missing contractual column %q:\n%s", col, joined)
		}
	}
	// Runner order preserved: cx-1 before claude cl-1 before cx-2.
	i1 := strings.Index(joined, "cx-1")
	i2 := strings.Index(joined, "cl-1")
	i3 := strings.Index(joined, "cx-2")
	if !(i1 > 0 && i2 > i1 && i3 > i2) {
		t.Fatalf("runner order not preserved:\n%s", joined)
	}
	if !strings.Contains(joined, "90%") {
		t.Fatalf("exact headroom not rendered: %s", joined)
	}
	if !strings.Contains(joined, "unknown_quota") || !strings.Contains(joined, "exhausted_quota") {
		t.Fatalf("rejection reasons not rendered: %s", joined)
	}
	// Missing headroom renders honestly — the "unknown" state word, never a
	// fabricated percentage or token count.
	if !strings.Contains(joined, "unknown") {
		t.Fatalf("missing headroom must render 'unknown': %s", joined)
	}

	// The /quota settings view surfaces mode, priority, bindings, thresholds,
	// and the same-provider cooldown — the TUI parity read of the runner doc.
	settings := client.QuotaRoutingSettings{
		Mode:                        "manual",
		ProviderPriority:            []string{"codex", "claude"},
		ModelBindings:               []client.ModelClassBinding{{ProviderKey: "codex", WorkloadClass: "coding", Model: "gpt-5.4"}},
		HeadroomLowPercent:          20,
		TelemetryTTLSeconds:         300,
		SameProviderCooldownSeconds: 20,
		PolicyVersion:               1,
	}
	slines := strings.Join(renderQuotaRoutingSettings(settings, 120), "\n")
	for _, want := range []string{"mode: manual", "codex, claude", "codex / coding → gpt-5.4", "20%", "20s"} {
		if !strings.Contains(slines, want) {
			t.Fatalf("settings view missing %q:\n%s", want, slines)
		}
	}
	// A never-saved document defaults to manual — auto is opt-in only.
	def := strings.Join(renderQuotaRoutingSettings(client.QuotaRoutingSettings{}, 120), "\n")
	if !strings.Contains(def, "mode: manual") {
		t.Fatalf("unset settings must render manual mode: %s", def)
	}
}

func TestTask450_TUICooldownCountdownUsesServerDeadline(t *testing.T) {
	// RFC3339 stamps are whole-second — truncate so the fixture models wire data.
	now := time.Now().UTC().Truncate(time.Second)
	started := now.Add(-8 * time.Second).Format(time.RFC3339)
	until := now.Add(12 * time.Second).Format(time.RFC3339) // 20s window, 12s left

	bar := renderQuotaCooldownBar(started, until, now)
	if !strings.Contains(bar, "12s") {
		t.Fatalf("bar must count down from the server deadline (12s left): %s", bar)
	}
	if !strings.Contains(bar, "same-provider") && !strings.Contains(bar, "same-IP") {
		t.Fatalf("bar must explain same-IP provider safety: %s", bar)
	}
	// Remount with a later "now" resumes mid-window — never restarts at 20s.
	later := renderQuotaCooldownBar(started, until, now.Add(10*time.Second))
	if !strings.Contains(later, "2s") {
		t.Fatalf("remounted bar must resume at the server deadline (2s left): %s", later)
	}
	// Elapsed window clamps to 0 — never negative.
	expired := renderQuotaCooldownBar(started, until, now.Add(25*time.Second))
	if !strings.Contains(expired, "0s") {
		t.Fatalf("elapsed bar must reach zero: %s", expired)
	}
}
