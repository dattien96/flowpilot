package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-567 / BUG-568 (live run-100368): a runner restart while the owner-debate
// overlay is mounted leaves two stacked wedges —
//
//	BUG-567: the debate_synthesis hub turn that unmounts the overlay
//	  (onVibeCpNodeDone → restoreVibeFlowAfterDebate) died with the
//	  process. Cohort buffers are RAM-only, so nothing ever re-drives it:
//	  the overlay stays mounted, the review cohort is invisible to
//	  flowRequiresHubMachineVerdict, and submit_review_outcome is never
//	  offered again.
//	BUG-568: resumedFlowStepRows builds rows from activeFlowNodes only —
//	  the debate graph. Sprint node rows never exist, so transition-log
//	  replay cannot restore them (applyStepTransitionReplay skips
//	  row-less nodes) and the post-restore merge reseed re-seeds a DONE
//	  coder as PENDING, failing vibeSprintEvidenceComplete forever.
//
// These tests pin the resume contract: parked-topology nodes must get real
// step rows (so replay restores their settled statuses), and a mounted
// debate with settled owners must re-drive debate_synthesis — replaying
// restoreVibeFlowAfterDebate directly when the synthesis already resolved.

func bug567DebateGraph() []agentpack.FlowNode {
	return []agentpack.FlowNode{
		{ID: "debate_trigger", Behavior: "hub.inline"},
		{ID: "owner_1", Behavior: "agent.delegate", Agent: "agents/owner.md"},
		{ID: "owner_2", Behavior: "agent.delegate", Agent: "agents/owner.md"},
		{ID: "debate_synthesis", Behavior: "hub.inline"},
	}
}

func bug567SprintGraph() []agentpack.FlowNode {
	return []agentpack.FlowNode{
		{ID: "preflight_contract_plan", Behavior: "agent.delegate", Agent: "agents/planner.md"},
		{ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold-architect.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
		{ID: "synthesis", Behavior: "hub.inline"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
}

func bug567ReconstructMidDebate(t *testing.T, synDone bool) (*InteractiveService, *ca810StepLogStore, string) {
	t.Helper()
	store := &ca810StepLogStore{fakeWorkflowStore: newFakeWorkflowStore(), lines: map[string][]stepTransitionLine{}}
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	runID := "run-debate-restart"

	// Owner legs settled pre-restart — durable session evidence.
	for _, label := range []string{"owner_1", "owner_2"} {
		if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
			RunID: "run-" + label, ProjectID: "proj", ParentRunID: runID,
			Label: label, AgentName: "owner", Status: RunStatusCompleted, TurnCount: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Pre-restart step truth in the durable sidecar: owners DONE, the sprint's
	// coder DONE (parked topology), debate_synthesis pending or done.
	lines := []stepTransitionLine{
		{RunID: runID, NodeID: "debate_trigger", Status: string(StepStatusDone), TS: "2026-10-02T20:00:00Z"},
		{RunID: runID, NodeID: "owner_1", Status: string(StepStatusDone), TS: "2026-10-02T20:01:00Z"},
		{RunID: runID, NodeID: "owner_2", Status: string(StepStatusDone), TS: "2026-10-02T20:02:00Z"},
		{RunID: runID, NodeID: "coder", Status: string(StepStatusDone), TS: "2026-10-02T19:50:00Z"},
	}
	if synDone {
		lines = append(lines, stepTransitionLine{RunID: runID, NodeID: "debate_synthesis", Status: string(StepStatusDone), TS: "2026-10-02T20:03:00Z"})
	}
	for _, l := range lines {
		if err := store.AppendStepTransition(context.Background(), runID, l); err != nil {
			t.Fatal(err)
		}
	}

	_, err := svc.reconstructRun(ProviderSessionState{
		RunID: runID, ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, ProviderAccountID: "default",
		Status:        RunStatusRunning,
		ChatFlowRef:   workingmode.PackPrefix + vibeOwnerDebateFlowID,
		FlowArm:       "started",
		WorkingMode:   workingmode.Vibe,
		TurnCount:     3,
		AutoOrchestrate: true,
		ActiveFlowNodes: bug567DebateGraph(),
		VibeParkedNodes: bug567SprintGraph(),
		VibeTaskPlan:    []string{"requirements/08-Task/todo/Task-025-x.md"},
		VibeSprintIndex: 1,
		VibeLockedCP:    "CP-02",
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	return svc, store, runID
}

// BUG-568: sprint nodes parked under the debate overlay must still get step
// rows on resume — the transition log's settled statuses (coder DONE) must
// survive reconstruction instead of being lost to a row-less replay skip.
func TestBUG568ResumeSeedsParkedSprintSteps(t *testing.T) {
	svc, _, runID := bug567ReconstructMidDebate(t, false)
	if got := svc.lookupFlowStepStatus(runID, "coder"); got != StepStatusDone {
		t.Fatalf("parked sprint node coder lost its settled status on restart: got %q want %q", got, StepStatusDone)
	}
}

// BUG-567: owners DONE + debate_synthesis never ran → resume must re-drive the
// synthesis hub so it can emit done → restoreVibeFlowAfterDebate. The signal
// is a scheduled hub turn on the parent (reinvoke or a fresh turn).
func TestBUG567ResumeRedrivesDebateSynthesis(t *testing.T) {
	svc, _, runID := bug567ReconstructMidDebate(t, false)
	waitLoop(t, "debate_synthesis hub turn re-driven after restart", 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[runID]
		if rs == nil {
			return false
		}
		// Either a reinvoke is armed/in-flight or a turn actually ran.
		return rs.reinvokeInFlight || rs.pendingHubReinvoke || rs.turnInFlight || rs.turnCount > 3
	})
}

// BUG-567 (resolved-before-restart shape): debate_synthesis already DONE in
// the sidecar but the process died before restoreVibeFlowAfterDebate ran —
// the parked overlay must still unmount on resume.
func TestBUG567ResumeRestoresWhenSynthesisAlreadyDone(t *testing.T) {
	svc, _, runID := bug567ReconstructMidDebate(t, true)
	waitLoop(t, "parked sprint topology restored after restart", 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[runID]
		return rs != nil && len(rs.vibeParkedNodes) == 0 && runHasFlowNode(rs, "coder")
	})
}
