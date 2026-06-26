package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/changeledger"
)

// mergeChatSummaryNDJSON imports only (run_id, feature_key) pairs not already
// present locally, so a Drive restore on a fresh machine seeds missing summaries
// without clobbering newer local ones (CP-37 Test C3, cross-PC sync-down).
func TestMergeChatSummaryNDJSONAdditive(t *testing.T) {
	dir := t.TempDir()
	ledger, err := changeledger.NewChatSummaryLedger(dir)
	if err != nil {
		t.Fatalf("NewChatSummaryLedger: %v", err)
	}

	// Local already has run-A/calc-core (authoritative on this machine).
	local := changeledger.ChatSummaryEntry{
		RunID: "run-A", TurnID: "t1", FeatureKey: "calc-core",
		StateKey: "local-A", Summary: "- local calc-core summary", CreatedAt: "2026-06-01T00:00:00Z",
	}
	if err := ledger.UpsertForRun(local); err != nil {
		t.Fatalf("seed local: %v", err)
	}

	// Drive payload: a stale copy of run-A (must NOT overwrite) + a new run-B/calc-format
	// from the other machine (must be imported) + a malformed line (skipped).
	drive := strings.Join([]string{
		`{"run_id":"run-A","turn_id":"t1","feature_key":"calc-core","state_key":"drive-A-stale","summary":"- DRIVE stale calc-core","created_at":"2026-05-01T00:00:00Z"}`,
		`{"run_id":"run-B","turn_id":"t9","feature_key":"calc-format","state_key":"drive-B","summary":"- drive calc-format summary","created_at":"2026-06-02T00:00:00Z"}`,
		`{not valid json`,
		``,
	}, "\n")

	imported, err := mergeChatSummaryNDJSON(ledger, []byte(drive))
	if err != nil {
		t.Fatalf("mergeChatSummaryNDJSON: %v", err)
	}
	if imported != 1 {
		t.Fatalf("imported = %d, want 1 (only the new run-B/calc-format)", imported)
	}

	// run-A stays the local copy (not clobbered by the stale Drive row).
	a, _ := ledger.GetFeatureSummariesForRun("calc-core", "run-A")
	if len(a) != 1 || a[0].StateKey != "local-A" {
		t.Fatalf("run-A should keep local copy, got %+v", a)
	}
	// run-B/calc-format was imported from Drive.
	b, _ := ledger.GetFeatureSummariesForRun("calc-format", "run-B")
	if len(b) != 1 || b[0].StateKey != "drive-B" {
		t.Fatalf("run-B/calc-format should be imported from drive, got %+v", b)
	}

	// Idempotent: a second merge of the same payload imports nothing.
	again, err := mergeChatSummaryNDJSON(ledger, []byte(drive))
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if again != 0 {
		t.Fatalf("second merge imported = %d, want 0 (idempotent)", again)
	}
}
