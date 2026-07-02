# BUG-154: step_definitions Supported-Model Check Blocks Registry Models

## Metadata

- Document ID: `BUG-154`
- Title: `step_definitions Supported-Model Check Blocks Registry Models`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/08-Task/done/Task-013-Dynamic-Way-To-Add-Support-Model.md`, `requirements/08-Task/done/Task-054-AI-Provider-Supported-Models-Edit-Delete.md`
- Child Documents: `none`
- Related Documents: `change-audit/CA-190-drop-step-definitions-supported-model-check.md`, `change-audit/CA-139-antigravity-cli-provider-detection.md`, `change-audit/CA-036-update-supported-models-constraint.md`
- Replaces: `none`
- Tags: `supabase, migration, ai-supported-models, step-definitions, workflow-steps, regression`

## AI Quick View

### Summary

- Saving a workflow step definition fails with `new row for relation "step_definitions" violates check constraint "step_definitions_supported_model_check"`.
- `step_definitions.model` is guarded by a Postgres `CHECK` constraint that hard-codes a fixed allow-list of model ids, last rewritten by the `20260628152000_update_antigravity_gemini_models.sql` migration.
- Task-013 added a dynamic `ai_supported_models` registry so admins can add a new model from `Settings > AI Providers` without a code/SQL change, and `workflow-steps/create.tsx` / `$stepType.tsx` already populate their model dropdown from that registry via `useSupportedModels()`.
- Any model added through the registry (manually or via "Check and import missing models") that is not one of the ~23 literal strings baked into the constraint can be selected in the UI, but `saveStepDefinition` passes `step.model` straight through to the `step_definitions` upsert — so saving the step fails at the database layer even though the app itself considers the model valid.
- Task-013 explicitly flagged this exact failure mode as a risk ("If only the UI is added and runtime validation stays static, new models will save but fail at execution time") and listed removing/relaxing this constraint as its deferred "Migration C"; that step was never implemented.

### Current Ask

- Stop the database from rejecting step definitions whose model is a legitimately registered (but not originally seeded) `ai_supported_models` row.

### Key Decisions

- `F-1` Implement Task-013's Migration C: drop `step_definitions_supported_model_check` outright rather than re-appending yet another hard-coded string list (the pattern that caused this bug three times already, per `CA-036` and the `20260628152000` migration).
- `F-2` No application-code change is required: `workflow-steps` forms already source their dropdown from `ai_supported_models` (`useSupportedModels()`), and `normalizeStepModel`/legacy alias handling stays intact for older saved values. The registry is the intended source of truth going forward, matching Task-013's "Recommended phase-1 rule".

### Constraints

- Do not touch `ai_supported_models`, its RLS policies, or the provider-detection/import flows — those are out of scope for this bug.
- Do not re-introduce a static allow-list; that is the root cause, not a missing entry.
- Migration is additive/destructive-of-constraint-only: it does not touch existing row data, unlike the earlier `20260628152000` alias-normalization migration.

### Open Questions

- None. Task-013 already scoped and recommended this exact remediation ("Migration C").

### Source Refs

- `apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.ts:556-578` (`saveStepDefinition` — `model: step.model` passed through unnormalized)
- `apps/admin-web/src/routes/_authenticated/workflow-steps/create.tsx:39-45` (`useSupportedModels()` builds the dropdown from the registry)
- `requirements/08-Task/done/Task-013-Dynamic-Way-To-Add-Support-Model.md` — "Migration C" and the "Risks" section
- `supabase/migrations/20260628152000_update_antigravity_gemini_models.sql` — most recent hard-coded rewrite of the constraint

## 1. Issue Summary

Creating or editing a workflow step definition with a model that exists in the `ai_supported_models` registry but was not one of the literal strings baked into the `step_definitions_supported_model_check` constraint fails with:

```
new row for relation "step_definitions" violates check constraint "step_definitions_supported_model_check"
```

The step appears selectable and valid in the admin-web UI (the model dropdown is registry-backed) but the save fails at the database layer.

## 2. Parent Links

- impacted coding plan: `none` (task-level, not coding-plan-level)
- impacted tech design: `none`
- impacted system spec: `none`
- specific upstream ids: `Task-013`, `Task-054`

## 3. Environment and Reproduction

- environment: admin-web, Supabase Postgres (any environment with the `step_definitions_supported_model_check` constraint from `20260628152000_update_antigravity_gemini_models.sql` applied)
- reproduction steps:
  1. Go to `Settings > AI Providers`, add a new supported model for any provider (manual `Add model`, or via `Check and import missing models`) using a model id that is not already in the constraint's allow-list.
  2. Go to `Workflow Steps > Create` (or edit an existing step type), select the newly added model from the model dropdown.
  3. Save the step definition.
- frequency: deterministic — occurs every time a step is saved with a model id outside the hard-coded list, which happens by design any time an admin registers a model through Task-013's dynamic-model UI.

## 4. Expected vs Actual

- expected: Any model present and enabled in `ai_supported_models` can be assigned to a step definition and saved successfully, per Task-013's acceptance criteria ("Workflow/project/step save flows accept the new model").
- actual: The save request throws a Postgres check-constraint violation and the step definition is not persisted.

## 5. Root Cause

- hypothesis: a stale/incomplete allow-list on the `model` column.
- confirmed cause: `step_definitions.model` is protected by a `CHECK (model in (...))` constraint containing a fixed set of ~23 model id strings (seeded by `20260528154900_update_supported_models_constraint.sql`, most recently rewritten by `20260628152000_update_antigravity_gemini_models.sql`). Task-013 introduced the `ai_supported_models` table as the dynamic, admin-editable source of truth for which models are valid, and wired every model dropdown (`useSupportedModels()`) plus `workflow-start-runtime.ts` runtime validation to read from that table instead of the static `STEP_MODEL_OPTIONS` constant. However, the `step_definitions` table itself was never migrated off the static constraint — Task-013's own plan calls this out by name as "Migration C" and lists it as not completed. `saveStepDefinition` (`supabase-workflow-engine-gateway.ts:556-578`) writes `step.model` to the row unmodified, so any registry model that post-dates the constraint's last hand-written string list is rejected by Postgres even though the app UI and runtime validation both accept it.
- evidence:
  - `supabase/migrations/20260628152000_update_antigravity_gemini_models.sql:59-90` — the literal, closed allow-list.
  - `apps/admin-web/src/routes/_authenticated/workflow-steps/create.tsx:39-45` — dropdown is built from `useSupportedModels()` (registry), not the closed list.
  - `apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.ts:570` — `model: step.model` passed straight to the upsert with no allow-list check.
  - `requirements/08-Task/done/Task-013-Dynamic-Way-To-Add-Support-Model.md` — "Migration C: stop relying on hard-coded `step_definitions_supported_model_check`... " listed under "Not completed" / "Partially completed" phases, and the exact failure mode described under "Risks".

## 6. Fix Strategy

- `F-1` Add `supabase/migrations/20260702110000_drop_step_definitions_supported_model_check.sql`, which drops `step_definitions_supported_model_check` from `public.step_definitions` and does not replace it with another hard-coded list, matching Task-013's Migration C recommendation and its Phase-1 rule ("Remove the rigid SQL list constraint that requires migrations for every new model").
- `F-2` No other schema or app change: model validity is enforced by the app (dropdowns sourced from `ai_supported_models`, `workflow-start-runtime.ts` runtime checks, legacy alias normalization in `normalizeStepModel`), which is the state Task-013 already moved the rest of the system into.

## 7. Validation

- `V-1` Reviewed all call sites that write `step_definitions.model` (`saveStepDefinition` in `supabase-workflow-engine-gateway.ts`) and confirmed no code path depends on the constraint rejecting a value (no test asserts on the constraint name or expects a rejection).
- `V-2` Confirmed no other table (`projects.default_model`, `workflows.model_override`, `workflow_steps.model_override`) carries an equivalent `CHECK` constraint, so this fix is scoped to the single table actually affected.
- `V-3` Not executed: applying the migration against a live/local Supabase instance and re-running the manual repro steps in section 3, because no local Supabase stack (`supabase` CLI) is available in this environment. This should be verified against a real database (local `supabase db reset` or a staging apply) before this migration ships.

## 8. Regression Guard

- tests: no existing automated test exercises the Postgres constraint directly (it can only be observed against a live database); none were added for the same reason `V-3` above could not be executed.
- alerts: none.
- audit checks: `change-audit/CA-036-update-supported-models-constraint.md` and CA-139 document the constraint's history; this fix should be recorded as a new change-audit entry so future model additions do not attempt to "fix" this by editing the constraint again.

## 9. Follow-Up Document Updates

- upstream docs that must change: `requirements/08-Task/done/Task-013-Dynamic-Way-To-Add-Support-Model.md` should have its "Migration C" line and Definition-of-Done checklist marked complete once this migration ships and is verified against a live database.
- notes left unchanged on purpose: Task-013's Phase 5/6 partial-completion notes (dynamic selectors, runtime validation) are unaffected by this fix and remain accurate as "partial".
