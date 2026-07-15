package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractPromptSourcePaths(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"backtick", "see `apps/local-runner/foo.go`", []string{"apps/local-runner/foo.go"}},
		{"quoted", `path "src/main.ts" here`, []string{"src/main.ts"}},
		{"windows", `touch apps\runner\x.go`, []string{"apps/runner/x.go"}},
		{"url skip", "http://example.com/a.go", nil},
		{"no ext", "see apps/local-runner/foo", nil},
		{"doc skip", "edit requirements/foo.md", nil},
		{"empty", "hello world", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPromptSourcePaths(tc.in)
			if len(tc.want) == 0 {
				if len(got) != 0 {
					t.Fatalf("got %v", got)
				}
				return
			}
			if len(got) != len(tc.want) || got[0] != tc.want[0] {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestExtractPromptSourcePathsCapsAt8(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 12; i++ {
		b.WriteString(" a/b/c")
		b.WriteByte(byte('a' + i))
		b.WriteString(".go")
	}
	got := extractPromptSourcePaths(b.String())
	if len(got) != 8 {
		t.Fatalf("len=%d want 8", len(got))
	}
}

func TestUncommittedChangedPathsNonGit(t *testing.T) {
	if paths := uncommittedChangedPaths(t.TempDir()); paths != nil {
		t.Fatalf("non-git want nil, got %v", paths)
	}
}

func TestUncommittedChangedPathsGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "tracked.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "tracked.go")
	run("commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "tracked.go"), []byte("package a\n// edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := uncommittedChangedPaths(dir)
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "tracked.go") || !strings.Contains(joined, "new.go") {
		t.Fatalf("got %v", got)
	}
	if strings.Contains(joined, "readme.md") {
		t.Fatalf("doc file should be filtered: %v", got)
	}
}
