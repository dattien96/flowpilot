package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-662: live PrivateVault run-640182 — Task-121..124 declare parentage as
// `- Parent Links: CP-12` (no `Parent Documents:` line), so the CP-12 scope
// resolved empty and the run fell back to the unscoped glob (Task-0312
// first). The collector must honor the Parent Links dialect exactly like
// Parent Documents, while still ignoring Related-* mentions (BUG-468's
// deliberate restriction).

const bug662TaskLinksOnly = "# Task\n\n## Metadata\n\n- Document ID: `Task-121`\n- Phase: `task`\n- Status: `draft`\n- Parent Links: `CP-12` (Air-Gap Offline Vault Migration), `Task-0310` (NFC Migration ECDH)\n"
const bug662TaskBothDialects = "# Task\n\n## Metadata\n\n- Document ID: `Task-122`\n- Phase: `task`\n- Status: `draft`\n- Parent Documents: `CP-12` (Air-Gap Offline Vault Migration)\n- Parent Links: `CP-12` (Air-Gap Offline Vault Migration)\n"
const bug662TaskSplitDialects = "# Task\n\n## Metadata\n\n- Document ID: `Task-123`\n- Phase: `task`\n- Status: `draft`\n- Parent Documents: `CP-11`\n- Parent Links: `CP-12` (Air-Gap Offline Vault Migration)\n"
const bug662TaskRelatedOnly = "# Task\n\n## Metadata\n\n- Document ID: `Task-124`\n- Phase: `task`\n- Status: `draft`\n- Related Links: `CP-12` (mentioned, not parented)\n- Related Documents: `CP-12` (prior plan)\n"

func bug662SeedTasks(t *testing.T, dir string) {
	t.Helper()
	taskDir := filepath.Join(dir, "requirements", "08-Task", "todo")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Task-121-links-only.md":   bug662TaskLinksOnly,
		"Task-122-both.md":         bug662TaskBothDialects,
		"Task-123-split.md":        bug662TaskSplitDialects,
		"Task-124-related-only.md": bug662TaskRelatedOnly,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(taskDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBUG662_CollectVibeTaskPlanForCPHonorsParentLinks(t *testing.T) {
	dir := t.TempDir()
	bug662SeedTasks(t, dir)

	got := collectVibeTaskPlanForCP(dir, "CP-12")
	bases := make([]string, 0, len(got))
	for _, rel := range got {
		bases = append(bases, filepath.Base(rel))
	}
	want := map[string]bool{
		"Task-121-links-only.md": true,
		"Task-122-both.md":       true,
		"Task-123-split.md":      true,
	}
	if len(bases) != len(want) {
		t.Fatalf("CP-12 scope = %v, want exactly %v", bases, want)
	}
	seen := map[string]int{}
	for _, b := range bases {
		seen[b]++
		if !want[b] {
			t.Fatalf("CP-12 scope = %v, want exactly %v", bases, want)
		}
	}
	for b, n := range seen {
		if n != 1 {
			t.Fatalf("CP-12 scope = %v, %q listed %d times (union must not duplicate)", bases, b, n)
		}
	}
	// CP-11 resolves through the Documents line of the split-dialect file.
	got = collectVibeTaskPlanForCP(dir, "CP-11")
	if len(got) != 1 || filepath.Base(got[0]) != "Task-123-split.md" {
		t.Fatalf("CP-11 scope = %v, want only Task-123-split", got)
	}
	// Related-* mentions never parent, in either dialect spelling.
	if got := collectVibeTaskPlanForCP(dir, "CP-12"); len(got) != 3 {
		t.Fatalf("CP-12 scope must exclude the Related-only file, got %v", got)
	}
}

func TestBUG662_CollectVibeDoneTasksForCPHonorsParentLinks(t *testing.T) {
	dir := t.TempDir()
	doneDir := filepath.Join(dir, "requirements", "08-Task", "done")
	if err := os.MkdirAll(doneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(doneDir, "Task-121-links-only.md"),
		[]byte(bug662TaskLinksOnly), 0o644); err != nil {
		t.Fatal(err)
	}
	got := collectVibeDoneTasksForCP(dir, "CP-12")
	if len(got) != 1 || filepath.Base(got[0]) != "Task-121-links-only.md" {
		t.Fatalf("CP-12 done scope = %v, want Task-121-links-only", got)
	}
}
