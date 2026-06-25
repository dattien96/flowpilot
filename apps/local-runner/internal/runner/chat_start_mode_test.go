package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrependModePrefixTaskOnlyOnFirstTurn(t *testing.T) {
	prompt := "Implement the requested change."

	first := prependModePrefix(prompt, 1, "task", "Task-114")
	if first == prompt {
		t.Fatal("expected first turn prompt to be prefixed")
	}
	for _, want := range []string{"Task-114", "requirements/08-Task/done/Task-114-<short-title>.md", "FORMAT-REFERENCE-TASK.md"} {
		if !strings.Contains(first, want) {
			t.Fatalf("first turn prompt missing %q\n%s", want, first)
		}
	}

	second := prependModePrefix(prompt, 2, "task", "Task-114")
	if second != prompt {
		t.Fatalf("second turn should not be prefixed, got %q", second)
	}
}

func TestBuildModePrefixBugUsesAutoAssignHintWhenIDMissing(t *testing.T) {
	prefix := buildModePrefix("bugfix", "")
	for _, want := range []string{"BUG-<next-available>", "requirements/09-BugFix/done/BUG-<next-available>-<short-title>.md", "FORMAT-REFERENCE-BUGFIX.md"} {
		if !strings.Contains(prefix, want) {
			t.Fatalf("bug prefix missing %q\n%s", want, prefix)
		}
	}
}

func TestResolveSourceDocIDTaskUsesNextAvailable(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "requirements", "08-Task", "done")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Task-114-existing.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Task-115-other.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveSourceDocID(root, "task", ""); got != "Task-116" {
		t.Fatalf("resolveSourceDocID() = %q, want Task-116", got)
	}
}

func TestResolveSourceDocIDKeepsExplicitValue(t *testing.T) {
	if got := resolveSourceDocID(t.TempDir(), "bugfix", "BUG-141"); got != "BUG-141" {
		t.Fatalf("resolveSourceDocID() = %q, want BUG-141", got)
	}
}
