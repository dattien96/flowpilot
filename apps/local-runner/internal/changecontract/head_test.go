package changecontract

import (
	"testing"
	"time"
)

func TestSaveHeadThenLoadHeadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	h := CanonicalHead{
		FeatureKey:        "calc-core",
		BehaviorStatement: "Divide returns an error on zero divisor",
		Status:            HeadStatusCurrent,
		SpecConfidence:    SpecConfidenceSpecBacked,
		UpdatedAt:         time.Now().UTC(),
	}
	h.IntentSignature = ComputeSignature(h)

	if err := SaveHead(dir, h); err != nil {
		t.Fatalf("SaveHead: %v", err)
	}

	loaded, ok, err := LoadHead(dir, "calc-core")
	if err != nil {
		t.Fatalf("LoadHead: %v", err)
	}
	if !ok {
		t.Fatal("expected LoadHead to find the saved Head")
	}
	if loaded.FeatureKey != h.FeatureKey || loaded.BehaviorStatement != h.BehaviorStatement {
		t.Fatalf("round-tripped Head mismatch: got %+v", loaded)
	}
	if loaded.IntentSignature != h.IntentSignature {
		t.Fatalf("IntentSignature mismatch after round-trip: %q != %q", loaded.IntentSignature, h.IntentSignature)
	}
}

func TestLoadHeadMissingFileReturnsOkFalseNoError(t *testing.T) {
	dir := t.TempDir()
	_, ok, err := LoadHead(dir, "never-existed")
	if err != nil {
		t.Fatalf("expected no error for a missing Head file, got %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for a missing Head file")
	}
}
