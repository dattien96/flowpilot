package changecontract

import (
	"reflect"
	"testing"
	"time"
)

func TestFrozenContractScopeDriftEmptyWhenWithinScope(t *testing.T) {
	rec := FrozenContractRecord{DeclaredPaths: []string{"src/calc.go", "src/util.go"}}
	if got := FrozenContractScopeDrift(rec, []string{"src/calc.go"}); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestFrozenContractScopeDriftReportsUnexpectedPaths(t *testing.T) {
	rec := FrozenContractRecord{DeclaredPaths: []string{"src/calc.go"}}
	got := FrozenContractScopeDrift(rec, []string{"src/calc.go", "src/other.go"})
	if !reflect.DeepEqual(got, []string{"src/other.go"}) {
		t.Fatalf("got %v, want [src/other.go]", got)
	}
}

func TestFrozenContractScopeDriftIgnoresBlankEntries(t *testing.T) {
	rec := FrozenContractRecord{DeclaredPaths: []string{"src/calc.go"}}
	got := FrozenContractScopeDrift(rec, []string{"", "   ", "src/calc.go"})
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestFrozenContractScopeDriftDedupesAndSorts(t *testing.T) {
	rec := FrozenContractRecord{DeclaredPaths: []string{"src/calc.go"}}
	got := FrozenContractScopeDrift(rec, []string{"z/z.go", "a/a.go", "z/z.go"})
	if !reflect.DeepEqual(got, []string{"a/a.go", "z/z.go"}) {
		t.Fatalf("got %v, want [a/a.go z/z.go]", got)
	}
}

func TestFrozenContractScopeDriftNormalizesEquivalentPaths(t *testing.T) {
	rec := FrozenContractRecord{DeclaredPaths: []string{"src/calc.go"}}
	// "./src/calc.go" and "src\\calc.go" are the same path in a different form
	// — must not read as drift.
	got := FrozenContractScopeDrift(rec, []string{"./src/calc.go", `src\calc.go`})
	if got != nil {
		t.Fatalf("got %v, want nil (equivalent forms of a declared path)", got)
	}
}

func TestFrozenContractScopeDriftNilDeclaredPathsFlagsEverything(t *testing.T) {
	rec := FrozenContractRecord{}
	got := FrozenContractScopeDrift(rec, []string{"src/calc.go"})
	if !reflect.DeepEqual(got, []string{"src/calc.go"}) {
		t.Fatalf("got %v, want [src/calc.go]", got)
	}
}

func TestIsFrozenStoreBookkeepingPathMatchesExactFiles(t *testing.T) {
	for _, p := range []string{
		".flowpilot/contracts/frozen_contracts.ndjson",
		".flowpilot/contracts/frozen_contract_events.ndjson",
		`.flowpilot\contracts\frozen_contracts.ndjson`,
	} {
		if !IsFrozenStoreBookkeepingPath(p) {
			t.Errorf("%q should be recognized as frozen-store bookkeeping", p)
		}
	}
}

func TestIsFrozenStoreBookkeepingPathRejectsEverythingElse(t *testing.T) {
	// CA-427 Finding 2: only the store's own two files are exempt — nothing
	// else under .flowpilot/**, and no doc/audit file either.
	for _, p := range []string{
		".flowpilot/settings/flow-rules.json",
		".flowpilot/canonical/calc-core.json",
		".flowpilot/canonical-pending/pending_canonical.ndjson",
		"requirements/08-Task/todo/Task-1.md",
		"change-audit/CA-1.md",
		"README.md",
		"src/calc.go",
	} {
		if IsFrozenStoreBookkeepingPath(p) {
			t.Errorf("%q must NOT be exempt from scope-drift comparison", p)
		}
	}
}

// TestIsPendingCanonicalStoreBookkeepingPathMatchesExactFiles is the
// regression test for the CP-55 P-8 finding: PendingCanonicalStore's own two
// files (CP-55 P-5) need the identical bookkeeping exemption FrozenStore's
// own two files already have — without it, a frozen writer's first gate pass
// staging a pending Canonical Head update would see that very write reported
// as scope drift on its OWN next gate pass (any validation-retry or
// review-loop retry), self-blocking every migrated Flow the moment it
// looped more than once.
func TestIsPendingCanonicalStoreBookkeepingPathMatchesExactFiles(t *testing.T) {
	for _, p := range []string{
		".flowpilot/canonical-pending/pending_canonical.ndjson",
		".flowpilot/canonical-pending/pending_canonical_events.ndjson",
		`.flowpilot\canonical-pending\pending_canonical.ndjson`,
	} {
		if !IsPendingCanonicalStoreBookkeepingPath(p) {
			t.Errorf("%q should be recognized as pending-canonical-store bookkeeping", p)
		}
	}
}

// TestIsPendingCanonicalStoreBookkeepingPathRejectsEverythingElse mirrors
// CA-427 Finding 2's own discipline for the new exemption: only the store's
// own two exact files are exempt — nothing else under .flowpilot/**
// (including FrozenStore's own files, which are a DIFFERENT store this
// function must not also recognize), and no doc/audit file either.
func TestIsPendingCanonicalStoreBookkeepingPathRejectsEverythingElse(t *testing.T) {
	for _, p := range []string{
		".flowpilot/contracts/frozen_contracts.ndjson",
		".flowpilot/settings/flow-rules.json",
		".flowpilot/canonical/calc-core.json",
		"requirements/08-Task/todo/Task-1.md",
		"change-audit/CA-1.md",
		"README.md",
		"src/calc.go",
	} {
		if IsPendingCanonicalStoreBookkeepingPath(p) {
			t.Errorf("%q must NOT be exempt from scope-drift comparison", p)
		}
	}
}

func newAmendTestFixture(t *testing.T) (dir string, store *FrozenStore, rec FrozenContractRecord) {
	t.Helper()
	dir = t.TempDir()
	var err error
	store, err = NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	draft := PreflightContractDraft{FeatureKey: "calc-core", Intent: "fix", DeclaredPaths: []string{"src/calc.go"}}
	rec, err = FreezeContract(dir, "run-1", "planner", "coder", draft, "sha1", nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}
	return dir, store, rec
}

func TestAmendFrozenContractWidensDeclaredPaths(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	amended, err := AmendFrozenContract(store, dir, rec, []string{"src/extra.go"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if amended.Version != 2 || amended.Supersedes != rec.ContractID {
		t.Fatalf("amended = %+v", amended)
	}
}

func TestAmendFrozenContractNoOpOnNilAdditionalPaths(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	same, err := AmendFrozenContract(store, dir, rec, nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if same.ContractID != rec.ContractID {
		t.Fatalf("expected a no-op returning the existing record unchanged, got %+v", same)
	}
}

func TestAmendFrozenContractNoOpOnAlreadyDeclaredSubset(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	same, err := AmendFrozenContract(store, dir, rec, []string{"src/calc.go"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if same.ContractID != rec.ContractID {
		t.Fatalf("re-declaring an already-declared path must be a no-op, got %+v", same)
	}
}

func TestAmendFrozenContractRejectsNonConcretePathExplicitly(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	if _, err := AmendFrozenContract(store, dir, rec, []string{"Makefile"}, time.Now().UTC()); err == nil {
		t.Fatal("expected an explicit error, not a silent no-op, for a non-concrete additional path")
	}
	if _, err := AmendFrozenContract(store, dir, rec, []string{"docs/readme.md"}, time.Now().UTC()); err == nil {
		t.Fatal("expected an explicit error for a doc-only additional path")
	}
	if _, err := AmendFrozenContract(store, dir, rec, []string{"internal/*.go"}, time.Now().UTC()); err == nil {
		t.Fatal("expected an explicit error for a glob additional path")
	}
}

func TestAmendFrozenContractPersistsAcrossReopen(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	amended, err := AmendFrozenContract(store, dir, rec, []string{"src/extra.go"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	reopened, err := NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	active, ok, err := reopened.GetFrozenForStep("run-1", "coder")
	if err != nil || !ok || active.ContractID != amended.ContractID {
		t.Fatalf("active=%+v ok=%v err=%v, want the amended version reloaded from disk", active, ok, err)
	}
	versions, err := reopened.ListVersionsForStep("run-1", "coder")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 persisted versions, got %d", len(versions))
	}
}
