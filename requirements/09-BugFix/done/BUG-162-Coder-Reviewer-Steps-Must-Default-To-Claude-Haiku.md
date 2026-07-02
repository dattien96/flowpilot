# BUG-162: Coder/Reviewer Steps Must Default To Claude Haiku

## Metadata

- Document ID: `BUG-162`
- Title: `Coder/Reviewer Steps Must Default To Claude Haiku`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-161-Add-Coder-Reviewer-Step-Types-And-Fix-Reasoning-Override.md`, `requirements/09-BugFix/done/BUG-160-Workflow-Step-Model-Override-Forced-And-Step-Identity-Hidden.md`
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, supabase, migration, step-definitions, workflow-steps`

## AI Quick View

### Summary

- Live-testing `BUG-161`: the new "1. coder" step (`flow-agent-delegate-coder`) still showed `Model override: GPT 5.4` / `Reasoning: Medium` after the fix — because those are *pre-existing, already-persisted* values on that specific `workflow_steps` row from before `BUG-160` fixed `addWorkflowStep`, not something the fix retroactively touches.
- The user clarified the actual expectation: the Coder step *type's own catalog default* (`step_definitions.model`) should be Claude Haiku, and a step added to a flow should end up running on Haiku unless the user explicitly overrides it — not on whatever the `model` column's schema default (`gpt-5.5`, a Codex model) happens to be.
- `BUG-161`'s migration deliberately left `model` unset on the two new `flow-agent-delegate-coder`/`flow-agent-delegate-reviewer` rows (to avoid guessing a content decision); this is that decision, now made explicit by the user.

### Current Ask

- Set the Coder/Reviewer step types' own default model to Claude Haiku.
- Clear the leftover stale `GPT 5.4`/`Medium` overrides on already-existing steps so they actually resolve to that Haiku default via `BUG-160`'s fallback chain (step's own override → step type's `step_definitions.model` → run's model).

### Key Decisions

- `F-1` New migration sets `step_definitions.model = 'claude-haiku'` for both `flow-agent-delegate-coder` and `flow-agent-delegate-reviewer`.
- `F-2` Same migration clears `workflow_steps.model_override`/`reasoning_effort_override` back to `null` wherever a row's step type is one of the CP-42 generic flow-engine types *and* the value still exactly matches the old forced-default (`model_override = 'gpt-5.4'`, `reasoning_effort_override = 'medium'`) — narrow enough that a deliberately-different override elsewhere is never touched, but wide enough to actually fix every row the now-fixed `addWorkflowStep` bug stamped before it was fixed.
- `F-3` No application code change needed: `BUG-160`'s `LoadRunSteps` fallback chain already reads `step_definitions.model` when `model_override` is null, and derives `Provider` from the resolved model — clearing the stale override plus setting the catalog default is sufficient for the sidebar to show Claude/claude-haiku end to end.

### Constraints

- The clear-override update is intentionally scoped by both step type *and* exact stale value — it must never touch a step whose override was deliberately set to something other than the old bug's forced default.

### Open Questions

- None.

### Source Refs

- `supabase/migrations/20260702130000_set_coder_reviewer_model_and_clear_stale_overrides.sql`
- `apps/local-runner/internal/runner/supabase_workflow_store.go` (`BUG-160`'s fallback chain — unchanged, just now fed a real Haiku default)

## 1. Issue Summary

After `BUG-161`, the existing "1. coder" step in the user's test workflow still displayed a Codex model/medium reasoning override, and the user clarified this should resolve to Claude Haiku end to end — both for this existing step (once its stale override is cleared) and for any step added going forward (via the step type's own catalog default).

## 2. Parent Links

- impacted coding plan: `none`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, `Settings > Workflows`, the "Analytics Review" workflow's "1. coder" step.
- reproduction steps: open the step editor; `Model override` shows `GPT 5.4`, `Reasoning` shows `Medium`, despite `BUG-161` adding a "No override" choice and a distinct Coder step type.
- frequency: deterministic for any step created before `BUG-160` landed.

## 4. Expected vs Actual

- expected: the step (and its step type's own default) resolve to Claude Haiku.
- actual: stale Codex/medium override values persisted from before the fix, and the new step type's catalog default was left unset (schema default, a GPT model).

## 5. Impact

- users affected: anyone with workflow steps created before `BUG-160`, or relying on the Coder/Reviewer step types' catalog default.
- workflows affected: `Settings > Workflows`, Flow Mode runtime sidebar display.
- severity: low — data/config correction, no execution-correctness bug.

## 6. Root Cause

- confirmed cause: (1) `BUG-161`'s migration inserted the new step types without an explicit `model`, taking the `gpt-5.5` schema default instead of the intended Claude Haiku; (2) `BUG-160`'s `addWorkflowStep` fix only changes behavior for steps created *after* the fix — it cannot retroactively change already-persisted `model_override`/`reasoning_effort_override` values on existing rows.
- evidence: user-provided screenshot of the step editor; `BUG-161`'s own `F-1` note that `model` was deliberately left unset pending this decision.

## 7. Fix Strategy

- `F-1`..`F-3` as described in Key Decisions.

## 8. Validation

- `V-1` Reviewed the migration for scope safety: both updates require an exact match on the specific stale value *and* a step type in the fixed CP-42 generic list — cannot affect a step whose override differs from the old bug's forced value.
- `V-2` Not executed: applying the migration against a live Supabase instance and confirming the "1. coder" step now shows "No override" with the sidebar resolving to Claude Haiku — no local Supabase stack available in this environment (same limitation as every prior fix this session).

## 9. Regression Guard

- tests: none added — pure data migration, no application code changed.
- alerts: none.
- audit checks: recorded in `change-audit/CA-199-set-coder-reviewer-haiku-default-and-clear-stale-overrides.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: none.
