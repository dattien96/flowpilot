package cli

import "testing"

func TestParseLaunchSelection(t *testing.T) {
	index, err := parseLaunchSelection("2\n", 3)
	if err != nil {
		t.Fatalf("expected valid selection, got error: %v", err)
	}
	if index != 1 {
		t.Fatalf("expected index 1, got %d", index)
	}
}

func TestParseLaunchSelectionRejectsOutOfRange(t *testing.T) {
	_, err := parseLaunchSelection("4", 3)
	if err == nil {
		t.Fatal("expected out-of-range error")
	}
	if got := err.Error(); got != "selection must be between 1 and 3" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestBuildClaudeUsageSummary(t *testing.T) {
	summary := buildClaudeUsageSummary(
		"pro",
		"",
		"2026-01-05T10:00:00Z",
		true,
		"",
		claudeAuthStatus{},
	)

	want := "Pro since 2026-01-05 | extra usage enabled"
	if summary != want {
		t.Fatalf("expected %q, got %q", want, summary)
	}
}

func TestCodexQuotaFromWindow(t *testing.T) {
	quota := codexQuotaFromWindow(map[string]any{
		"used_percent":         35.0,
		"limit_window_seconds": 18000,
		"reset_at":             "2026-06-09T12:00:00Z",
	})
	if quota == nil {
		t.Fatal("expected quota to be parsed")
	}
	if quota.remainingPercent != 65 {
		t.Fatalf("expected remaining percent 65, got %d", quota.remainingPercent)
	}
	if quota.windowSeconds != 18000 {
		t.Fatalf("expected window seconds 18000, got %d", quota.windowSeconds)
	}
}

// TestGrokQuotaFromBillingMonthly uses the exact response shape captured
// live from GET {cli-chat-proxy.grok.com}/v1/billing (Task-216, corrects the
// CP-46 Q-5/Task-210 Q-1 "no endpoint" finding).
func TestGrokQuotaFromBillingMonthly(t *testing.T) {
	line := grokQuotaFromBilling(grokBillingResponse{
		Config: grokBillingConfig{
			MonthlyLimit:     &grokBillingValue{Val: 15000},
			Used:             &grokBillingValue{Val: 56},
			BillingPeriodEnd: "2026-08-01T00:00:00+00:00",
		},
	})
	if line == nil {
		t.Fatal("expected a usage line to be parsed")
	}
	if line.label != "Team Credits (Monthly)" {
		t.Fatalf("expected monthly label, got %q", line.label)
	}
	if line.remainingPercent != 99 {
		t.Fatalf("expected remaining percent 99, got %d", line.remainingPercent)
	}
	if line.resetAt != "2026-08-01T00:00:00Z" {
		t.Fatalf("expected reset time normalized to UTC RFC3339, got %q", line.resetAt)
	}
}

func TestGrokQuotaFromBillingWeeklyFallback(t *testing.T) {
	line := grokQuotaFromBilling(grokBillingResponse{
		Config: grokBillingConfig{
			WeeklyLimit: &grokBillingValue{Val: 100},
			Used:        &grokBillingValue{Val: 40},
		},
	})
	if line == nil {
		t.Fatal("expected a usage line to be parsed")
	}
	if line.label != "Team Credits (Weekly)" {
		t.Fatalf("expected weekly label, got %q", line.label)
	}
	if line.remainingPercent != 60 {
		t.Fatalf("expected remaining percent 60, got %d", line.remainingPercent)
	}
}

func TestGrokQuotaFromBillingMissingLimitReturnsNil(t *testing.T) {
	if line := grokQuotaFromBilling(grokBillingResponse{}); line != nil {
		t.Fatalf("expected nil when no limit field is present, got %+v", line)
	}
}

func TestLoadGrokQuotaEmptyTokenReturnsNil(t *testing.T) {
	if line := loadGrokQuota(""); line != nil {
		t.Fatalf("expected nil for empty bearer token, got %+v", line)
	}
}
