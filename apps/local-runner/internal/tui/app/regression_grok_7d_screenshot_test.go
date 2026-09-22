package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Per user request: status line is empty (chrome in input frame), quota hidden
// (sidebar only session+steps, footer Model·reason·YOLO).

func TestRegression_Grok7dQuota_AlwaysOnStatusLine(t *testing.T) {
	seven := 87
	summary := "Team 54b113be-a4be-4876-9577-d30fa68041d"
	reset := "2026-09-04T00:00:00Z"
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

	got := stripANSI(m.renderStatusLine())
	if strings.Contains(got, "7d:87%") {
		t.Fatalf("quota must NOT be on status line (hidden per new UI), got %q", got)
	}
	if strings.Contains(got, "Team 54b113be") {
		t.Fatalf("status must not show Team UUID as quota, got %q", got)
	}
	// Email now lives in session display (sidebar session), not status
	enableSidebarForTest(m)
	m.width, m.fullWidth = tuiSidebarMinWidth+10, tuiSidebarMinWidth+10
	side := strings.Join(m.renderRightSidebar(30), "\n")
	if !strings.Contains(side, "trashname899@gmail.com") && !strings.Contains(m.sessionDisplayLine(), "trashname899@gmail.com") {
		t.Fatalf("session must keep the account email, side=%q session=%q", side, m.sessionDisplayLine())
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
	got := stripANSI(m.renderStatusLine())
	if strings.Contains(got, "Team 54b113be") {
		t.Fatalf("status must not render Team UUID as quota, got %q", got)
	}
	if strings.Contains(got, "7d:") {
		t.Fatalf("status must not hallucinate 7d when quota nil, got %q", got)
	}
}

func TestRegression_ClaudeAndGrokParity_ShowsOwnWindow(t *testing.T) {
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
			// Quota hidden per new UI, but formatter still works
			if strings.Contains(stripANSI(m.renderStatusLine()), tc.expect) {
				t.Fatalf("%s must NOT show %q on status line (hidden), got %q", tc.name, tc.expect, stripANSI(m.renderStatusLine()))
			}
			if !strings.Contains(formatAccountLimits(&a), tc.expect) {
				t.Fatalf("%s formatter must still contain %q, got %q", tc.name, tc.expect, formatAccountLimits(&a))
			}
		})
	}
}
