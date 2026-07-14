package changecontract

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
)

func writeGoverningDoc(t *testing.T, workspace, stem, content string) {
	t.Helper()
	dir := filepath.Join(workspace, "requirements", "05-System-Specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, stem+".md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
}

func TestBuildHeadBirthWithNoHistoryOrDocsIsSpecLess(t *testing.T) {
	dir := t.TempDir()
	birth := &Contract{Intent: "add zero-divisor guard"}

	h := BuildHead(dir, "calc-core", nil, nil, birth)

	if h.Status != HeadStatusSpecLess {
		t.Fatalf("expected status spec_less, got %q", h.Status)
	}
	if h.SpecConfidence != SpecConfidenceSpecLess {
		t.Fatalf("expected spec_confidence spec_less, got %q", h.SpecConfidence)
	}
	if h.BehaviorStatement != "add zero-divisor guard" {
		t.Fatalf("expected behavior_statement from birth contract, got %q", h.BehaviorStatement)
	}
	if h.IntentSignature == "" {
		t.Fatal("expected a non-empty intent_signature")
	}
}

func TestBuildHeadWithGoverningDocsIsSpecBacked(t *testing.T) {
	dir := t.TempDir()
	writeGoverningDoc(t, dir, "SS-14-Code-Context-And-Regression-Safety", "spec content")

	catalog := featurecatalog.New()
	catalog.Add(featurecatalog.Feature{
		Key:     "calc-core",
		DocRefs: []string{"SS-14-Code-Context-And-Regression-Safety"},
	})

	h := BuildHead(dir, "calc-core", nil, catalog, nil)

	if h.Status != HeadStatusCurrent {
		t.Fatalf("expected status current, got %q", h.Status)
	}
	if h.SpecConfidence != SpecConfidenceSpecBacked {
		t.Fatalf("expected spec_confidence spec_backed, got %q", h.SpecConfidence)
	}
	if h.GoverningDocHashes["SS-14-Code-Context-And-Regression-Safety"] == "" {
		t.Fatal("expected a non-empty hash recorded for the governing doc")
	}
}

func TestBuildHeadBackfillsFromLedgerHistory(t *testing.T) {
	dir := t.TempDir()
	ledger, err := changeledger.New(filepath.Join(dir, ".flowpilot"))
	if err != nil {
		t.Fatalf("changeledger.New: %v", err)
	}
	if err := ledger.Upsert([]changeledger.Entry{
		{
			CommitHash:  "abc123",
			FeatureKey:  "calc-core",
			ChangeType:  "feature",
			Summary:     "add divide function",
			CAExcerpt:   "Divide returns an error on zero divisor",
			CommittedAt: "2026-01-01T00:00:00Z",
			OrderIndex:  1,
			Confidence:  "high",
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	h := BuildHead(dir, "calc-core", ledger, nil, nil)

	if h.HeadCommit != "abc123" {
		t.Fatalf("expected head_commit backfilled from ledger, got %q", h.HeadCommit)
	}
	if h.BehaviorStatement != "Divide returns an error on zero divisor" {
		t.Fatalf("expected behavior_statement to prefer CA excerpt over summary, got %q", h.BehaviorStatement)
	}
}

func TestResolveDocRefPathMissingDocReturnsEmptyNotPanic(t *testing.T) {
	dir := t.TempDir()
	if got := resolveDocRefPath(dir, "does-not-exist-anywhere"); got != "" {
		t.Fatalf("expected empty path for a missing doc ref, got %q", got)
	}
}

func TestHashGoverningDocsRecordsEmptyHashForMissingDoc(t *testing.T) {
	dir := t.TempDir()
	hashes := hashGoverningDocs(dir, []string{"missing-doc"})
	if hash, ok := hashes["missing-doc"]; !ok || hash != "" {
		t.Fatalf("expected an empty hash entry for a missing doc, got %q (present=%v)", hash, ok)
	}
}
