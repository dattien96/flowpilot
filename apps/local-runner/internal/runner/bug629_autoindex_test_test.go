package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-629: ensureGitNexusIndexAsync spawned `gitnexus analyze` inside every
// unindexed git workspace — including go-test fixture tempdirs — and its
// .gitnexus/ writes raced t.TempDir() cleanup ("directory not empty" FAILs
// across the whole suite). Under `go test` the auto-index must never arm.

func TestBUG629_AutoIndexNeverArmsUnderGoTest(t *testing.T) {
	if !runningUnderGoTest() {
		t.Fatal("test binary must detect go-test mode")
	}
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &InteractiveService{gitnexusAnalyzeOnce: map[string]bool{}}
	svc.ensureGitNexusIndexAsync(cwd)
	if svc.gitnexusAnalyzeOnce[cwd] {
		t.Fatal("auto-index armed for a go-test workspace — would race TempDir cleanup")
	}
}