package changeledger

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// --- chunk parsing (pure, no git) -------------------------------------------

func TestParseChangedPathsChunkReadsHashThenFiles(t *testing.T) {
	hash, paths := parseChangedPathsChunk("abc123\n\nsrc/b.go\nsrc/a.go\n")
	if hash != "abc123" {
		t.Fatalf("hash = %q, want abc123", hash)
	}
	if !reflect.DeepEqual(paths, []string{"src/a.go", "src/b.go"}) {
		t.Fatalf("paths = %v, want sorted [src/a.go src/b.go]", paths)
	}
}

func TestParseChangedPathsChunkSortsForDeterminism(t *testing.T) {
	// Same files, opposite emission order, must produce the same slice
	// (BUG-266: ordering must never depend on what git happened to print).
	_, first := parseChangedPathsChunk("h\nz.go\nm.go\na.go\n")
	_, second := parseChangedPathsChunk("h\na.go\nz.go\nm.go\n")
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("ordering not normalized: %v vs %v", first, second)
	}
}

func TestParseChangedPathsChunkEmptyYieldsNoHash(t *testing.T) {
	if hash, paths := parseChangedPathsChunk("\n  \n"); hash != "" || paths != nil {
		t.Fatalf("empty chunk should yield no hash and no paths, got %q %v", hash, paths)
	}
}

func TestParseChangedPathsChunkCommitWithNoFiles(t *testing.T) {
	hash, paths := parseChangedPathsChunk("deadbeef\n")
	if hash != "deadbeef" {
		t.Fatalf("hash = %q, want deadbeef", hash)
	}
	if paths != nil {
		t.Fatalf("a commit touching no files must yield nil paths, got %v", paths)
	}
}

// --- attach -----------------------------------------------------------------

func TestAttachChangedPathsOnlyFillsMatchingCommits(t *testing.T) {
	entries := []Entry{{CommitHash: "aaa"}, {CommitHash: "bbb"}}
	attachChangedPaths(entries, map[string][]string{"aaa": {"x.go"}})

	if !reflect.DeepEqual(entries[0].ChangedPaths, []string{"x.go"}) {
		t.Fatalf("entry aaa = %v, want [x.go]", entries[0].ChangedPaths)
	}
	if entries[1].ChangedPaths != nil {
		t.Fatalf("entry bbb had no git record; want nil, got %v", entries[1].ChangedPaths)
	}
}

func TestAttachChangedPathsEmptyMapIsNoOp(t *testing.T) {
	entries := []Entry{{CommitHash: "aaa", ChangedPaths: []string{"keep.go"}}}
	attachChangedPaths(entries, map[string][]string{})
	if !reflect.DeepEqual(entries[0].ChangedPaths, []string{"keep.go"}) {
		t.Fatalf("existing paths must survive an empty map, got %v", entries[0].ChangedPaths)
	}
}

// --- backward compatibility -------------------------------------------------

func TestEntryWithoutChangedPathsFieldLoadsAsNil(t *testing.T) {
	// An NDJSON line written before CP-54 P-1 has no changed_paths key at all.
	legacy := `{"commit_hash":"old1","feature_key":"calc-core","summary":"s","committed_at":"2026-01-01T00:00:00Z","order_index":0,"confidence":"high"}`

	var e Entry
	if err := json.Unmarshal([]byte(legacy), &e); err != nil {
		t.Fatalf("legacy entry must still unmarshal: %v", err)
	}
	if e.ChangedPaths != nil {
		t.Fatalf("legacy entry must yield nil ChangedPaths, got %v", e.ChangedPaths)
	}
	if e.CommitHash != "old1" || e.FeatureKey != "calc-core" {
		t.Fatalf("legacy fields lost: %+v", e)
	}
}

func TestEntryWithoutChangedPathsOmitsKeyOnMarshal(t *testing.T) {
	out, err := json.Marshal(Entry{CommitHash: "h"})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); contains(got, "changed_paths") {
		t.Fatalf("omitempty should drop the key when unset, got %s", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

// --- git-backed (real repo in a temp dir) -----------------------------------

// newTempRepo builds a throwaway git repo and returns its directory.
// Skips the test when git is unavailable rather than failing.
func newTempRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	return dir
}

func writeAndCommit(t *testing.T, dir, msg string, files ...string) {
	t.Helper()
	for _, f := range files {
		full := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", "-A")
	run("commit", "-m", msg)
}

func TestChangedPathsByCommitReportsEachCommitsFiles(t *testing.T) {
	dir := newTempRepo(t)
	writeAndCommit(t, dir, "[feature][calc-core] first", "src/a.go")
	writeAndCommit(t, dir, "[feature][calc-core] second", "src/b.go", "docs/readme.md")

	byCommit := changedPathsByCommit(dir, "")
	if len(byCommit) != 2 {
		t.Fatalf("expected 2 commits, got %d: %v", len(byCommit), byCommit)
	}

	var sawFirst, sawSecond bool
	for _, paths := range byCommit {
		switch {
		case reflect.DeepEqual(paths, []string{"src/a.go"}):
			sawFirst = true
		case reflect.DeepEqual(paths, []string{"docs/readme.md", "src/b.go"}):
			sawSecond = true
		}
	}
	if !sawFirst {
		t.Errorf("missing the single-file commit: %v", byCommit)
	}
	if !sawSecond {
		t.Errorf("missing the multi-file commit (must be sorted): %v", byCommit)
	}
}

func TestChangedPathsByCommitIsDeterministic(t *testing.T) {
	dir := newTempRepo(t)
	writeAndCommit(t, dir, "[feature][calc-core] c", "z/z.go", "a/a.go", "m/m.go")

	if first, second := changedPathsByCommit(dir, ""), changedPathsByCommit(dir, ""); !reflect.DeepEqual(first, second) {
		t.Fatalf("two runs disagreed:\n%v\n%v", first, second)
	}
}

func TestChangedPathsByCommitNonFatalWithoutGitRepo(t *testing.T) {
	// A directory that is not a git repo must degrade to an empty map, not panic.
	if got := changedPathsByCommit(t.TempDir(), ""); len(got) != 0 {
		t.Fatalf("expected empty map outside a repo, got %v", got)
	}
}

func TestChangedPathsByCommitNonFatalWithBadCursor(t *testing.T) {
	dir := newTempRepo(t)
	writeAndCommit(t, dir, "[feature][calc-core] only", "a.go")

	if got := changedPathsByCommit(dir, "0000000000000000000000000000000000000000"); len(got) != 0 {
		t.Fatalf("a cursor that is not an ancestor must degrade to empty, got %v", got)
	}
}

func TestParseRepoAttachesChangedPaths(t *testing.T) {
	dir := newTempRepo(t)
	writeAndCommit(t, dir, "[feature][calc-core] add guard", "src/calc.go", "src/calc_test.go")

	entries, err := ParseRepo(dir, t.TempDir())
	if err != nil {
		t.Fatalf("ParseRepo: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	want := []string{"src/calc.go", "src/calc_test.go"}
	if !reflect.DeepEqual(entries[0].ChangedPaths, want) {
		t.Fatalf("ChangedPaths = %v, want %v", entries[0].ChangedPaths, want)
	}
}

func TestBuildBackfillsChangedPathsOnLegacyEntries(t *testing.T) {
	repo := newTempRepo(t)
	writeAndCommit(t, repo, "[feature][calc-core] legacy work", "src/legacy.go")

	dotFP := t.TempDir()

	// Seed the ledger the way a pre-CP-54 build would have: a real commit hash,
	// but no changed_paths field.
	hashOut, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	hash := string(hashOut[:len(hashOut)-1])
	if n := len(hash); n > 0 && hash[n-1] == '\r' {
		hash = hash[:n-1]
	}

	l, err := New(dotFP)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Upsert([]Entry{{
		CommitHash:  hash,
		FeatureKey:  "calc-core",
		Summary:     "legacy work",
		CommittedAt: "2026-01-01T00:00:00Z",
		Confidence:  ConfidenceHigh,
	}}); err != nil {
		t.Fatal(err)
	}

	// Mark the cursor at HEAD so ParseRepo finds nothing new: the backfill path
	// is the only thing that can fill this entry.
	if err := os.WriteFile(CursorPath(dotFP), []byte(hash), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Build(repo, dotFP); err != nil {
		t.Fatalf("Build: %v", err)
	}

	reopened, err := New(dotFP)
	if err != nil {
		t.Fatal(err)
	}
	all := reopened.AllEntries()
	if len(all) != 1 {
		t.Fatalf("expected 1 entry after build, got %d", len(all))
	}
	if !reflect.DeepEqual(all[0].ChangedPaths, []string{"src/legacy.go"}) {
		t.Fatalf("backfill did not fill ChangedPaths, got %v", all[0].ChangedPaths)
	}
}

func TestBackfillChangedPathsSkipsGitWhenNothingMissing(t *testing.T) {
	dotFP := t.TempDir()
	l, err := New(dotFP)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Upsert([]Entry{{CommitHash: "h1", ChangedPaths: []string{"a.go"}}}); err != nil {
		t.Fatal(err)
	}
	if l.hasEntryMissingChangedPaths() {
		t.Fatal("a fully-populated ledger must not report missing entries")
	}
	// Pointing at a non-repo proves no git work is attempted and nothing breaks.
	l.backfillChangedPaths(t.TempDir())
	if got := l.AllEntries()[0].ChangedPaths; !reflect.DeepEqual(got, []string{"a.go"}) {
		t.Fatalf("existing paths must be untouched, got %v", got)
	}
}
