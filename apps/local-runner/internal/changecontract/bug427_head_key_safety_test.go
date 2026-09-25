package changecontract

import "testing"

// TestBug427_StageHeadWriteRejectsUnportableFeatureKey covers BUG-427(2): a
// feature key containing an NTFS-illegal character ("?", also < > : " / \ | *
// or a control char) or a traversal segment ("." / "..") must fail Head
// staging on every platform — the file can never be written portably, and a
// ".." key would escape .flowpilot/canonical entirely. The pre-existing
// finalize two-phase-commit regression test relies on this failing
// deterministically even on POSIX.
func TestBug427_StageHeadWriteRejectsUnportableFeatureKey(t *testing.T) {
	dir := t.TempDir()
	for _, key := range []string{
		"zzz-bad?feature",
		`bad\key`,
		"bad/key",
		"bad:key",
		"bad*key",
		"bad|key",
		"bad<key",
		`bad"key`,
		"bad>key",
		"..",
		".",
		"bad\x01key",
		"",
		"   ",
	} {
		_, _, err := StageHeadWrite(dir, CanonicalHead{FeatureKey: key, BehaviorStatement: "x"})
		if err == nil {
			t.Fatalf("expected StageHeadWrite to reject feature key %q", key)
		}
	}
}

// TestBug427_LoadHeadUnportableKeyIsAbsent mirrors the read side: an unsafe
// key can never correspond to a stored Head, so LoadHead must report it
// absent rather than following a traversal path.
func TestBug427_LoadHeadUnportableKeyIsAbsent(t *testing.T) {
	dir := t.TempDir()
	if _, found, err := LoadHead(dir, ".."); err != nil || found {
		t.Fatalf("LoadHead(%q): found=%v err=%v — unsafe keys must read as absent", "..", found, err)
	}
	if _, found, err := LoadHead(dir, "zzz-bad?feature"); err != nil || found {
		t.Fatalf("LoadHead(%q): found=%v err=%v — unsafe keys must read as absent", "zzz-bad?feature", found, err)
	}
}

// TestBug427_LegalKeysStillStage guards the negative space: ordinary
// kebab-case keys (incl. uppercase) must keep working byte-for-byte.
func TestBug427_LegalKeysStillStage(t *testing.T) {
	dir := t.TempDir()
	for _, key := range []string{"calc-core", "SD-01-Auth", "mega-noise", "a"} {
		tmp, final, err := StageHeadWrite(dir, CanonicalHead{FeatureKey: key, BehaviorStatement: "ok"})
		if err != nil {
			t.Fatalf("StageHeadWrite(%q) unexpectedly failed: %v", key, err)
		}
		if err := CommitHeadWrite(tmp, final); err != nil {
			t.Fatalf("CommitHeadWrite(%q): %v", key, err)
		}
		if _, found, err := LoadHead(dir, key); err != nil || !found {
			t.Fatalf("LoadHead(%q): found=%v err=%v", key, found, err)
		}
	}
}
