package cli

import "testing"

func TestGrokQuotaFromCreditsFormat_OmitsUsagePercentAsFullRemaining(t *testing.T) {
	// Live grok 1.0.3 GET /v1/billing?format=credits omits creditUsagePercent
	// when it is 0 (serde skip). CLI still has a weekly remaining of 100%.
	line := grokQuotaFromBilling(grokBillingResponse{
		Config: grokBillingConfig{
			CurrentPeriod: &grokBillingPeriod{
				Type: "USAGE_PERIOD_TYPE_WEEKLY",
				End:  "2026-08-19T23:46:37.792216+00:00",
			},
			BillingPeriodEnd: "2026-08-19T23:46:37.792216+00:00",
		},
	})
	if line == nil {
		t.Fatal("expected weekly remaining from credits format")
	}
	if line.label != "Weekly limit" {
		t.Fatalf("label=%q", line.label)
	}
	if line.remainingPercent != 100 {
		t.Fatalf("remainingPercent=%d want 100 when creditUsagePercent omitted", line.remainingPercent)
	}
	if line.resetAt != "2026-08-19T23:46:37Z" {
		t.Fatalf("resetAt=%q", line.resetAt)
	}
}

func TestGrokQuotaFromCreditsFormat_UsedPercentMapsToRemaining(t *testing.T) {
	used := 1.0
	line := grokQuotaFromBilling(grokBillingResponse{
		Config: grokBillingConfig{
			CreditUsagePercent: &used,
			CurrentPeriod: &grokBillingPeriod{
				Type: "USAGE_PERIOD_TYPE_WEEKLY",
				End:  "2026-07-16T07:31:03.807421+00:00",
			},
		},
	})
	if line == nil {
		t.Fatal("expected weekly remaining")
	}
	if line.remainingPercent != 99 {
		t.Fatalf("remainingPercent=%d want 99 (1%% used)", line.remainingPercent)
	}
}

func TestApplyGrokQuotaLine_SetsRemaining7dForWeekly(t *testing.T) {
	var meta accountLaunchMetadata
	applyGrokQuotaLine(&meta, &usageDetailLine{
		label:            "Weekly limit",
		remainingPercent: 87,
		resetAt:          "2026-08-19T23:46:37Z",
	})
	if len(meta.usageDetailLines) != 1 {
		t.Fatalf("usageDetailLines=%d", len(meta.usageDetailLines))
	}
	if meta.remaining7dPercent == nil || *meta.remaining7dPercent != 87 {
		t.Fatalf("remaining7dPercent=%v want 87", meta.remaining7dPercent)
	}
	if meta.remaining7dResetAt != "2026-08-19T23:46:37Z" {
		t.Fatalf("remaining7dResetAt=%q", meta.remaining7dResetAt)
	}
}

func TestApplyGrokQuotaLine_TeamCreditsDoesNotSet7d(t *testing.T) {
	var meta accountLaunchMetadata
	applyGrokQuotaLine(&meta, &usageDetailLine{
		label:            "Team Credits (Monthly)",
		remainingPercent: 40,
	})
	if meta.remaining7dPercent != nil {
		t.Fatalf("team credits must not fill remaining7d, got %d", *meta.remaining7dPercent)
	}
}
