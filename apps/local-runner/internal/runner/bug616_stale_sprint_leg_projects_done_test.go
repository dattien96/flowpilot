package runner

import (
	"context"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-616: a stopped mid-sprint vibe run skipped Task-032 on resume
// (live repro run-150388 -- handoff-sprint-2.yaml written at restart with
// VaultContainer.cpp still all stub sentinels and the task file still in
// todo/). resumedFlowStepRows projects each durable child session onto a flow
// node by label/agent-name only -- `vibe_task_index` is stamped on every vibe
// leg at spawn but was never consulted. Sprint-1's coder/reviewer legs stay
// `leg:active` forever (legs close only at run end), so at resume into
// sprint-2 the *previous* sprint's completed coder (run-162468,
// vibe_task_index=1) satisfied the *current* sprint's `coder` node ->
// vibeSprintEvidenceComplete -> audit auto-finalize -> sprint boundary
// advanced to Task-033 with the sprint-2 coder never run.
//
// Fix: a child session carrying a vibe_task_index that differs from the
// reconstructed run's current vibeSprintIndex is foreign-sprint evidence and
// must not satisfy this sprint's node. Index-0 (unstamped/legacy) sessions
// keep the old behavior -- an unprovenanced leg fails open rather than
// silently re-dispatching real work.

func bug616VibeSprintNodes() []agentpack.FlowNode {
	return []agentpack.FlowNode{
		{ID: "preflight_contract_plan", Behavior: "agent.delegate", Agent: "contract-planner"},
		{ID: "context", Behavior: "agent.delegate", Agent: "context-scout"},
		{ID: "tdd", Behavior: "agent.delegate", Agent: "scaffold-architect", DependsOn: []string{"context"}},
		{ID: "coder", Behavior: "agent.code", Agent: "pkv-coder", DependsOn: []string{"tdd"}},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "vault-reviewer", DependsOn: []string{"coder"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "synthesizer", DependsOn: []string{"reviewer"}},
		{ID: "audit", Behavior: "hub.inline", Agent: "auditor", DependsOn: []string{"synthesis"}},
	}
}

// TestResumedFlowStepRowsForeignSprintLegsDoNotSatisfyNodes is the BUG-616
// repro: resuming sprint 2 while sprint-1 coder/reviewer legs are still
// durable `completed` must NOT mark sprint-2 coder/reviewer DONE.
func TestResumedFlowStepRowsForeignSprintLegsDoNotSatisfyNodes(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	ctx := context.Background()
	nodes := bug616VibeSprintNodes()
	now := "2026-10-03T11:24:00Z"
	for _, session := range []ProviderSessionState{
		{
			// Sprint-1 coder: finished its sprint, leg never closed (legs
			// close at run end). Must NOT satisfy sprint-2's coder node.
			RunID: "run-162468", ParentRunID: "run-hub-616", Label: "coder",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			AgentName: "pkv-coder", Role: "pkv-coder",
			Status: RunStatusCompleted, VibeTaskIndex: 1,
			StartedAt: now, UpdatedAt: now,
		},
		{
			// Sprint-1 reviewer under a validate-round cohort.
			RunID: "run-162738", ParentRunID: "run-hub-616", Label: "reviewer",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			AgentName: "vault-reviewer", Role: "vault-reviewer",
			Status: RunStatusCompleted, VibeTaskIndex: 1,
			FlowCohortID: "flow-auto-validate-round-14",
			StartedAt:    now, UpdatedAt: now,
		},
		{
			// Current-sprint tdd leg: real evidence for sprint-2's tdd node.
			RunID: "run-163587", ParentRunID: "run-hub-616", Label: "tdd",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			AgentName: "scaffold-architect", Role: "scaffold-architect",
			Status: RunStatusRunning, VibeTaskIndex: 2,
			StartedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertProviderSession(ctx, session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}

	_, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-hub-616",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyClaude,
		WorkflowID:      "wf-616",
		RunKind:         "workflow",
		Status:          RunStatusRunning,
		StartedAt:       now,
		UpdatedAt:       now,
		ActiveFlowNodes: nodes,
		AutoOrchestrate: true,
		VibeSprintIndex: 2,
		VibeTaskPlan:    []string{"Task-031", "Task-032", "Task-033"},
		LoopState:       AgentLoopState{Status: "running", Round: 9, Cap: 20},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if got := flowStepStatus(t, svc, "run-hub-616", "tdd"); got != StepStatusCanceled {
		t.Fatalf("tdd = %q, want CANCELED (in-flight at kill -> demoted, re-driven by the flow)", got)
	}
	if got := flowStepStatus(t, svc, "run-hub-616", "coder"); got != StepStatusPending {
		t.Fatalf("coder = %q, want PENDING -- BUG-616 repro: sprint-1 leg stamped it DONE", got)
	}
	if got := flowStepStatus(t, svc, "run-hub-616", "reviewer"); got != StepStatusPending {
		t.Fatalf("reviewer = %q, want PENDING -- BUG-616 repro: sprint-1 leg stamped it DONE", got)
	}
	if got := flowStepStatus(t, svc, "run-hub-616", "audit"); got != StepStatusPending {
		t.Fatalf("audit = %q, want PENDING (never ran) -- BUG-616: stamped DONE unblocked the boundary", got)
	}
}

// TestVibeSprintEvidenceSeesParkedTopology covers the live repro's second
// half: the restart happened while the owner-debate overlay was mounted, so
// activeFlowNodes held ONLY debate nodes. vibeSprintEvidenceComplete scanned
// activeFlowNodes, found no agent.code node, and vacuously passed — audit
// auto-finalized while the parked sprint's coder step was still PENDING.
// With the union, the parked coder node is visible and its PENDING row fails
// the check closed.
func TestVibeSprintEvidenceSeesParkedTopology(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	now := "2026-10-03T11:24:00Z"
	debateNodes := []agentpack.FlowNode{
		{ID: "debate_trigger", Behavior: "hub.inline", Agent: "synthesizer"},
		{ID: "owner_1", Behavior: "agent.delegate", Agent: "owner", DependsOn: []string{"debate_trigger"}},
		{ID: "debate_synthesis", Behavior: "hub.inline", Agent: "synthesizer", DependsOn: []string{"owner_1"}},
	}
	_, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-hub-616c",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyClaude,
		WorkflowID:      "wf-616c",
		RunKind:         "workflow",
		Status:          RunStatusRunning,
		StartedAt:       now,
		UpdatedAt:       now,
		ActiveFlowNodes: debateNodes,
		VibeParkedNodes: bug616VibeSprintNodes(),
		AutoOrchestrate: true,
		VibeSprintIndex: 2,
		VibeTaskPlan:    []string{"Task-031", "Task-032", "Task-033"},
		LoopState:       AgentLoopState{Status: "running", Round: 9, Cap: 20},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	// The parked sprint's coder never ran → its row is PENDING → evidence
	// incomplete. Pre-fix: debate-only activeFlowNodes yielded zero coder IDs
	// and the check vacuously returned true.
	if svc.vibeSprintEvidenceComplete("run-hub-616c") {
		t.Fatal("vibeSprintEvidenceComplete = true with parked coder PENDING — BUG-616 repro")
	}
}

// TestResumedFlowStepRowsSameSprintLegsStillProject is the parity guard: legs
// stamped with the CURRENT sprint index keep satisfying their nodes, and
// unstamped (index-0) legs fail open exactly as before.
func TestResumedFlowStepRowsSameSprintLegsStillProject(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	ctx := context.Background()
	nodes := bug616VibeSprintNodes()
	now := "2026-10-03T11:24:00Z"
	for _, session := range []ProviderSessionState{
		{
			// Current-sprint coder: same index as the run -> DONE evidence.
			RunID: "run-coder-616", ParentRunID: "run-hub-616b", Label: "coder",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			AgentName: "pkv-coder", Role: "pkv-coder",
			Status: RunStatusCompleted, VibeTaskIndex: 2,
			StartedAt: now, UpdatedAt: now,
		},
		{
			// Legacy unstamped reviewer (index 0): fails open -> still counts.
			RunID: "run-reviewer-616", ParentRunID: "run-hub-616b", Label: "reviewer",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			AgentName: "vault-reviewer", Role: "vault-reviewer",
			Status: RunStatusCompleted,
			StartedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertProviderSession(ctx, session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}

	_, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-hub-616b",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyClaude,
		WorkflowID:      "wf-616b",
		RunKind:         "workflow",
		Status:          RunStatusRunning,
		StartedAt:       now,
		UpdatedAt:       now,
		ActiveFlowNodes: nodes,
		AutoOrchestrate: true,
		VibeSprintIndex: 2,
		VibeTaskPlan:    []string{"Task-031", "Task-032", "Task-033"},
		LoopState:       AgentLoopState{Status: "running", Round: 9, Cap: 20},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if got := flowStepStatus(t, svc, "run-hub-616b", "coder"); got != StepStatusDone {
		t.Fatalf("coder = %q, want DONE (same-sprint evidence)", got)
	}
	if got := flowStepStatus(t, svc, "run-hub-616b", "reviewer"); got != StepStatusDone {
		t.Fatalf("reviewer = %q, want DONE (index-0 leg fails open)", got)
	}
}
