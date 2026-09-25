package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/changecontract"
)

// BUG-463: provider file-change events may report ABSOLUTE paths (devin did
// for run-37268's sprint-2 scaffold turn). Two downstream breaks:
//
//  a) scaffoldSignatureSnapshot does filepath.Join(cwd, abs) → read fails →
//     empty SignatureHash/LockedSignatures pinned on the frozen contract →
//     the BUG-462 coder gate then has no contract evidence and false-parks.
//  b) recordScaffoldArtifactsLock stores those absolute paths into
//     ReadOnlyPaths (LockScaffoldArtifacts has no relativize — the
//     BUG-388 relativizing wrapper LockScaffoldArtifactsForStep is bypassed),
//     so the read-only deny-list never matches the workspace-relative writes
//     it is meant to block.
//
// Fix normalizes once at ingestion (finalizeInputLocked) and defensively in
// recordScaffoldArtifactsLock for carried/legacy records.

func TestBUG463_FinalizeRelativizesAbsoluteChangedFiles(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	dir := t.TempDir()
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workspaceCwd = dir
	rs.events = append(rs.events,
		ProviderEvent{Type: EventFileChanged, ProviderTurnID: "turn-1", Path: filepath.Join(dir, "snake", "loop.go")},
		ProviderEvent{Type: EventFileChanged, ProviderTurnID: "turn-1", Path: filepath.Join(dir, "snake", "loop_test.go")},
		ProviderEvent{Type: EventFileChanged, ProviderTurnID: "turn-1", Path: "already/relative.go"},
		// outside-workspace absolute stays verbatim — cannot be relativized
		ProviderEvent{Type: EventFileChanged, ProviderTurnID: "turn-1", Path: "/elsewhere/outside.go"},
	)
	svc.mu.Unlock()

	svc.mu.Lock()
	in := svc.finalizeInputLocked(rs, "turn-1")
	svc.mu.Unlock()

	want := []string{"snake/loop.go", "snake/loop_test.go", "already/relative.go", "/elsewhere/outside.go"}
	if len(in.ChangedFiles) != len(want) {
		t.Fatalf("ChangedFiles=%v want %v", in.ChangedFiles, want)
	}
	for i := range want {
		if in.ChangedFiles[i] != want[i] {
			t.Fatalf("ChangedFiles[%d]=%q want %q (all %v)", i, in.ChangedFiles[i], want[i], in.ChangedFiles)
		}
	}
}

func TestBUG463_ScaffoldLockRelativizesAndPinsSignatures(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := vibeSprintWriterChain()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "preflight_contract_freeze")
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected freeze to be handled")
	}

	// tdd's own contract is what flowWriterNodeIDForRun binds in this fixture
	// (first agent.code). Write a real stub file so signature extraction has
	// something to parse.
	stub := "package calc\n\n// Add returns the sum.\nfunc Add(a, b int) int {\n\tpanic(\"not implemented\")\n}\n"
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "calc.go"), []byte(stub), 0o644); err != nil {
		t.Fatal(err)
	}
	testFile := "package calc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {}\n"
	if err := os.WriteFile(filepath.Join(dir, "src", "calc_test.go"), []byte(testFile), 0o644); err != nil {
		t.Fatal(err)
	}

	// What devin emitted for the sprint-2 scaffold turn: absolute paths.
	abs := []string{filepath.Join(dir, "src", "calc.go"), filepath.Join(dir, "src", "calc_test.go")}
	svc.recordScaffoldArtifactsLock(dir, runID, abs)

	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, err := store.GetFrozenForStep(runID, "tdd")
	if err != nil || !ok {
		t.Fatalf("frozen contract for tdd missing: ok=%v err=%v", ok, err)
	}
	if len(rec.LockedSignatures) == 0 || rec.SignatureHash == "" {
		t.Fatalf("absolute written paths must still pin signatures; got sigs=%v hash=%q", rec.LockedSignatures, rec.SignatureHash)
	}
	for _, p := range rec.ReadOnlyPaths {
		if filepath.IsAbs(filepath.FromSlash(p)) {
			t.Fatalf("ReadOnlyPaths must be workspace-relative, got %q (all %v)", p, rec.ReadOnlyPaths)
		}
	}
}
