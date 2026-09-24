package worktree

// CP-71 71-i gap-fill (KR-005): ensureGitignore side-effect — the worktree
// root must land in the repo's .gitignore exactly once, on create and on
// pre-existing files without a trailing newline.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureGitignore_AppendsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	ensureGitignore(dir)
	raw, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore must be created: %v", err)
	}
	if strings.TrimSpace(string(raw)) != GitignoreEntry {
		t.Fatalf(".gitignore=%q want exactly %q", raw, GitignoreEntry)
	}
}

func TestEnsureGitignore_AppendsAfterNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	ignore := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(ignore, []byte("node_modules"), 0o644); err != nil {
		t.Fatal(err)
	}
	ensureGitignore(dir)
	raw, err := os.ReadFile(ignore)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 || lines[0] != "node_modules" || lines[1] != GitignoreEntry {
		t.Fatalf("entry must land on its own line: %q", raw)
	}
}

func TestEnsureGitignore_Idempotent(t *testing.T) {
	dir := t.TempDir()
	ensureGitignore(dir)
	ensureGitignore(dir)
	ensureGitignore(dir)
	raw, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), GitignoreEntry) != 1 {
		t.Fatalf("entry must appear exactly once: %q", raw)
	}
}
