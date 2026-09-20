package changecontract

import (
	"strings"
	"testing"
	"time"
)

// CP-67 P-2 (Task-379 B-8.3): the scaffold handover must land as ONE version
// bump — tests read-only + signature hash + locked signatures together.

func TestLockScaffoldArtifactsSingleVersionBump(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFrozenStore(dir)
	if err != nil {
		t.Fatalf("NewFrozenStore: %v", err)
	}
	existing := FrozenContractRecord{
		ContractID:    "c-1",
		Version:       1,
		RunID:         "run-1",
		CoderStepID:   "implement",
		FeatureKey:    "contract-first-tdd",
		Intent:        "scaffold lock test",
		DeclaredPaths: []string{"service.go"},
		DeclaredAt:    time.Now().UTC(),
	}
	if err := store.SaveFrozen(existing); err != nil {
		t.Fatalf("SaveFrozen: %v", err)
	}

	locked, err := LockScaffoldArtifacts(store, existing,
		[]string{"service_test.go"},
		"abc123",
		[]string{"func GetUser(id string) (*User, error)"},
		time.Now().UTC())
	if err != nil {
		t.Fatalf("LockScaffoldArtifacts: %v", err)
	}
	if locked.Version != 2 {
		t.Fatalf("version = %d, want exactly ONE bump to 2", locked.Version)
	}
	found := false
	for _, p := range locked.ReadOnlyPaths {
		if strings.HasSuffix(p, "service_test.go") {
			found = true
		}
	}
	if !found {
		t.Fatalf("ReadOnlyPaths must contain the test file, got %v", locked.ReadOnlyPaths)
	}
	if locked.SignatureHash != "abc123" {
		t.Fatalf("SignatureHash = %q, want abc123", locked.SignatureHash)
	}
	if len(locked.LockedSignatures) != 1 || !strings.Contains(locked.LockedSignatures[0], "GetUser") {
		t.Fatalf("LockedSignatures = %v, want the GetUser signature", locked.LockedSignatures)
	}

	// Re-locking the same content is a no-op — no pointless version churn.
	again, err := LockScaffoldArtifacts(store, locked,
		[]string{"service_test.go"},
		"abc123",
		[]string{"func GetUser(id string) (*User, error)"},
		time.Now().UTC())
	if err != nil {
		t.Fatalf("re-lock: %v", err)
	}
	if again.Version != 2 {
		t.Fatalf("no-op re-lock must not bump the version, got v%d", again.Version)
	}
}
