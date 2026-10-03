package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-617 (live run-150388 / Task-032 audit draft): PrivateVault's
// change-audit/FEATURE-KEYS.md writes keys backtick-quoted —
// "- `crypto-ndk` — Native C++20 ..." — while featureKeyRegistered matched
// only the unquoted "- crypto-ndk " form. Every registered key read as
// unregistered → every vibe audit drafted blocked_missing_feature_key and
// could never finalize, so no sprint ever committed its task file todo→done.
// The auto-finalize path (CA-1096) papers over the block for the hub, but
// the draft status stays a lie and Task-032's audit card showed exactly
// that.
func TestFeatureKeyRegisteredBacktickQuotedRegistry(t *testing.T) {
	workspace := t.TempDir()
	ca := filepath.Join(workspace, "change-audit")
	if err := os.MkdirAll(ca, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := "- `crypto-ndk` — Native C++20 Argon2id KDF\n" +
		"- `feature-vault` — Zero-Knowledge Safe Locker\n"
	if err := os.WriteFile(filepath.Join(ca, "FEATURE-KEYS.md"), []byte(registry), 0o644); err != nil {
		t.Fatal(err)
	}
	if !featureKeyRegistered(workspace, "crypto-ndk") {
		t.Fatal("backtick-quoted registry key read as unregistered — BUG-617 repro")
	}
	if !featureKeyRegistered(workspace, "feature-vault") {
		t.Fatal("second backtick-quoted key read as unregistered")
	}
	// Prefix-safety: `crypto-ndk` must not match `crypto-ndk-extra`.
	if err := os.WriteFile(filepath.Join(ca, "FEATURE-KEYS.md"),
		[]byte("- `crypto-ndk-extra` — other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if featureKeyRegistered(workspace, "crypto-ndk") {
		t.Fatal("prefix collision: crypto-ndk matched crypto-ndk-extra")
	}
}
