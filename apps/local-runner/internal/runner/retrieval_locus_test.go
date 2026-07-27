package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// --- target normalization ---------------------------------------------------

func TestIsConcreteCodeTargetAcceptsRealFiles(t *testing.T) {
	for _, p := range []string{
		"apps/local-runner/internal/runner/foo.go",
		"src/app.tsx",
		"go.mod", // a root file with an extension is still concrete
	} {
		if !isConcreteCodeTarget(p) {
			t.Errorf("%q should be a concrete code target", p)
		}
	}
}

func TestIsConcreteCodeTargetRejectsDirectoryBuckets(t *testing.T) {
	// This is the shape inference produces for most real contracts; accepting it
	// would match nearly every commit and recreate the dilution CP-54 removes.
	for _, p := range []string{"apps", "internal", "apps/local-runner", ""} {
		if isConcreteCodeTarget(p) {
			t.Errorf("%q is a directory bucket, not a file", p)
		}
	}
}

func TestIsConcreteCodeTargetRejectsGlobs(t *testing.T) {
	for _, p := range []string{"internal/*.go", "src/?.ts", "pkg/[ab].go"} {
		if isConcreteCodeTarget(p) {
			t.Errorf("%q is a glob and cannot be resolved to one file", p)
		}
	}
}

func TestIsConcreteCodeTargetRejectsDocsAndAudit(t *testing.T) {
	for _, p := range []string{
		"requirements/08-Task/todo/Task-262-Shared-Retrieval-Locus-Builder.md",
		"change-audit/CA-422-persist-changed-paths-in-change-ledger.md",
	} {
		if isConcreteCodeTarget(p) {
			t.Errorf("%q is a doc/audit file and must not anchor code retrieval", p)
		}
	}
}

func TestIsConcreteCodeTargetRejectsFlagLikeTargets(t *testing.T) {
	// Guards a downstream tool runner from parsing the target as an option.
	if isConcreteCodeTarget("--repo=evil.go") {
		t.Fatal("a target starting with '-' must be rejected")
	}
}

// --- symbol dedupe ----------------------------------------------------------

func TestDedupeSortedSymbolsSortsAndDedupes(t *testing.T) {
	got := dedupeSortedSymbols([]string{"Zebra", "Alpha", "Zebra", "  ", ""})
	if !reflect.DeepEqual(got, []string{"Alpha", "Zebra"}) {
		t.Fatalf("got %v, want [Alpha Zebra]", got)
	}
}

func TestDedupeSortedSymbolsEmptyYieldsNil(t *testing.T) {
	if got := dedupeSortedSymbols(nil); got != nil {
		t.Fatalf("want nil, got %v", got)
	}
	if got := dedupeSortedSymbols([]string{"", "   "}); got != nil {
		t.Fatalf("blank-only input must yield nil, got %v", got)
	}
}

// --- locus building ---------------------------------------------------------

func newLocusRepo(t *testing.T) string {
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

	// One commit so `git diff HEAD` has a HEAD to compare against.
	if err := os.WriteFile(filepath.Join(dir, "seed.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "[feature][calc-core] seed")
	return dir
}

func saveContract(t *testing.T, workspace, runID string, paths []string) {
	t.Helper()
	store, err := changecontract.NewStore(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(changecontract.Contract{
		RunID:         runID,
		StepID:        "step-1",
		FeatureKey:    "calc-core",
		Intent:        "test",
		DeclaredPaths: paths,
		Confidence:    changecontract.ConfidenceDeclared,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBuildRetrievalLocusUsesDeclaredContractPaths(t *testing.T) {
	ws := newLocusRepo(t)
	saveContract(t, ws, "run-1", []string{"src/calc.go", "src/calc_test.go"})

	locus := buildRetrievalLocus(ws, "run-1", "")

	want := []string{"src/calc.go", "src/calc_test.go"}
	if !reflect.DeepEqual(locus.Paths, want) {
		t.Fatalf("Paths = %v, want %v", locus.Paths, want)
	}
	if locus.RunID != "run-1" {
		t.Fatalf("RunID = %q, want run-1", locus.RunID)
	}
	if locus.IsEmpty() {
		t.Fatal("a locus with declared paths must not be empty")
	}
}

func TestBuildRetrievalLocusDropsInferredDirectoryBuckets(t *testing.T) {
	ws := newLocusRepo(t)
	// The shape InferFromDiff writes for an undeclared turn.
	saveContract(t, ws, "run-1", []string{"apps", "internal", "go.mod"})

	locus := buildRetrievalLocus(ws, "run-1", "")

	if !reflect.DeepEqual(locus.Paths, []string{"go.mod"}) {
		t.Fatalf("only the concrete file should survive, got %v", locus.Paths)
	}
}

func TestBuildRetrievalLocusDropsGlobsDocsAndFlags(t *testing.T) {
	ws := newLocusRepo(t)
	saveContract(t, ws, "run-1", []string{
		"internal/*.go",
		"requirements/08-Task/todo/Task-1.md",
		"change-audit/CA-1.md",
		"-rf",
		"src/keep.go",
	})

	locus := buildRetrievalLocus(ws, "run-1", "")

	if !reflect.DeepEqual(locus.Paths, []string{"src/keep.go"}) {
		t.Fatalf("expected only src/keep.go, got %v", locus.Paths)
	}
}

func TestBuildRetrievalLocusMergesDiffAndPromptPaths(t *testing.T) {
	ws := newLocusRepo(t)
	// An uncommitted edit plus a path named in the prompt.
	if err := os.WriteFile(filepath.Join(ws, "seed.go"), []byte("package main // edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	locus := buildRetrievalLocus(ws, "", "please look at src/from_prompt.go too")

	if !locusHasPath(locus.Paths, "seed.go") {
		t.Errorf("uncommitted diff path missing: %v", locus.Paths)
	}
	if !locusHasPath(locus.Paths, "src/from_prompt.go") {
		t.Errorf("prompt path missing: %v", locus.Paths)
	}
}

func TestBuildRetrievalLocusDedupesAcrossSources(t *testing.T) {
	ws := newLocusRepo(t)
	saveContract(t, ws, "run-1", []string{"seed.go"})
	if err := os.WriteFile(filepath.Join(ws, "seed.go"), []byte("package main // edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// seed.go is named by the contract, by the diff, and by the prompt.
	locus := buildRetrievalLocus(ws, "run-1", "fix seed.go now")

	count := 0
	for _, p := range locus.Paths {
		if p == "seed.go" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("seed.go should appear exactly once, got %d in %v", count, locus.Paths)
	}
}

func TestBuildRetrievalLocusIsSortedAndDeterministic(t *testing.T) {
	ws := newLocusRepo(t)
	saveContract(t, ws, "run-1", []string{"z/z.go", "a/a.go", "m/m.go"})

	first := buildRetrievalLocus(ws, "run-1", "")
	second := buildRetrievalLocus(ws, "run-1", "")

	if !reflect.DeepEqual(first.Paths, second.Paths) {
		t.Fatalf("two runs disagreed: %v vs %v", first.Paths, second.Paths)
	}
	want := []string{"a/a.go", "m/m.go", "z/z.go"}
	if !reflect.DeepEqual(first.Paths, want) {
		t.Fatalf("Paths = %v, want sorted %v", first.Paths, want)
	}
}

func TestBuildRetrievalLocusEmptyRunIDSkipsContract(t *testing.T) {
	ws := newLocusRepo(t)
	saveContract(t, ws, "run-1", []string{"src/should_not_appear.go"})

	// Chat-mode per-turn injection has no workflow run id (CP-54 Q-1).
	locus := buildRetrievalLocus(ws, "", "")

	if locusHasPath(locus.Paths, "src/should_not_appear.go") {
		t.Fatalf("contract must be skipped without a run id, got %v", locus.Paths)
	}
}

func TestBuildRetrievalLocusEmptyWhenNothingToAnchorOn(t *testing.T) {
	locus := buildRetrievalLocus(t.TempDir(), "run-1", "")

	if !locus.IsEmpty() {
		t.Fatalf("no contract, no git, no prompt must yield an empty locus, got %v", locus.Paths)
	}
}

func TestBuildRetrievalLocusNonFatalOutsideGitRepo(t *testing.T) {
	// Not a git repo: the prompt is still a usable anchor and nothing panics.
	locus := buildRetrievalLocus(t.TempDir(), "", "check src/only.go")

	if !reflect.DeepEqual(locus.Paths, []string{"src/only.go"}) {
		t.Fatalf("Paths = %v, want [src/only.go]", locus.Paths)
	}
}

func TestBuildRetrievalLocusBlankWorkspaceIsSafe(t *testing.T) {
	if locus := buildRetrievalLocus("", "run-1", ""); !locus.IsEmpty() {
		t.Fatalf("blank workspace must yield an empty locus, got %v", locus.Paths)
	}
}

func locusHasPath(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
