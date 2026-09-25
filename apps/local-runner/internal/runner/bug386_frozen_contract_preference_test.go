package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-386 (live run-19151): frozenContractForRun returned the FIRST
// agent.code/scaffold writer's record in topology order — the scaffold node
// (test_signatures) precedes the coder, and its record has no SignatureHash.
// coderSignaturesLocked never armed and isSignatureLockedCoderChild was false,
// so the coder shipped a signature that drifted from the locked SS with zero
// r-signature-lock violations and the renegotiate_signatures tool was never
// advertised.
func TestBug386_FrozenContractPrefersSignatureLockedRecord(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newReproduceFixture(t, dir)
	// Scaffold-then-coder topology: the scaffold writer is frozen too (sibling
	// binding), but only the coder record carries the locked SignatureHash.
	svc.mu.Lock()
	svc.runs[parentID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "test_signatures", Behavior: "agent.scaffold", Agent: "agents/scaffold.md"},
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	svc.runs[parentID].flowEngineDriven = true
	svc.mu.Unlock()
	freezeP4Contract(t, dir, parentID, "test_signatures", head, []string{"calc/calc.go"})
	freezeP4Contract(t, dir, parentID, "implement", head, []string{"calc/calc.go"})

	writeRepoFile(t, dir, "calc/calc.go", "package calc\n\nimport \"errors\"\n\nfunc Add(a, b int) error {\n\treturn errors.New(\"not implemented\")\n}\n")
	writeRepoFile(t, dir, "calc/scaffold_test.go", "package calc\n\nimport \"testing\"\n\nfunc TestRed(t *testing.T) { if err := Add(1,2); err == nil { t.Fatal() } }\n")
	svc.recordScaffoldArtifactsLock(dir, parentID, []string{"calc/calc.go", "calc/scaffold_test.go"})

	rec, ok := svc.frozenContractForRun(dir, parentID)
	if !ok {
		t.Fatal("no frozen contract found")
	}
	if rec.SignatureHash == "" {
		t.Fatalf("frozenContractForRun must prefer the signature-locked record; got step=%q hash=%q", rec.CoderStepID, rec.SignatureHash)
	}
	if rec.CoderStepID != "implement" {
		t.Fatalf("want the coder's locked record, got step %q", rec.CoderStepID)
	}
	coder := newReproduceChildRun(svc, "child-coder", parentID, dir, head, "implement")
	if !svc.isSignatureLockedCoderChild(coder) {
		t.Fatal("coder child on the signature-pinned contract must be detected (renegotiate tool gate)")
	}
}
