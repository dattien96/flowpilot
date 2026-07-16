package flowgate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestObserveGitDiffRenameUsesDestinationPath is the focused regression test
// requested by BUG-288 P2-05 (Vòng 12). For `git status --porcelain -z`,
// rename/copy records use two NUL-separated path fields where the FIRST is the
// CURRENT/destination path and the SECOND is the ORIGINAL/source path (this is
// the reverse of the human-readable "ORIG_PATH -> PATH" form — see git-status(1)
// and BUG-288 R13-24). parsePorcelainZ (via ObserveGitDiff) must report the
// destination path for staged renames so r-ca / r-contract / scope evaluate the
// file the AI actually wrote to.
func TestObserveGitDiffRenameUsesDestinationPath(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")

	oldPath := filepath.Join(dir, "old_name.go")
	writeFileFatal(t, oldPath, "package main\n")
	run("add", "old_name.go")
	run("commit", "-m", "init")

	// Rename (git mv stages it directly as a rename entry).
	run("mv", "old_name.go", "new_name.go")

	files, err := ObserveGitDiff(dir)
	if err != nil {
		t.Fatalf("ObserveGitDiff: %v", err)
	}

	var sawNew, sawOld bool
	for _, f := range files {
		switch f.Path {
		case "new_name.go":
			sawNew = true
		case "old_name.go":
			sawOld = true
		}
	}
	if !sawNew {
		t.Fatalf("expected renamed entry to report destination path new_name.go, got files=%+v", files)
	}
	if sawOld {
		t.Fatalf("renamed entry must not report the stale source path old_name.go as a changed path, got files=%+v", files)
	}
}

func writeFileFatal(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
