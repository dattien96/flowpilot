package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// CP-64 P-4 (Task-367): bug-fix lifecycle E2E through the reproduce gate.
// Mock provider turns (the runner fixtures) + a REAL go toolchain and REAL
// oracle, so the red -> green transition is proven physically, never mocked.
// New file — no pre-existing test is modified.

// newBugFixE2EWorkspace creates a git workspace with one real bug (Add returns
// a+b+1) plus a bug-agnostic sanity test, seeds a GREEN baseline fixture, and
// returns the workspace + its HEAD.
func newBugFixE2EWorkspace(t *testing.T, testCmd string) (dir, head string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}
	dir = t.TempDir()
	// macOS: t.TempDir() sits under /var (a symlink to /private/var). The
	// frozen preflight contract resolves declared paths through symlinks, so
	// freeze with the physical path or an existing test file would read as
	// escaping the workspace (paths.go symlink-escape check).
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
	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun("init")
	gitRun("config", "user.email", "t@e")
	gitRun("config", "user.name", "t")
	writeFile("go.mod", "module calcapp\n\ngo 1.21\n")
	// THE BUG: Add must return a+b.
	writeFile("calc/calc.go", "package calc\n\n// Add returns the sum of a and b.\nfunc Add(a, b int) int { return a + b + 1 }\n")
	// A pre-existing test that stays green BOTH before and after the fix (it
	// must never pin the buggy behaviour — additive-tests-only).
	writeFile("calc/sanity_test.go", "package calc\n\nimport \"testing\"\n\nfunc TestSanity(t *testing.T) {\n\tif got := Add(1, 1); got == 999 {\n\t\tt.Fatalf(\"impossible result: %d\", got)\n\t}\n}\n")
	// The oracle baseline is committed INTO the seed (tracked, unchanged
	// since BaseSHA): it is the r-reg ground truth, and an untracked runner
	// file would otherwise show up in the coder turn's frozen-scope diff.
	// (Production handles pre-existing dirt via the freeze's BaselineWorktree
	// snapshot; this fixture's freeze helper passes nil there, so the seed
	// commit keeps the model exact.) Green baseline fixture (Task-156 shape):
	// the hub captured it at flow start, BEFORE the reproducer wrote anything.
	// The reproduce test is added LATER (per turn), so the baseline reflects
	// a green suite and the turn diffs stay minimal. The oracle scopes
	// `go test ./...` to the changed package dirs (scope.go), so the baseline
	// keeps BOTH the scoped and unscoped name shapes.
	gitRun("add", "-A")
	gitRun("commit", "-m", "seed buggy calc")
	seedHead, err := captureGitHead(dir)
	if err != nil || seedHead == "" {
		t.Fatalf("captureGitHead: %q, %v", seedHead, err)
	}
	writeBugFixBaseline(t, dir, seedHead, testCmd, []string{"calc.TestSanity", "TestSanity"})
	gitRun("add", ".flowpilot/guard/test_baseline.json")
	gitRun("commit", "-m", "seed oracle baseline")
	h, err := captureGitHead(dir)
	if err != nil || h == "" {
		t.Fatalf("captureGitHead: %q, %v", h, err)
	}
	return dir, h
}

func writeBugFixBaseline(t *testing.T, dir, headSHA, testCmd string, green []string) {
	t.Helper()
	dotFP := filepath.Join(dir, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(dotFP, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}
	baseline := flowgate.Baseline{
		CapturedAt:  time.Now().UTC().Format(time.RFC3339),
		GreenTests:  green,
		TestCmd:     testCmd,
		HeadSHA:     headSHA,
		SuitePassed: true,
	}
	data, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	// CP-53 layout: the baseline lives under .flowpilot/guard/ (LoadBaseline).
	if err := os.WriteFile(filepath.Join(dotFP, "guard", "test_baseline.json"), data, 0o644); err != nil {
		if mkErr := os.MkdirAll(filepath.Join(dotFP, "guard"), 0o755); mkErr != nil {
			t.Fatal(mkErr)
		}
		if err := os.WriteFile(filepath.Join(dotFP, "guard", "test_baseline.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const (
	bugFixRedTest = `package calc

import "testing"

// Scenario: adding two numbers returns their sum.
func TestAddReturnsSum(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}
`
	// A test that PASSES against the buggy code — the bug was not reproduced.
	bugFixGreenTest = `package calc

import "testing"

func TestReproducesNothing(t *testing.T) {
	if got := Add(2, 3); got != 6 {
		t.Fatalf("expected 6, got %d", got)
	}
}
`
	bugFixBrokenTest = `package calc

import "testing"

func TestNeverCompiles(t *testing.T) {
	MissingFunction()
}
`
	// Legacy empty frame: the pre-CP-64 tester wrote bodies later filled by
	// the coder. It compiles and passes trivially — and proves nothing.
	bugFixEmptyFrameTest = `package calc

import "testing"

func TestEmptySignature(t *testing.T) {}
`
)

// runReproduceTurn drives one reproduce-node turn through the real child gate.
func runReproduceTurn(t *testing.T, svc *InteractiveService, parentID, dir, head, testContent, turnID string) (blocked bool, rs *interactiveRun) {
	t.Helper()
	writeBugFixFile(t, dir, "calc/reproduce_test.go", testContent)
	rs = newReproduceChildRun(svc, "repro-"+turnID, parentID, dir, head, "reproduce_test")
	blocked = svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, turnID,
		finalizeInput{FinalMessage: "wrote the reproduction test", ChangedFiles: []string{"calc/reproduce_test.go"}}, 0)
	return blocked, rs
}

func writeBugFixFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// commitBugFixTree commits the model's turn files (git add -A) with the fixture
// author identity, modelling a real flow turn that starts from a committed
// tree. It must NOT be used to hide the model's own writes inside the SUT
// assertions — the gate diffs used by the assertions are taken before it.
//
// Runner-owned bookkeeping under .flowpilot/ is deliberately left UNTRACKED:
// in a real flow nothing commits it mid-flow (the coder prompt forbids
// unapproved commits), and committing the oracle baseline
// (.flowpilot/guard/test_baseline.json — the r-reg ground truth) would make
// the coder turn's frozen-scope check correctly flag it as drift. Exempting
// the baseline in production would let a writer forge green tests, so the
// fixture keeps it untracked instead.
func commitBugFixTree(t *testing.T, dir, msg string) {
	t.Helper()
	run := func(args ...string) {
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
	run("add", "-A", "--", ".", ":!.flowpilot")
	run("commit", "-m", msg)
}

// bugFixHead returns the current HEAD of the fixture workspace.
func bugFixHead(t *testing.T, dir string) string {
	t.Helper()
	h, err := captureGitHead(dir)
	if err != nil || h == "" {
		t.Fatalf("captureGitHead: %q, %v", h, err)
	}
	return h
}

func assertNoImplementChild(t *testing.T, svc *InteractiveService) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	for id, rs := range svc.runs {
		if rs.label == "implement" || rs.stepID == "implement" {
			t.Fatalf("implement child %q must never be dispatched before the bug is reproduced", id)
		}
	}
}

func assertCoderContractUnlocked(t *testing.T, dir, parentID string) {
	t.Helper()
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, err := store.GetFrozenForStep(parentID, "implement")
	if err != nil || !ok {
		t.Fatalf("frozen contract for implement missing: ok=%v err=%v", ok, err)
	}
	if len(rec.ReadOnlyPaths) != 0 {
		t.Fatalf("a failed reproduction must never write the read-only lock, got %v", rec.ReadOnlyPaths)
	}
}

// Scenario: vong doi fix bug hoan chinh — reproducer viet test DO (assertion),
// gate pass va khoa file test, coder sua production (write vao file test bi
// deny), suite chuyen XANH va gate coder pass.
func TestBugFixLifecycleEndToEndWithReproduceGate(t *testing.T) {
	t.Setenv(reproduceGateFlag, "1")
	dir, head := newBugFixE2EWorkspace(t, "go test -v ./...")
	svc, parentID := newReproduceFixture(t, dir)
	// CP-63 adds a live-diagnostics allow-path hook on every gate pass. The
	// default server set would probe for a local gopls install, so pin the
	// hook OFF explicitly — this lifecycle test must never depend on one.
	svc.lspChecker = &lspFakeChecker{}
	// The fixture commits everything BEFORE the contract freezes, so the
	// frozen BaseSHA matches the coder's turn-start reference — exactly like a
	// real freeze happening before any writer runs.
	freezeP4Contract(t, dir, parentID, "implement", head, []string{"calc/calc.go"})
	baseline, err := flowgate.LoadBaseline(filepath.Join(dir, ".flowpilot"))
	if err != nil {
		t.Fatal(err)
	}

	// --- RED: the oracle itself proves the reproduce test fails on assertion.
	writeBugFixFile(t, dir, "calc/reproduce_test.go", bugFixRedTest)
	redDiff := []flowgate.ChangedFile{{Path: "calc/reproduce_test.go", Status: "A"}}
	red := flowgate.RunOracleContext(context.Background(), dir, baseline, redDiff, nil)
	if red.SuitePassed || len(red.Failed) != 1 || red.Failed[0] != "TestAddReturnsSum" {
		t.Fatalf("pre-fix oracle must show the failing reproduction test, got %+v", red)
	}
	if flowgate.ClassifySuiteOutput(baseline.TestCmd, red.Output) {
		t.Fatalf("the red run is an assertion failure, not a compile error: %q", red.Output)
	}

	// --- Reproduce turn: the gate must PASS the red test and lock the file.
	blocked, _ := runReproduceTurn(t, svc, parentID, dir, head, bugFixRedTest, "turn-red")
	if blocked {
		t.Fatal("a genuine assertion-failure reproduction must pass the r-reproduce gate")
	}
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, err := store.GetFrozenForStep(parentID, "implement")
	if err != nil || !ok {
		t.Fatalf("frozen contract missing after the reproduce pass: ok=%v err=%v", ok, err)
	}
	if got := changecontract.ReadOnlyLockedPaths(rec); len(got) != 1 || got[0] != "calc/reproduce_test.go" {
		t.Fatalf("reproduce test not locked read-only: %v", got)
	}

	// --- Coder turn: writing the locked test is silent-denied at the bridge.
	coder := newReproduceChildRun(svc, "coder-1", parentID, dir, head, "implement")
	if decision, reason, handled := svc.decideReproduceTestLock(coder, ApprovalDetails{Kind: "file", Command: "calc/reproduce_test.go", Reason: "Write"}); !handled || decision != "deny" {
		t.Fatalf("coder write to the locked test must be denied, got decision=%q handled=%v reason=%q", decision, handled, reason)
	}

	// --- Fix production code (calc/calc.go only) and re-run the coder gate.
	// Commit the pre-fix tree first so the turn-scoped diff of the coder turn
	// contains ONLY the fix (a real flow's coder turn also starts from a
	// committed tree — its turnStartGitHead is captured at turn start).
	commitBugFixTree(t, dir, "pre-fix tree: bug reproduced, test locked")
	writeBugFixFile(t, dir, "calc/calc.go", "package calc\n\n// Add returns the sum of a and b.\nfunc Add(a, b int) int { return a + b }\n")
	// The turn's own reference point is the commit just made, like the real
	// turnStartGitHead wiring captures at turn start.
	coder.turnStartGitHead = bugFixHead(t, dir)
	if blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), coder, "turn-fix",
		finalizeInput{FinalMessage: "fixed Add", ChangedFiles: []string{"calc/calc.go"}}, 0); blocked {
		t.Fatal("the coder's scoped production fix must pass the gate")
	}

	// --- GREEN: the same oracle now passes, and the reproduce test flipped
	// from Failed to Passed — the physical red -> green transition (T-3).
	greenDiff := []flowgate.ChangedFile{{Path: "calc/calc.go", Status: "M"}}
	green := flowgate.RunOracleContext(context.Background(), dir, baseline, greenDiff, nil)
	if !green.SuitePassed {
		t.Fatalf("post-fix suite must be green, got %+v output=%q", green, green.Output)
	}
	found := false
	for _, p := range green.Passed {
		if p == "TestAddReturnsSum" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the reproduce test must appear in Passed after the fix, got %+v", green.Passed)
	}
	if len(green.Regressed) != 0 {
		t.Fatalf("the fix must not regress the sanity test: %v", green.Regressed)
	}
}

// Scenario: the bug is NOT demonstrated — the gate must fail CLOSED (Task-367
// T-2). Three shapes: the test passes on arrival, the test does not compile,
// and the suite never runs (EnvError). Every shape ends in a reprompt at the
// reproduce node: no implement child is ever dispatched, no read-only lock is
// written, and production code is untouched.
func TestBugFixFailsClosedWhenBugNotReproduced(t *testing.T) {
	t.Setenv(reproduceGateFlag, "1")

	setup := func(t *testing.T, testCmd string) (svc *InteractiveService, parentID, dir, head string) {
		t.Helper()
		dir, head = newBugFixE2EWorkspace(t, testCmd)
		svc, parentID = newReproduceFixture(t, dir)
		svc.lspChecker = &lspFakeChecker{}
		freezeP4Contract(t, dir, parentID, "implement", head, []string{"calc/calc.go"})
		return svc, parentID, dir, head
	}
	assertFailClosed := func(t *testing.T, svc *InteractiveService, dir, parentID string) {
		t.Helper()
		assertNoImplementChild(t, svc)
		assertCoderContractUnlocked(t, dir, parentID)
		raw, err := os.ReadFile(filepath.Join(dir, "calc", "calc.go"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "a + b + 1") {
			t.Fatalf("production code must be untouched when the bug is not reproduced, got:\n%s", raw)
		}
	}

	t.Run("green on arrival", func(t *testing.T) {
		svc, parentID, dir, head := setup(t, "go test -v ./...")
		// The test PASSES against the unfixed code — nothing was reproduced.
		blocked, _ := runReproduceTurn(t, svc, parentID, dir, head, bugFixGreenTest, "turn-green")
		if !blocked {
			t.Fatal("a test that passes on arrival must NOT pass the r-reproduce gate")
		}
		assertFailClosed(t, svc, dir, parentID)
	})

	t.Run("compile error", func(t *testing.T) {
		svc, parentID, dir, head := setup(t, "go test -v ./...")
		// The test does not build — a compile error is never a reproduction.
		blocked, _ := runReproduceTurn(t, svc, parentID, dir, head, bugFixBrokenTest, "turn-broken")
		if !blocked {
			t.Fatal("a test with a compile error must NOT pass the r-reproduce gate")
		}
		assertFailClosed(t, svc, dir, parentID)
	})

	t.Run("suite never ran", func(t *testing.T) {
		// No suite binary: the oracle reports EnvError, so the gate observes
		// no run at all — fail closed, never open.
		svc, parentID, dir, head := setup(t, "flowpilot-nonexistent-suite-runner test ./...")
		blocked, _ := runReproduceTurn(t, svc, parentID, dir, head, bugFixRedTest, "turn-noenv")
		if !blocked {
			t.Fatal("a reproduce turn whose suite never ran must NOT pass the r-reproduce gate")
		}
		assertFailClosed(t, svc, dir, parentID)
	})
}

// Scenario (Task-367 T-3): with the flag ON, a feature-flow turn that never
// asked for reproduction (no reproduce node in the topology — the task-harness
// shape) is governed by the ordinary rules only. Empty test signatures pass
// exactly as before CP-64, and no read-only lock is written anywhere.
func TestTaskHarnessSignatureTurnPassesWithReproduceGateOn(t *testing.T) {
	t.Setenv(reproduceGateFlag, "1")
	dir, head := newBugFixE2EWorkspace(t, "go test -v ./...")
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parentID]
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "test_signatures", Behavior: "agent.code", Agent: "agents/tester.md", PromptTemplate: "prompts/test-signatures.md"},
	}
	prs.workspaceCwd = dir
	svc.mu.Unlock()
	svc.lspChecker = &lspFakeChecker{}
	freezeP4Contract(t, dir, parentID, "test_signatures", head, []string{"calc/empty_test.go"})

	writeBugFixFile(t, dir, "calc/empty_test.go", bugFixEmptyFrameTest)
	sig := newReproduceChildRun(svc, "sig-1", parentID, dir, head, "test_signatures")
	if svc.reproduceTurnForRun(sig) {
		t.Fatal("a flow without a reproduce node must never arm r-reproduce")
	}
	if blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), sig, "turn-sig",
		finalizeInput{FinalMessage: "wrote empty signatures", ChangedFiles: []string{"calc/empty_test.go"}}, 0); blocked {
		t.Fatal("an empty-signature feature turn must pass with the reproduce flag on, as before CP-64")
	}
	store, storeErr := changecontract.NewFrozenStore(dir)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	rec, ok, recErr := store.GetFrozenForStep(parentID, "test_signatures")
	if recErr != nil || !ok {
		t.Fatalf("frozen contract missing: ok=%v err=%v", ok, recErr)
	}
	if len(rec.ReadOnlyPaths) != 0 {
		t.Fatalf("a non-reproduce turn must never write the read-only lock, got %v", rec.ReadOnlyPaths)
	}
}

// CP-67 B-9 supersession of Task-367 T-4 / CP-64 §8: the flag is retired, so
// the "legacy path with the gate off" no longer exists. The test now pins the
// ALWAYS-ON contract with the env var UNSET — the reproduce turn still arms
// r-reproduce, an all-green empty-frame turn still fails the gate, the test
// file is still locked, and the coder is still denied writes to it.
func TestBugHarnessReproduceGateAlwaysOnAfterFlagRetire(t *testing.T) {
	t.Setenv(reproduceGateFlag, "") // retired: no effect either way
	dir, head := newBugFixE2EWorkspace(t, "go test -v ./...")
	svc, parentID := newReproduceFixture(t, dir)
	svc.lspChecker = &lspFakeChecker{}
	freezeP4Contract(t, dir, parentID, "implement", head, []string{"calc/calc.go"})

	writeBugFixFile(t, dir, "calc/empty_test.go", bugFixEmptyFrameTest)
	child := newReproduceChildRun(svc, "repro-1", parentID, dir, head, "reproduce_test")
	if !svc.reproduceTurnForRun(child) {
		t.Fatal("the gate is always-on: the reproduce turn must arm r-reproduce even with the env var unset")
	}
	// With the gate always-on, an all-green empty-frame turn is exactly what
	// r-reproduce rejects: the gate BLOCKS with the reprompt (the legacy
	// flag-off pass-through no longer exists).
	if blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), child, "turn-repro",
		finalizeInput{FinalMessage: "wrote empty signatures", ChangedFiles: []string{"calc/empty_test.go"}}, 0); !blocked {
		t.Fatal("an all-green empty-frame turn must block under the always-on r-reproduce gate")
	}
	// The turn was blocked, so no read-only lock is written yet (the lock is a
	// PASS-path side effect); the always-on bridge-enforcement matrix lives in
	// TestCoderTestFileLockAlwaysOnAfterFlagRetire (reproduce_lock_test.go).
	assertCoderContractUnlocked(t, dir, parentID)
}
