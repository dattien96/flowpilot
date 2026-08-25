package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// Run-144900: leftover untracked skill dir (.agents/skills/flow-mode-orchestrator/SKILL.md
// created 10:28 before the flow) was present when test_signatures started. The frozen
// writer gate used ObserveGitDiffSince(BaseSHA) without subtracting pre-existing dirt,
// so the leftover looked like the tester wrote outside [format.go, format_test.go] and
// parked the step as WAITING_USER_APPROVAL. A = pre-existing skill on .agents, .claude,
// .grok simultaneously must not drift. B,C = true drift still must block. D,E = continue
// on hub-ful flow must retry the writer, not fake a synthesis review.

// ---------------------------------------------------------------------------
// Gate: pre-existing skill does NOT drift
// ---------------------------------------------------------------------------

func TestRun144900_PreexistingSkillDoesNotCauseScopeDrift(t *testing.T) {
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
			dir, head := newContractFreezeTestRepo(t)
			svc, parentID := newP4CodeWriterFixture(t, dir)

			// Leftover skill present BEFORE freeze (simulating 10:28 untracked dir).
			// Content must be captured in the freeze BaselineWorktree after the fix.
			p4WriteFile(t, dir, ".agents/skills/flow-mode-orchestrator/SKILL.md", "# Flow Mode Orchestrator\n")
			// Freeze with a complete baseline (now includes .md, no 20-cap).
			store, err := changecontract.NewFrozenStore(dir)
			if err != nil {
				t.Fatalf("NewFrozenStore: %v", err)
			}
			baseline := baselineWorktreeFingerprint(dir)
			if baseline == nil || baseline[".agents/skills/flow-mode-orchestrator/SKILL.md"] == "" {
				t.Fatalf("baseline must capture leftover SKILL.md, got %+v", baseline)
			}
			draft := changecontract.PreflightContractDraft{FeatureKey: "calc-format", Intent: "Add ClampChecked", DeclaredPaths: []string{"format.go", "format_test.go"}}
			rec, err := changecontract.FreezeContract(dir, parentID, "planner", "test_signatures", draft, head, baseline, "", 1, time.Now().UTC())
			if err != nil {
				t.Fatalf("FreezeContract: %v", err)
			}
			if err := store.SaveFrozen(rec); err != nil {
				t.Fatalf("SaveFrozen: %v", err)
			}
			// Wire parent workspace for gate's cwd resolution.
			svc.mu.Lock()
			if pr := svc.runs[parentID]; pr != nil {
				pr.workspaceCwd = dir
			}
			svc.mu.Unlock()

			rs := newP4ChildRun(svc, "child-ts", parentID, dir, head)
			rs.label = "test_signatures"
			// Also reflect the pre-existing dirt in the turn-start snapshot so
			// the turn-scoped filter (rs.turnStartWorktree) would also hide it
			// even if the freeze baseline were stale.
			rs.turnStartWorktree = snapshotWorktreeFingerprints(dir)

			// Writer only touches declared file.
			p4WriteFile(t, dir, "format_test.go", "package gatesandbox\n")
			blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{
				FinalMessage: "done",
				ChangedFiles: []string{"format_test.go", ".agents/skills/flow-mode-orchestrator/SKILL.md"},
			}, 0)
			if blocked {
				t.Fatal("pre-existing .agents/skills/flow-mode-orchestrator/SKILL.md must not cause scope drift block")
			}
		})
	}
}

func TestRun144900_PreexistingMultipleSkillCopiesDoNotDrift(t *testing.T) {
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
			dir, head := newContractFreezeTestRepo(t)
			svc, parentID := newP4CodeWriterFixture(t, dir)
			p4WriteFile(t, dir, ".agents/skills/flow-mode-orchestrator/SKILL.md", "# flow\n")
			p4WriteFile(t, dir, ".claude/skills/flow-mode-orchestrator/SKILL.md", "# flow\n")
			p4WriteFile(t, dir, ".grok/skills/flow-mode-orchestrator/SKILL.md", "# flow\n")

			store, _ := changecontract.NewFrozenStore(dir)
			baseline := baselineWorktreeFingerprint(dir)
			draft := changecontract.PreflightContractDraft{FeatureKey: "calc-format", Intent: "Add ClampChecked", DeclaredPaths: []string{"format.go", "format_test.go"}}
			rec, _ := changecontract.FreezeContract(dir, parentID, "planner", "test_signatures", draft, head, baseline, "", 1, time.Now().UTC())
			_ = store.SaveFrozen(rec)
			svc.mu.Lock()
			if pr := svc.runs[parentID]; pr != nil {
				pr.workspaceCwd = dir
			}
			svc.mu.Unlock()

			rs := newP4ChildRun(svc, "child-ts2", parentID, dir, head)
			rs.label = "test_signatures"
			rs.turnStartWorktree = snapshotWorktreeFingerprints(dir)

			p4WriteFile(t, dir, "format_test.go", "package gatesandbox\n")
			blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-2", finalizeInput{
				FinalMessage: "done",
				ChangedFiles: []string{"format_test.go", ".agents/skills/flow-mode-orchestrator/SKILL.md", ".claude/skills/flow-mode-orchestrator/SKILL.md", ".grok/skills/flow-mode-orchestrator/SKILL.md"},
			}, 0)
			if blocked {
				t.Fatal("pre-existing skill copies on .agents/.claude/.grok must not trigger drift")
			}
		})
	}
}

func TestRun144900_TrueDriftViaExtraFileStillBlocks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider ProviderKey
	}{
		{"grok", ProviderKeyGrok},
		{"codex", ProviderKeyCodex},
		{"claude", ProviderKeyClaude},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, head := newContractFreezeTestRepo(t)
			svc, parentID := newP4CodeWriterFixture(t, dir)
			p4WriteFile(t, dir, ".agents/skills/flow-mode-orchestrator/SKILL.md", "# flow\n")
			store, _ := changecontract.NewFrozenStore(dir)
			baseline := baselineWorktreeFingerprint(dir)
			draft := changecontract.PreflightContractDraft{FeatureKey: "calc-format", Intent: "x", DeclaredPaths: []string{"format.go", "format_test.go"}}
			rec, _ := changecontract.FreezeContract(dir, parentID, "planner", "test_signatures", draft, head, baseline, "", 1, time.Now().UTC())
			_ = store.SaveFrozen(rec)
			svc.mu.Lock()
			if pr := svc.runs[parentID]; pr != nil {
				pr.workspaceCwd = dir
			}
			svc.mu.Unlock()
			rs := newP4ChildRun(svc, "child-extra", parentID, dir, head)
			rs.label = "test_signatures"
			rs.turnStartWorktree = snapshotWorktreeFingerprints(dir)

			p4WriteFile(t, dir, "format_test.go", "package gatesandbox\n")
			p4WriteFile(t, dir, "src/extra.go", "package calc\n")
			blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-3", finalizeInput{
				FinalMessage: "done",
				ChangedFiles: []string{"format_test.go", "src/extra.go"},
			}, 0)
			if !blocked {
				t.Fatal("expected scope drift block for src/extra.go outside declared paths")
			}
		})
	}
}

func TestRun144900_MutatedLeftoverStillBlocks(t *testing.T) {
	for _, tc := range []struct {
		name string
	}{
		{"grok"}, {"codex"}, {"claude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, head := newContractFreezeTestRepo(t)
			svc, parentID := newP4CodeWriterFixture(t, dir)
			p4WriteFile(t, dir, ".agents/skills/flow-mode-orchestrator/SKILL.md", "# v1\n")
			store, _ := changecontract.NewFrozenStore(dir)
			baseline := baselineWorktreeFingerprint(dir)
			draft := changecontract.PreflightContractDraft{FeatureKey: "calc-format", Intent: "x", DeclaredPaths: []string{"format.go", "format_test.go"}}
			rec, _ := changecontract.FreezeContract(dir, parentID, "planner", "test_signatures", draft, head, baseline, "", 1, time.Now().UTC())
			_ = store.SaveFrozen(rec)
			svc.mu.Lock()
			if pr := svc.runs[parentID]; pr != nil {
				pr.workspaceCwd = dir
			}
			svc.mu.Unlock()
			rs := newP4ChildRun(svc, "child-mut", parentID, dir, head)
			rs.label = "test_signatures"
			rs.turnStartWorktree = snapshotWorktreeFingerprints(dir)

			// Mutate the leftover's content this turn.
			_ = os.MkdirAll(filepath.Join(dir, ".agents", "skills", "flow-mode-orchestrator"), 0o755)
			_ = os.WriteFile(filepath.Join(dir, ".agents", "skills", "flow-mode-orchestrator", "SKILL.md"), []byte("# v2 mutated\n"), 0o644)

			p4WriteFile(t, dir, "format_test.go", "package gatesandbox\n")
			blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-4", finalizeInput{
				FinalMessage: "done",
				ChangedFiles: []string{"format_test.go", ".agents/skills/flow-mode-orchestrator/SKILL.md"},
			}, 0)
			if !blocked {
				t.Fatal("mutated leftover skill must still trigger drift (only unchanged fingerprints are exempt)")
			}
			if !strings.Contains(svc.agentGraphSnapshot(parentID).LoopState.GateReason, "SKILL.md") {
				t.Fatalf("gate reason must mention SKILL.md, got %q", svc.agentGraphSnapshot(parentID).LoopState.GateReason)
			}
		})
	}
}

func TestRun144900_TurnStartWorktreeAloneHidesPreExistingDirt(t *testing.T) {
	// Freeze was taken before the leftover existed (stale baseline = nil).
	// Turn-start snapshot alone must be sufficient to hide the leftover.
	for _, tc := range []struct {
		name string
	}{
		{"grok"}, {"codex"}, {"claude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, head := newContractFreezeTestRepo(t)
			svc, parentID := newP4CodeWriterFixture(t, dir)
			// Freeze first, baseline nil.
			freezeP4Contract(t, dir, parentID, "test_signatures", head, []string{"format.go", "format_test.go"})
			// Leftover appears AFTER freeze but BEFORE this child's turn.
			p4WriteFile(t, dir, ".agents/skills/flow-mode-orchestrator/SKILL.md", "# flow\n")
			svc.mu.Lock()
			if pr := svc.runs[parentID]; pr != nil {
				pr.workspaceCwd = dir
			}
			svc.mu.Unlock()
			rs := newP4ChildRun(svc, "child-stale", parentID, dir, head)
			rs.label = "test_signatures"
			rs.turnStartWorktree = snapshotWorktreeFingerprints(dir)

			p4WriteFile(t, dir, "format_test.go", "package gatesandbox\n")
			blocked := svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-5", finalizeInput{
				FinalMessage: "done",
				ChangedFiles: []string{"format_test.go", ".agents/skills/flow-mode-orchestrator/SKILL.md"},
			}, 0)
			if blocked {
				t.Fatal("turn-start snapshot must hide leftover even when freeze baseline is stale (nil)")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Continue: writer park on hub-FUL live rag-harness must reinvoke child
// ---------------------------------------------------------------------------

func liveRagHarnessWithHubNodes() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	// Live rag-harness shape includes hub.inline=synthesis (unlike the hub-less
	// ragHarnessNodes() helper used by CA-627). Continue from a writer park must
	// still find the writer child even though hubInline != "".
	edges := []agentpack.FlowEdge{
		{From: "validate", To: "implement", When: "continue", Kind: "back"},
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "preflight_contract_plan", Behavior: "agent.delegate", Agent: "agents/contract-planner.md"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze"},
		{ID: "context", Behavior: "context.produce"},
		{ID: "test_signatures", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Cohort: "review", Join: "all"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Join: "all"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	return edges, nodes
}

func TestRun144900_ContinueOnHubFulFlowReinvokesWriterNotHub(t *testing.T) {
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
			edges, nodes := liveRagHarnessWithHubNodes()
			svc.mu.Lock()
			p := svc.runs[pid]
			p.activeFlowNodes = nodes
			p.activeFlowEdges = edges
			p.activeFlowAcceptanceNodes = []string{"validate", "audit"}
			p.autoOrchestrate = true
			p.flowEngineDriven = true
			p.chatFlowRef = "flowpilot-core-flow-pack/rag-harness"
			p.modelName = tc.model
			p.providerKey = tc.provider
			p.lastEscalatedInlineNodeID = "test_signatures"
			// Ensure the resolver sees a workspace (empty temp is fine).
			if p.workspaceCwd == "" {
				p.workspaceCwd = t.TempDir()
			}
			svc.mu.Unlock()
			svc.reseedFlowStepRuntime(pid, nodes)
			svc.setFlowStepStatus(context.Background(), pid, "test_signatures", StepStatusWaitingUserApr)

			child, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
			svc.mu.Lock()
			crs := svc.runs[child.RunID]
			crs.parentRunID = pid
			crs.agentName = "coder"
			crs.label = "test_signatures"
			crs.role = "coder"
			crs.providerKey = tc.provider
			crs.modelName = tc.model
			crs.status = RunStatusWaitingUserApr
			crs.agentStatus = "waiting_user_approval"
			svc.mu.Unlock()
			svc.agentOrchestrator.registerChild(pid, child.RunID)
			svc.agentOrchestrator.upsertSummary(pid, AgentRunSummary{RunID: child.RunID, AgentName: "coder", Label: "test_signatures", Role: "coder", Status: RunStatusWaitingUserApr, ParentRunID: pid})

			_, err := svc.resumeFlowWithFeedback(pid, "continue")
			if err != nil {
				t.Fatalf("%s: resumeFlowWithFeedback: %v", tc.name, err)
			}

			svc.mu.Lock()
			activationSeq := svc.runs[child.RunID].activationSeq
			lastEsc := svc.runs[pid].lastEscalatedInlineNodeID
			svc.mu.Unlock()

			if lastEsc != "" {
				t.Fatalf("%s: lastEscalatedInlineNodeID not cleared, got %q", tc.name, lastEsc)
			}
			if activationSeq != 1 {
				t.Fatalf("%s: writer child not reinvoked (activationSeq=%d) — hub must not swallow the writer escalate on hub-ful flow", tc.name, activationSeq)
			}
		})
	}
}

func TestRun144900_ContinueOnHubFulFlowReinvokesImplementNotHub(t *testing.T) {
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
			edges, nodes := liveRagHarnessWithHubNodes()
			svc.mu.Lock()
			p := svc.runs[pid]
			p.activeFlowNodes = nodes
			p.activeFlowEdges = edges
			p.activeFlowAcceptanceNodes = []string{"validate", "audit"}
			p.autoOrchestrate = true
			p.flowEngineDriven = true
			p.chatFlowRef = "flowpilot-core-flow-pack/rag-harness"
			p.modelName = tc.model
			p.providerKey = tc.provider
			p.lastEscalatedInlineNodeID = "implement"
			if p.workspaceCwd == "" {
				p.workspaceCwd = t.TempDir()
			}
			svc.mu.Unlock()
			svc.reseedFlowStepRuntime(pid, nodes)
			svc.setFlowStepStatus(context.Background(), pid, "implement", StepStatusWaitingUserApr)

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
			svc.mu.Unlock()
			if activationSeq != 1 {
				t.Fatalf("%s: implement child not reinvoked on hub-ful flow (activationSeq=%d)", tc.name, activationSeq)
			}
		})
	}
}
