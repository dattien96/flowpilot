package runner

import (
	"context"
	"strings"
	"testing"
)

func TestBUG370_MarkdownWritesDoNotParkFrozenGate(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, "change-audit/FEATURE-KEYS.md", "- snake-mvp — terminal snake\n")
	p4WriteFile(t, dir, "requirements/.flowpilot/vibe/tdd-signatures.md", "// signatures\nfunc TestX(t *testing.T) {}\n")

	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{
			"src/calc.go",
			"change-audit/FEATURE-KEYS.md",
			"requirements/.flowpilot/vibe/tdd-signatures.md",
		},
	}, 0)
	if blocked {
		snap := svc.agentGraphSnapshot(parentID)
		t.Fatalf("markdown writes must not park frozen gate, gate=%q", snap.LoopState.GateReason)
	}
}

func TestBUG370_ExtraGoStillParks(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-2", parentID, dir, head)

	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, "src/extra.go", "package calc\n")

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"src/calc.go", "src/extra.go"},
	}, 0) {
		t.Fatal("src/extra.go outside declared paths must still park")
	}
	snap := svc.agentGraphSnapshot(parentID)
	if !strings.Contains(snap.LoopState.GateReason, "src/extra.go") {
		t.Fatalf("gate=%q want extra.go", snap.LoopState.GateReason)
	}
}

func TestBUG370_FlowRulesJSONStillParks(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-3", parentID, dir, head)

	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", "{}\n")

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"src/calc.go", ".flowpilot/settings/flow-rules.json"},
	}, 0) {
		t.Fatal("rewriting flow-rules.json must still park (CA-427)")
	}
}
