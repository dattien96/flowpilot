# Task-307: Harness Plan Artifact Types And Panel Parity

## Metadata

- Document ID: `Task-307`
- Title: `Harness Plan Artifact Types And Panel Parity`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-27`
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

- result:
- follow-ups:
- upstream docs updated: `CP-58` `P-4`, `P-6`
