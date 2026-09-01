package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// CP-58 Task-306 runner-level proofs for cp-harness. New file — no
// pre-existing test is modified.

// TestCpHarnessEntrySpawnIsScoutOnly (Task-306 T-1) proves the slice-only
// flow cannot leak coding into the entry spawn: the flow executor spawns ALL
// entry delegate nodes at flow start, so cp-harness must resolve to exactly
// one entry — preflight_contract_plan. A wrongly declared coding-chain node
// (no incoming forward edge, or an unreferenced agent node) would appear here.
func TestCpHarnessEntrySpawnIsScoutOnly(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var cpDef agentpack.FlowDefinition
	found := false
	for _, def := range pack.Flows {
		if def.ID == "cp-harness" {
			cpDef = def
			found = true
		}
	}
	if !found {
		t.Fatal("cp-harness missing from builtin pack")
	}
	if err := agentpack.ValidateFlowDefinition(cpDef); err != nil {
		t.Fatalf("ValidateFlowDefinition(cp-harness) = %v, want nil", err)
	}
	entries := entryDelegateNodes(cpDef)
	if len(entries) != 1 || entries[0].ID != "preflight_contract_plan" {
		t.Fatalf("cp-harness entry delegate nodes = %+v, want exactly [preflight_contract_plan]", entries)
	}
}

// TestCpHarnessPlanLoopContinueReusesCpPlanWriter drives the CP plan loop on
// the live edge shape: cp_synthesis's flow_control("continue") re-enters
// cp_plan_writer on the SAME child session, never a fresh child.
func TestCpHarnessPlanLoopContinueReusesCpPlanWriter(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				prompts = append(prompts, req.Prompt)
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})

	edges := []agentpack.FlowEdge{
		{From: "cp_plan_writer", To: "cp_reviewer", When: "done", Kind: "forward"},
		{From: "cp_reviewer", To: "cp_synthesis", When: "done", Kind: "forward"},
		{From: "cp_synthesis", To: "cp_plan_writer", When: "continue", Kind: "back"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "cp_plan_writer", Behavior: "agent.delegate", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "cp_reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "spawn", Cohort: "plan", Join: "all"},
		{ID: "cp_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = "cp_synthesis"
	svc.mu.Unlock()

	if _, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "agents/coder.md", Prompt: "write the CP architecture doc", Label: "cp_plan_writer", Wait: false,
	}); err != nil {
		t.Fatalf("spawn cp_plan_writer: %v", err)
	}
	waitLoop(t, "cp_plan_writer first turn completed", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(prompts) >= 1
	})

	fc, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status: "continue", Summary: "CP DOD-3 is not measurable",
	})
	if fcErr != nil {
		t.Fatalf("applyFlowControl(cp continue): %v", fcErr)
	}
	if fc.NextAction != "looping" {
		t.Fatalf("NextAction = %q, want looping", fc.NextAction)
	}

	waitLoop(t, "cp_plan_writer reinvoked with CP review findings", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range prompts {
			if strings.Contains(p, "Feedback received") && strings.Contains(p, "not measurable") {
				return true
			}
		}
		return false
	})
	svc.mu.Lock()
	children := 0
	for _, run := range svc.runs {
		if run.parentRunID == parent.RunID && run.label == "cp_plan_writer" {
			children++
		}
	}
	svc.mu.Unlock()
	if children != 1 {
		t.Fatalf("cp_plan_writer child count = %d, want 1 (plan-loop session reuse)", children)
	}
}
