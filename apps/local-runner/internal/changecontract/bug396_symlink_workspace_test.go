package changecontract

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-396 (live + deterministic macOS test failures): NormalizeDeclaredCodePaths
// resolved each existing declared path with EvalSymlinks but kept the workspace
// root unresolved (Abs only) — under a symlinked workspace (macOS
// /var->/private/var TMPDIR, symlinked project dirs) the resolved file lands
// outside the unresolved prefix and the freeze always blocks with
// "resolves outside workspace via symlink".
func TestBug396_DeclaredPathsUnderSymlinkedWorkspace(t *testing.T) {
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "calc.go"), []byte("package calc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Link a workspace path whose REAL location differs (the same asymmetry as
	// macOS /var/folders -> /private/var/folders).
	linkParent := t.TempDir()
	link := filepath.Join(linkParent, "ws-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got, err := NormalizeDeclaredCodePaths(link, []string{"calc.go"})
	if err != nil {
		t.Fatalf("declared path under a symlinked workspace must not report symlink escape: %v", err)
	}
	if len(got) != 1 || got[0] != "calc.go" {
		t.Fatalf("got %v, want [calc.go]", got)
	}
}
