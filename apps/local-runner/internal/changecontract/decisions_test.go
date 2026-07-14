package changecontract

import (
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/changeledger"
)

func TestFoldDecisionsExtractsRejectedFromChatSummary(t *testing.T) {
	dir := t.TempDir()
	dotFP := filepath.Join(dir, ".flowpilot")
	chatLedger, err := changeledger.NewChatSummaryLedger(dotFP)
	if err != nil {
		t.Fatalf("NewChatSummaryLedger: %v", err)
	}
	if err := chatLedger.Append([]changeledger.ChatSummaryEntry{
		{
			RunID:      "run-1",
			FeatureKey: "calc-core",
			Summary:    "- Goal: add divide function.\n- Decision: tried a lookup-table approach, rejected because it did not handle negative divisors.\n- Adopted a direct arithmetic guard instead.",
			CreatedAt:  "2026-01-01T00:00:00Z",
		},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	decisions, err := FoldDecisions("calc-core", chatLedger, nil)
	if err != nil {
		t.Fatalf("FoldDecisions: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d: %+v", len(decisions), decisions)
	}
	if decisions[0].Outcome != DecisionRejected {
		t.Fatalf("expected outcome rejected, got %q", decisions[0].Outcome)
	}
	if decisions[0].At == nil {
		t.Fatal("expected a parsed decision timestamp")
	}
}

func TestFoldDecisionsIgnoresBulletsWithNoRejectionMarker(t *testing.T) {
	dir := t.TempDir()
	dotFP := filepath.Join(dir, ".flowpilot")
	chatLedger, err := changeledger.NewChatSummaryLedger(dotFP)
	if err != nil {
		t.Fatalf("NewChatSummaryLedger: %v", err)
	}
	if err := chatLedger.Append([]changeledger.ChatSummaryEntry{
		{RunID: "run-1", FeatureKey: "calc-core", Summary: "- Goal: add divide function.\n- Implemented direct arithmetic guard.", CreatedAt: "2026-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	decisions, err := FoldDecisions("calc-core", chatLedger, nil)
	if err != nil {
		t.Fatalf("FoldDecisions: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("expected no decisions, got %+v", decisions)
	}
}

func TestFoldDecisionsExtractsRevertedFromLedgerBugfix(t *testing.T) {
	dir := t.TempDir()
	dotFP := filepath.Join(dir, ".flowpilot")
	ledger, err := changeledger.New(dotFP)
	if err != nil {
		t.Fatalf("changeledger.New: %v", err)
	}
	if err := ledger.Upsert([]changeledger.Entry{
		{
			CommitHash:  "abc123",
			FeatureKey:  "calc-core",
			ChangeType:  "bugfix",
			SourceDocID: "BUG-999",
			Summary:     "revert the lookup-table divide approach, it broke negative divisors",
			CommittedAt: "2026-01-02T00:00:00Z",
			OrderIndex:  1,
			Confidence:  "high",
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	decisions, err := FoldDecisions("calc-core", nil, ledger)
	if err != nil {
		t.Fatalf("FoldDecisions: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d: %+v", len(decisions), decisions)
	}
	if decisions[0].Outcome != DecisionReverted {
		t.Fatalf("expected outcome reverted, got %q", decisions[0].Outcome)
	}
	if decisions[0].SourceDocID != "BUG-999" {
		t.Fatalf("expected source_doc_id BUG-999, got %q", decisions[0].SourceDocID)
	}
}

func TestFoldDecisionsIgnoresNonBugfixLedgerEntries(t *testing.T) {
	dir := t.TempDir()
	dotFP := filepath.Join(dir, ".flowpilot")
	ledger, err := changeledger.New(dotFP)
	if err != nil {
		t.Fatalf("changeledger.New: %v", err)
	}
	if err := ledger.Upsert([]changeledger.Entry{
		{
			CommitHash:  "abc123",
			FeatureKey:  "calc-core",
			ChangeType:  "feature",
			Summary:     "revert-sounding text but not a bugfix",
			CommittedAt: "2026-01-02T00:00:00Z",
			OrderIndex:  1,
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	decisions, err := FoldDecisions("calc-core", nil, ledger)
	if err != nil {
		t.Fatalf("FoldDecisions: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("expected no decisions for a non-bugfix entry, got %+v", decisions)
	}
}

func TestFoldDecisionsNilSourcesReturnsEmptyNotPanic(t *testing.T) {
	decisions, err := FoldDecisions("calc-core", nil, nil)
	if err != nil {
		t.Fatalf("expected no error with nil sources, got %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("expected no decisions with nil sources, got %+v", decisions)
	}
}
