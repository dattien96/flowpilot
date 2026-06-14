package cli

import (
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/runner"
)

func TestBuildProviderAccountSummaryResponsesSortsAndFallsBack(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	accounts := []runner.ProviderAccount{
		{
			ID:          "codex-inactive",
			ProviderKey: "codex",
			DisplayName: "Codex Inactive",
			HomePath:    filepath.Join(base, "codex-inactive"),
			SlotIndex:   2,
			IsActive:    false,
			AuthStatus:  "connected",
			CreatedAt:   "2026-06-12T10:00:00Z",
		},
		{
			ID:          "claude-failed",
			ProviderKey: "claude",
			DisplayName: "Claude Failed",
			HomePath:    filepath.Join(base, "claude-failed"),
			SlotIndex:   1,
			IsActive:    false,
			AuthStatus:  "failed",
			CreatedAt:   "2026-06-12T10:00:00Z",
		},
		{
			ID:          "codex-active",
			ProviderKey: "codex",
			DisplayName: "Codex Active",
			HomePath:    filepath.Join(base, "codex-active"),
			SlotIndex:   1,
			IsActive:    true,
			AuthStatus:  "connected",
			CreatedAt:   "2026-06-12T10:00:00Z",
		},
	}

	got, err := buildProviderAccountSummaryResponses(accounts)
	if err != nil {
		t.Fatalf("buildProviderAccountSummaryResponses() error = %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 summaries, got %d", len(got))
	}

	if got[0].ProviderKey != "claude" || got[0].ID != "claude-failed" {
		t.Fatalf("expected claude summary first, got %#v", got[0])
	}
	if got[1].ProviderKey != "codex" || got[1].ID != "codex-active" {
		t.Fatalf("expected active codex summary second, got %#v", got[1])
	}
	if got[2].ProviderKey != "codex" || got[2].ID != "codex-inactive" {
		t.Fatalf("expected inactive codex summary third, got %#v", got[2])
	}

	if got[1].DisplayLabel != "Codex Active" {
		t.Fatalf("expected display label fallback to display name, got %q", got[1].DisplayLabel)
	}
	if got[1].AuthStorePath == nil || *got[1].AuthStorePath == "" {
		t.Fatalf("expected auth store path fallback, got %#v", got[1].AuthStorePath)
	}
	if got[1].UsageSource != "unavailable" {
		t.Fatalf("expected unavailable usage source without auth metadata, got %q", got[1].UsageSource)
	}
}
