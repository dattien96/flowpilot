package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Regression: screenshot 2026-08-20 showed grok | trashname899@gmail.com |
// Team 54b113be-a4be-4876-9577-d30fa68041d with no 7d, both expanded and
// collapsed (▸ F4 | Team 54b... | ready). Live billing for that account is
// weekly 11% used → 89% remaining, so TUI must show 7d:89% and never the Team
// UUID as a quota chip. This was green before the F4 pin / UsageSummary
// fallback regressed into showing the UUID.
func TestRegression_GrokScreenshot_Weekly89Shows7dNotTeamUUID(t *testing.T) {
	seven := 89
	reset := "2026-08-26T23:46:37Z"
	summary := "Team 54b113be-a4be-4876-9577-d30fa68041d"
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m.account = &client.ProviderAccountSummary{
		ProviderKey:        "grok",
		DisplayLabel:       "trashname899@gmail.com",
		Remaining7dPercent: &seven,
		Remaining7dResetAt: &reset,
		UsageDetailLines: []client.ProviderAccountUsageLine{
			{Label: "Weekly limit", RemainingPercent: 89, ResetAt: reset},
		},
		UsageSummary: &summary,
		IsActive:     true,
	}
	m.accountLabel = "trashname899@gmail.com"

	for _, collapsed := range []bool{false, true} {
		m.statusDetailsCollapsed = collapsed
		got := stripANSI(m.renderStatusLine())
		if !strings.Contains(got, "7d:89%") {
			t.Fatalf("collapsed=%v must show 7d:89%% (weekly 11%% used), got %q", collapsed, got)
		}
		if strings.Contains(got, "Team 54b113be") {
			t.Fatalf("collapsed=%v must not show Team UUID as quota, got %q", collapsed, got)
		}
	}
	// Expanded must still show the email, not leak Team UUID via quota fallback.
	m.statusDetailsCollapsed = false
	if !strings.Contains(stripANSI(m.renderStatusLine()), "trashname899@gmail.com") {
		t.Fatalf("expanded must keep email, got %q", stripANSI(m.renderStatusLine()))
	}
}

func TestRegression_GrokNoQuota_ShowsNeither7dNorTeamUUID(t *testing.T) {
	summary := "Team 54b113be-a4be-4876-9577-d30fa68041d"
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m.account = &client.ProviderAccountSummary{
		ProviderKey:  "grok",
		DisplayLabel: "trashname899@gmail.com",
		UsageSummary: &summary,
		IsActive:     true,
	}
	m.accountLabel = "trashname899@gmail.com"
	for _, collapsed := range []bool{false, true} {
		m.statusDetailsCollapsed = collapsed
		got := stripANSI(m.renderStatusLine())
		if strings.Contains(got, "Team 54b113be") {
			t.Fatalf("collapsed=%v must not render Team UUID as quota, got %q", collapsed, got)
		}
		if strings.Contains(got, "7d:") {
			t.Fatalf("collapsed=%v must not hallucinate 7d when quota nil, got %q", collapsed, got)
		}
	}
}

func TestRegression_ClaudeAndGrokParity_CollapsedShowsOwnWindow(t *testing.T) {
	five, seven := 72, 40
	cases := []struct {
		name   string
		acc    client.ProviderAccountSummary
		expect string
	}{
		{"claude 5h", client.ProviderAccountSummary{ProviderKey: "claude", Remaining5hPercent: &five}, "5h:72%"},
		{"grok 7d", client.ProviderAccountSummary{ProviderKey: "grok", Remaining7dPercent: &seven}, "7d:40%"},
		{"both", client.ProviderAccountSummary{ProviderKey: "claude", Remaining5hPercent: &five, Remaining7dPercent: &seven}, "7d:40%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: tc.acc.ProviderKey}, "http://127.0.0.1:4317")
			a := tc.acc
			a.DisplayLabel = "a@b.com"
			a.IsActive = true
			m.account = &a
			m.accountLabel = "a@b.com"
			m.statusDetailsCollapsed = true
			if !strings.Contains(stripANSI(m.renderStatusLine()), tc.expect) {
				t.Fatalf("collapsed %s must show %q, got %q", tc.name, tc.expect, stripANSI(m.renderStatusLine()))
			}
		})
	}
}
