//go:build unix

package runner

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestOpenWorkspaceRegularFileRejectsSymlinkedIntermediateDir proves the
// openat(2)-based component walk rejects a symlink at an INTERMEDIATE path
// component, not just the final one (BUG-288 P1-11). The pre-fix approach
// (filepath.EvalSymlinks on the full path, then reopen by string path) only
// protected the final component — an intermediate directory swapped for a
// symlink between validation and open could escape the workspace.
func TestOpenWorkspaceRegularFileRejectsSymlinkedIntermediateDir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secretPath := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("outside workspace"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	// workspace/evil-dir -> outside (symlinked intermediate directory).
	evilLink := filepath.Join(root, "evil-dir")
	if err := os.Symlink(outside, evilLink); err != nil {
		t.Skipf("symlink not supported in this environment: %v", err)
	}

	target := filepath.Join(root, "evil-dir", "secret.txt")
	if _, err := openWorkspaceRegularFile(root, target); err == nil {
		t.Fatal("expected openWorkspaceRegularFile to reject a symlinked intermediate directory, got nil error")
	}
}

// TestOpenWorkspaceRegularFileRejectsFIFO proves a FIFO leaf is reported via
// the distinct errNotRegularFile sentinel rather than a generic
// symlink-resolution error (BUG-288 P1-11 / TestReadSourceExcerptsRejectsNonRegular).
func TestOpenWorkspaceRegularFileRejectsFIFO(t *testing.T) {
	root := t.TempDir()
	fifoPath := filepath.Join(root, "pipe.fifo")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Skipf("mkfifo not supported in this environment: %v", err)
	}

	_, err := openWorkspaceRegularFile(root, fifoPath)
	if err == nil {
		t.Fatal("expected an error opening a FIFO as a source excerpt")
	}
	if !errors.Is(err, errNotRegularFile) {
		t.Fatalf("err = %v, want errNotRegularFile", err)
	}
}

// TestOpenWorkspaceRegularFileAllowsRegularFile is the fast/happy path: a
// plain regular file nested under a real (non-symlinked) subdirectory opens
// successfully.
func TestOpenWorkspaceRegularFileAllowsRegularFile(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "pkg")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(subdir, "file.go")
	if err := os.WriteFile(target, []byte("package pkg\n"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	f, err := openWorkspaceRegularFile(root, target)
	if err != nil {
		t.Fatalf("openWorkspaceRegularFile: %v", err)
	}
	defer f.Close()
	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	if string(buf[:n]) != "package pkg\n" {
		t.Fatalf("read content = %q", string(buf[:n]))
	}
}
