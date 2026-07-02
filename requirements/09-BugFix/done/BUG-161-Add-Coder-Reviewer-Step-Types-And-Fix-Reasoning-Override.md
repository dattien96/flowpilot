# BUG-161: Add Coder/Reviewer Step Types, And Fix Reasoning Override Forcing

## Metadata

- Document ID: `BUG-161`
- Title: `Add Coder/Reviewer Step Types, And Fix Reasoning Override Forcing`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-160-Workflow-Step-Model-Override-Forced-And-Step-Identity-Hidden.md`
- Child Documents: `none`
- Related Documents: `supabase/migrations/20260701090000_add_flow_engine_attrs_to_workflows.sql`
- Replaces: `none`
- Tags: `agent-flow-engine, desktop, workflow-steps, settings, supabase, migration`

## AI Quick View

### Summary

- Follow-up to `BUG-160`: the user asked for a real SQL migration to give the manual workflow builder ("Add step" dropdown) distinct Coder and Reviewer step types, instead of the single generic "Flow: Agent Delegate" catalog entry — the `nodeId`/`agentRef` display fix from `BUG-160` didn't add a way to *choose* a coder-vs-reviewer step at creation time, only to *see* the distinction after the fact.
- The user also reported still seeing a step's Reasoning forced to a concrete value after `BUG-160` — that fix only added a "no override" choice to the Model override select; the identical gap existed on the Reasoning select and in `addWorkflowStep`'s initial value, left unfixed.

### Current Ask

- Add distinct step types for Coder and Reviewer so a manually-built workflow can pick between them directly from "Add step," not just distinguish them after adding a generic step and manually filling in `nodeId`/`agentRef`.
- Stop forcing a concrete Reasoning value on a step — same "let it inherit unless I explicitly override" request as `BUG-160` made for Model.

### Key Decisions

- `F-1` New migration `20260702120000_split_flow_agent_delegate_coder_reviewer.sql` adds `flow-agent-delegate-coder` ("Flow: Coder") and `flow-agent-delegate-reviewer` ("Flow: Reviewer") step_definitions rows — both declaring the same runtime contract as `flow-agent-delegate` (`behaviorHandler behaviorAgentDelegate`), just purpose-named for the manual builder's dropdown.
- `F-2` Does **not** delete the original generic `flow-agent-delegate` row, despite the literal "del" in the request — deleting it would violate `workflow_steps.step_type`'s NOT NULL FK for every row still referencing it (built-in flow-pack mirrors like rag-harness's `implement` node, or any hand-built step this migration's reassignment can't confidently classify) and break flows unrelated to this change. Kept as the fallback/generic option instead.
- `F-3` Existing `workflow_steps` rows are best-effort reassigned by the migration itself: a row with `agent_ref ilike '%coder%'` moves to `flow-agent-delegate-coder`, `agent_ref ilike '%review%'` moves to `flow-agent-delegate-reviewer`; anything without a matching `agent_ref` is left on the generic row rather than guessed at.
- `F-4` `addWorkflowStep` (`WorkflowsSettings.tsx`) now defaults `reasoningEffortOverride` to `null` (previously `stepDefinition?.reasoningEffort ?? DEFAULT_REASONING`, forcing a concrete level exactly like the model bug `BUG-160` fixed).
- `F-5` The `Reasoning` `<select>` gains the same `No override (use run's reasoning)` empty option `BUG-160` added to `Model override`, and its displayed value now reflects `step.reasoningEffortOverride ?? ""` instead of always showing `DEFAULT_REASONING` as if it were an active selection.

### Constraints

- Only the per-STEP `reasoningEffortOverride` selects were touched. The per-WORKFLOW `reasoningEffort`/`reasoningEffortOverride` fields (the workflow's own base default, used when no step overrides anything) are a different, legitimate concept and were left untouched.
- The new step types are additive only — no existing workflow_steps row loses its data; ambiguous rows keep pointing at the generic `flow-agent-delegate` row.

### Open Questions

- None.

### Source Refs

- `supabase/migrations/20260702120000_split_flow_agent_delegate_coder_reviewer.sql`
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (`addWorkflowStep`, the `Reasoning` `<select>`)

## 1. Issue Summary

`BUG-160` fixed *display* of per-step identity and *clearing* of an unwanted model override, but didn't add a way to *choose* Coder vs Reviewer when adding a step, and left the identical "forced value, no way to opt out" bug on the Reasoning override that it had just fixed for Model.

## 2. Parent Links

- impacted coding plan: `none`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, `Settings > Workflows`.
- reproduction steps: open "Add step" — only one generic "Flow: Agent Delegate" option exists for any agent-delegating step; add a step and open it — Reasoning shows a concrete level (e.g. "Medium") with no way to select "none."
- frequency: deterministic.

## 4. Expected vs Actual

- expected: "Add step" offers distinct Coder/Reviewer options; Reasoning can be left unset.
- actual: only the generic option existed; Reasoning was always forced to a concrete value.

## 5. Impact

- users affected: anyone manually building a custom workflow from the generic flow step types.
- workflows affected: `Settings > Workflows` authoring UI.
- severity: low — same class as `BUG-160`, authoring-UX and catalog-completeness, not an execution-correctness bug.

## 6. Root Cause

- confirmed cause: (1) the CP-42 seed migration intentionally created only one generic dispatch-category row for `agent.delegate`, which is correct for built-in flow-pack mirrors but insufficient for the manual builder's step-type picker; (2) `addWorkflowStep`'s `reasoningEffortOverride` initializer and the Reasoning `<select>`'s missing empty option are the exact same pattern `BUG-160` fixed for `modelOverride`/the Model override select — just not applied to Reasoning in that pass.
- evidence: see Source Refs above; `BUG-160`'s own `F-1`/`F-2` for the parallel Model-side fix this mirrors.

## 7. Fix Strategy

- `F-1`..`F-5` as described in Key Decisions.

## 8. Validation

- `V-1` `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-2` Reviewed the new migration for FK safety: the two inserts are additive (`on conflict do nothing`), and the two updates only ever move rows *off* the generic row onto a more specific one that is guaranteed to already exist (inserted immediately above in the same migration) — no row can end up pointing at a step_type that doesn't exist.
- `V-3` Not executed: applying the migration against a live Supabase instance and confirming the new "Flow: Coder"/"Flow: Reviewer" options appear in the `Settings > Workflows` "Add step" dropdown, and that Reasoning can be cleared and persists as cleared — no local Supabase stack or running desktop app available in this environment (same limitation as `BUG-154`, `BUG-155`, `BUG-156`, `BUG-158`, `BUG-159`, `BUG-160`).

## 9. Regression Guard

- tests: none added — no existing test harness covers `WorkflowsSettings.tsx` or exercises Supabase migrations directly in this environment.
- alerts: none.
- audit checks: recorded in `change-audit/CA-198-add-coder-reviewer-step-types-and-reasoning-override-fix.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: the generic `flow-agent-delegate` row remains in the catalog on purpose (`F-2`) — not a leftover oversight.
