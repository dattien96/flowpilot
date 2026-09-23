package changecontract

import (
	"path/filepath"
	"testing"
	"time"
)

// BUG-388 (live): the reproduce gate stored WrittenPaths verbatim — absolute
// paths — into ReadOnlyPaths, while the approval bridge relativizes its
// candidate to the workspace. normalizeScopePath never relativizes, so the two
// sides never matched and the coder overwrote the locked RED test twice with
// no reproduce_test_locked deny.
func TestBug388_LockReproduceTestPathsStoresWorkspaceRelative(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	draft := PreflightContractDraft{FeatureKey: "calc", Intent: "fix", DeclaredPaths: []string{"calc/calc.go"}}
	rec, err := FreezeContract(dir, "run-1", "planner", "implement", draft, "sha", nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(dir, "calc", "repro_test.go")
	locked, err := LockReproduceTestPaths(dir, "run-1", "implement", []string{abs}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !IsReadOnlyLockedPath(locked, "calc/repro_test.go") {
		t.Fatalf("absolute written path must be locked read-only; ReadOnlyPaths=%v", locked.ReadOnlyPaths)
	}
	// Relative inputs still lock.
	locked2, err := LockReproduceTestPaths(dir, "run-1", "implement", []string{"calc/other_test.go"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !IsReadOnlyLockedPath(locked2, "calc/other_test.go") {
		t.Fatalf("relative path lock regressed: %v", locked2.ReadOnlyPaths)
	}
}

// Records frozen before the fix carry absolute ReadOnlyPaths. A workspace-aware
// lookup must still deny writes to them (in-flight runs must not silently
// unlock on upgrade).
func TestBug388_LegacyAbsoluteReadOnlyPathStillMatches(t *testing.T) {
	dir := t.TempDir()
	rec := FrozenContractRecord{
		ReadOnlyPaths: []string{filepath.Join(dir, "calc", "repro_test.go")},
	}
	if !IsReadOnlyLockedPathUnder(rec, "calc/repro_test.go", dir) {
		t.Fatal("legacy absolute ReadOnlyPath must match the relative candidate under the workspace")
	}
	if IsReadOnlyLockedPathUnder(rec, "calc/other_test.go", dir) {
		t.Fatal("non-locked path must not match")
	}
	// Without a workspace the legacy absolute path cannot be relativized — it
	// must not silently claim a match (fail visible, not fail-masked).
	if IsReadOnlyLockedPathUnder(rec, "calc/repro_test.go", "") {
		t.Fatal("no workspace -> cannot relativize absolute stored path")
	}
}
