package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-469: live fp-beds/full run-96489 → task_slicer child run-96970. The
// slicer's composed prompt carried NO bound-input section at all — the
// mirrored vibe-cp-ingest definition lost its artifactBindings (seed map
// only covered task/cp-harness, and the seeder only ran for newly-synced
// records) — so the agent fell back to the task-splitter template's "find
// the newest requirements/07-Coding-Plan/todo/CP-*.md" and sliced CP-02,
// writing Task-15/16/17 parented to CP-02, while the run had ingested and
// locked CP-01. The prompt must name the run's own CP — the file the
// operator locked — never a glob-for-newest guess on a multi-CP bed.

// The slicer node's composed prompt must point at the run's locked/source
// CP, not "find the newest CP-*.md".
func TestBUG469_SlicerPromptNamesRunsLockedCPNotNewest(t *testing.T) {
	node := agentpack.FlowNode{
		ID:       "task_slicer",
		Behavior: "agent.delegate",
		Agent:    "agents/doc-writer.md",
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{
				Direction:          "input",
				SlotName:           "cp_md",
				ArtifactInstanceID: "cp_md",
				ArtifactTypeID:     ArtifactTypeFile,
				Required:           true,
				ConfigJSON: map[string]any{
					"pathTemplate": "requirements/07-Coding-Plan/todo/CP-{{cpID}}-{{slug}}.md",
				},
			},
		},
	}
	locked := "requirements/07-Coding-Plan/todo/CP-01-snake-mvp-sprint-plan.md"
	prompt := appendResolvedVibeTemplatedInputs("handoff", node, locked)
	if !strings.Contains(prompt, locked) {
		t.Fatalf("prompt lacks the run's locked CP path %q:\n%s", locked, prompt)
	}
	if strings.Contains(prompt, "newest") {
		t.Fatalf("prompt still tells the agent to find the newest CP:\n%s", prompt)
	}
}

// A node/run with no resolved source keeps the template-only behavior
// (nothing appended) — harness flows and unpinned vibe runs unchanged.
func TestBUG469_SlicerPromptNoResolvedSourceLeavesTemplate(t *testing.T) {
	node := agentpack.FlowNode{
		ID: "task_slicer",
		ArtifactBindings: []agentpack.FlowArtifactBinding{
			{
				Direction:  "input",
				ConfigJSON: map[string]any{"pathTemplate": "requirements/07-Coding-Plan/todo/CP-{{cpID}}-{{slug}}.md"},
			},
		},
	}
	if got := appendResolvedVibeTemplatedInputs("base", node, ""); got != "base" {
		t.Fatalf("empty resolved path must not append anything, got %q", got)
	}
}

// A builtin mirror whose step_artifact_bindings rows are absent (synced
// before the seed existed, or while the flow id was missing from the seed
// map) reconstructs nodes with no ArtifactBindings — recordFromWorkflowRow
// must restore them from the embedded pack, matching the BUG-458
// run/posture/config restore. This is what heals the live fp-beds/full
// mirror without a DB re-seed.
func TestBUG469_MirrorRowRestoresArtifactBindingsFromPack(t *testing.T) {
	packID, packFlowID := "flowpilot-core-flow-pack", "vibe-cp-ingest"
	slicerID, slicerBehavior := "task_slicer", "agent.delegate"
	row := dbWorkflowRow{
		ID: "9f8d7c6b-5a4b-4c3d-2e1f-0a9b8c7d6e5f", Name: "Vibe CP Ingest",
		IsBuiltin: true, PackID: &packID, PackFlowID: &packFlowID,
		EdgesJSON: []dbFlowEdgeRow{
			{From: "cp_lock", To: "task_slicer", When: "approved", Kind: "forward"},
		},
		WorkflowSteps: []dbWorkflowStepRow{
			{StepType: "x_task_slicer", OrderIndex: 0, StepDefinition: dbStepDefinitionRow{
				StepType: "x_task_slicer", NodeID: &slicerID, BehaviorID: &slicerBehavior}},
		},
		// ArtifactBindings deliberately absent — the stale-mirror state.
	}
	rec := recordFromWorkflowRow(row)
	var slicer *agentpack.FlowNode
	for i := range rec.Definition.Nodes {
		if rec.Definition.Nodes[i].ID == "task_slicer" {
			slicer = &rec.Definition.Nodes[i]
		}
	}
	if slicer == nil {
		t.Fatalf("task_slicer node missing from reconstructed definition")
	}
	if len(slicer.ArtifactBindings) == 0 {
		t.Fatalf("task_slicer.ArtifactBindings empty — builtin mirror must restore pack-declared bindings")
	}
	hasCpInput := false
	for _, b := range slicer.ArtifactBindings {
		if b.Direction == "input" && b.ArtifactInstanceID == "cp_md" {
			hasCpInput = true
		}
	}
	if !hasCpInput {
		t.Fatalf("task_slicer missing cp_md input binding after restore: %#v", slicer.ArtifactBindings)
	}
}

// The mirror seed list must cover every builtin flow that declares
// file_artifact bindings — vibe-ingest/vibe-cp-ingest/bug-plan-harness/
// vibe-sprint were missing, so their mirrors silently drop bindings.
func TestBUG469_SeedMapCoversAllBuiltinFlowsWithBindings(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	for _, def := range pack.Flows {
		hasBindings := false
		for _, n := range def.Nodes {
			if len(n.ArtifactBindings) > 0 {
				hasBindings = true
			}
		}
		if !hasBindings {
			continue
		}
		if _, ok := builtinHarnessArtifactFlowIDs[def.ID]; !ok {
			t.Fatalf("builtin flow %q declares artifactBindings but is not in builtinHarnessArtifactFlowIDs — mirror drops them", def.ID)
		}
	}
}

// The seed must heal mirrors that are already "fresh" (hash+version match):
// EnsureAllBuiltinArtifactBindingsWithStore seeds every builtin record, not
// only the ones SyncBuiltins re-upserted this boot.
func TestBUG469_AllBuiltinBindingsSeedHealsUnchangedMirror(t *testing.T) {
	original := httpRequestFn
	defer func() { httpRequestFn = original }()

	calls := 0
	httpRequestFn = func(_ context.Context, _ string, _ string, _ map[string]string, _ []byte) (int, []byte, error) {
		calls++
		return 201, []byte("[]"), nil
	}
	store := NewSupabaseWorkflowFlowStore(SupabaseWorkspaceConfig{APIURL: "https://example.supabase.co"}, "test-key")
	if err := EnsureAllBuiltinArtifactBindingsWithStore(context.Background(), store); err != nil {
		t.Fatalf("EnsureAll: %v", err)
	}
	// One POST per harness-mapped flow + one for the context flow.
	want := len(builtinHarnessArtifactFlowIDs) + 1
	if calls != want {
		t.Fatalf("seed POSTs = %d, want %d (every builtin flow with bindings, not just newly-synced)", calls, want)
	}
}
