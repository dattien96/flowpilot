package runner

// BUG-458 (found live via run-16693): recordFromWorkflowRow rebuilds
// FlowNode rows from step_definitions columns — but node `run` (inline |
// delegate), `posture`, `context_profile` and the free-form `config:` map
// have no step_definitions column, so every mirrored definition silently
// dropped them. The tournament-harness `parallel_rollout` marker is
// behaviorless and identified ONLY by `run: inline`, so the rollout
// passthrough rejected the mirrored node (flow_advance_target_not_spawnable)
// and the flow stalled after problem_scout. For built-in mirrors the
// embedded pack is the authoritative source those fields were synced FROM
// (no column exists for an admin to edit them), so reconstruction must
// restore them from the embedded definition; for non-builtin rows a
// behavior-derived fallback keeps `run` correct (agent.* -> delegate,
// everything else incl. behaviorless markers -> inline).
// New file — no pre-existing test is modified.

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// TestRecordFromWorkflowRowBuiltinMirrorRestoresRunInline reproduces the
// run-16693 shape: a mirrored tournament-harness row whose parallel_rollout
// step_definitions row carries node_id/lifecycle but no run/config columns.
// The reconstructed node must regain run:inline from the embedded pack so
// tournamentRolloutPassthrough accepts it as the cohort fan-out marker.
func TestRecordFromWorkflowRowBuiltinMirrorRestoresRunInline(t *testing.T) {
	packID, packFlowID := "flowpilot-core-flow-pack", "tournament-harness"
	scoutID, scoutBehavior, scoutLifecycle := "problem_scout", "agent.delegate", "once"
	rolloutID, rolloutLifecycle := "parallel_rollout", "once"
	row := dbWorkflowRow{
		ID: "40532e1e-9190-435b-9d81-dff01b6fe263", Name: "Tournament Harness",
		IsBuiltin: true, PackID: &packID, PackFlowID: &packFlowID,
		EdgesJSON: []dbFlowEdgeRow{
			{From: "problem_scout", To: "parallel_rollout", When: "done", Kind: "forward"},
			{From: "parallel_rollout", To: "candidate-a", When: "done", Kind: "forward"},
			{From: "parallel_rollout", To: "candidate-b", When: "done", Kind: "forward"},
		},
		WorkflowSteps: []dbWorkflowStepRow{
			{StepType: "x_problem_scout", OrderIndex: 0, StepDefinition: dbStepDefinitionRow{
				StepType: "x_problem_scout", NodeID: &scoutID, BehaviorID: &scoutBehavior, NodeLifecycle: &scoutLifecycle}},
			{StepType: "x_parallel_rollout", OrderIndex: 1, StepDefinition: dbStepDefinitionRow{
				StepType: "x_parallel_rollout", NodeID: &rolloutID, NodeLifecycle: &rolloutLifecycle}},
		},
	}
	rec := recordFromWorkflowRow(row)
	var rollout *agentpack.FlowNode
	for i := range rec.Definition.Nodes {
		if rec.Definition.Nodes[i].ID == "parallel_rollout" {
			rollout = &rec.Definition.Nodes[i]
		}
	}
	if rollout == nil {
		t.Fatalf("parallel_rollout node missing from reconstructed definition")
	}
	if rollout.Run != "inline" {
		t.Fatalf("parallel_rollout.Run = %q, want %q restored from the embedded pack (mirror has no run column)", rollout.Run, "inline")
	}
	// The rollout marker is only valid if its config: (candidates/serial)
	// survived the mirror round-trip too — it has no column either.
	if _, ok := rollout.Config["candidates"]; !ok {
		t.Fatalf("parallel_rollout.Config missing \"candidates\" — pack-declared config must be restored for builtin mirrors, got %#v", rollout.Config)
	}
}

// TestRecordFromWorkflowRowBuiltinMirrorRestoresNodeModel covers the
// tournament candidate model: column (`step_definitions.model` exists but
// mirror sync never writes pack values — it is reserved for admin
// overrides), so a mirrored candidate reconstructed Model="" and spawned on
// the parent's provider instead of its pack-declared one (run-17745: both
// candidates inherited devin and hit the free-tier rate limit). Restoring
// the pack value keeps admin precedence (resolveConfiguredModelForAgent
// consults the row first) and reproduces embedded-pack resolution.
func TestRecordFromWorkflowRowBuiltinMirrorRestoresNodeModel(t *testing.T) {
	packID, packFlowID := "flowpilot-core-flow-pack", "tournament-harness"
	candAID, candBehavior, candLifecycle := "candidate-a", "agent.delegate", "spawn"
	candBID := "candidate-b"
	row := dbWorkflowRow{
		ID: "wf-tournament-2", Name: "Tournament Harness",
		IsBuiltin: true, PackID: &packID, PackFlowID: &packFlowID,
		WorkflowSteps: []dbWorkflowStepRow{
			{StepType: "x_candidate_a", OrderIndex: 0, StepDefinition: dbStepDefinitionRow{
				StepType: "x_candidate_a", NodeID: &candAID, BehaviorID: &candBehavior, NodeLifecycle: &candLifecycle}},
			{StepType: "x_candidate_b", OrderIndex: 1, StepDefinition: dbStepDefinitionRow{
				StepType: "x_candidate_b", NodeID: &candBID, BehaviorID: &candBehavior, NodeLifecycle: &candLifecycle}},
		},
	}
	rec := recordFromWorkflowRow(row)
	got := map[string]string{}
	for _, n := range rec.Definition.Nodes {
		got[n.ID] = n.Model
	}
	if got["candidate-a"] != "claude-sonnet" || got["candidate-b"] != "gpt-5.4-mini" {
		t.Fatalf("candidate models = %#v, want pack-declared claude-sonnet / gpt-5.4-mini restored from the embedded pack", got)
	}
	if err := agentpack.ValidateFlowDefinition(rec.Definition); err != nil {
		t.Fatalf("ValidateFlowDefinition = %v, want nil (restored delegate models must satisfy Task-320)", err)
	}
}

// TestRecordFromWorkflowRowBuiltinMirrorRestoresArbiterConfig covers the
// tournament_arbiter node's config: (auto_pick/max_attempts) — silently
// dropped by the same gap, which degrades max_attempts 2 -> the 1 default
// and removes the retry round A-65 requires.
func TestRecordFromWorkflowRowBuiltinMirrorRestoresArbiterConfig(t *testing.T) {
	packID, packFlowID := "flowpilot-core-flow-pack", "tournament-harness"
	arbiterID, arbiterBehavior := "tournament_arbiter", "tournament.arbiter"
	row := dbWorkflowRow{
		ID: "wf-tournament", Name: "Tournament Harness",
		IsBuiltin: true, PackID: &packID, PackFlowID: &packFlowID,
		WorkflowSteps: []dbWorkflowStepRow{
			{StepType: "x_arbiter", OrderIndex: 0, StepDefinition: dbStepDefinitionRow{
				StepType: "x_arbiter", NodeID: &arbiterID, BehaviorID: &arbiterBehavior}},
		},
	}
	rec := recordFromWorkflowRow(row)
	if len(rec.Definition.Nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(rec.Definition.Nodes))
	}
	node := rec.Definition.Nodes[0]
	if node.Run != "inline" {
		t.Fatalf("tournament_arbiter.Run = %q, want inline", node.Run)
	}
	if got, ok := node.Config["max_attempts"]; !ok || got != float64(2) && got != 2 {
		t.Fatalf("tournament_arbiter.Config[max_attempts] = %#v (ok=%v), want 2 from the embedded pack", got, ok)
	}
}

// TestRecordFromWorkflowRowBuiltinMirrorRestoresContextProfiles covers the
// flow-level `contextProfiles` map — it has no workflows/step_definitions
// column either, so mirrored vibe-sprint rows reconstructed with an empty map
// while BUG-458's node-level restore re-attached `contextProfile: scout` to
// preflight_contract_plan. ValidateFlowContextSources then failed the flow
// load ("node ... declares unknown context profile \"scout\"") and the vibe
// sprint could not start (live run-34947). For builtin mirrors the embedded
// pack is again the authoritative source — restore the whole map so node
// profile refs resolve.
func TestRecordFromWorkflowRowBuiltinMirrorRestoresContextProfiles(t *testing.T) {
	packID, packFlowID := "flowpilot-core-flow-pack", "vibe-sprint"
	planID, planBehavior, planLifecycle := "preflight_contract_plan", "agent.delegate", "once"
	row := dbWorkflowRow{
		ID: "wf-vibe-sprint", Name: "Vibe Sprint",
		IsBuiltin: true, PackID: &packID, PackFlowID: &packFlowID,
		WorkflowSteps: []dbWorkflowStepRow{
			{StepType: "x_plan", OrderIndex: 0, StepDefinition: dbStepDefinitionRow{
				StepType: "x_plan", NodeID: &planID, BehaviorID: &planBehavior, NodeLifecycle: &planLifecycle}},
		},
	}
	rec := recordFromWorkflowRow(row)
	if len(rec.Definition.ContextProfiles) == 0 {
		t.Fatalf("ContextProfiles empty on builtin mirror — pack-declared profiles must be restored (node contextProfile refs fail validation otherwise)")
	}
	if _, ok := rec.Definition.ContextProfiles["scout"]; !ok {
		t.Fatalf("ContextProfiles missing \"scout\" — got keys %#v", rec.Definition.ContextProfiles)
	}
	if err := ValidateFlowContextSources(rec.Definition); err != nil {
		t.Fatalf("ValidateFlowContextSources = %v, want nil after ContextProfiles restore", err)
	}
}

// TestRecordFromWorkflowRowDerivesRunForUserRows covers non-builtin rows
// (cloned/user-authored flows): no embedded counterpart exists, so `run`
// must be derived from the node's behavior — agent.* spawns a child
// (delegate), every other behavior and every behaviorless marker executes
// in-process (inline). step_definitions has no run column, so "" is never
// a correct reconstructed value.
func TestRecordFromWorkflowRowDerivesRunForUserRows(t *testing.T) {
	markerID := "rollout_marker"
	delegateID, delegateBehavior := "coder", "agent.delegate"
	inlineID, inlineBehavior := "validate", "command.validate"
	row := dbWorkflowRow{
		ID: "wf-user-1", Name: "user flow",
		WorkflowSteps: []dbWorkflowStepRow{
			{StepType: "u_marker", OrderIndex: 0, StepDefinition: dbStepDefinitionRow{
				StepType: "u_marker", NodeID: &markerID}},
			{StepType: "u_coder", OrderIndex: 1, StepDefinition: dbStepDefinitionRow{
				StepType: "u_coder", NodeID: &delegateID, BehaviorID: &delegateBehavior}},
			{StepType: "u_validate", OrderIndex: 2, StepDefinition: dbStepDefinitionRow{
				StepType: "u_validate", NodeID: &inlineID, BehaviorID: &inlineBehavior}},
		},
	}
	rec := recordFromWorkflowRow(row)
	got := map[string]string{}
	for _, n := range rec.Definition.Nodes {
		got[n.ID] = n.Run
	}
	want := map[string]string{"rollout_marker": "inline", "coder": "delegate", "validate": "inline"}
	for id, w := range want {
		if got[id] != w {
			t.Fatalf("node %q Run = %q, want %q (derived from behavior)", id, got[id], w)
		}
	}
}
