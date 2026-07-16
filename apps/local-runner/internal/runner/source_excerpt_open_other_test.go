//go:build !unix

package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// source_excerpt_open_other_test.go: focused regression coverage for BUG-288
// P1-19 (Vòng 12) — the non-unix (incl. Windows) openWorkspaceRegularFile
// path, tightened to walk via os.Root instead of re-Lstat-then-reopen by
// string path. Mirrors source_excerpt_open_unix_test.go's style for the
// unix build.

func TestOpenWorkspaceRegularFileHappyPath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(sub, "file.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := openWorkspaceRegularFile(dir, target)
	if err != nil {
		t.Fatalf("openWorkspaceRegularFile: %v", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Mode().IsRegular() {
		t.Fatal("expected a regular file handle")
	}
}

func TestOpenWorkspaceRegularFileRejectsIntermediateSymlink(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(realDir, "file.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkedDir := filepath.Join(dir, "linked")
	if err := os.Symlink(realDir, linkedDir); err != nil {
		t.Skipf("symlink creation unavailable in this environment: %v", err)
	}

	// Path traverses the workspace root via the symlinked intermediate
	// component — must be rejected outright (the unix build's same policy:
	// "no symlinks anywhere in the path", not merely "no escape").
	viaLink := filepath.Join(linkedDir, "file.go")
	if _, err := openWorkspaceRegularFile(dir, viaLink); err == nil {
		t.Fatal("expected an error opening a path through a symlinked intermediate directory")
	}
}

func TestOpenWorkspaceRegularFileRejectsNonRegularLeaf(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "not_a_file")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The leaf itself is a directory, not a regular file.
	if _, err := openWorkspaceRegularFile(dir, subdir); err == nil {
		t.Fatal("expected an error opening a directory as the leaf")
	}
}
