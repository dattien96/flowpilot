package changecontract

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeSignatureReproducible(t *testing.T) {
	h := CanonicalHead{
		FeatureKey:         "calc-core",
		GoverningDocIDs:    []string{"SS-14", "SD-21"},
		GoverningDocHashes: map[string]string{"SS-14": "aaa", "SD-21": "bbb"},
		BehaviorStatement:  "Divide returns an error on zero divisor",
	}
	sig1 := ComputeSignature(h)
	sig2 := ComputeSignature(h)
	if sig1 != sig2 {
		t.Fatalf("ComputeSignature is not reproducible: %q != %q", sig1, sig2)
	}
	if sig1 == "" {
		t.Fatal("ComputeSignature returned empty string")
	}
}

func TestComputeSignatureSensitiveToDocHashChange(t *testing.T) {
	h := CanonicalHead{
		FeatureKey:         "calc-core",
		GoverningDocIDs:    []string{"SS-14"},
		GoverningDocHashes: map[string]string{"SS-14": "aaa"},
		BehaviorStatement:  "same behavior",
	}
	before := ComputeSignature(h)

	h.GoverningDocHashes["SS-14"] = "changed-hash"
	after := ComputeSignature(h)

	if before == after {
		t.Fatal("expected a changed governing doc hash to change the signature")
	}
}

func TestComputeSignatureInsensitiveToWhitespaceOnlyBehaviorChange(t *testing.T) {
	h1 := CanonicalHead{FeatureKey: "calc-core", BehaviorStatement: "Divide by zero returns an error"}
	h2 := CanonicalHead{FeatureKey: "calc-core", BehaviorStatement: "  Divide   by zero returns   an error  "}
	if ComputeSignature(h1) != ComputeSignature(h2) {
		t.Fatal("expected whitespace-only behavior_statement differences to yield the same signature")
	}
}

func TestComputeSignatureSensitiveToFeatureKey(t *testing.T) {
	h1 := CanonicalHead{FeatureKey: "calc-core", BehaviorStatement: "x"}
	h2 := CanonicalHead{FeatureKey: "user-id", BehaviorStatement: "x"}
	if ComputeSignature(h1) == ComputeSignature(h2) {
		t.Fatal("expected different feature_key to yield a different signature")
	}
}

func TestHashDocReturnsErrorNotPanicOnMissingFile(t *testing.T) {
	_, err := HashDoc(filepath.Join(t.TempDir(), "does-not-exist.md"))
	if err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}

func TestHashDocStableForSameContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SS-14.md")
	if err := os.WriteFile(path, []byte("# SS-14\n\nsome spec text"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	h1, err := HashDoc(path)
	if err != nil {
		t.Fatalf("HashDoc: %v", err)
	}
	h2, err := HashDoc(path)
	if err != nil {
		t.Fatalf("HashDoc (2nd): %v", err)
	}
	if h1 != h2 {
		t.Fatalf("HashDoc not stable: %q != %q", h1, h2)
	}
}

func TestHashDocChangesWithContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SS-14.md")
	os.WriteFile(path, []byte("version 1"), 0o644)
	h1, _ := HashDoc(path)
	os.WriteFile(path, []byte("version 2"), 0o644)
	h2, _ := HashDoc(path)
	if h1 == h2 {
		t.Fatal("expected a content change to change the hash")
	}
}
