package skillpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readGitignore(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	return string(data)
}

func TestEnsureGitignoreAIEntries_CreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()

	changed, err := EnsureGitignoreAIEntries(dir)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for a missing .gitignore")
	}

	content := readGitignore(t, dir)
	if !strings.Contains(content, gitignoreAIHeader) {
		t.Fatalf("missing header in .gitignore:\n%s", content)
	}
	for _, entry := range gitignoreAIEntries {
		if !strings.Contains(content, entry+"\n") && !strings.HasSuffix(content, entry) {
			t.Fatalf("missing entry %q in .gitignore:\n%s", entry, content)
		}
	}
}

func TestEnsureGitignoreAIEntries_AppendsBlockToExistingFile(t *testing.T) {
	dir := t.TempDir()
	original := "node_modules/\nbuild/\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(original), 0o644); err != nil {
		t.Fatalf("seed .gitignore: %v", err)
	}

	changed, err := EnsureGitignoreAIEntries(dir)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when entries are missing")
	}

	content := readGitignore(t, dir)
	if !strings.HasPrefix(content, original) {
		t.Fatalf("existing content not preserved:\n%s", content)
	}
	for _, entry := range gitignoreAIEntries {
		if !strings.Contains(content, "\n"+entry+"\n") && !strings.HasSuffix(content, "\n"+entry) {
			t.Fatalf("missing entry %q in .gitignore:\n%s", entry, content)
		}
	}
}

func TestEnsureGitignoreAIEntries_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if _, err := EnsureGitignoreAIEntries(dir); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	first := readGitignore(t, dir)

	changed, err := EnsureGitignoreAIEntries(dir)
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if changed {
		t.Fatalf("expected changed=false on second run; content:\n%s", readGitignore(t, dir))
	}
	if readGitignore(t, dir) != first {
		t.Fatal("second run modified an already-complete .gitignore")
	}
}

func TestEnsureGitignoreAIEntries_OnlyAddsMissingEntries(t *testing.T) {
	dir := t.TempDir()
	original := "node_modules/\n\n# AI Rules\n.claude/\n"
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(original), 0o644); err != nil {
		t.Fatalf("seed .gitignore: %v", err)
	}

	changed, err := EnsureGitignoreAIEntries(dir)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true while entries are still missing")
	}

	content := readGitignore(t, dir)
	if strings.Count(content, ".claude/") != 1 {
		t.Fatalf(".claude/ duplicated:\n%s", content)
	}
	if strings.Count(content, gitignoreAIHeader) != 1 {
		t.Fatalf("header duplicated:\n%s", content)
	}
	for _, entry := range gitignoreAIEntries {
		if strings.Count(content, entry) != 1 {
			t.Fatalf("entry %q count != 1:\n%s", entry, content)
		}
	}
}

func TestEnsureGitignoreAIEntries_HandlesMissingTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("dist/"), 0o644); err != nil {
		t.Fatalf("seed .gitignore: %v", err)
	}

	if _, err := EnsureGitignoreAIEntries(dir); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	content := readGitignore(t, dir)
	if !strings.Contains(content, "dist/\n\n"+gitignoreAIHeader) {
		t.Fatalf("block not separated from unterminated content:\n%s", content)
	}
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "dist/") && strings.TrimSpace(line) != "dist/" {
			t.Fatalf("entry glued to user line: %q", line)
		}
	}
}
