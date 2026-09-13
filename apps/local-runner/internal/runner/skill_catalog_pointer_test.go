package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Task-347 (CP-62 P-5 catalog tier): the one-shot prompt-execution path must
// inject skills as pointer entries (name + description + path), never as full
// file bodies — same contract as the chat path's Task-260 pointer-only
// precedent. The provider harness triggers an installed skill natively; an
// agent with read tools loads the body from the pointer path on demand.

func catalogSkillFixture(t *testing.T, ws, id, description, body string) {
	t.Helper()
	dir := filepath.Join(ws, ".agents", "skills", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "---\nname: " + id + "\ndescription: " + description + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// Scenario: one-shot path injects pointer (name + description + path), KHÔNG gắn body
func TestInjectSkillContent_PointerBlock_NoBody(t *testing.T) {
	ws := t.TempDir()
	const bodyMarker = "UNIQUE_SKILL_BODY_MARKER_XYZ_9137"
	catalogSkillFixture(t, ws, "reverse-string", "Reverse strings safely with full rune handling.", "Step 1: "+bodyMarker)
	r := &Runner{workspace: ws}

	out := r.injectSkillContent(ws, "do the task", []string{"reverse-string"})
	if !strings.Contains(out, "## Selected Skills") {
		t.Fatalf("must emit the pointer catalog header, got %q", out)
	}
	if !strings.Contains(out, "/reverse-string") {
		t.Fatalf("must contain the skill pointer, got %q", out)
	}
	if !strings.Contains(out, "Reverse strings safely with full rune handling.") {
		t.Fatalf("must carry the frontmatter description, got %q", out)
	}
	if strings.Contains(out, bodyMarker) {
		t.Fatalf("body must NEVER be injected (pointer only), got %q", out)
	}
	if !strings.Contains(out, "do the task") {
		t.Fatalf("original prompt must be preserved, got %q", out)
	}
}

// Scenario: nhiều skill -> nhiều dòng pointer; id lạ -> bare entry không path
func TestInjectSkillContent_MultipleAndUnknownSkipped(t *testing.T) {
	ws := t.TempDir()
	catalogSkillFixture(t, ws, "skill-a", "Alpha.", "Alpha body")
	catalogSkillFixture(t, ws, "skill-b", "Beta.", "Beta body")
	r := &Runner{workspace: ws}

	out := r.injectSkillContent(ws, "do the task", []string{"skill-a", "ghost-skill", "skill-b"})
	if strings.Count(out, "→ ") != 2 {
		t.Fatalf("resolved skills must have 2 path pointers, got %q", out)
	}
	if !strings.Contains(out, "- /ghost-skill") {
		t.Fatalf("unresolvable id keeps a bare pointer entry (Task-260 precedent), got %q", out)
	}
	if strings.Contains(out, "Alpha body") || strings.Contains(out, "Beta body") {
		t.Fatalf("no bodies may be injected, got %q", out)
	}
}

// Scenario: không có skill nào -> prompt byte-identical
func TestInjectSkillContent_EmptyIds_ByteIdentical(t *testing.T) {
	r := &Runner{workspace: t.TempDir()}
	if got := r.injectSkillContent(t.TempDir(), "do the task", nil); got != "do the task" {
		t.Fatalf("nil ids must return the bare prompt, got %q", got)
	}
	if got := r.injectSkillContent(t.TempDir(), "do the task", []string{"  "}); got != "do the task" {
		t.Fatalf("blank ids must return the bare prompt, got %q", got)
	}
}
