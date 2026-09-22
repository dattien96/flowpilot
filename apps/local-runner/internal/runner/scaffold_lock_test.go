package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

func writeRepoFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// CP-67 P-5 (Task-382 T-5): a passed scaffold turn locks its test files
// read-only AND pins the signature hash in ONE version bump.

func TestScaffoldTestLockAndSignatureSnapshotSingleVersion(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newReproduceFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "implement", head, []string{"calc/calc.go"})

	writeRepoFile(t, dir, "calc/calc.go", "package calc\n\nimport \"errors\"\n\n// Add returns the sum of a and b.\nfunc Add(a, b int) error {\n\treturn errors.New(\"not implemented\")\n}\n")
	writeRepoFile(t, dir, "calc/scaffold_test.go", "package calc\n\nimport \"testing\"\n\nfunc TestScaffoldRed(t *testing.T) {\n\tif err := Add(1, 2); err == nil {\n\t\tt.Fatal(\"expected not-implemented\")\n\t}\n}\n")

	svc.recordScaffoldArtifactsLock(dir, parentID, []string{
		"calc/calc.go",
		"calc/scaffold_test.go",
	})

	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, err := store.GetFrozenForStep(parentID, "implement")
	if err != nil || !ok {
		t.Fatalf("frozen contract missing: ok=%v err=%v", ok, err)
	}
	if rec.Version != 2 {
		t.Fatalf("version = %d, want ONE bump to 2 (B-8.3 single write)", rec.Version)
	}
	locked := changecontract.ReadOnlyLockedPaths(rec)
	if len(locked) != 1 || filepath.Base(locked[0]) != "scaffold_test.go" {
		t.Fatalf("ReadOnlyPaths = %v, want only the scaffold test file", locked)
	}
	if changecontract.IsReadOnlyLockedPath(rec, "calc/calc.go") {
		t.Fatal("the stub file must NOT be read-only — the coder fills its body")
	}
	if rec.SignatureHash == "" {
		t.Fatal("SignatureHash must be pinned after a passing scaffold turn")
	}
	foundSig := false
	for _, sig := range rec.LockedSignatures {
		if contains(sig, "func Add(") {
			foundSig = true
		}
	}
	if !foundSig {
		t.Fatalf("LockedSignatures must include the Add signature, got %v", rec.LockedSignatures)
	}
	// And the lock is enforced at the bridge.
	coder := newReproduceChildRun(svc, "child-coder", parentID, dir, head, "implement")
	if _, _, handled := svc.decideReproduceTestLock(coder, ApprovalDetails{Kind: "file", Command: "calc/scaffold_test.go", Reason: "Write"}); !handled {
		t.Fatal("the locked scaffold test file must be silent-denied at the bridge")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestScaffoldBodyViolationFeedsGateSignal pins the runner-side static body
// check wiring: a non-stub body is detected, a canonical stub is not, and the
// violations carry symbol:line for the reprompt.
func TestScaffoldBodyViolationFeedsGateSignal(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "svc.go", "package svc\n\nfunc GetUser(id string) (*User, error) {\n\tif id == \"\" {\n\t\treturn nil, nil\n\t}\n\treturn nil, nil\n}\n")
	nonStub, syms := (&InteractiveService{}).scaffoldStaticBodyViolations(dir, []string{"svc.go"})
	if !nonStub || len(syms) == 0 {
		t.Fatalf("control-flow body must be flagged, got %v %v", nonStub, syms)
	}

	writeRepoFile(t, dir, "stub.go", "package svc\n\nfunc Zero() (int, error) {\n\treturn 0, nil\n}\n")
	clean, syms2 := (&InteractiveService{}).scaffoldStaticBodyViolations(dir, []string{"stub.go"})
	if clean || len(syms2) != 0 {
		t.Fatalf("canonical stub must pass the static whitelist, got %v %v", clean, syms2)
	}
	// Sanity: the language router knows all four CP-67 language families.
	for _, ext := range []string{".go", ".tsx", ".kt", ".cpp"} {
		if flowgate.LangForPath("x"+ext) == "" {
			t.Fatalf("LangForPath must map %s", ext)
		}
	}
}
