package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/flowgate"
)

// BUG-531 (live run-134 on /tmp/fp-live2): the reproduce gate reprompted
// "the suite passed, so the bug was not reproduced" while its own oracle
// observed `go test` exit 1. Cause: tr.Tests.Failed is only populated under
// `!oracle.SuitePassed && !oracle.HasRegression` — a green baseline plus a
// real bug (which also fails a baseline-green test) routes every named
// failure into oracle.Regressed and leaves Failed empty. The reproduce rule
// then reads "no failed tests" as "suite passed", burns the bounded reprompt
// budget on a false verdict, and escalates.
//
// This test builds exactly the live shape: baseline captured green with the
// pre-existing test, then the bug is introduced (uncommitted, like an
// operator re-seed), then the reproduce turn adds a red assertion test —
// both tests fail, and the gate MUST pass.

const bug531ReproTest = `package calc

import "testing"

func TestAddRepro(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}
`

// newBug531Workspace seeds a CORRECT implementation plus a baseline-green
// assertion test, captures the oracle baseline, then introduces the bug
// uncommitted — the exact live sequence (operator re-seeded base.go).
func newBug531Workspace(t *testing.T, testCmd string) (dir, head string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	dir = t.TempDir()
	if physical, err := filepath.EvalSymlinks(dir); err == nil {
		dir = physical
	}
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
			"HOME="+dir,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeBugFixFile(t, dir, "go.mod", "module calcapp\n\ngo 1.21\n")
	writeBugFixFile(t, dir, "calc/calc.go", "package calc\n\nfunc Add(a, b int) int { return a + b }\n")
	// Pre-existing test asserting correct behaviour — green at baseline.
	writeBugFixFile(t, dir, "calc/calc_test.go", "package calc\n\nimport \"testing\"\n\nfunc TestAddReturnsSum(t *testing.T) {\n\tif got := Add(2, 3); got != 5 {\n\t\tt.Fatalf(\"expected 5, got %d\", got)\n\t}\n}\n")
	gitRun("init")
	gitRun("add", "-A")
	gitRun("commit", "-m", "seed green calc")
	h, err := captureGitHead(dir)
	if err != nil || h == "" {
		t.Fatalf("captureGitHead: %q, %v", h, err)
	}
	writeBugFixBaseline(t, dir, h, testCmd, []string{"calc.TestAddReturnsSum", "TestAddReturnsSum"})
	gitRun("add", ".flowpilot/guard/test_baseline.json")
	gitRun("commit", "-m", "seed oracle baseline")
	h, err = captureGitHead(dir)
	if err != nil || h == "" {
		t.Fatalf("captureGitHead: %q, %v", h, err)
	}
	// Introduce the bug AFTER the baseline — uncommitted, like the live
	// operator re-seed of base.go between the question and the retry.
	writeBugFixFile(t, dir, "calc/calc.go", "package calc\n\nfunc Add(a, b int) int { return a + b + 1 }\n")
	return dir, h
}

func TestBug531_ReproduceGatePassesWhenBaselineTestAlsoRegresses(t *testing.T) {
	t.Setenv(reproduceGateFlag, "1")
	dir, head := newBug531Workspace(t, "go test -v ./...")
	svc, parentID := newReproduceFixture(t, dir)
	svc.lspChecker = &lspFakeChecker{}
	freezeP4Contract(t, dir, parentID, "implement", head, []string{"calc/calc.go"})

	// Oracle sanity: the suite must now be red AND the baseline-green test
	// must surface as a regression — the exact masking shape.
	baseline, err := flowgate.LoadBaseline(filepath.Join(dir, ".flowpilot"))
	if err != nil {
		t.Fatal(err)
	}
	writeBugFixFile(t, dir, "calc/reproduce_test.go", bug531ReproTest)
	red := flowgate.RunOracleContext(context.Background(), dir, baseline,
		[]flowgate.ChangedFile{
			{Path: "calc/calc.go", Status: "M"},
			{Path: "calc/reproduce_test.go", Status: "A"},
		}, nil)
	if red.SuitePassed {
		t.Fatalf("suite must be red after the bug is introduced: %+v", red)
	}
	if !red.HasRegression || len(red.Failed) < 2 {
		t.Fatalf("expected regression + named failures (TestAddReturnsSum + TestAddRepro), got %+v", red)
	}

	// The reproduce turn itself: the child only wrote the repro test.
	blocked, _ := runReproduceTurn(t, svc, parentID, dir, head, bug531ReproTest, "turn-531")
	if blocked {
		t.Fatal("r-reproduce must pass when the suite is red — regressions of baseline tests must not mask the reproduction (BUG-531)")
	}
}
