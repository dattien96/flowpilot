package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BUG-518 (live run-49109): a repo whose ignore rules cover .flowpilot/
// (the runner installs `.flowpilot/` into .git/info/exclude) made the
// Diff intent-to-add sweep fail —
//   git add -N -- . ':(exclude).flowpilot'
//   → "The following paths are ignored by one of your .gitignore files:
//      .flowpilot" (exit 1)
// The arbiter's patch snapshot therefore errored, verdict-time cleanup
// wiped every candidate worktree, and the run parked blocked with no
// decision card — the tournament could never merge.
func TestBUG518_DiffToleratesIgnoredFlowpilotDir(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	// The runner-owned exclude: .flowpilot/ is runtime state in this repo.
	infoDir := filepath.Join(r, ".git", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(infoDir, "exclude"), []byte(".flowpilot/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	info, err := m.Create(context.Background(), r, "cand", "candidate-", base, "")
	if err != nil {
		t.Fatal(err)
	}
	// Runtime metadata lands inside the candidate's own .flowpilot dir.
	if err := os.MkdirAll(filepath.Join(info.Path, ".flowpilot", "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(info.Path, ".flowpilot", "logs", "x.log"), []byte("rt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Real candidate output: one new file, one modified tracked file.
	if err := os.WriteFile(filepath.Join(info.Path, "mathx.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(info.Path, "seed.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	patch, err := m.Diff(context.Background(), r, "cand", "candidate-")
	if err != nil {
		t.Fatalf("Diff must not fail when .flowpilot is gitignored: %v", err)
	}
	if !strings.Contains(string(patch), "mathx.go") || !strings.Contains(string(patch), "seed.txt") {
		t.Fatalf("patch missing candidate output:\n%s", patch)
	}
	if strings.Contains(string(patch), ".flowpilot") {
		t.Fatalf("runtime metadata must not leak into the patch:\n%s", patch)
	}
}

// BUG-518 second vector (live run-69516): the untracked case passed, but a
// repo that TRACKS files under .flowpilot/ still died — `ls-files
// --modified` reports tracked .flowpilot/... entries regardless of
// --exclude-standard, and `git add -N` rejects ignored paths outright:
//   → "The following paths are ignored by one of your .gitignore files:
//      .flowpilot" (exit 1)
// .flowpilot is excluded from the merge patch anyway; the intent-to-add
// sweep must drop those entries before feeding `git add`.
func TestBUG518_DiffToleratesTrackedFlowpilotFiles(t *testing.T) {
	r := repo(t)
	// Tracked runner metadata under an ignored dir — the live bed shape.
	if err := os.MkdirAll(filepath.Join(r, ".flowpilot", "catalog"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r, ".flowpilot", "catalog", "features.ndjson"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, r, "add", "-f", ".flowpilot/catalog/features.ndjson")
	git(t, r, "commit", "-m", "track flowpilot metadata")
	base := git(t, r, "rev-parse", "HEAD")
	infoDir := filepath.Join(r, ".git", "info")
	if err := os.WriteFile(filepath.Join(infoDir, "exclude"), []byte(".flowpilot/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager()
	info, err := m.Create(context.Background(), r, "cand", "candidate-", base, "")
	if err != nil {
		t.Fatal(err)
	}
	// The runner rewrites its tracked metadata inside the candidate worktree.
	if err := os.WriteFile(filepath.Join(info.Path, ".flowpilot", "catalog", "features.ndjson"), []byte("{\"changed\":true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Real candidate output.
	if err := os.WriteFile(filepath.Join(info.Path, "mathx.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	patch, err := m.Diff(context.Background(), r, "cand", "candidate-")
	if err != nil {
		t.Fatalf("Diff must not fail when tracked .flowpilot files are modified+ignored: %v", err)
	}
	if !strings.Contains(string(patch), "mathx.go") {
		t.Fatalf("patch missing candidate output:\n%s", patch)
	}
	if strings.Contains(string(patch), ".flowpilot") {
		t.Fatalf("tracked runtime metadata must not leak into the patch:\n%s", patch)
	}
}
