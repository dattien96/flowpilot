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

	// Writer changed declared file AND runner wrote manifest/ledger files AND coder wrote CA note
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, ".flowpilot/manifest.json", `{"version":1}`+"\n")
	p4WriteFile(t, dir, ".flowpilot/ledger/chat_summary.ndjson", `{"summary":"hello"}`+"\n")
	p4WriteFile(t, dir, ".flowpilot/ledger/feature_history.ndjson", `{"feature":"calc-core"}`+"\n")
	p4WriteFile(t, dir, "change-audit/CA-914-calc-format-clamp-checked.md", "# CA-914\n")

	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{
			"src/calc.go",
			".flowpilot/manifest.json",
			".flowpilot/ledger/chat_summary.ndjson",
			".flowpilot/ledger/feature_history.ndjson",
			"change-audit/CA-914-calc-format-clamp-checked.md",
		},
	}, 0)

	if blocked {
		t.Fatal("manifest, ledger, and change-audit writes must not cause scope drift block")
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

func TestBUG327_EscalateParkPreservesSummaryFields(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	svc.agentOrchestrator.registerChild(parentID, rs.id)
	svc.agentOrchestrator.upsertSummary(parentID, AgentRunSummary{
		RunID:         rs.id,
		ParentRunID:   parentID,
		AgentName:     "coder",
		Role:          "coder",
		Status:        RunStatusRunning,
		ActivationSeq: 42,
		ProviderKey:   "codex",
		ModelName:     "gpt-5.1",
		WaitForResult: true,
		DependsOn:     []string{"dep-1", "dep-2"},
	})

	svc.parkFlowForAwaitingUser(parentID)

	snap := svc.agentGraphSnapshot(parentID)
	var childSummary *AgentRunSummary
	for _, r := range snap.Runs {
		if r.RunID == rs.id {
			childSummary = &r
			break
		}
	}
	if childSummary == nil {
		t.Fatal("child summary not found")
	}
	if childSummary.Status != RunStatusWaitingUserApr {
		t.Fatalf("status = %q, want %q", childSummary.Status, RunStatusWaitingUserApr)
	}
	if childSummary.ActivationSeq != 42 {
		t.Fatalf("ActivationSeq = %d, want 42", childSummary.ActivationSeq)
	}
	if childSummary.ProviderKey != "codex" {
		t.Fatalf("ProviderKey = %q, want codex", childSummary.ProviderKey)
	}
	if childSummary.ModelName != "gpt-5.1" {
		t.Fatalf("ModelName = %q, want gpt-5.1", childSummary.ModelName)
	}
	if !childSummary.WaitForResult {
		t.Fatal("WaitForResult want true")
	}
	if len(childSummary.DependsOn) != 2 || childSummary.DependsOn[0] != "dep-1" {
		t.Fatalf("DependsOn corrupted: %v", childSummary.DependsOn)
	}
}

func TestBUG327_GateBlockPostTurnSettleKeepsWaitingUserApproval(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	svc.agentOrchestrator.registerChild(parentID, rs.id)
	svc.agentOrchestrator.upsertSummary(parentID, AgentRunSummary{
		RunID:       rs.id,
		ParentRunID: parentID,
		AgentName:   "coder",
		Role:        "coder",
		Status:      RunStatusRunning,
	})

	// Writer changed declared file + extra.go to trigger scope drift gate block
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, "src/extra.go", "package calc\n")

	rs.pendingGateChangedFiles = []string{"src/calc.go", "src/extra.go"}
	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"src/calc.go", "src/extra.go"},
	}, 0)

	if !blocked {
		t.Fatal("expected scope drift block")
	}

	// Verify loop is blocked
	if loop := svc.agentOrchestrator.loopStateFor(parentID); loop.Status != "blocked" {
		t.Fatalf("expected loop blocked, got %q", loop.Status)
	}

	// Simulate finishTurn post-gate settle branch
	svc.mu.Lock()
	if rs.pendingFlowGateSettle {
		rs.pendingFlowGateSettle = false
		rs.pendingFlowGateFinalMsg = ""
		rs.pendingFlowGateOccurredAt = ""
		rs.pendingFlowGateTurnID = ""
		rs.pendingGateChangedFiles = nil
		if rs.parentRunID != "" && svc.agentOrchestrator.loopStateFor(rs.parentRunID).Status == "blocked" {
			rs.status = RunStatusWaitingUserApr
			rs.agentStatus = "waiting_user_approval"
		} else {
			rs.status = RunStatusRunning
			rs.agentStatus = string(RunStatusRunning)
		}
		rs.turnInFlight = false
		rs.postTurnGateCancel = nil
	}
	svc.mu.Unlock()

	if rs.status != RunStatusWaitingUserApr {
		t.Fatalf("rs.status = %q, want %q", rs.status, RunStatusWaitingUserApr)
	}
	if rs.agentStatus != "waiting_user_approval" {
		t.Fatalf("rs.agentStatus = %q, want waiting_user_approval", rs.agentStatus)
	}
	if rs.pendingGateChangedFiles != nil {
		t.Fatalf("expected pendingGateChangedFiles cleared, got %v", rs.pendingGateChangedFiles)
	}
}

func TestBUG327_ScopeDriftEscalateStampsLastEscalatedNodeID(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "implement", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	rs.label = "implement"

	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, "src/extra.go", "package calc\n")

	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"src/calc.go", "src/extra.go"},
	}, 0)

	if !blocked {
		t.Fatal("expected scope drift block")
	}

	// Verify parent.lastEscalatedInlineNodeID was stamped with implement
	svc.mu.Lock()
	parent := svc.runs[parentID]
	lastEsc := parent.lastEscalatedInlineNodeID
	svc.mu.Unlock()

	if lastEsc != "implement" {
		t.Fatalf("parent.lastEscalatedInlineNodeID = %q, want implement", lastEsc)
	}
}
