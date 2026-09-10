package changecontract

import (
	"strings"
	"testing"
	"time"
)

func TestBUG366_ForAllowAcceptsFeatureKeysAndStopsDrift(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	const extra = "change-audit/FEATURE-KEYS.md"

	if _, err := AmendFrozenContract(store, dir, rec, []string{extra}, time.Now().UTC()); err == nil {
		t.Fatal("AmendFrozenContract must still reject FEATURE-KEYS.md (CA-427 library contract)")
	}

	amended, err := AmendFrozenContractForAllow(store, dir, rec, []string{extra}, time.Now().UTC())
	if err != nil {
		t.Fatalf("ForAllow FEATURE-KEYS.md: %v", err)
	}
	if amended.ContractID == rec.ContractID {
		t.Fatal("ForAllow must mint a new version when extras are new")
	}
	if amended.Version != rec.Version+1 {
		t.Fatalf("Version = %d, want %d", amended.Version, rec.Version+1)
	}
	found := false
	for _, p := range amended.AllowedExtraPaths {
		if p == extra {
			found = true
		}
	}
	if !found {
		t.Fatalf("AllowedExtraPaths = %v, want %q", amended.AllowedExtraPaths, extra)
	}
	for _, p := range amended.DeclaredPaths {
		if p == extra {
			t.Fatal("FEATURE-KEYS.md must not enter DeclaredPaths (retrieval stays code-only)")
		}
	}

	if drift := FrozenContractScopeDrift(amended, []string{"src/calc.go", extra}); drift != nil {
		t.Fatalf("after Allow, FEATURE-KEYS.md must not be drift: %v", drift)
	}
}

func TestBUG366_ForAllowMixedConcreteAndDoc(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	amended, err := AmendFrozenContractForAllow(store, dir, rec, []string{
		"src/extra.go",
		"change-audit/FEATURE-KEYS.md",
		"README.md",
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	hasExtra := false
	for _, p := range amended.DeclaredPaths {
		if p == "src/extra.go" {
			hasExtra = true
		}
		if strings.HasSuffix(p, ".md") {
			t.Fatalf("DeclaredPaths leaked a doc path: %v", amended.DeclaredPaths)
		}
	}
	if !hasExtra {
		t.Fatalf("DeclaredPaths missing src/extra.go: %v", amended.DeclaredPaths)
	}
	wantExtras := map[string]bool{"change-audit/FEATURE-KEYS.md": false, "README.md": false}
	for _, p := range amended.AllowedExtraPaths {
		if _, ok := wantExtras[p]; ok {
			wantExtras[p] = true
		}
	}
	for p, ok := range wantExtras {
		if !ok {
			t.Fatalf("AllowedExtraPaths missing %q: %v", p, amended.AllowedExtraPaths)
		}
	}
	if drift := FrozenContractScopeDrift(amended, []string{"src/calc.go", "src/extra.go", "change-audit/FEATURE-KEYS.md", "README.md"}); drift != nil {
		t.Fatalf("mixed Allow must cover all written paths, drift=%v", drift)
	}
}

func TestBUG366_ForAllowStillRejectsGlobFlagAndMakefile(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	for _, p := range []string{"Makefile", "internal/*.go", "--repo=evil.go", "apps"} {
		if _, err := AmendFrozenContractForAllow(store, dir, rec, []string{p}, time.Now().UTC()); err == nil {
			t.Fatalf("ForAllow must still reject %q (CA-427 Finding 5)", p)
		}
	}
}

func TestBUG366_ForAllowNoOpWhenExtraAlreadyAllowed(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	first, err := AmendFrozenContractForAllow(store, dir, rec, []string{"change-audit/FEATURE-KEYS.md"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	second, err := AmendFrozenContractForAllow(store, dir, first, []string{"change-audit/FEATURE-KEYS.md"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if second.ContractID != first.ContractID {
		t.Fatalf("re-Allowing the same extra must be a no-op, got %+v", second)
	}
}

func TestBUG366_ConcreteAmendPreservesAllowedExtras(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	withExtra, err := AmendFrozenContractForAllow(store, dir, rec, []string{"change-audit/FEATURE-KEYS.md"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	widened, err := AmendFrozenContract(store, dir, withExtra, []string{"src/extra.go"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range widened.AllowedExtraPaths {
		if p == "change-audit/FEATURE-KEYS.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("later concrete amend dropped AllowedExtraPaths: %v", widened.AllowedExtraPaths)
	}
	if drift := FrozenContractScopeDrift(widened, []string{"src/extra.go", "change-audit/FEATURE-KEYS.md"}); drift != nil {
		t.Fatalf("preserved extras must still be in-scope, drift=%v", drift)
	}
}

func TestBUG366_IsUserAllowableDriftPath(t *testing.T) {
	allow := []string{
		"src/calc.go",
		"change-audit/FEATURE-KEYS.md",
		"README.md",
		"requirements/09-BugFix/todo/BUG-366.md",
		"docs/readme.md",
	}
	reject := []string{
		"",
		"Makefile",
		"apps",
		"internal/*.go",
		"--repo=evil.go",
	}
	for _, p := range allow {
		if !IsUserAllowableDriftPath(p) {
			t.Errorf("%q should be Allow-able", p)
		}
	}
	for _, p := range reject {
		if IsUserAllowableDriftPath(p) {
			t.Errorf("%q must not be Allow-able", p)
		}
	}
}
