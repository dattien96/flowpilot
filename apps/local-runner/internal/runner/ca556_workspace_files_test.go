package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFilterWorkspaceFilePathsMatchesQueryAndCaps(t *testing.T) {
	paths := []string{
		"apps/desktop-flowpilot/src/components/ChatInput.tsx",
		"apps/local-runner/internal/runner/chat_session_sync.go",
		"apps/local-runner/internal/runner/workspace_files.go",
		"change-audit/CA-556.md",
	}
	got := filterWorkspaceFilePaths(paths, "ChatInput", 40)
	if len(got) != 1 || got[0] != paths[0] {
		t.Fatalf("ChatInput filter = %#v", got)
	}
	got = filterWorkspaceFilePaths(paths, "runner", 2)
	if len(got) != 2 {
		t.Fatalf("cap 2: %#v", got)
	}
	if len(filterWorkspaceFilePaths(paths, "nope", 40)) != 0 {
		t.Fatal("expected no matches")
	}
	if len(filterWorkspaceFilePaths(paths, "", 2)) != 2 {
		t.Fatal("empty query should still cap")
	}
}

func TestListWorkspaceFilePathsUsesGitIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("add", ".")
	got := listWorkspaceFilePaths(dir, "main.go")
	if len(got) != 1 || got[0] != "src/main.go" {
		t.Fatalf("list = %#v, want [src/main.go]", got)
	}
	if listWorkspaceFilePaths("", "main") != nil {
		t.Fatal("empty cwd must be nil")
	}
}
