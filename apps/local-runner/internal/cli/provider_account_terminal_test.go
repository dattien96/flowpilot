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
