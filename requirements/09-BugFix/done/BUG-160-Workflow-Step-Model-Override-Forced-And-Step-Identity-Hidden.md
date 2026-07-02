# BUG-160: Workflow Step Model Override Forced, And Step Identity Hidden In Settings

## Metadata

- Document ID: `BUG-160`
- Title: `Workflow Step Model Override Forced, And Step Identity Hidden In Settings`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-155-Flow-Mode-Sidebar-Shows-Generic-Step-Label-Not-Node-Identity.md`, `requirements/09-BugFix/done/BUG-158-Flow-Timeline-Missing-Step-Name-Model-Agent-Flow-Yolo.md`
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, desktop, workflow-steps, workflow-steps-runtime, settings`

## AI Quick View

### Summary

- Following up on `BUG-158`'s live testing, the user found the actual root cause of the wrong model badge: the manually-built "Analytics Review" workflow's steps in `Settings > Workflows` all carry a `model_override` of `GPT 5.4` (Codex) — not because anyone intentionally chose Codex, but because `addWorkflowStep` (`WorkflowsSettings.tsx`) forced a concrete `modelOverride` onto every newly-added step, and the `Model override` `<select>` never offered an empty/"no override" choice to undo it.
- Once no override exists, the runtime had no further fallback: `RuntimeWorkflowStep.Model` stayed empty rather than reading what the step *type* is actually configured to run on (`step_definitions.model`), so a "no override" step just inherited the run's own model — which, for a Codex-started run, still shows Codex even though the user expects Claude Haiku.
- Separately, the same `Settings > Workflows` step list the user was looking at (image 3) has the identical `BUG-155` display bug: every `flow-agent-delegate` step shows the identical generic "Flow: Agent Delegate" title, with no way to tell a coder-flavored step from a reviewer-flavored one — `BUG-155`'s node-identity fix only reached the desktop *runtime* sidebar, not this *settings/editor* step list.

### Current Ask

- Let a step be saved with no model override at all (true inheritance), not just "one of several forced concrete models."
- A step with no override should show what its step type is actually configured to run on, not just the run's model.
- Show the real per-step identity in the `Settings > Workflows` step list, same as the runtime sidebar already does.

### Key Decisions

- `F-1` `addWorkflowStep` now defaults a new step's `modelOverride` to `null` instead of forcing `stepDefinition?.model ?? modelOptions[0]?.value ?? DEFAULT_MODEL`.
- `F-2` The `Model override` `<select>` gains an explicit `<option value="">No override (use run's model)</option>` at the top, so an existing forced override can actually be cleared back to "no override" — previously every option was a concrete model, so opening the select could switch which model but never remove the override entirely.
- `F-3` `LoadRunSteps` (`supabase_workflow_store.go`) now resolves a 3-level fallback: the step's own `model_override` → the step type's `step_definitions.model` → (unchanged) the run's own model, applied client-side in `FlowStepTimeline`/`FlowTimelineSidebar`. `Provider` is derived from whichever model wins via `providerKeyFromModel` when `provider_override` is also unset, so the provider badge matches the resolved model instead of defaulting to the run's provider.
- `F-4` The `Settings > Workflows` step list title now prefers `step.nodeId || step.agentRef` over the shared `step_definitions.name`, mirroring `BUG-155`'s fix for the runtime sidebar — a `nodeId`/`agentRef` field already exists on the step form (this was a display-only gap, not a missing data model).
- `F-5` Did **not** add new distinct `step_definitions` catalog rows for "Coder" vs "Reviewer" (the literal "need 2 separated steps" request) — `nodeId`/`agentRef` already let a user distinguish two `flow-agent-delegate` steps from each other without a schema change; `F-4` just makes that existing distinction visible in the list, which is the actually-missing piece.
- `F-6` Did **not** change `step_definitions.model`'s actual value for `flow-agent-delegate`/`flow-hub-inline` (currently the column's schema default, `gpt-5.5`) — whether that catalog default should be Claude Haiku is a content decision for whoever owns the step-definitions catalog (`Settings > AI Providers` / a future step-definitions editor), not something to guess and silently rewrite as part of this fix.

### Constraints

- `F-3`'s fallback only changes what is *displayed* and only when no override exists — it does not change which model a step actually executes on; that execution-time model resolution is a separate concern outside this fix's scope.
- The `Settings > Workflows` step title fallback (`F-4`) must not break for classic (non-flow-engine) steps, which have no `nodeId`/`agentRef` — the fallback chain ends at the existing `step_definitions.name || step.stepType` behavior for those.

### Open Questions

- Should `step_definitions.model` for the generic `flow-*` dispatch categories be changed away from the `gpt-5.5` schema default? Left to the catalog owner — flagged, not decided, in `F-6`.

### Source Refs

- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (`addWorkflowStep`, the `Model override` `<select>`, the step list title)
- `apps/local-runner/internal/runner/supabase_workflow_store.go` (`LoadRunSteps` fallback chain)
- `supabase/migrations/20260522093000_add_step_prompt_base_and_model_defaults.sql:13` (`alter column model set default 'gpt-5.5'` — the actual source of the "gpt-5.5"/Codex default for any step type whose `model` was never explicitly set, including the CP-42 `flow-*` seed rows)

## 1. Issue Summary

A manually-built workflow's steps all showed a Codex provider/model badge in the runtime sidebar that the user did not expect (expected Claude Haiku), traced to a forced `model_override` at step-creation time with no way to clear it, and no fallback to the step type's own configured default once cleared. The same settings screen also still shows the generic `BUG-155` step label instead of the step's real identity.

## 2. Parent Links

- impacted coding plan: `none`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, `Settings > Workflows`, a manually-built workflow ("Analytics Review") composed of `flow-agent-delegate`/`flow-hub-inline` steps.
- reproduction steps:
  1. Add a step via `Settings > Workflows > Add Step`.
  2. Open it: `Model override` shows a concrete model (e.g. `GPT 5.4`) with no way to select "none."
  3. Start a Flow Mode run using that workflow; the sidebar shows the run's Codex badge on every step, never the step's own intended model.
  4. In the same step list, every step reads "N. Flow: Agent Delegate" regardless of what agent it actually delegates to.
- frequency: deterministic for any manually-built workflow using the generic `flow-*` step types.

## 4. Expected vs Actual

- expected: a step can have no override at all; a no-override step shows its step type's own configured model; the step list shows which node/agent each step actually is.
- actual: every step gets a forced override at creation with no way to clear it; a no-override step (if it existed) would show only the run's model; the step list shows an identical generic label for every step of the same type.

## 5. Impact

- users affected: anyone authoring a custom workflow from the generic flow step types.
- workflows affected: `Settings > Workflows` authoring UI and the Flow Mode runtime sidebar's display of it.
- severity: low-medium — no execution correctness bug, but the model/identity shown was actively misleading, and there was no UI path to actually fix the misconfigured data once it existed.

## 6. Root Cause

- confirmed cause: three independent gaps compounding:
  1. `addWorkflowStep` (`WorkflowsSettings.tsx:462`, pre-fix) forced `modelOverride` to a concrete value (`stepDefinition?.model ?? modelOptions[0]?.value ?? DEFAULT_MODEL`) for every new step — for the CP-42 generic `flow-*` step types, `stepDefinition.model` resolves to the `step_definitions.model` column's schema default (`gpt-5.5`, per `20260522093000_add_step_prompt_base_and_model_defaults.sql:13`), a Codex model — explaining the Codex badge.
  2. The `Model override` `<select>` had no empty-value option, so even after discovering the unwanted override, the user had no way to select "none" and clear it.
  3. `LoadRunSteps`'s fallback chain stopped at the run's own model when no per-step override existed — it never consulted `step_definitions.model`, so even a correctly-cleared override wouldn't have shown the step type's real configured default.
  4. Separately, the settings step list title (`WorkflowsSettings.tsx`, pre-fix) used only `step_definitions.name || step.stepType`, the exact same generic-label bug `BUG-155` fixed for the runtime sidebar, just never applied to this screen.
- evidence: see Source Refs above.

## 7. Fix Strategy

- `F-1`..`F-4` as described in Key Decisions.

## 8. Validation

- `V-1` `go build ./...` in `apps/local-runner` — passes.
- `V-2` `go test ./internal/runner/... -run 'TestSupabaseStoreLoadRunSteps|TestWorkflowStepsRuntime'` — passes, including a new `TestSupabaseStoreLoadRunStepsFallsBackToStepDefinitionModel` asserting the model falls back to `step_definitions.model` and the provider is derived from it when both overrides are unset.
- `V-3` `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-4` Not executed: a live re-check in the running desktop app (add a step, confirm "No override" is selectable and persists as cleared, confirm the runtime sidebar then shows the step type's configured model, confirm the settings step list shows real node/agent identity) — no backend/Supabase instance available in this environment, same limitation as prior Flow Mode sidebar fixes this session.

## 9. Regression Guard

- tests: `TestSupabaseStoreLoadRunStepsFallsBackToStepDefinitionModel` locks in the 3-level fallback.
- alerts: none.
- audit checks: recorded in `change-audit/CA-197-workflow-step-model-override-and-identity.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: `step_definitions.model`'s actual default value for the generic `flow-*` step types (`F-6`) — a content decision, not addressed here.
