package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// CA-1092 (live run-3362 review): vibe-sprint shipped without a dedicated
// review step — coder → validate → synthesis → audit, with the synthesis hub
// self-reviewing via its continue back-edge. task-harness and cp-harness both
// run a read-only reviewer cohort between validate and synthesis whose
// machine verdict (submit_review_outcome) gates the hub's done edge.
// These tests pin the runtime wiring, not just the yaml topology:
//   - validate --done--> spawns the reviewer delegate (cohort=review)
//   - synthesis done is blocked until that reviewer's machine verdict lands
//   - a changes_requested verdict satisfies hubDoneCohortHasChangesRequested
//     so the hub's continue routes back to the coder, not to a user gate.
//
// Provider-agnostic: every path here is pack-data + engine routing (no
// provider call site differs per key), so one provider leg suffices — same
// precedent as the BUG-318 cohort tests.
func loadVibeSprintDefForTest(t *testing.T) agentpack.FlowDefinition {
	t.Helper()
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, def := range pack.Flows {
		if def.ID == "vibe-sprint" {
			return def
		}
	}
	t.Fatal("vibe-sprint not in builtin pack")
	return agentpack.FlowDefinition{}
}

func armVibeSprintRun(t *testing.T, svc *InteractiveService) string {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	def := loadVibeSprintDefForTest(t)
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeFlowNodes = def.Nodes
	rs.activeFlowEdges = def.Edges
	rs.activeFlowAcceptanceNodes = append([]string(nil), def.AcceptanceNodes...)
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.reseedFlowStepRuntime(parent.RunID, def.Nodes)
	return parent.RunID
}

func TestCA1092_VibeSprintValidateDoneSpawnsReviewer(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "reviewed"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	runID := armVibeSprintRun(t, svc)

	// The exact advance runValidateNode performs once the suite passes.
	// CA-1151: validate --done--> now fans out to BOTH review-cohort members
	// in parallel — spec_align (re-derives requirements from the SS/SD/CP/
	// Task chain and checks tests+code against them; green is not proof)
	// and reviewer — because a cohort member settles into the cohort join,
	// not into a per-node done edge.
	if !svc.tryAdvanceFlowFromNode(runID, "validate", "suite green") {
		t.Fatal("tryAdvanceFlowFromNode(validate) returned false — review cohort never dispatched")
	}
	waitLoop(t, "spec_align child spawned", 5*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "spec_align") >= 1
	})
	if got := flowStepStatus(t, svc, runID, "spec_align"); got != StepStatusRunning && got != StepStatusDone {
		t.Fatalf("spec_align step=%v want RUNNING (spawned) — DONE only if the turn already settled", got)
	}
	waitLoop(t, "reviewer child spawned", 5*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "reviewer") >= 1
	})
	if got := flowStepStatus(t, svc, runID, "reviewer"); got != StepStatusRunning && got != StepStatusDone {
		t.Fatalf("reviewer step=%v want RUNNING (spawned) — DONE only if the turn already settled", got)
	}
}

func TestCA1092_VibeSprintSynthesisDoneGatedOnReviewerVerdict(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armVibeSprintRun(t, svc)

	// With a review cohort declared, the synthesis hub's done edge must be
	// machine-verdict gated (CP-61 P-1 harness contract, now applied to
	// vibe-sprint): no verdict recorded → hard block, not silent pass.
	svc.mu.Lock()
	rs := svc.runs[runID]
	gated := flowRequiresHubMachineVerdict(rs, "synthesis")
	svc.mu.Unlock()
	if !gated {
		t.Fatal("flowRequiresHubMachineVerdict(synthesis)=false — the reviewer cohort is not wired to gate the hub")
	}
	if err := svc.synthesisDoneVerdictError(runID); err == nil {
		t.Fatal("synthesis done must be blocked while no reviewer verdict is recorded")
	}

	// A changes_requested verdict must satisfy the cohort join and mark the
	// continue branch — synthesis routes the round back to the coder.
	// CA-1151: the review cohort now also carries spec_align; its verdict must
	// be present alongside reviewer's.
	svc.snapshotReviewCohortVerdicts(runID, []cohortEntry{
		{Label: "spec_align", Status: "completed", MachineVerdict: "approved"},
		{Label: "reviewer", Status: "completed", MachineVerdict: "changes_requested"},
	})
	if !svc.hubDoneCohortHasChangesRequested(runID, "synthesis") {
		t.Fatal("changes_requested verdict not visible to the synthesis hub")
	}
	if err := svc.synthesisDoneVerdictError(runID); err == nil {
		t.Fatal("synthesis done must stay blocked on a changes_requested verdict")
	}

	// An approved verdict unblocks done — both cohort members approved.
	svc.snapshotReviewCohortVerdicts(runID, []cohortEntry{
		{Label: "spec_align", Status: "completed", MachineVerdict: "approved"},
		{Label: "reviewer", Status: "completed", MachineVerdict: "approved"},
	})
	if err := svc.synthesisDoneVerdictError(runID); err != nil {
		t.Fatalf("synthesis done must pass on approved verdict: %v", err)
	}
}
