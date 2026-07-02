# BUG-164: Remove workflow_steps Model/Provider/Reasoning Overrides

## Metadata

- Document ID: `BUG-164`
- Title: `Remove workflow_steps Model/Provider/Reasoning Overrides`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md`, `requirements/09-BugFix/done/BUG-160-Workflow-Step-Model-Override-Forced-And-Step-Identity-Hidden.md`, `requirements/09-BugFix/done/BUG-163-Flow-Pack-Mirror-Sync-Stamps-Codex-Provider-Override.md`
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, supabase, migration, workflow-steps, settings`

## AI Quick View

### Summary

- Follow-up to `BUG-160`/`BUG-163`: after reviewing SS-05 §2.2 and SD-06 §6 (the actual documented model-resolution architecture), the user decided `workflow_steps` should not carry its own `provider_override`/`model_override`/`reasoning_effort_override` at all — it's a relationship table (workflow ↔ step type, with ordering), and a step's model/provider/reasoning should be entirely determined by its step type's own `step_definitions` catalog row.
- This removes the entire class of drift these three columns caused across `BUG-160`/`161`/`162`/`163`: forced defaults at step-creation time, no way to truly clear an override, a schema-level `'codex'` default silently winning over the real configured model, etc.
- Live investigation confirmed the current (non-legacy) desktop/local-runner path never relied on these columns for anything that would break by removing them: Go's `WorkflowStore` has no `workflow_runs`/`workflow_steps` insert capability at all (only reads/patches), and the desktop's own single-step chat flow keeps its model choice entirely in-memory on the run, never persisting it through `workflow_steps`. The hundreds of `workflow_steps` rows with per-row `model_override` values found in a live data export are residue from the admin-web path the user confirmed is legacy and unused.

### Current Ask

- Drop `provider_override`, `model_override`, `reasoning_effort_override` from `workflow_steps`.
- A step's model/provider is now always exactly what its step type (`step_definitions`) says — no per-workflow-instance override layer.
- Make sure the `Settings > Workflows` step editor still works correctly with these fields gone.

### Key Decisions

- `F-1` Migration drops the three columns from `workflow_steps` outright — `workflows.provider_override`/`model_override`/`reasoning_effort_override` (the workflow-level default, a different and legitimate concept) are untouched.
- `F-2` Go's `RuntimeWorkflowStep.Provider`/`.Model` are now unconditionally derived from `step_definitions.model` (via the already-joined `step_definitions` embed) plus `providerKeyFromModel` — no more per-step override field to read at all, so the field no longer needs a fallback branch, just a single assignment.
- `F-3` `LoadRunSteps`'s PostgREST select no longer requests `provider_override`/`model_override` from `workflow_steps` (columns don't exist).
- `F-4` The flow-pack mirror-sync (`insertSteps`) no longer needs the explicit `provider_override: nil` workaround from `BUG-163` — the column it was overriding a default on doesn't exist anymore.
- `F-5` `WorkflowStep` (`packages/flowpilot-client-core/src/domain/adminModels.ts`) drops `providerOverride`/`modelOverride`/`reasoningEffortOverride`; `supabaseAdminRepository.ts`'s `mapWorkflowStep`, `saveWorkflow`'s step insert, and `cloneWorkflow`'s step-copy insert all stop reading/writing them (the corresponding `Workflow`-level fields in the same file are untouched — different table, different columns, deliberately kept).
- `F-6` `WorkflowsSettings.tsx`: removed the per-step "Model override" and "Reasoning" `<select>` fields entirely (there is nothing to configure anymore — a step's model comes from its step type); `addWorkflowStep` no longer initializes those fields; the workflow-dirty-check snapshot (`normalizeWorkflowSnapshot`) no longer includes them for steps (still includes them for the workflow-level draft, unchanged).
- `F-7` Left `apps/admin-web`'s own (separate, non-shared) workflow-engine code untouched per the user's explicit statement that admin-web is legacy and unused — its queries referencing these columns will simply start failing against the new schema, which is accepted collateral, not silently worked around.

### Constraints

- Did not touch `workflows.provider_override`/`model_override`/`reasoning_effort_override` — the workflow-level default is a distinct, still-valid concept (SS-05/SD-06's "Workflow Level" tier) and was never in scope for removal.
- Did not implement the missing "Flow"-level (workflow/run) model resolution for the live desktop/Go path — that gap was investigated and reported to the user as a separate, larger feature; this fix is scoped to removing the `workflow_steps` override columns and the drift they caused, not to building out full Step > Flow > Project > default resolution end-to-end.

### Open Questions

- None for this fix. The broader "Flow/Project-level model resolution isn't implemented for the live desktop path" finding was surfaced to the user but explicitly deferred, not part of this fix's scope.

### Source Refs

- `supabase/migrations/20260702140000_drop_workflow_steps_overrides.sql`
- `apps/local-runner/internal/runner/workflow_state_machine.go` (`RuntimeWorkflowStep`)
- `apps/local-runner/internal/runner/supabase_workflow_store.go` (`LoadRunSteps`)
- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go` (`insertSteps`)
- `apps/local-runner/internal/runner/phase5_test.go`
- `packages/flowpilot-client-core/src/domain/adminModels.ts` (`WorkflowStep`)
- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` (`mapWorkflowStep`, `saveWorkflow`, `cloneWorkflow`)
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
- `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md` §2.2, §3
- `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md` §6

## 1. Issue Summary

`workflow_steps.provider_override`/`model_override`/`reasoning_effort_override` caused repeated, hard-to-diagnose drift (`BUG-160` through `BUG-163`: forced defaults, no true "no override" state, a schema default silently overriding the real model). After reviewing the actual documented resolution architecture (SS-05/SD-06) and confirming the current live system doesn't depend on these columns for anything load-bearing, the decision was made to remove them entirely rather than keep patching around them.

## 2. Parent Links

- impacted coding plan: `none`
- impacted tech design: `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md` (§6.1 lists `workflow_steps.provider_override`/`model_override`/`reasoning_effort_override` as a persistence location — this fix removes that location; the doc itself is not amended here, flagged as a follow-up)
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md` (§2.2/§3 describe a "Step Level (Granular override)" tier this fix removes the storage for — also flagged as a follow-up, not amended here)

## 3. Environment and Reproduction

- environment: desktop-flowpilot, local-runner, Supabase.
- reproduction steps: n/a — this is a deliberate architectural simplification requested by the user after live investigation, not a defect reproduction.
- frequency: n/a

## 4. Expected vs Actual

- expected (post-fix): `workflow_steps` has no override columns; a step's model/provider is always its step type's catalog value; `Settings > Workflows` step editor has no per-step Model/Reasoning fields.
- actual (pre-fix): `workflow_steps` carried its own override columns that repeatedly drifted from the intended value across multiple prior bugfixes.

## 5. Impact

- users affected: anyone authoring or running workflows through `Settings > Workflows` / Flow Mode.
- workflows affected: workflow step authoring UI, Flow Mode runtime sidebar's model/provider display.
- severity: low — this is a simplification/cleanup, not a regression fix; verified the live system doesn't depend on the removed columns for anything currently working.

## 6. Root Cause

- confirmed cause: n/a — not a defect. The removal directly addresses the *pattern* of causes behind `BUG-160`/`161`/`162`/`163` (a per-step override layer whose column defaults and empty-state handling kept drifting from intent) by eliminating that layer entirely.
- evidence: SS-05 §2.2/§3 and SD-06 §6 document Project → Workflow → Step resolution tiers, but the "Step" tier they describe is the workflow *template's* explicit choice — which, per this fix, now lives entirely on the step type's catalog row rather than a separate per-instance column that needed its own resolution logic.

## 7. Fix Strategy

- `F-1`..`F-7` as described in Key Decisions.

## 8. Validation

- `V-1` `go build ./...` in `apps/local-runner` — passes.
- `V-2` `go test ./internal/runner/... -run 'TestSupabaseStoreLoadRunSteps|TestWorkflowStepsRuntime|TestSupabaseWorkflowFlowStore'` — passes; `TestSupabaseStoreLoadRunStepsShaping` updated to drop `provider_override`/`model_override` from the mocked row and select-string assertion, and to assert Model/Provider are unconditionally derived from `step_definitions.model`. The now-redundant `TestSupabaseStoreLoadRunStepsFallsBackToStepDefinitionModel` (which tested the old conditional fallback) was folded into the main shaping test since the fallback is no longer conditional.
- `V-3` `npm run typecheck` in `apps/desktop-flowpilot` — clean; confirms `packages/flowpilot-client-core`'s `WorkflowStep` field removal didn't leave any dangling reference in desktop-flowpilot.
- `V-4` Reviewed `apps/admin-web`'s own workflow-engine code and confirmed it is a *separate*, non-shared implementation (not affected by the `adminModels.ts`/`supabaseAdminRepository.ts` changes) — left untouched per the user's explicit "legacy, unused" statement; its raw column references to `workflow_steps.provider_override`/`model_override`/`reasoning_effort_override` will fail against the new schema, which is accepted collateral for this fix, not silently patched around.
- `V-5` Not executed: applying the migration against a live Supabase instance and manually re-verifying the `Settings > Workflows` step editor renders/saves correctly with the fields removed — no local Supabase stack or running desktop app available in this environment (same limitation noted in every prior fix this session).

## 9. Regression Guard

- tests: `TestSupabaseStoreLoadRunStepsShaping` (updated) locks in the unconditional catalog-derived Model/Provider.
- alerts: none.
- audit checks: recorded in `change-audit/CA-201-remove-workflow-steps-overrides.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: `SS-05-Workflow-Ai-Provider.md` §2.2/§3 and `SD-06-AI-Provider-Integration.md` §6.1 both still describe a per-step "Step Level (Granular override)" storage tier on `workflow_steps` that no longer exists — these should be updated to reflect that step-level configuration now lives entirely on `step_definitions`, not flagged as done in this fix (deliberately out of scope; a documentation-only follow-up).
- notes left unchanged on purpose: the missing workflow-level/project-level model resolution for the live desktop/Go path (discovered during this investigation) is a real gap but was explicitly deferred by the user as separate, larger follow-up work, not part of this fix.
