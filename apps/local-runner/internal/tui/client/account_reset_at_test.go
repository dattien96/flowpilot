package client

import (
	"encoding/json"
	"testing"
)

func TestProviderAccountSummary_UnmarshalsResetAt(t *testing.T) {
	raw := []byte(`{
		"id": "acc-1",
		"provider_key": "grok",
		"display_label": "acct@example.com",
		"remaining_7d_percent": 87,
		"remaining_7d_reset_at": "2026-08-19T06:46:00Z",
		"remaining_5h_percent": 72,
		"remaining_5h_reset_at": "2026-08-13T12:00:00Z",
		"usage_detail_lines": [
			{"label": "Weekly limit", "remaining_percent": 99, "reset_at": "2026-08-19T06:46:00Z"}
		]
	}`)
	var acc ProviderAccountSummary
	if err := json.Unmarshal(raw, &acc); err != nil {
		t.Fatal(err)
	}
	if acc.Remaining7dPercent == nil || *acc.Remaining7dPercent != 87 {
		t.Fatalf("remaining_7d_percent=%v", acc.Remaining7dPercent)
	}
	if acc.Remaining7dResetAt == nil || *acc.Remaining7dResetAt != "2026-08-19T06:46:00Z" {
		t.Fatalf("remaining_7d_reset_at=%v", acc.Remaining7dResetAt)
	}
	if acc.Remaining5hResetAt == nil || *acc.Remaining5hResetAt != "2026-08-13T12:00:00Z" {
		t.Fatalf("remaining_5h_reset_at=%v", acc.Remaining5hResetAt)
	}
	if len(acc.UsageDetailLines) != 1 || acc.UsageDetailLines[0].ResetAt != "2026-08-19T06:46:00Z" {
		t.Fatalf("usage_detail_lines=%v", acc.UsageDetailLines)
	}
}
