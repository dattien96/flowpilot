package flowgate

import (
	"strings"
	"testing"
)

// CP-67 P-2 (Task-379): r-signature-lock evaluator contract. Additive-only —
// no pre-existing flowgate test is edited.

func TestRuleSignatureLockFailsOnSignatureModification(t *testing.T) {
	before := CanonicalSignatureHash([]string{"func GetUser(id string) (*User, error)"})
	after := CanonicalSignatureHash([]string{"func GetUser(id string, forceRefresh bool) (*User, error)"})
	tr := TurnResult{
		SignatureHashBefore: before,
		SignatureHashAfter:  after,
		SignatureDrift:      []string{"modified: func GetUser(...)"},
	}
	v := checkSignatureLockRule(SignatureLockRule(), tr)
	if v == nil {
		t.Fatal("silent signature modification must violate r-signature-lock")
	}
	if !strings.Contains(v.Detail, "GetUser") {
		t.Fatalf("reprompt must name the drifted declaration, got %q", v.Detail)
	}
}

func TestRuleSignatureLockFailsOnAdditiveFunction(t *testing.T) {
	// B-8.1: adding a helper function shifts the signature-only hash — there
	// is no "additive change is fine" exemption.
	before := CanonicalSignatureHash([]string{"func GetUser(id string) (*User, error)"})
	after := CanonicalSignatureHash([]string{
		"func GetUser(id string) (*User, error)",
		"func validateID(id string) error",
	})
	tr := TurnResult{SignatureHashBefore: before, SignatureHashAfter: after}
	if v := checkSignatureLockRule(SignatureLockRule(), tr); v == nil {
		t.Fatal("adding a function must violate r-signature-lock (B-8.1 strict semantics)")
	}
}

func TestRuleSignatureLockPassesWhenOnlyBodyModified(t *testing.T) {
	// Body edits never move the signature-only hash — that is the coder's
	// whole job.
	before := CanonicalSignatureHash([]string{"func GetUser(id string) (*User, error)"})
	after := CanonicalSignatureHash([]string{"func GetUser(id string) (*User, error)"})
	tr := TurnResult{SignatureHashBefore: before, SignatureHashAfter: after}
	if v := checkSignatureLockRule(SignatureLockRule(), tr); v != nil {
		t.Fatalf("body-only edits must pass, got violation: %v", v.Detail)
	}
}

func TestRuleSignatureLockBypassesWhenRenegotiating(t *testing.T) {
	before := CanonicalSignatureHash([]string{"func GetUser(id string) (*User, error)"})
	after := CanonicalSignatureHash([]string{"func GetUser(id string, forceRefresh bool) (*User, error)"})
	tr := TurnResult{
		SignatureHashBefore: before,
		SignatureHashAfter:  after,
		CoderRenegotiating:  true,
	}
	if v := checkSignatureLockRule(SignatureLockRule(), tr); v != nil {
		t.Fatalf("a batched renegotiation is the legal bypass, got violation: %v", v.Detail)
	}
}

func TestRuleSignatureLockNoSignalWithoutSnapshot(t *testing.T) {
	// Pre-CP-67 contracts carry no hash — the rule must stay silent.
	tr := TurnResult{SignatureHashBefore: "", SignatureHashAfter: ""}
	if v := checkSignatureLockRule(SignatureLockRule(), tr); v != nil {
		t.Fatal("missing snapshot must be a no-signal pass")
	}
}

func TestEnabledSignatureLockRulesIdempotent(t *testing.T) {
	base := DefaultRules()
	once := EnabledSignatureLockRules(base, true)
	twice := EnabledSignatureLockRules(once, true)
	if len(once) != len(base)+1 || len(twice) != len(once) {
		t.Fatalf("append-once contract broken: base=%d once=%d twice=%d", len(base), len(once), len(twice))
	}
	if got := EnabledSignatureLockRules(base, false); len(got) != len(base) {
		t.Fatal("expected=false must return base unchanged")
	}
}
