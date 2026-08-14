package app

import (
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestFormatAccountLimits_UsesUsageDetailLinesWhenWindowsMissing(t *testing.T) {
	acc := &client.ProviderAccountSummary{
		UsageDetailLines: []client.ProviderAccountUsageLine{
			{Label: "Weekly limit", RemainingPercent: 99},
		},
	}
	if got := formatAccountLimits(acc); got != "Weekly limit:99%" {
		t.Fatalf("got %q", got)
	}
}

func TestStatusLine_KeepsGrokRemainingWhenWidthIsTight(t *testing.T) {
	seven := 87
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.width = 72
	m.reasoningEffort = "high"
	m.account = &client.ProviderAccountSummary{
		ProviderKey:        "grok",
		DisplayLabel:       "trashname899@gmail.com",
		Remaining7dPercent: &seven,
	}
	m.accountLabel = "trashname899@gmail.com"
	got := m.renderStatusLine()
	if !strings.Contains(got, "7d:87%") {
		t.Fatalf("tight statusline dropped remaining quota:\n%s", got)
	}
}

func TestSessionDisplayLine_IncludesRemainingQuota(t *testing.T) {
	seven := 87
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.account = &client.ProviderAccountSummary{
		ProviderKey:        "grok",
		DisplayLabel:       "acct",
		Remaining7dPercent: &seven,
	}
	m.accountLabel = "acct"
	got := m.sessionDisplayLine()
	if !strings.Contains(got, "7d:87%") {
		t.Fatalf("session line missing remaining: %q", got)
	}
}

func TestFormatAccountLimits_IncludesResetDateLikeDesktop(t *testing.T) {
	seven := 87
	reset := "2026-08-19T06:46:00Z"
	acc := &client.ProviderAccountSummary{
		Remaining7dPercent: &seven,
		Remaining7dResetAt: &reset,
	}
	got := formatAccountLimits(acc)
	wantWhen := mustLocalQuotaReset(t, reset)
	want := "7d:87% · resets " + wantWhen
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatAccountLimits_Claude5hAndGrok7dBothShowReset(t *testing.T) {
	five, seven := 72, 40
	reset5h := "2026-08-13T12:00:00Z"
	reset7d := "2026-08-19T06:46:00Z"
	acc := &client.ProviderAccountSummary{
		Remaining5hPercent: &five,
		Remaining7dPercent: &seven,
		Remaining5hResetAt: &reset5h,
		Remaining7dResetAt: &reset7d,
	}
	got := formatAccountLimits(acc)
	want := "5h:72% · resets " + mustLocalQuotaReset(t, reset5h) +
		" 7d:40% · resets " + mustLocalQuotaReset(t, reset7d)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatAccountLimits_OmitsResetWhenMissing(t *testing.T) {
	five, seven := 72, 40
	acc := &client.ProviderAccountSummary{Remaining5hPercent: &five, Remaining7dPercent: &seven}
	if got := formatAccountLimits(acc); got != "5h:72% 7d:40%" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatAccountLimits_UsageDetailLinesIncludeReset(t *testing.T) {
	reset := "2026-08-19T06:46:00.123Z"
	acc := &client.ProviderAccountSummary{
		UsageDetailLines: []client.ProviderAccountUsageLine{
			{Label: "Weekly limit", RemainingPercent: 99, ResetAt: reset},
		},
	}
	got := formatAccountLimits(acc)
	want := "Weekly limit:99% · resets " + mustLocalQuotaReset(t, reset)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestStatusLine_KeepsGrokResetDateWhenWidthIsTight(t *testing.T) {
	seven := 87
	reset := "2026-08-19T06:46:00Z"
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.width = 72
	m.reasoningEffort = "high"
	m.account = &client.ProviderAccountSummary{
		ProviderKey:        "grok",
		DisplayLabel:       "acct@example.com",
		Remaining7dPercent: &seven,
		Remaining7dResetAt: &reset,
	}
	m.accountLabel = "acct@example.com"
	got := m.renderStatusLine()
	if !strings.Contains(got, "7d:87%") || !strings.Contains(got, "resets") {
		t.Fatalf("tight statusline dropped remaining reset date:\n%s", got)
	}
	if !strings.Contains(got, mustLocalQuotaReset(t, reset)) {
		t.Fatalf("tight statusline missing formatted reset:\n%s", got)
	}
}

func mustLocalQuotaReset(t *testing.T, raw string) string {
	t.Helper()
	got := formatQuotaResetAt(raw)
	if got == "" {
		t.Fatalf("could not format reset %q", raw)
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, raw)
	}
	if err != nil {
		t.Fatal(err)
	}
	want := parsed.Local().Format("Jan 2, 15:04")
	if got != want {
		t.Fatalf("formatQuotaResetAt=%q want %q", got, want)
	}
	return got
}

func TestFormatContextLimits_IncludesRemainPercentAndInputTokens(t *testing.T) {
	win := int64(128000)
	usage := &client.TokenUsageSnapshot{
		ModelContextWindow: &win,
		Total:              &client.TokenUsageBreakdown{TotalTokens: 12000, InputTokens: 9000},
		Last:               &client.TokenUsageBreakdown{TotalTokens: 800, InputTokens: 500},
	}
	got := formatContextLimits(usage, 0)
	if !strings.Contains(got, "91%") || !strings.Contains(got, "remain") {
		t.Fatalf("missing remain percent: %q", got)
	}
	if !strings.Contains(got, "12.0k") || !strings.Contains(got, "left") {
		t.Fatalf("old used/window/left contract broken: %q", got)
	}
	if !strings.Contains(got, "in:500") {
		t.Fatalf("missing last-turn input tokens: %q", got)
	}
}

func TestContextRemainingPercent_RoundsNearest(t *testing.T) {
	if got := contextRemainingPercent(12000, 128000); got != 91 {
		t.Fatalf("got %d want 91", got)
	}
	if got := contextRemainingPercent(128000, 128000); got != 0 {
		t.Fatalf("full window remain=%d", got)
	}
	if got := contextRemainingPercent(0, 100); got != 100 {
		t.Fatalf("empty window remain=%d", got)
	}
}
