package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-362 (run-210188): after Approve on a churned plan_approval park, freeze
// went DONE but plan_synthesis stayed WAITING_USER_APPROVAL; a later validate
// escalate stamped validate WAITING too, so TUI Now: pointed at the dead plan
// phase while the live gate was validate. Two defects:
//
// F-1a: runValidateNode's max-retries branch called setFlowStepAwaitingUser,
// whose first-hub fallback stamps plan_synthesis on dual-hub flows — it must
// stamp the validate node itself (applyFlowControl re-stamps it anyway).
// F-1b: freeze DONE must settle a stale plan_synthesis WAITING (approve path,
// CA-758 skip branch, freeze-complete) but never while freeze is pending.
// New file; no pre-existing test is modified.
//
// R2: all touched paths take no providerKey (Case 1 agnostic); the matrix
// below locks against future provider-specific drift.

func bug362Providers() []ProviderKey {
	return []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
}

// TestBug362ApproveSettlesPlanSynthesisDone drives the live shape: churned
// park -> human approve -> freeze DONE must also settle plan_synthesis DONE.
func TestBug362ApproveSettlesPlanSynthesisDone(t *testing.T) {
	for _, pk := range bug362Providers() {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := run207435PlanServiceWithCohort(t, pk)
			task325SeedWriter(t, svc, runID, pk, 1)
			run207435SeedVerdicts(t, svc, runID, map[string]string{"plan_reviewer": "approved"})
			if _, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft}); !handled {
				t.Fatalf("%s: churned plan done must park, got handled=false", pk)
			}
			if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason != planApprovalBlockReason {
				t.Fatalf("%s: loop = %+v, want plan_approval park before approve", pk, st)
			}
			svc.mu.Lock()
			svc.runs[runID].preflightDraftResult = validPlannerDraft
			svc.mu.Unlock()
			// Approve through the real Continue path (like the TUI [Approve]
			// chip): resumeFlowWithFeedback unblocks the parked loop before
			// resumePlanApproval drives freeze. Calling resumePlanApproval
			// directly on a blocked loop skips freeze (loopIsAdvancing).
			snap, err := svc.resumeFlowWithFeedback(runID, "")
			if err != nil {
				t.Fatalf("%s: approve resume: %v", pk, err)
			}
			if snap.LoopState.Status == "blocked" {
				t.Fatalf("%s: approve must unblock the loop: %+v", pk, snap.LoopState)
			}
			if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
				t.Fatalf("%s: freeze = %v, want DONE after approve", pk, got)
			}
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusDone {
				t.Fatalf("%s: plan_synthesis = %v, want DONE after approve+freeze (stale WAITING shadows Now:)", pk, got)
			}
		})
	}
}

// TestBug362SkipBranchSettlesStaleWaiting covers the CA-758 shape: freeze
// already DONE with a leftover plan_synthesis WAITING — the stale done must
// advance without re-parking AND settle the hub.
func TestBug362SkipBranchSettlesStaleWaiting(t *testing.T) {
	for _, pk := range bug362Providers() {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := run207435PlanServiceWithCohort(t, pk)
			task325SeedWriter(t, svc, runID, pk, 1)
			run207435SeedVerdicts(t, svc, runID, map[string]string{"plan_reviewer": "approved"})
			svc.setFlowStepStatus(context.Background(), runID, "plan_synthesis", StepStatusWaitingUserApr)
			svc.setFlowStepStatus(context.Background(), runID, "preflight_contract_freeze", StepStatusDone)

			res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
			if !handled {
				t.Fatalf("%s: stale done after freeze must be claimed, got handled=false", pk)
			}
			if res.NextAction != "advancing" {
				t.Fatalf("%s: NextAction = %q, want advancing (no re-park)", pk, res.NextAction)
			}
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusDone {
				t.Fatalf("%s: plan_synthesis = %v, want DONE settled beside DONE freeze", pk, got)
			}
			if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == planApprovalBlockReason {
				t.Fatalf("%s: loop re-parked plan_approval after freeze DONE: %+v", pk, st)
			}
		})
	}
}

// TestBug362SettleNoOpWhileFreezePending locks CA-749: a WAITING hub with a
// pending freeze must stay WAITING — the settle fires only on DONE freeze.
func TestBug362SettleNoOpWhileFreezePending(t *testing.T) {
	for _, pk := range bug362Providers() {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := run207435PlanServiceWithCohort(t, pk)
			svc.setFlowStepStatus(context.Background(), runID, "plan_synthesis", StepStatusWaitingUserApr)

			svc.settlePlanSynthesisAfterFreezeDone(runID)

			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusWaitingUserApr {
				t.Fatalf("%s: plan_synthesis = %v, want still WAITING (freeze pending, CA-749 park intact)", pk, got)
			}
		})
	}
}

func bug362DualHubValidateTopology() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "plan_synthesis", To: "preflight_contract_freeze", When: "done", Kind: "forward"},
		{From: "plan_synthesis", To: "plan_writer", When: "continue", Kind: "back"},
		{From: "validate", To: "audit", When: "done", Kind: "forward"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze", Lifecycle: "once"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "validate", Behavior: "command.validate", Lifecycle: "once"},
		{ID: "audit", Behavior: "artifact.audit_draft", Lifecycle: "once"},
	}
	return edges, nodes
}

// bug362ValidateService builds the post-approve shape: plan DONE, freeze
// DONE, loop running on the code phase — with a validate command that fails
// fast and a retry budget of one, so one runValidateNode call exhausts it.
func bug362ValidateService(t *testing.T, pk ProviderKey) (*InteractiveService, string) {
	t.Helper()
	svc := newFreezeTestServiceForProvider(t, pk)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// Portable fast-failing command (no network, no repo): go errors on the
	// missing go.mod immediately. Mirror bug288_round12's explicit-config
	// recipe so baseline recapture keeps this command.
	const failCmd = "go build ./this-package-does-not-exist-xyz"
	guardDir := filepath.Join(dir, ".flowpilot", "guard")
	settingsDir := filepath.Join(dir, ".flowpilot", "settings")
	if err := os.MkdirAll(guardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseline := `{"captured_at":"2026-01-01T00:00:00Z","test_command":"` + failCmd + `"}`
	if err := os.WriteFile(filepath.Join(guardDir, "test_baseline.json"), []byte(baseline), 0o644); err != nil {
		t.Fatal(err)
	}
	testConfig := `{"test_command":"` + failCmd + `"}`
	if err := os.WriteFile(filepath.Join(settingsDir, "test-config.json"), []byte(testConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	edges, nodes := bug362DualHubValidateTopology()
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = dir
	rs.activeHubNodeID = "synthesis"
	rs.currentTurnID = "turn-362-validate"
	rs.flowValidationRetryState = &FlowValidationRetryState{ValidationCommand: failCmd, MaxRetries: 1}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5})
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "plan_synthesis", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "preflight_contract_freeze", StepStatusDone)
	return svc, parent.RunID
}

// TestBug362ValidateEscalateDoesNotStampPlanSynthesis replays run-210188:
// validate exhausts on a dual-hub flow whose plan phase already closed. Only
// validate may read WAITING; the DONE plan hub must not resurrect.
func TestBug362ValidateEscalateDoesNotStampPlanSynthesis(t *testing.T) {
	for _, pk := range bug362Providers() {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := bug362ValidateService(t, pk)
			node := agentpack.FlowNode{ID: "validate", Behavior: "command.validate"}
			if !svc.runValidateNode(context.Background(), runID, nil, nil, node, "did something") {
				t.Fatalf("%s: exhausted validate must escalate handled=true", pk)
			}
			loop := svc.agentOrchestrator.loopStateFor(runID)
			if loop.Status != "blocked" || loop.BlockReason != "escalate" {
				t.Fatalf("%s: loop = %+v, want blocked/escalate", pk, loop)
			}
			if !strings.Contains(loop.GateReason, "maximum number of retries") {
				t.Fatalf("%s: GateReason = %q, want validate-exhausted copy", pk, loop.GateReason)
			}
			if got := flowStepStatus(t, svc, runID, "validate"); got != StepStatusWaitingUserApr {
				t.Fatalf("%s: validate = %v, want WAITING (live gate)", pk, got)
			}
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusDone {
				t.Fatalf("%s: plan_synthesis = %v, want still DONE (first-hub fallback must not resurrect it)", pk, got)
			}
			if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
				t.Fatalf("%s: freeze = %v, want still DONE", pk, got)
			}
		})
	}
}
