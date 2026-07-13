package changecontract

import (
	"path/filepath"
	"testing"
)

func TestStoreSaveAndGetRoundTrip(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	c := Contract{
		RunID:      "run-1",
		StepID:     "step-1",
		FeatureKey: "calc-core",
		Intent:     "add DivideChecked",
		Confidence: ConfidenceDeclared,
	}
	if err := store.Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, ok := store.Get("run-1", "step-1")
	if !ok {
		t.Fatal("expected Get to find the saved contract")
	}
	if got.FeatureKey != "calc-core" || got.Intent != "add DivideChecked" {
		t.Errorf("got = %+v, want feature_key=calc-core intent=%q", got, "add DivideChecked")
	}
}

func TestStoreLastWinsByRunAndStep(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.Save(Contract{RunID: "run-1", StepID: "step-1", Intent: "first"}); err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	if err := store.Save(Contract{RunID: "run-1", StepID: "step-1", Intent: "second"}); err != nil {
		t.Fatalf("Save 2: %v", err)
	}

	got, ok := store.Get("run-1", "step-1")
	if !ok {
		t.Fatal("expected a contract for (run-1, step-1)")
	}
	if got.Intent != "second" {
		t.Errorf("Intent = %q, want %q (last write wins)", got.Intent, "second")
	}
}

func TestStoreGetMissingReturnsFalse(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, ok := store.Get("no-such-run", "no-such-step"); ok {
		t.Fatal("expected ok=false for a run/step never saved")
	}
}

func TestStoreLoadsExistingFileOnReopen(t *testing.T) {
	dir := t.TempDir()
	first, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := first.Save(Contract{RunID: "run-1", StepID: "step-1", Intent: "persisted"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	second, err := NewStore(dir) // simulate a process restart
	if err != nil {
		t.Fatalf("NewStore (reopen): %v", err)
	}
	got, ok := second.Get("run-1", "step-1")
	if !ok {
		t.Fatal("expected the reopened store to load the persisted contract from disk")
	}
	if got.Intent != "persisted" {
		t.Errorf("Intent = %q, want %q", got.Intent, "persisted")
	}
}

func TestStoreFileLivesUnderFlowpilotContracts(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	want := filepath.Join(dir, ".flowpilot", "contracts", "contracts.ndjson")
	if store.filePath != want {
		t.Errorf("filePath = %q, want %q", store.filePath, want)
	}
}
