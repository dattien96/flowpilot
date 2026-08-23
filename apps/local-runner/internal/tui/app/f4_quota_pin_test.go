package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestF4Collapsed_ShowsGrok7d(t *testing.T) {
	seven := 87
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.account = &client.ProviderAccountSummary{ProviderKey: "grok", DisplayLabel: "grok@example.com", Remaining7dPercent: &seven, IsActive: true}
	m.accountLabel = "grok@example.com"
	m.statusDetailsCollapsed = true
	got := stripANSI(m.renderStatusLine())
	if !strings.Contains(got, "7d:87%") {
		t.Fatalf("collapsed F4 must still show Grok 7d:87%%, got %q", got)
	}
}

func TestF4Collapsed_ShowsClaude5h(t *testing.T) {
	five := 72
	m := New(config.ChatConfig{Provider: "claude", Model: "sonnet"}, "http://127.0.0.1:4317")
	m.account = &client.ProviderAccountSummary{ProviderKey: "claude", DisplayLabel: "claude@example.com", Remaining5hPercent: &five, IsActive: true}
	m.accountLabel = "claude@example.com"
	m.statusDetailsCollapsed = true
	got := stripANSI(m.renderStatusLine())
	if !strings.Contains(got, "5h:72%") {
		t.Fatalf("collapsed F4 must still show Claude 5h:72%%, got %q", got)
	}
}

func TestF4Collapsed_ShowsBothWhenPresent(t *testing.T) {
	five, seven := 72, 40
	m := New(config.ChatConfig{Provider: "claude", Model: "sonnet"}, "http://127.0.0.1:4317")
	m.account = &client.ProviderAccountSummary{ProviderKey: "claude", DisplayLabel: "a@b.com", Remaining5hPercent: &five, Remaining7dPercent: &seven, IsActive: true}
	m.accountLabel = "a@b.com"
	m.statusDetailsCollapsed = true
	got := stripANSI(m.renderStatusLine())
	if !strings.Contains(got, "5h:72%") || !strings.Contains(got, "7d:40%") {
		t.Fatalf("collapsed with both windows must show 5h and 7d, got %q", got)
	}
}

func TestF4Collapsed_UsageDetailFallbackShowsWeekly(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.account = &client.ProviderAccountSummary{
		ProviderKey:  "grok",
		DisplayLabel: "grok@example.com",
		UsageDetailLines: []client.ProviderAccountUsageLine{{Label: "Weekly limit", RemainingPercent: 99}},
		IsActive: true,
	}
	m.accountLabel = "grok@example.com"
	m.statusDetailsCollapsed = true
	got := stripANSI(m.renderStatusLine())
	if !strings.Contains(got, "Weekly limit:99%") {
		t.Fatalf("collapsed fallback must show weekly line, got %q", got)
	}
}

func TestF4Collapsed_TeamCreditsFallback(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	summary := "Team 54b113be-a4be-4876-9577-d30fa68041d"
	m.account = &client.ProviderAccountSummary{
		ProviderKey:  "grok",
		DisplayLabel: "trashname899@gmail.com",
		UsageDetailLines: []client.ProviderAccountUsageLine{{Label: "Team Credits (Monthly)", RemainingPercent: 42}},
		UsageSummary: &summary,
		IsActive: true,
	}
	m.accountLabel = "trashname899@gmail.com"
	m.statusDetailsCollapsed = true
	got := stripANSI(m.renderStatusLine())
	if !strings.Contains(got, "Team Credits") {
		t.Fatalf("collapsed Team Credits monthly must show fallback, got %q", got)
	}
	if strings.Contains(got, "Team 54b113be") {
		t.Fatalf("collapsed must not show Team UUID as quota, got %q", got)
	}
}

func TestF4Collapsed_TeamUUIDNotShownAsQuota(t *testing.T) {
	summary := "Team 54b113be-a4be-4876-9577-d30fa68041d"
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.account = &client.ProviderAccountSummary{ProviderKey: "grok", DisplayLabel: "trashname899@gmail.com", UsageSummary: &summary, IsActive: true}
	m.accountLabel = "trashname899@gmail.com"
	m.statusDetailsCollapsed = true
	got := stripANSI(m.renderStatusLine())
	if strings.Contains(got, "Team 54b113be") {
		t.Fatalf("collapsed must not show Team UUID as quota, got %q", got)
	}
	m.statusDetailsCollapsed = false
	got = stripANSI(m.renderStatusLine())
	if strings.Contains(got, "Team 54b113be") {
		t.Fatalf("expanded must not show Team UUID as quota, got %q", got)
	}
}

func TestF4ExpandedAndCollapsed_BothShow7d(t *testing.T) {
	seven := 99
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.account = &client.ProviderAccountSummary{ProviderKey: "grok", DisplayLabel: "grok@example.com", Remaining7dPercent: &seven, IsActive: true}
	m.accountLabel = "grok@example.com"
	m.statusDetailsCollapsed = false
	if !strings.Contains(stripANSI(m.renderStatusLine()), "7d:99%") {
		t.Fatalf("expanded must show 7d")
	}
	m.statusDetailsCollapsed = true
	if !strings.Contains(stripANSI(m.renderStatusLine()), "7d:99%") {
		t.Fatalf("collapsed must show 7d")
	}
}
