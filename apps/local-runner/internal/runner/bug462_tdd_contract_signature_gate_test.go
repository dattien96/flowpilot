package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/workingmode"
)

// BUG-462: the vibe coder spawn gate (hasVibeTddOutput) only looks at the
// filesystem — tdd-signatures.md or git-new signature-only *_test.go frames.
// When the sanctioned tdd/scaffold step produced (or adopted) a FULL-BODY
// test file, the filesystem check correctly refuses to count it (CA-769) —
// but the scaffold gate's own durable evidence, the frozen contract's
// LockedSignatures/SignatureHash pinned on the coder step's record, is
// ignored. Result: vibe-sprint parks "tdd artifact missing" even though TDD
// provably ran (live run-37268 parked after tdd DONE + signature pin).
//
// Contract-pinned signatures are strictly stronger evidence than a path glob:
// they only exist after the scaffold gate passed for THIS run's contract, so
// counting them cannot be fooled by stale full-body test files.

const bug462FullBodyTest = `package snake

import "testing"

func TestInBounds(t *testing.T) {
	if !InBounds(Point{0, 0}) {
		t.Fatal("origin must be in bounds")
	}
}
`

func bug462SeedLockedContract(t *testing.T, svc *InteractiveService, dir, runID string) {
	t.Helper()
	edges, nodes := vibeSprintWriterChain()
	freezeNode, _ := findFlowNode(nodes, "preflight_contract_freeze")
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected freeze to be handled")
	}
	// Post-scaffold state: signatures pinned on the coder contract (v2) while
	// the workspace test file carries full bodies — not a filesystem artifact.
	if _, err := changecontract.LockScaffoldArtifactsForStep(dir, runID, "coder",
		[]string{"snake/model_test.go"},
		"ab6692057932",
		[]string{"func InBounds(p Point) bool", "func NewGame(r *rand.Rand) *Game"},
		time.Now()); err != nil {
		t.Fatalf("LockScaffoldArtifactsForStep: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "snake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snake", "model_test.go"), []byte(bug462FullBodyTest), 0o644); err != nil {
		t.Fatal(err)
	}
	if hasVibeTddOutput(dir) {
		t.Fatal("fixture invalid: full-body test file must NOT satisfy the filesystem artifact check")
	}
}

func TestBUG462_AdvanceTddToCoderUsesContractSignatures(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := vibeSprintWriterChain()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.mu.Lock()
	svc.runs[runID].workingMode = workingmode.Vibe
	svc.mu.Unlock()

	bug462SeedLockedContract(t, svc, dir, runID)

	if !svc.tryAdvanceFlowFromNode(runID, "tdd", "scaffold gate passed") {
		t.Fatal("advance tdd->coder must not park when the frozen contract carries locked signatures")
	}
	waitLoop(t, "coder spawned via contract-pinned signatures", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") == 1
	})
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if strings.Contains(loop.GateReason, "tdd artifact missing") {
		t.Fatalf("must not false-park on tdd artifact: gate=%q", loop.GateReason)
	}
}

func TestBUG462_ResumeCoderUsesContractSignatures(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := vibeSprintWriterChain()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.workingMode = workingmode.Vibe
	rs.vibeCheckpointNode = "tdd"
	svc.mu.Unlock()

	bug462SeedLockedContract(t, svc, dir, runID)

	svc.maybeResumeVibeCoderAfterTdd(runID)
	waitLoop(t, "coder spawned on resume via contract-pinned signatures", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") == 1
	})
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status == "blocked" && strings.Contains(loop.GateReason, "tdd artifact missing") {
		t.Fatalf("resume must not false-park on tdd artifact: %+v", loop)
	}
}

// Live sprint-2 shape (run-37268): the scaffold gate locked test files
// read-only but pinned EMPTY signatures (BUG-463's absolute-path read
// failure). The ReadOnlyPaths on the coder contract are still the runner's
// own post-freeze attestation — the gate must accept them.
func TestBUG462_ReadOnlyLockWithoutSignaturesSatisfiesGate(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := vibeSprintWriterChain()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.workingMode = workingmode.Vibe
	rs.vibeCheckpointNode = "tdd"
	svc.mu.Unlock()

	freezeNode, _ := findFlowNode(nodes, "preflight_contract_freeze")
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected freeze to be handled")
	}
	if _, err := changecontract.LockReproduceTestPaths(dir, runID, "coder",
		[]string{"snake/model_test.go"}, time.Now()); err != nil {
		t.Fatalf("LockReproduceTestPaths: %v", err)
	}
	if hasVibeTddOutput(dir) {
		t.Fatal("fixture invalid: no filesystem artifact must exist")
	}

	svc.maybeResumeVibeCoderAfterTdd(runID)
	waitLoop(t, "coder spawned via read-only lock evidence", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") == 1
	})
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if strings.Contains(loop.GateReason, "tdd artifact missing") {
		t.Fatalf("read-only lock on the contract must satisfy the tdd gate: %+v", loop)
	}
}

// Guard rail: a frozen contract WITHOUT locked signatures (preflight freeze
// only, tdd never ran) must still park — the contract check must not weaken
// the fail-closed gate.
func TestBUG462_FrozenContractWithoutSignaturesStillParks(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := vibeSprintWriterChain()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.workingMode = workingmode.Vibe
	rs.vibeCheckpointNode = "tdd"
	svc.mu.Unlock()

	// Freeze only — no LockScaffoldArtifacts, no filesystem artifact.
	freezeNode, _ := findFlowNode(nodes, "preflight_contract_freeze")
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected freeze to be handled")
	}

	svc.maybeResumeVibeCoderAfterTdd(runID)
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if !(loop.Status == "blocked" && strings.Contains(loop.GateReason, "tdd artifact missing")) {
		t.Fatalf("bare frozen contract must not satisfy the tdd gate: %+v", loop)
	}
	if n := countChildrenWithLabel(svc, runID, "coder"); n != 0 {
		t.Fatalf("coder must not spawn without tdd evidence, children=%d", n)
	}
}
