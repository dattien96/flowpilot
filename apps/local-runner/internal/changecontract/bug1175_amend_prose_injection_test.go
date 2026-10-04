package changecontract

import (
	"strings"
	"testing"
	"time"
)

// Live sample from run-183756 (Task-038 sprint): the scope-drift gate reason
// concatenates the drift list with "; not written via this leg's tool calls:
// ..." and " — if these are operator edits, amend ..." prose. Parsed naively,
// the whole tail was injected into declared_paths as ONE entry — the real path
// stayed unsanctioned so every Allow looped.
const bug1175GarbagePath = "core/crypto-ndk/src/main/cpp/CMakeLists.txt; not written via this leg's tool calls: core/crypto-ndk/src/main/cpp/CMakeLists.txt — if these are operator edits"

// TestIsConcreteCodeTarget_RejectsProseInjectedPath: prose-punctuated strings
// must never be treated as concrete code targets — filepath.Ext on such a
// string returns a non-empty "extension" (".txt — if these...") which slipped
// the garbage through every layer into declared_paths.
func TestIsConcreteCodeTarget_RejectsProseInjectedPath(t *testing.T) {
	for _, p := range []string{
		bug1175GarbagePath,
		"amend the contract to sanction them (no progress since last continue)",
		"foo.go; rm -rf /",
		"a/b.go — em dash tail",
		"scheme:http/x.go",
		"quoted'file.go",
		`quoted"file.go`,
		"back`tick.go",
	} {
		if IsConcreteCodeTarget(p) {
			t.Fatalf("IsConcreteCodeTarget(%q)=true, want false", p)
		}
		if IsUserAllowableDriftPath(p) {
			t.Fatalf("IsUserAllowableDriftPath(%q)=true, want false", p)
		}
	}
}

// TestIsConcreteCodeTarget_LegitPathsSurvive: real repo-relative paths —
// including ones with spaces in directory names — keep passing. Doc-class
// files (.txt/.md) are still not concrete targets by design; they route to
// AllowedExtraPaths via IsUserAllowableDriftPath instead.
func TestIsConcreteCodeTarget_LegitPathsSurvive(t *testing.T) {
	for _, p := range []string{
		"apps/local-runner/internal/runner/gate_hook.go",
		"core/crypto-ndk/src/main/cpp/src/VaultService.cpp",
		"dir with space/file.go",
		"path/with(parens).go",
	} {
		if !IsConcreteCodeTarget(p) {
			t.Fatalf("IsConcreteCodeTarget(%q)=false, want true", p)
		}
	}
	for _, p := range []string{
		"core/crypto-ndk/src/main/cpp/CMakeLists.txt",
		"change-audit/FEATURE-KEYS.md",
		"requirements/08-Task/done/Task-038-crypto-ndk.md",
	} {
		if IsConcreteCodeTarget(p) {
			t.Fatalf("IsConcreteCodeTarget(%q)=true, want false (doc-class routes to extras)", p)
		}
		if !IsUserAllowableDriftPath(p) {
			t.Fatalf("IsUserAllowableDriftPath(%q)=false, want true (Allow must still widen scope)", p)
		}
	}
}

// TestStripDriftProseTail: the amend endpoint recovers the real path when a
// client forwards the gate reason's prose tail as part of a path entry.
func TestStripDriftProseTail(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{bug1175GarbagePath, "core/crypto-ndk/src/main/cpp/CMakeLists.txt"},
		{"a.go; not written via this leg's tool calls: a.go — tail", "a.go"},
		{"a.go — if these are operator edits", "a.go"},
		{"a.go – en dash", "a.go"},
		{"core/a.go", "core/a.go"},
		{"; only prose", ""},
		{"", ""},
	} {
		if got := StripDriftProseTail(tt.in); got != tt.want {
			t.Fatalf("StripDriftProseTail(%q)=%q want %q", tt.in, got, tt.want)
		}
	}
}

// TestAmendFrozenContractForAllow_RejectsGarbageEntry: the amend path itself is
// the last line of defense — a prose-injected entry must surface the CA-427
// explicit error, never land in DeclaredPaths.
func TestAmendFrozenContractForAllow_RejectsGarbageEntry(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	_, err := AmendFrozenContractForAllow(store, dir, rec, []string{bug1175GarbagePath}, time.Now().UTC())
	if err == nil {
		t.Fatal("AmendFrozenContractForAllow accepted a prose-injected path — want explicit error")
	}
	if !strings.Contains(err.Error(), "not a concrete code target") {
		t.Fatalf("error %q does not explain rejection", err)
	}
}

// TestAmendFrozenContractForAllow_MixedGarbageAndReal: a batch holding one
// garbage entry plus the real path must amend only the real one — the
// endpoint partitions amendable/unamendable before calling in, but a direct
// caller must not see the garbage swallowed silently alongside good paths.
func TestAmendFrozenContractForAllow_MixedGarbageAndReal(t *testing.T) {
	dir, store, rec := newAmendTestFixture(t)
	_, err := AmendFrozenContractForAllow(store, dir, rec, []string{
		bug1175GarbagePath,
		"src/extra.go",
	}, time.Now().UTC())
	if err == nil {
		t.Fatal("mixed batch must still surface the rejection loudly")
	}
}
