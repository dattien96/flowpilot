package changecontract

import (
	"testing"
	"time"
)

func TestGetLatestForRunAppendOrderWinsOverDeclaredAt(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	oldTS := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newTS := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// First append: newer DeclaredAt
	if err := store.Save(Contract{
		RunID: "r1", StepID: "s1", FeatureKey: "first", DeclaredPaths: []string{"a.go"},
		Confidence: ConfidenceDeclared, DeclaredAt: newTS,
	}); err != nil {
		t.Fatal(err)
	}
	// Second append: older DeclaredAt must still win (append-order last-wins)
	if err := store.Save(Contract{
		RunID: "r1", StepID: "s2", FeatureKey: "second", DeclaredPaths: []string{"b.go"},
		Confidence: ConfidenceDeclared, DeclaredAt: oldTS,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok := store.GetLatestForRun("r1")
	if !ok {
		t.Fatal("expected contract")
	}
	if got.FeatureKey != "second" || got.DeclaredPaths[0] != "b.go" {
		t.Fatalf("want second append, got %+v", got)
	}
	// Reload from disk — append order must survive
	store2, err := OpenStoreReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	got2, ok := store2.GetLatestForRun("r1")
	if !ok || got2.FeatureKey != "second" {
		t.Fatalf("reload want second, got ok=%v %+v", ok, got2)
	}
}
