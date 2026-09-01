# Task-307: Harness Plan Artifact Types And Panel Parity

## Metadata

- Document ID: `Task-307`
- Title: `Harness Plan Artifact Types And Panel Parity`
- Phase: `task`
- Status: `inprogress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-09-01`
- Parent Documents: [CP-58: Bug / Task / CP Harness With Plan Artifact And Dual Review Loops](../../07-Coding-Plan/todo/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md)
- Child Documents: `None`
- Related Documents: [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [CP-45: Generic Artifact Types And Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md), [CP-55: Flow-First Preflight Contract](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- Replaces: `None`
- Tags: `artifact-framework, file-artifact, harness, panel, flow`

## AI Quick View

### Summary

- Seeds `file_artifact` typed instances for `plan_md` / `cp_md` / `task_md` (`is_builtin=true`, service role only) and binds them to `plan_writer` / `cp_plan_writer` / `task_splitter` nodes (OUTPUT) and to reviewers/splitter (INPUT).
- Makes harness md outputs deterministic, validated, and visible in Desktop artifact panel with zero new type DSL — reuses `file_artifact` `pathTemplate` only.
- Adds harness picker labels and ensures cloned harness edits preserve `acceptance_nodes` (CP-55 P-1 regression).

### Current Ask

- Wire the `file_artifact` plumbing so `task-harness` and `cp-harness` md outputs are not free-form writes but validated artifact slots.

### Key Decisions

- `T-1` Reuse `file_artifact` only (`SD-23` D-11) — no new artifact type, only new instances (`plan_md`, `cp_md`, `task_md`) with `ConfigJSON: {pathTemplate, required}`.
- `T-2` Bindings are `FlowArtifactBinding` denormalized at `recordFromWorkflowRow` (mirror CP-45) — executor never does second DB lookup.
- `T-3` Panel parity is additive: harness choice in `/flow` picker is data (`selectableIn:flow`), not provider branching.

### Constraints

- `artifact_types` / `artifact_instances` with `is_builtin=true` writable only via service role; `authenticated` RLS blocks (SD-23 §3.11, `workflows` pattern).
- No change to `ValidateFlowSafetyTopology` acceptance logic.
- Pack-level artifact files must be declared in `manifest.yaml` if embedded.

### Open Questions

- `Q-1` `pathTemplate` for `task_md`: fixed `requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md` vs model-chosen `declared_paths` within `requirements/...` whitelist?
- `Q-2` Should `plan_md` for `task-harness` write to `requirements/08-Task/todo/` or also emit a `07-Coding-Plan` fragment?

### Source Refs

- `CP-58` `P-6`, §4 `P-4`; `SD-23` D-11 `ArtifactTypeRegistry` + `supabase_workflow_flow_store.go` `recordFromWorkflowRow`; `pack.go:141 FlowArtifactBinding`; `artifact_type_registry.go` Task-201/202 path injection; `CP-45`.

## 1. Goal

Make every harness md output (`Task-*.md`, `CP-*.md`) a `file_artifact` slot — validated at authoring/load time, rendered in the artifact panel, and traceable via `step_artifact_bindings` — without introducing a new artifact type.

## 2. Parent Links

- coding plan: `CP-58` `P-6`, `P-4`
- tech design: `SD-23` D-11, `SD-21`
- system spec: `SS-13` (doc contract), `SS-14`
- specific upstream ids: `CP-58 P-6`, `SD-23 §3.11`, `pack.go:141`

## 3. Trigger

Without bindings, `plan_writer` free-writes `requirements/08-Task/todo/*.md` with no validation — a Task-harness run can silently write to `requirements/09-BugFix/` or outside `requirements/` and still pass `plan_synthesis`. `SD-23` requires slot validation (fail at authoring/load, not opaque provider failure).

## 4. Exact Change

- `T-1` Seed `artifact_types` row `file_artifact` if not already (CP-45) — service role only.
- `T-2` Seed `artifact_instances` rows (service role, `is_builtin=true`): `plan_md` (`file_artifact`, `config:{pathTemplate:"requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md", required:true, description:"Task plan md from task-harness"}`), `cp_md` (`config:{pathTemplate:"requirements/07-Coding-Plan/todo/CP-{{cpID}}-{{slug}}.md", required:true}` where `{{cpID}}` = next free CP number resolved at authoring from the manifest/plan folder — no wildcard in the template), `task_md` (`config:{pathTemplate:"requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md", required:true, count:"N per P-*"}`).
- `T-3` Wire `FlowNode.ArtifactBindings` in `task-harness.yaml` / `cp-harness.yaml`: `plan_writer` OUTPUT `plan_md`, `plan_reviewer` INPUT `plan_md` (required), `cp_plan_writer` OUTPUT `cp_md`, `cp_reviewer` INPUT `cp_md`, `task_splitter` INPUT `cp_md` OUTPUT `task_md[]`.
- `T-4` `apps/local-runner/internal/runner/artifact_type_registry.go` — reuse existing `file_artifact` resolver: INPUT → path-list mention only (BUG-276), OUTPUT → required-path write contract + template guidance (Task-223/224). No new resolver.
- `T-5` `apps/local-runner/internal/runner/supabase_workflow_flow_store.go` — ensure `recordFromWorkflowRow` denormalizes new bindings (join `step_artifact_bindings` → `artifact_instances` → `FlowArtifactBinding.ConfigJSON`), mirroring `context_artifact` path (Task-201).
- `T-6` `supabase/migrations/*_add_harness_artifacts.sql` (if pack not embedding): inserts for `plan_md`/`cp_md`/`task_md` with `is_builtin=true`; RLS `authenticated` blocked; `EnsureBuiltinArtifactBindingsWithStore` sync verified (`builtin_artifact_bindings.go:36`, called from `internal/cli/root.go:176`).
- `T-7` Desktop `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` harness labels (optional): picker shows `bug-harness` / `task-harness` / `cp-harness` with description; clone preserves `acceptance_nodes` (already fixed CP-55 P-1, re-assert).
- `T-8` Pack/runner tests: `TestHarnessArtifactBindings` asserts each harness node has expected `ArtifactBindings` length/direction/required/ArtifactTypeID; `TestHarnessArtifactBindingRejectsPathOutsideRequirements` — binding validation fails deterministically when an OUTPUT path escapes `requirements/` (e.g. `../../etc/passwd` or `D:\out.md`), at authoring/load time, not as an opaque provider error.

## Code Guide

### CG-1: SQL Migration — Artifact Instance Seeding

```sql
-- supabase/migrations/20260828_add_harness_plan_artifacts.sql

-- Ensure artifact_type 'file_artifact' exists (idempotent, from CP-45)
INSERT INTO artifact_types (id, name, description, is_builtin)
VALUES ('file_artifact', 'File Artifact', 'Markdown file output bound to a flow node', true)
ON CONFLICT (id) DO NOTHING;

-- Seed artifact instances for plan outputs (is_builtin=true, service role only)
INSERT INTO artifact_instances (id, artifact_type_id, is_builtin, config)
VALUES
  ('plan_md', 'file_artifact', true, '{
    "pathTemplate": "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md",
    "required": true,
    "description": "Task plan md from task-harness plan_writer"
  }'::jsonb),
  ('cp_md', 'file_artifact', true, '{
    "pathTemplate": "requirements/07-Coding-Plan/todo/CP-{{cpID}}-{{slug}}.md",
    "required": true,
    "description": "CP architecture md from cp-harness cp_plan_writer"
  }'::jsonb),
  ('task_md', 'file_artifact', true, '{
    "pathTemplate": "requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md",
    "required": true,
    "count": "N per P-*",
    "description": "Individual Task md files from cp-harness task_splitter"
  }'::jsonb)
ON CONFLICT (id) DO UPDATE SET config = EXCLUDED.config;

-- RLS: authenticated role CANNOT write is_builtin rows
-- (already enforced by existing policy on artifact_instances, verify)
```

### CG-2: YAML Artifact Bindings — `task-harness.yaml` Changes

Add `artifactBindings` to relevant nodes in `task-harness.yaml`:

```yaml
# In task-harness.yaml, update plan_writer node:
  - id: plan_writer
    # ... existing fields ...
    artifactBindings:
      - direction: output
        slotName: plan_md
        artifactInstanceId: plan_md
        required: true
        position: 0

# Update plan_reviewer node:
  - id: plan_reviewer
    # ... existing fields ...
    artifactBindings:
      - direction: input
        slotName: plan_md
        artifactInstanceId: plan_md
        required: true
        position: 0
```

### CG-3: YAML Artifact Bindings — `cp-harness.yaml` Changes

```yaml
# In cp-harness.yaml, update cp_plan_writer node:
  - id: cp_plan_writer
    # ... existing fields ...
    artifactBindings:
      - direction: output
        slotName: cp_md
        artifactInstanceId: cp_md
        required: true
        position: 0

# Update cp_reviewer node:
  - id: cp_reviewer
    # ... existing fields ...
    artifactBindings:
      - direction: input
        slotName: cp_md
        artifactInstanceId: cp_md
        required: true
        position: 0

# Update task_splitter node:
  - id: task_splitter
    # ... existing fields ...
    artifactBindings:
      - direction: input
        slotName: cp_md
        artifactInstanceId: cp_md
        required: true
        position: 0
      - direction: output
        slotName: task_md
        artifactInstanceId: task_md
        required: true
        position: 1
```

### CG-4: `recordFromWorkflowRow` — Binding Denormalization Path

**NO SIGNATURE CHANGE** — the existing code already handles `ArtifactBindings`:

```go
// supabase_workflow_flow_store.go:138-275 — recordFromWorkflowRow
// This already denormalizes artifact bindings at lines ~250-268:
for _, b := range defn.ArtifactBindings {
    binding := agentpack.FlowArtifactBinding{
        Direction:          b.Direction,
        SlotName:           b.SlotName,
        ArtifactInstanceID: b.ArtifactInstanceID,
        Required:           b.Required,
        Position:           b.Position,
    }
    if b.ArtifactInstance != nil {
        binding.ArtifactTypeID = b.ArtifactInstance.ArtifactTypeID
        binding.ConfigJSON = b.ArtifactInstance.ConfigJSON
    }
    node.ArtifactBindings = append(node.ArtifactBindings, binding)
}
// NEW instances (plan_md/cp_md/task_md) flow through this existing path
// as long as step_artifact_bindings rows are seeded via EnsureBuiltinArtifactBindingsWithStore
```

### CG-5: `ArtifactTypeRegistry` — Resolver Reuse

**NO NEW RESOLVER** — reuse existing `fileArtifactResolver`:

```go
// artifact_type_registry.go — already registered:
var defaultArtifactTypeRegistry = func() *ArtifactTypeRegistry {
    r := NewArtifactTypeRegistry()
    if err := r.Register(&fileArtifactResolver{}); err != nil {
        panic(err)
    }
    return r
}()

// The fileArtifactResolver handles:
// - INPUT: path-list mention only (reads file at pathTemplate, injects into prompt)
// - OUTPUT: required-path write contract + template guidance
// plan_md/cp_md/task_md all use ArtifactTypeID="file_artifact" → same resolver

// ArtifactResolver interface (for reference, NO CHANGE):
type ArtifactResolver interface {
    ArtifactTypeID() string
    Resolve(workspaceCwd string, binding agentpack.FlowArtifactBinding) (ArtifactResolveResult, error)
}
```

### CG-6: Path Validation — Binding Security Check

Add validation in `fileArtifactResolver.Resolve()` or at pack load time:

```go
// Ensure OUTPUT paths stay within requirements/ directory
func validateArtifactOutputPath(pathTemplate string) error {
    // Normalize and check the template doesn't escape requirements/
    normalized := filepath.Clean(pathTemplate)
    if !strings.HasPrefix(normalized, "requirements/") &&
       !strings.HasPrefix(normalized, "requirements\\") {
        return fmt.Errorf("file_artifact OUTPUT path %q must be under requirements/", pathTemplate)
    }
    // Reject path traversal
    if strings.Contains(normalized, "..") {
        return fmt.Errorf("file_artifact OUTPUT path %q contains path traversal", pathTemplate)
    }
    return nil
}
```

### CG-7: Desktop Harness Labels (`WorkflowsSettings.tsx`)

```tsx
// apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx
// Add harness display labels (additive, no existing code change)

const HARNESS_LABELS: Record<string, { label: string; description: string }> = {
  'bug-harness': {
    label: 'Bug / Hotfix',
    description: '9-step: TDD + Code Review (fast, no plan overhead)',
  },
  'task-harness': {
    label: 'Task / Feature',
    description: '11-step: Plan Writer + Plan Review + TDD + Code Review',
  },
  'cp-harness': {
    label: 'Coding Plan',
    description: '8-node: CP Plan + Review + Auto Task Splitter (slice-only)',
  },
};
```

### CG-8: Test Signatures

```go
// --- apps/local-runner/internal/runner/artifact_binding_test.go (NEW) ---

func TestHarnessArtifactBindings(t *testing.T) {
    pack, err := agentpack.LoadBuiltinPack()
    require.NoError(t, err)

    t.Run("task-harness bindings", func(t *testing.T) {
        def := findFlowDef(t, pack, "task-harness")

        // plan_writer has 1 OUTPUT binding (plan_md)
        pw := findNode(t, def, "plan_writer")
        assert.Len(t, pw.ArtifactBindings, 1)
        assert.Equal(t, "output", pw.ArtifactBindings[0].Direction)
        assert.Equal(t, "plan_md", pw.ArtifactBindings[0].SlotName)
        assert.Equal(t, "file_artifact", pw.ArtifactBindings[0].ArtifactTypeID)
        assert.True(t, pw.ArtifactBindings[0].Required)

        // plan_reviewer has 1 INPUT binding (plan_md)
        pr := findNode(t, def, "plan_reviewer")
        assert.Len(t, pr.ArtifactBindings, 1)
        assert.Equal(t, "input", pr.ArtifactBindings[0].Direction)
        assert.Equal(t, "plan_md", pr.ArtifactBindings[0].SlotName)
        assert.True(t, pr.ArtifactBindings[0].Required)
    })

    t.Run("cp-harness bindings", func(t *testing.T) {
        def := findFlowDef(t, pack, "cp-harness")

        // cp_plan_writer has 1 OUTPUT binding (cp_md)
        cpw := findNode(t, def, "cp_plan_writer")
        assert.Len(t, cpw.ArtifactBindings, 1)
        assert.Equal(t, "output", cpw.ArtifactBindings[0].Direction)
        assert.Equal(t, "cp_md", cpw.ArtifactBindings[0].SlotName)
        assert.Equal(t, "file_artifact", cpw.ArtifactBindings[0].ArtifactTypeID)
        assert.True(t, cpw.ArtifactBindings[0].Required)

        // cp_reviewer has 1 INPUT binding (cp_md)
        cpr := findNode(t, def, "cp_reviewer")
        assert.Len(t, cpr.ArtifactBindings, 1)
        assert.Equal(t, "input", cpr.ArtifactBindings[0].Direction)
        assert.Equal(t, "cp_md", cpr.ArtifactBindings[0].SlotName)

        // task_splitter has 1 INPUT (cp_md) + 1 OUTPUT (task_md)
        ts := findNode(t, def, "task_splitter")
        assert.Len(t, ts.ArtifactBindings, 2)

        inputBinding := findBinding(ts.ArtifactBindings, "input", "cp_md")
        assert.NotNil(t, inputBinding)
        assert.True(t, inputBinding.Required)

        outputBinding := findBinding(ts.ArtifactBindings, "output", "task_md")
        assert.NotNil(t, outputBinding)
        assert.True(t, outputBinding.Required)
    })
}

func TestHarnessArtifactBindingRejectsPathOutsideRequirements(t *testing.T) {
    // Create a FlowArtifactBinding with pathTemplate outside requirements/
    binding := agentpack.FlowArtifactBinding{
        Direction:      "output",
        SlotName:       "plan_md",
        ArtifactTypeID: "file_artifact",
        ConfigJSON: map[string]any{
            "pathTemplate": "../../etc/passwd",
            "required":     true,
        },
        Required: true,
    }

    // Assert: validation fails deterministically at binding validation
    err := validateArtifactOutputPath(binding.ConfigJSON["pathTemplate"].(string))
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "must be under requirements/")
}

func TestHarnessArtifactBindingRejectsWindowsPathTraversal(t *testing.T) {
    binding := agentpack.FlowArtifactBinding{
        Direction:      "output",
        SlotName:       "plan_md",
        ArtifactTypeID: "file_artifact",
        ConfigJSON: map[string]any{
            "pathTemplate": "D:\\out.md",
            "required":     true,
        },
        Required: true,
    }

    err := validateArtifactOutputPath(binding.ConfigJSON["pathTemplate"].(string))
    assert.Error(t, err)
}

// --- Helper functions (shared across harness tests) ---

func findBinding(bindings []agentpack.FlowArtifactBinding, dir, slot string) *agentpack.FlowArtifactBinding {
    for i := range bindings {
        if bindings[i].Direction == dir && bindings[i].SlotName == slot {
            return &bindings[i]
        }
    }
    return nil
}
```

## 5. Touched Areas

- files: `supabase/migrations/*_add_harness_artifacts.sql` (new if needed), `apps/local-runner/internal/agentpack/flow-pack/artifact_instances/*.yaml` or pack embedded JSON, `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` + `cp-harness.yaml` (bindings), `apps/local-runner/internal/runner/artifact_type_registry.go`, `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`, `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (labels)
- modules: artifact framework (type registry + instance mirror); flow-pack loader; Supabase workflow store; desktop WorkflowsSettings
- routes: none
- tables: `artifact_types` (read), `artifact_instances` (`is_builtin=true` rows), `step_artifact_bindings` (additive rows per workflow)

## 6. Acceptance Check

- `go test ./internal/runner -run TestArtifact` green; `TestHarnessArtifactBindings` passes for both harnesses (plan md OUTPUT required, reviewer INPUT required, splitter INPUT cp_md OUTPUT task_md); `TestHarnessArtifactBindingRejectsPathOutsideRequirements` green.
- Manual `task-harness` run: artifact panel shows `plan_md` output with link to `requirements/08-Task/todo/Task-xxx.md`; invalid path (outside `requirements/`) fails at binding validation, not as opaque provider error.
- Manual `cp-harness` run: artifact panel shows `cp_md` + `task_md × N` with `P-*` traceability; `go test ./internal/agentpack -run Pack` green.

## 7. Out of Scope

- Engine dual-loop fix — Task-304.
- Harness YAML topology — Task-305/306 (this task only adds bindings, not nodes/edges).
- New artifact type DSL — explicitly not in v1 (SD-23).

## 8. Completion Notes

- result: implemented (commit a5626cd7). LoadFlowFS now parses node artifactBindings (the FS loader silently dropped them before — only the Supabase mirror populated the field); ValidateFlowDefinition fails closed on malformed OUTPUT bindings and restricts file_artifact OUTPUT paths to requirements/; templated (pathTemplate) write contracts are prompt-level and excluded from the flowgate exact-path check; migration 20260831090000 seeds plan_md/cp_md/task_md instances.
- tests added: TestValidateArtifactOutputPath, TestValidateFlowDefinitionRejectsBadOutputBinding, TestTaskHarnessPlanArtifactBindings (agentpack); TestHarnessTemplatedPlanOutputPromptContract, TestHarnessTemplatedInputMentionForPlanReviewer (runner).
- follow-ups: artifact panel rendering on a live run (T-7 labels optional — skipped as data-driven via selectableIn); step_artifact_bindings seeding for the new instances beyond the Task-201 context path; Q-1/Q-2 pathTemplate-vs-declared_paths decision stays as recorded in the CP.
- upstream docs updated: `CP-58` `P-4`, `P-6`
