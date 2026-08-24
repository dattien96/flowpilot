package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
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
		AgentStatus:   string(RunStatusRunning),
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
	if childSummary.AgentStatus != "waiting_user_approval" {
		t.Fatalf("AgentStatus = %q, want waiting_user_approval (Desktop Agents panel reads agentStatus)", childSummary.AgentStatus)
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

	// Writer changed declared file + extra.go to trigger a real scope-drift
	// gate block, which parks the parent loop to blocked + escalates.
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
	if loop := svc.agentOrchestrator.loopStateFor(parentID); loop.Status != "blocked" {
		t.Fatalf("expected loop blocked, got %q", loop.Status)
	}

	// finishTurn's post-gate settle calls the PRODUCTION helper; with the parent
	// loop blocked it must keep the child WAITING_USER_APPROVAL (park), not reset
	// it to Running (which would keep TUI Thinking on and hide Continue/Stop).
	svc.mu.Lock()
	svc.settleChildStatusAfterGateBlockLocked(rs)
	svc.mu.Unlock()

	if rs.status != RunStatusWaitingUserApr {
		t.Fatalf("rs.status = %q, want %q", rs.status, RunStatusWaitingUserApr)
	}
	if rs.agentStatus != "waiting_user_approval" {
		t.Fatalf("rs.agentStatus = %q, want waiting_user_approval", rs.agentStatus)
	}
	// The helper must push the park to the orchestrator graph the TUI/desktop
	// read — a settle-only write to rs.status would leave the Agents panel stale.
	{
		snap := svc.agentGraphSnapshot(parentID)
		found := false
		for _, r := range snap.Runs {
			if r.RunID == rs.id {
				found = true
				if r.Status != RunStatusWaitingUserApr || r.AgentStatus != "waiting_user_approval" {
					t.Fatalf("graph summary after blocked settle = status %q agentStatus %q, want %q/%q",
						r.Status, r.AgentStatus, RunStatusWaitingUserApr, "waiting_user_approval")
				}
			}
		}
		if !found {
			t.Fatal("child summary missing from graph after settle")
		}
	}

	// The same settle on a NON-blocked loop keeps Running (the reprompt path).
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	svc.settleChildStatusAfterGateBlockLocked(rs)
	svc.mu.Unlock()
	if rs.status != RunStatusRunning {
		t.Fatalf("non-blocked settle: rs.status = %q, want %q", rs.status, RunStatusRunning)
	}
	{
		snap := svc.agentGraphSnapshot(parentID)
		for _, r := range snap.Runs {
			if r.RunID == rs.id {
				if r.Status != RunStatusRunning {
					t.Fatalf("graph summary after non-blocked settle = %q, want %q", r.Status, RunStatusRunning)
				}
			}
		}
	}
}

// flowNodeInlineDispatchable must stay in lock-step with
// tryAdvanceFlowThroughInline's dispatch switch: writer/delegate nodes are not
// inline-dispatchable (BUG-327 — Continue must retry the child, not no-op).
// tryAdvance is guarded by the same predicate, so a non-dispatchable behavior
// can never be silently dispatched (and thus never no-op the Continue path).
func TestBUG327_FlowNodeInlineDispatchable(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	cases := []struct {
		behavior string
		want     bool
	}{
		{"agent.code", false},
		{"agent.delegate", false},
		{"command.validate", true},
		{"artifact.audit_draft", true},
		{"contract.freeze", true},
		{"telegram.notify", true},
		{"hub.notify", true},
		{"", false},
		{"bogus", false},
	}
	for _, tc := range cases {
		if got := flowNodeInlineDispatchable(agentpack.FlowNode{Behavior: tc.behavior}); got != tc.want {
			t.Errorf("flowNodeInlineDispatchable(%q) = %v, want %v", tc.behavior, got, tc.want)
		}
		if !tc.want {
			// On a REAL, non-terminal run, tryAdvance must refuse to dispatch a
			// writer/delegate behavior: if it ever returns true here, Continue
			// would treat the escalated writer node as "advanced" (a no-op) and
			// hang the flow — the exact run-221516 failure mode. This locks the
			// lock-step: a future switch case must ALSO update the helper.
			if adv := svc.tryAdvanceFlowThroughInline(parent.RunID, nil, nil, agentpack.FlowNode{Behavior: tc.behavior}, ""); adv {
				t.Errorf("tryAdvanceFlowThroughInline(%q) must return false (guard)", tc.behavior)
			}
		}
	}
}

// Continue after a scope-drift escalate on a rag-harness implement (agent.code)
// must retry the implement child — not tryAdvance no-op, not hub->done skip.
//
// The implement child's activationSeq is the deterministic reinvoke signal:
// reinvokeMatchingFlowChild increments it synchronously and a re-park (the
// retried coder writing out-of-scope again, correct behavior for genuine drift)
// preserves it — whereas both broken paths leave it at 0. Provider-agnostic
// routing (no providerKey branch), matrixed claude/codex/grok per house rule.
func TestBUG327_ScopeDriftContinueReinvokesImplementChild(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider ProviderKey
		model    string
	}{
		{"grok", ProviderKeyGrok, "grok-4.5"},
		{"codex", ProviderKeyCodex, "gpt-5.4-mini"},
		{"claude", ProviderKeyClaude, "claude-sonnet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
			parent, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
			pid := parent.RunID
			svc.agentOrchestrator.setLoop(pid, AgentLoopState{Status: "blocked", Mode: "explicit", Cap: 3, RoundCap: 3, BlockReason: "escalate"})
			nodes := ragHarnessNodes()
			svc.mu.Lock()
			p := svc.runs[pid]
			p.activeFlowNodes = nodes
			p.activeFlowAcceptanceNodes = []string{"validate", "audit"}
			p.autoOrchestrate = true
			p.flowEngineDriven = true
			p.chatFlowRef = "flowpilot-core-flow-pack/rag-harness"
			p.modelName = tc.model
			p.providerKey = tc.provider
			p.lastEscalatedInlineNodeID = "implement"
			svc.mu.Unlock()
			svc.reseedFlowStepRuntime(pid, nodes)
			svc.setFlowStepStatus(context.Background(), pid, "implement", StepStatusWaitingUserApr)

			// implement child parked (CA-627 park stamp)
			child, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
			svc.mu.Lock()
			crs := svc.runs[child.RunID]
			crs.parentRunID = pid
			crs.agentName = "coder"
			crs.label = "implement"
			crs.role = "coder"
			crs.providerKey = tc.provider
			crs.modelName = tc.model
			crs.status = RunStatusWaitingUserApr
			crs.agentStatus = "waiting_user_approval"
			svc.mu.Unlock()
			svc.agentOrchestrator.registerChild(pid, child.RunID)
			svc.agentOrchestrator.upsertSummary(pid, AgentRunSummary{RunID: child.RunID, AgentName: "coder", Label: "implement", Role: "coder", Status: RunStatusWaitingUserApr, ParentRunID: pid})

			_, err := svc.resumeFlowWithFeedback(pid, "")
			if err != nil {
				t.Fatalf("%s: resumeFlowWithFeedback: %v", tc.name, err)
			}

			svc.mu.Lock()
			activationSeq := svc.runs[child.RunID].activationSeq
			childStatus := svc.runs[child.RunID].status
			lastEsc := svc.runs[pid].lastEscalatedInlineNodeID
			loop := svc.agentOrchestrator.loopStateFor(pid)
			svc.mu.Unlock()

			if lastEsc != "" {
				t.Fatalf("%s: lastEscalatedInlineNodeID not cleared, got %q", tc.name, lastEsc)
			}
			if activationSeq != 1 {
				t.Fatalf("%s: implement child was not reinvoked (activationSeq=%d, want 1) — Continue must retry the child, not leave it parked", tc.name, activationSeq)
			}
			// The reinvoke schedules the child synchronously (Running). By the
			// time we read, the retried turn may already have completed
			// (completed + loop running) or the gate re-parked it (waiting +
			// loop blocked) — all legitimate retry outcomes. The ONLY forbidden
			// state is the old hang: child still parked with the loop still
			// running (tryAdvance no-op never scheduled anything).
			if childStatus == RunStatusWaitingUserApr && loop.Status != "blocked" {
				t.Fatalf("%s: child still parked (status=%q) with loop=%q — Continue must retry the child, not no-op", tc.name, childStatus, loop.Status)
			}
		})
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

// FEATURE-KEYS.md (the feature registry) must NOT be treated as a coder's own
// change-audit note — writing it is real scope drift and must still block
// (BUG-278 allowed CA-* notes only).
func TestBUG327_FeatureKeysWriteStillBlocks(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, "change-audit/FEATURE-KEYS.md", "# keys\n")

	blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "done",
		ChangedFiles: []string{"src/calc.go", "change-audit/FEATURE-KEYS.md"},
	}, 0)

	if !blocked {
		t.Fatal("expected block: change-audit/FEATURE-KEYS.md is not exempt")
	}
	snap := svc.agentGraphSnapshot(parentID)
	if !strings.Contains(snap.LoopState.GateReason, "change-audit/FEATURE-KEYS.md") {
		t.Fatalf("expected gate reason to name FEATURE-KEYS.md, got %q", snap.LoopState.GateReason)
	}
}

// Restart twin of the gate-block settle: resumePendingFlowGate re-runs the
// pending gate after a crash. When the parent loop is ALREADY blocked (park),
// it must keep the child WAITING_USER_APPROVAL, not force it back to Running —
// the old hardcoded Running undid the park and re-armed TUI Thinking.
func TestBUG327_ResumePendingFlowGateKeepsWaitingWhenLoopBlocked(t *testing.T) {
	t.Run("loop_already_blocked_early_return", func(t *testing.T) {
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
			Status:      RunStatusWaitingUserApr,
		})

		// Park already in effect: loop blocked + child stamped waiting.
		svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "blocked", Cap: 3, RoundCap: 3, BlockReason: "escalate"})
		svc.mu.Lock()
		rs.status = RunStatusWaitingUserApr
		rs.agentStatus = "waiting_user_approval"
		rs.pendingFlowGateSettle = true
		rs.pendingFlowGateTurnID = "turn-1"
		rs.pendingGateChangedFiles = []string{"src/extra.go"}
		svc.mu.Unlock()

		svc.resumePendingFlowGate(rs.id)

		svc.mu.Lock()
		defer svc.mu.Unlock()
		if rs.status != RunStatusWaitingUserApr {
			t.Fatalf("child status = %q, want %q (park must survive resume)", rs.status, RunStatusWaitingUserApr)
		}
		if rs.pendingFlowGateSettle {
			t.Fatal("stale pendingFlowGateSettle must be cleared on blocked loop")
		}
	})

	t.Run("rerun_gate_parks_not_running", func(t *testing.T) {
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

		// Crash window: settle armed, loop still running, out-of-scope file
		// already in the worktree. Resume re-runs the child gate → escalate →
		// park; the settle must keep the child WAITING, not reset Running.
		p4WriteFile(t, dir, "src/calc.go", "package calc\n")
		p4WriteFile(t, dir, "src/extra.go", "package calc\n")
		svc.mu.Lock()
		rs.pendingFlowGateSettle = true
		rs.pendingFlowGateTurnID = "turn-1"
		rs.pendingGateChangedFiles = []string{"src/calc.go", "src/extra.go"}
		svc.mu.Unlock()

		svc.resumePendingFlowGate(rs.id)

		svc.mu.Lock()
		defer svc.mu.Unlock()
		if rs.status != RunStatusWaitingUserApr {
			t.Fatalf("child status after re-run gate block = %q, want %q (park, not Running)", rs.status, RunStatusWaitingUserApr)
		}
		if loop := svc.agentOrchestrator.loopStateFor(parentID); loop.Status != "blocked" {
			t.Fatalf("loop = %q, want blocked", loop.Status)
		}
	})
}

// A late stray event after park (e.g. a second flow_gate_violation) must NOT
// unpark the child — the default emitLocked branch must skip WAITING_USER_APPROVAL.
func TestBUG327_EmitLockedDoesNotUnparkWaitingChild(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	svc.agentOrchestrator.registerChild(parentID, rs.id)
	svc.agentOrchestrator.upsertSummary(parentID, AgentRunSummary{
		RunID:       rs.id,
		ParentRunID: parentID,
		AgentName:   "coder",
		Role:        "coder",
		Status:      RunStatusWaitingUserApr,
	})
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "blocked", Cap: 3, RoundCap: 3, BlockReason: "escalate"})
	svc.mu.Lock()
	rs.status = RunStatusWaitingUserApr
	rs.agentStatus = "waiting_user_approval"
	svc.mu.Unlock()

	svc.mu.Lock()
	svc.emitLocked(rs, ProviderEvent{Type: EventFlowGateViolation, Error: "stray", Status: "block"})
	svc.mu.Unlock()

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs.status != RunStatusWaitingUserApr {
		t.Fatalf("child status after stray event = %q, want %q (park must not unpark)", rs.status, RunStatusWaitingUserApr)
	}
	if rs.agentStatus != "waiting_user_approval" {
		t.Fatalf("agentStatus = %q, want waiting_user_approval", rs.agentStatus)
	}
	// The graph summary must stay parked too — emitLocked's default upserts it.
	snap := svc.agentGraphSnapshot(parentID)
	for _, r := range snap.Runs {
		if r.RunID == rs.id {
			if r.Status != RunStatusWaitingUserApr || r.AgentStatus != "waiting_user_approval" {
				t.Fatalf("graph summary after stray event = status %q agentStatus %q, want parked", r.Status, r.AgentStatus)
			}
		}
	}
}
