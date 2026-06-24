package runner

import (
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

func TestBuildModePrefixBugUsesPlaceholderWhenIDMissing(t *testing.T) {
	prefix := buildModePrefix("bugfix", "")
	for _, want := range []string{"BUG-<NNN>", "requirements/09-BugFix/done/BUG-<NNN>-<short-title>.md", "FORMAT-REFERENCE-BUGFIX.md"} {
		if !strings.Contains(prefix, want) {
			t.Fatalf("bug prefix missing %q\n%s", want, prefix)
		}
	}
}
