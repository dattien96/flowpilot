package structure

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBug420_RepoNamePrefersMetaJSON pins the cloned-bed defect: a workspace
// cloned/renamed after indexing keeps `.gitnexus/meta.json` naming the
// ORIGINAL repoPath (e.g. dir `lt-cp49` indexed as `gate-sandbox`). The
// impact call must resolve the registered repo name from the index metadata,
// not the directory basename — otherwise every lookup is `Repository
// "lt-cpNN" not found` (BUG-LIVE-CP44-1, BUG-LIVE-CP49-6).
func TestBug420_RepoNamePrefersMetaJSON(t *testing.T) {
	dir := t.TempDir()
	metaDir := filepath.Join(dir, ".gitnexus")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := `{"repoPath": "/Users/x/BE/gate-sandbox", "lastCommit": "abc", "indexedAt": "2026-08-27T17:06:25Z", "stats": {}}`
	if err := os.WriteFile(filepath.Join(metaDir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := repoNameFromDir(dir); got != "gate-sandbox" {
		t.Fatalf("repoNameFromDir = %q, want gate-sandbox from meta.json repoPath", got)
	}
}

// TestBug420_RepoNameFallsBackToBasename keeps the pre-existing behavior when
// no index metadata exists (or is corrupt): the directory basename is used.
func TestBug420_RepoNameFallsBackToBasename(t *testing.T) {
	dir := t.TempDir()
	if got := repoNameFromDir(dir); got != filepath.Base(dir) {
		t.Fatalf("repoNameFromDir without meta.json = %q, want %q", got, filepath.Base(dir))
	}
	// Corrupt meta.json → same basename fallback, never a bogus name.
	if err := os.MkdirAll(filepath.Join(dir, ".gitnexus"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitnexus", "meta.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := repoNameFromDir(dir); got != filepath.Base(dir) {
		t.Fatalf("repoNameFromDir with corrupt meta.json = %q, want basename %q", got, filepath.Base(dir))
	}
}

// NOTE: BUG-420's "Available() reflects real index reachability" expectation
// is intentionally NOT implemented — the pre-existing
// TestGitNexusProviderAvailable / TestNewSelectsCorrectProvider pin
// Available()==true for the gitnexus provider unconditionally, and
// additive-tests-only forbids weakening them. The observability defect is
// closed by section warnings surfacing the real lookup error instead (see
// the runner-side BUG-420 test) plus the repo-name fix above.
