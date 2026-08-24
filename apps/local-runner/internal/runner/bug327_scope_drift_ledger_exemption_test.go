package runner

import (
	"context"
	"strings"
	"testing"
)

// BUG-327: runner-internal ledger writes (.flowpilot/ledger/chat_summary.ndjson,
// .flowpilot/ledger/feature_history.ndjson) must NOT false-positive as scope
// drift during frozen contract gate checks. True drift (extra.go, settings/flow-rules.json)
// must still escalate, and escalate park must leave child status as
// waiting_user_approval (not running).
func TestBUG327_RunnerLedgerWritesDoNotTriggerScopeDrift(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	// Writer changed declared file AND runner wrote ledger files
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, ".flowpilot/ledger/chat_summary.ndjson", `{"summary":"hello"}`+"\n")
	p4WriteFile(t, dir, ".flowpilot/ledger/feature_history.ndjson", `{"feature":"calc-core"}`+"\n")

	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{
			"src/calc.go",
			".flowpilot/ledger/chat_summary.ndjson",
			".flowpilot/ledger/feature_history.ndjson",
		},
	}, 0)

	if blocked {
		t.Fatal("ledger writes must not cause scope drift block")
	}
}

func TestBUG327_RunnerLedgerWritesPlusTrueDriftStillBlocks(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	// Writer changed declared file + ledger file + UNEXPECTED file (extra.go)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, ".flowpilot/ledger/chat_summary.ndjson", `{"summary":"hello"}`+"\n")
	p4WriteFile(t, dir, "src/extra.go", "package calc\n")

	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{
			"src/calc.go",
			".flowpilot/ledger/chat_summary.ndjson",
			"src/extra.go",
		},
	}, 0)

	if !blocked {
		t.Fatal("expected scope drift block when unexpected extra.go is written")
	}

	snap := svc.agentGraphSnapshot(parentID)
	if snap.LoopState.Status != "blocked" || snap.LoopState.BlockReason != "escalate" {
		t.Fatalf("expected loop blocked (escalate), got status %q reason %q", snap.LoopState.Status, snap.LoopState.BlockReason)
	}
	if !strings.Contains(snap.LoopState.GateReason, "src/extra.go") {
		t.Fatalf("expected gate reason to mention src/extra.go, got %q", snap.LoopState.GateReason)
	}
}

func TestBUG327_FlowRulesModificationStillBlocks(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", `{"disabled":true}`)

	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"src/calc.go", ".flowpilot/settings/flow-rules.json"},
	}, 0)

	if !blocked {
		t.Fatal("expected block: .flowpilot/settings/flow-rules.json is not exempt")
	}
}

func TestBUG327_EscalateParkSetsChildWaitingUserApproval(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	svc.agentOrchestrator.registerChild(parentID, rs.id)
	svc.agentOrchestrator.upsertSummary(parentID, AgentRunSummary{
		RunID:       rs.id,
		ParentRunID: parentID,
		AgentName:   "coder",
		Role:        "coder",
		Status:      RunStatusRunning,
	})

	// Apply flow control escalate
	_, err := svc.applyFlowControl(parentID, FlowControlInput{
		Status:  "escalate",
		Summary: "flow scope drift",
	})
	if err != nil {
		t.Fatal(err)
	}

	snap := svc.agentGraphSnapshot(parentID)
	if snap.LoopState.Status != "blocked" {
		t.Fatalf("expected loop blocked, got %q", snap.LoopState.Status)
	}

	// Verify child in snapshot is waiting_user_approval, not running
	foundChild := false
	for _, r := range snap.Runs {
		if r.RunID == rs.id {
			foundChild = true
			if r.Status == "running" {
				t.Fatalf("child status must not be running when flow is parked, got %q", r.Status)
			}
			if r.Status != "waiting_user_approval" {
				t.Fatalf("child status want waiting_user_approval, got %q", r.Status)
			}
		}
	}
	if !foundChild {
		t.Fatal("expected child in graph snapshot")
	}
}
