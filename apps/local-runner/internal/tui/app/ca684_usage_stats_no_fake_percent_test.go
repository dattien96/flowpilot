package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Operator report (2026-08-29, CP-57 test guide run): the TUI session sidebar
// rendered opencode stats lines with a fabricated ":0%" suffix —
// "stats: 86 sessions · 15 days:0% cost: $137.86 total · $9.19/day:0% …".
// Cause: formatQuotaChip formats EVERY usage line as a quota meter
// ("label:pct%"); CA-682's opencode stats rows are informational text with
// RemainingPercent=0 and no reset. Fix: a line with no real percent AND no
// reset renders as its label alone (provider-agnostic — grok/gemini meters
// with real percents or reset times are untouched). Additive tests only.

func TestFormatQuotaChipInformationalStatsLineHasNoFakePercent(t *testing.T) {
	if got := formatQuotaChip("stats: 86 sessions · 15 days", 0, nil); got != "stats: 86 sessions · 15 days" {
		t.Fatalf("informational line must render bare, got %q", got)
	}
	if got := formatQuotaChip("cost: $137.86 total · $9.19/day", 0, nil); strings.Contains(got, ":0%") {
		t.Fatalf("no fabricated :0%% on informational line, got %q", got)
	}
}

func TestFormatQuotaChipRealMetersUnchanged(t *testing.T) {
	// Real quota meters keep their percent.
	if got := formatQuotaChip("5h window", 42, nil); got != "5h window:42%" {
		t.Fatalf("meter with pct = %q", got)
	}
	// A true 0%-remaining meter WITH a reset timestamp still shows the percent.
	reset := "2026-08-30T00:00:00Z"
	got := formatQuotaChip("5h window", 0, &reset)
	if !strings.Contains(got, ":0%") || !strings.Contains(got, "resets") {
		t.Fatalf("0%% meter with reset must keep percent + reset, got %q", got)
	}
}

func TestFormatAccountLimitsOpencodeStatsNoFakePercent(t *testing.T) {
	acc := &client.ProviderAccountSummary{
		ProviderKey: "opencode",
		UsageDetailLines: []client.ProviderAccountUsageLine{
			{Label: "stats: 86 sessions · 15 days"},
			{Label: "cost: $137.86 total · $9.19/day"},
			{Label: "tokens: 12.4M avg/session · 8.8K median"},
			{Label: "token limit: N/A — zen proxy, depends on the upstream model"},
		},
	}
	got := formatAccountLimits(acc)
	for _, bad := range []string{":0%"} {
		if strings.Contains(got, bad) {
			t.Fatalf("opencode stats must not render %q, got %q", bad, got)
		}
	}
	for _, want := range []string{"86 sessions", "$137.86 total", "12.4M avg/session", "token limit: N/A"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stats content must survive, missing %q in %q", want, got)
		}
	}
}

func TestCollapsedQuotaChipOpencodeStatsNoFakePercent(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.account = &client.ProviderAccountSummary{
		ProviderKey: "opencode",
		UsageDetailLines: []client.ProviderAccountUsageLine{
			{Label: "cost: $137.86 total · $9.19/day"},
		},
	}
	got := m.collapsedQuotaChip()
	if strings.Contains(got, ":0%") {
		t.Fatalf("collapsed chip must not fabricate :0%%, got %q", got)
	}
	if !strings.Contains(got, "$137.86") {
		t.Fatalf("collapsed chip must keep stats content, got %q", got)
	}
}
