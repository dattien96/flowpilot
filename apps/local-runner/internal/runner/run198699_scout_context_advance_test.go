package runner

// run-198699 (CP-58 S2 / task-harness): the scout (preflight_contract_plan)
// completed, but the flow never advanced scout -> context -> plan_writer:
// tryAdvanceFlowThroughInline had no context.produce case, so the scout's
// completion fell back to the note+reinvoke-hub path. The hub prose-answered
// without submit_review_outcome, parked hub_stalled after 2m, and each Retry
// continue no-op'd on a never-spawned plan_writer (reinvoke lifecycle, no
// child) — the S2 retry loop. New file; no pre-existing test is modified.
//
// Provider parity: the changed paths (tryAdvanceFlowThroughInline's
// context.produce dispatch, maybeReinvokeCoderForContinue) never branch on a
// provider key — graph dispatch + edge routing only — so the subtest matrix
// over Claude/Codex/Grok (registerKeyedCapture, like bug353) guards a future
// provider-specific drift.

import (
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

func run198699HarnessService(t *testing.T, pk ProviderKey) (*InteractiveService, string, chan TurnRequest) {
	t.Helper()
	ch := make(chan TurnRequest, 1)
	reg := newProviderRegistry()
	registerKeyedCapture(reg, pk, ch)
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	return svc, parent.RunID, ch
}

func setRun198699Topology(t *testing.T, svc *InteractiveService, runID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, hub string) {
	t.Helper()
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = hub
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(runID, nodes)
}

// TestRun198699ScoutAdvancesThroughContextAndSpawnsPlanWriter drives the real
// advanceOrNotifyHub entry (what the scout's completion handler calls). It
// must take the context.produce inline path: context step DONE, a plan_writer
// child spawned, and its turn prompt carrying the rendered context package
// (flowContextHandoffPrefix) + scout plan — never a hub reinvoke for the scout.
func TestRun198699ScoutAdvancesThroughContextAndSpawnsPlanWriter(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, ch := run198699HarnessService(t, pk)
			edges := []agentpack.FlowEdge{
				{From: "preflight_contract_plan", To: "context", When: "done", Kind: "forward"},
				{From: "context", To: "plan_writer", When: "done", Kind: "forward"},
			}
			nodes := []agentpack.FlowNode{
				{ID: "preflight_contract_plan", Behavior: "agent.delegate", Agent: "agents/contract-planner.md", Lifecycle: "once"},
				{ID: "context", Behavior: "context.produce", Lifecycle: "once"},
				{ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
			}
			setRun198699Topology(t, svc, runID, edges, nodes, "")

			// Lock-step guard (additive only — TestBUG327 stays unedited): the
			// helper must agree with tryAdvanceFlowThroughInline for the new
			// behavior.
			if !flowNodeInlineDispatchable(agentpack.FlowNode{Behavior: "context.produce"}) {
				t.Fatalf("%s: flowNodeInlineDispatchable(context.produce) = false, want true (lock-step with the switch)", pk)
			}

			planJSON := `{"feature_key": "calc-core", "intent": "Add integer GCD to calc.go", "declared_paths": ["calc.go", "gcd_test.go"]}`
			svc.advanceOrNotifyHub(runID, "preflight_contract_plan", "contract-planner", planJSON)

			if got := flowStepStatus(t, svc, runID, "context"); got != StepStatusDone {
				t.Fatalf("%s: context step = %v, want DONE (context.produce must be dispatched mid-flow)", pk, got)
			}
			svc.mu.Lock()
			planWriterChildren := 0
			for _, run := range svc.runs {
				if run.parentRunID == runID && run.label == "plan_writer" {
					planWriterChildren++
				}
			}
			svc.mu.Unlock()
			if planWriterChildren != 1 {
				t.Fatalf("%s: plan_writer child count = %d, want 1 (scout must auto-advance through context, not reinvoke the hub)", pk, planWriterChildren)
			}

			select {
			case req := <-ch:
				if !strings.Contains(req.Prompt, "calc-core") || !strings.Contains(req.Prompt, "gcd_test.go") {
					t.Fatalf("%s: plan_writer prompt must carry the scout plan; got %.120q", pk, req.Prompt)
				}
				if !strings.Contains(req.Prompt, flowContextHandoffPrefix) {
					t.Fatalf("%s: plan_writer prompt must carry the rendered context package (FCP handoff); got %.120q", pk, req.Prompt)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("%s: plan_writer turn was never scheduled after scout advance", pk)
			}
		})
	}
}

// TestRun198699ContinueDelegateWithNoChildSpawns locks the second half of the
// retry loop: a continue whose reinvoke-lifecycle target is an agent.delegate
// with no prior child (task-harness plan_writer parked by hub_stalled before it
// ever ran) must spawn a fresh child instead of silently no-op'ing — and the
// spawned child must carry the previously produced context package (FCP handoff).
func TestRun198699ContinueDelegateWithNoChildSpawns(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, ch := run198699HarnessService(t, pk)
			edges := []agentpack.FlowEdge{
				{From: "plan_synthesis", To: "plan_writer", When: "continue", Kind: "back"},
			}
			nodes := []agentpack.FlowNode{
				{ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
				{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
			}
			setRun198699Topology(t, svc, runID, edges, nodes, "plan_synthesis")

			// A prior context.produce (run-198699: context is stamped DONE but
			// plan_writer never spawned before the stall) left the package on the
			// run — the continue spawn must render it into the writer prompt.
			svc.mu.Lock()
			pkg := FlowContextPackage{WorkflowRunID: runID, PackageID: "pkg-continue-" + string(pk)}
			svc.runs[runID].planContextPackage = &pkg
			svc.mu.Unlock()

			svc.maybeReinvokeCoderForContinue(runID, "Feedback received: plan missing DeclaredPaths")

			svc.mu.Lock()
			planWriterChildren := 0
			for _, run := range svc.runs {
				if run.parentRunID == runID && run.label == "plan_writer" {
					planWriterChildren++
				}
			}
			svc.mu.Unlock()
			if planWriterChildren != 1 {
				t.Fatalf("%s: plan_writer child count = %d, want 1 (continue on a never-spawned reinvoke delegate must spawn)", pk, planWriterChildren)
			}
			if got := flowStepStatus(t, svc, runID, "plan_writer"); got != StepStatusRunning {
				t.Fatalf("%s: plan_writer step = %v, want RUNNING (continue re-entry)", pk, got)
			}
			select {
			case req := <-ch:
				if !strings.Contains(req.Prompt, "Feedback received") {
					t.Fatalf("%s: continue-spawn prompt must carry the continue feedback; got %.120q", pk, req.Prompt)
				}
				if !strings.Contains(req.Prompt, flowContextHandoffPrefix) {
					t.Fatalf("%s: continue-spawn prompt must render the stored context package (FCP handoff); got %.120q", pk, req.Prompt)
				}
			case <-time.After(3 * time.Second):
				t.Fatalf("%s: continue-spawned plan_writer turn was never scheduled", pk)
			}
		})
	}
}

// TestRun198699ContinueAgentCodeWithNoChildKeepsNoop guards the near-miss:
// an agent.code reinvoke target with no child and no frozen contract stays a
// no-op (preserves the BUG-327/dual-loop fixture contract — the code path
// must not gain an unguarded cascade here).
func TestRun198699ContinueAgentCodeWithNoChildKeepsNoop(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, _ := run198699HarnessService(t, pk)
			edges := []agentpack.FlowEdge{
				{From: "plan_synthesis", To: "plan_writer", When: "continue", Kind: "back"},
			}
			nodes := []agentpack.FlowNode{
				{ID: "plan_writer", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
				{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
			}
			setRun198699Topology(t, svc, runID, edges, nodes, "plan_synthesis")

			svc.maybeReinvokeCoderForContinue(runID, "Feedback received: cover restart case")

			svc.mu.Lock()
			planWriterChildren := 0
			for _, run := range svc.runs {
				if run.parentRunID == runID && run.label == "plan_writer" {
					planWriterChildren++
				}
			}
			svc.mu.Unlock()
			if planWriterChildren != 0 {
				t.Fatalf("%s: plan_writer child count = %d, want 0 (agent.code continue with no child must keep the no-op)", pk, planWriterChildren)
			}
		})
	}
}
