# BUG-230: Navigator Catalog Mapping Drops Model And YoloMode

## Metadata

- Document ID: `BUG-230`
- Title: `Navigator Catalog Mapping Drops Model And YoloMode`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [Task-183: User-Owned Model/Provider In Flow Mode](../../08-Task/todo/Task-183-User-Owned-Model-Provider-Resolution-Across-Chat-And-Flow.md), [BUG-227: Flow Mode Main Card Shows Pre-Run Catalog Model Instead Of Resolved Model](./BUG-227-Flow-Mode-Main-Card-Shows-Pre-Run-Catalog-Model-Instead-Of-Resolved-Model.md)
- Child Documents: `none`
- Related Documents: [BUG-229: Single-Step Launch Falls Back To Project Model](./BUG-229-Single-Step-Launch-Falls-Back-To-Project-Model.md), [BUG-228: Step-Level Model Override Not Re-Resolved Mid-Flow](./BUG-228-Step-Level-Model-Not-Reresolved-Mid-Flow.md)
- Replaces: `none`
- Tags: `agent-flow-engine, desktop, navigator, model-resolution, mapping-bug`

## AI Quick View

### Summary

- After `BUG-227`'s fix (main card prefers `workflowStepRuntimeMeta` over the pre-run catalog preview), the user still saw the `main` card show `CODEX gpt-5.4-mini` before a run started, for the built-in "Review Loop" workflow whose Settings page clearly shows `Model override: Claude Haiku`.
- Traced to a deeper, more fundamental bug than `BUG-227`: `mapNavigatorWorkflow`/`mapNavigatorStep` (`navigatorCatalog.ts`), which convert the admin/client-core `Workflow`/`StepDefinition` shape into the desktop's lightweight `Workflow`/`Step` contract type, never copied `modelOverride`/`yoloMode` (workflow) or `model`/`yoloMode` (step) at all — every mapped workflow/step's `.model`/`.yoloMode` was always `undefined`/`false`, for every workflow and every project, regardless of what was actually configured.
- This meant the desktop's pre-run preview (`AgentsPanel`'s `resolvedModel = selectedWorkflow?.model || project?.model`, `FlowTimelineSidebar`) could never see a workflow's or step's own model/yoloMode — it always fell straight through to the project's default model. `BUG-227`'s fix only affected the *post-run* priority order (once `workflowStepRuntimeMeta` is populated from an active run); it did not help the state shown before a run starts, which is exactly what this screenshot shows.

### Current Ask

- `mapNavigatorWorkflow`/`mapNavigatorStep` must carry `model`/`yoloMode` through from the source admin data so the pre-run preview reflects each workflow's/step's actual configured override.

### Key Decisions

- `F-1` `mapNavigatorWorkflow` now maps `model: workflow.modelOverride ?? undefined` and `yoloMode: workflow.yoloMode`.
- `F-2` `mapNavigatorStep` now maps `model: definition.model || undefined` and `yoloMode: definition.yoloMode`.
- `F-3` Added direct unit tests asserting a non-null `modelOverride`/`model` and `yoloMode` actually survive the mapping — the exact case the pre-existing tests never covered (both existing tests only used `null`/default values, which happened to still "pass" with the fields silently dropped).

### Constraints

- Do not change `AdminWorkflow`/`AdminStepDefinition` (`@flowpilot/client-core`) — they already carry the correct data; this was purely a mapping-layer drop.
- Do not touch `BUG-227`'s priority-order fix — it remains correct and necessary for the post-run state; this fix addresses the separate pre-run preview data source.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/app/navigatorCatalog.ts` (`mapNavigatorWorkflow`, `mapNavigatorStep`)
- `packages/flowpilot-client-core/src/domain/adminModels.ts:102-113` (`Workflow.modelOverride`/`yoloMode`), `:183-200` (`StepDefinition.model`/`yoloMode`)
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx:51-60` (`resolvedModel` — the pre-run preview consumer)
- `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx` (another consumer of the same mapped `Step`/`Workflow` data)
- `tests/phase1/navigatorCatalog.test.ts`

## 1. Issue Summary

Every workflow's and step type's own configured model/YOLO override was silently discarded when the desktop mapped admin catalog data into its navigator state, so the pre-run preview shown in Flow Mode (before a run starts) always displayed the project's default model instead of the workflow's/step's own configuration — for every workflow, every step, in every project.

## 2. Parent Links

- impacted coding plan: `CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor`
- impacted tech design: `none` (mapping-layer bug; no resolution-rule change — `SS-05`/`SD-06` already correctly describe the intended behavior this bug prevented from being displayed)
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, Flow Mode, any workflow or step with a non-empty `model_override`/`step_definitions.model` different from the project's own default model.
- reproduction steps: open Settings › Workflows, confirm a workflow (e.g. built-in "Review Loop") shows a `Model override` (e.g. Claude Haiku); switch to Flow Mode, select that same workflow in the picker, and observe the `main` card / step-timeline sidebar before starting a run.
- frequency: deterministic — every workflow/step's pre-run preview was affected, always, regardless of which one was selected.

## 4. Expected vs Actual

- expected: the pre-run preview shows the selected workflow's/step's own configured model, falling back to the project default only when the workflow/step itself has none.
- actual (pre-fix): the pre-run preview always showed the project's default model, as if every workflow/step had no model of its own configured — even when Settings clearly showed otherwise.

## 5. Impact

- users affected: everyone using Flow Mode with a workflow/step model different from the project default — i.e. the exact scenario Task-183 was built to support (user-owned per-workflow model overrides).
- workflows affected: the desktop pre-run preview only (`AgentsPanel`, `FlowTimelineSidebar`); actual run execution was unaffected — the Go runner independently re-resolves the real model at run start (`BUG-165`/`BUG-229`) from the same underlying database columns, not from this desktop-side mapped copy.
- severity: medium — highly visible, confusing UI (looks like the whole model-override feature is broken) even though execution itself was correct.

## 6. Root Cause

- confirmed cause: `mapNavigatorWorkflow` and `mapNavigatorStep` in `navigatorCatalog.ts` never included `model`/`yoloMode` fields in their return object literals, despite the source `AdminWorkflow`/`AdminStepDefinition` types already carrying `modelOverride`/`model` and `yoloMode`.
- evidence: see Source Refs; confirmed both mapping functions' full source and the upstream type definitions directly.

## 7. Fix Strategy

- `F-1`..`F-3` as described in Key Decisions.

## 8. Validation

- `V-1` New unit tests in `tests/phase1/navigatorCatalog.test.ts` (`mapNavigatorWorkflow carries modelOverride and yoloMode through...`, `mapNavigatorStep carries model and yoloMode through...`) — pass. Updated the two pre-existing tests' expectations to include the now-mapped fields.
- `V-2` `npx tsx --test` across all `apps/desktop-flowpilot` unit test files + `tests/phase1/navigatorCatalog.test.ts` — 111 passed, 2 failed (same pre-existing, environment-specific failures as the established baseline — `localStorage` unavailable in the Node test runner, and one timing-sensitive test — confirmed unrelated via prior `git stash` baseline check).
- `V-3` `npm run typecheck` and `npm run build` in `apps/desktop-flowpilot` — both clean/pass.
- `V-4` Noted but not caused by this fix: `npm run test:phase1` (a separate `tsc`-then-`node --test` pipeline against compiled output) currently fails to compile for reasons unrelated to this change — `desktopSupabaseAuthRepository.test.ts` and `settingsHelpers.test.ts` reference fake test doubles missing newer required interface methods (`applySupabaseMigrations`, `pickDirectory`). Confirmed via `git stash` that this pre-dates every change in this session. Not fixed here (out of scope — pre-existing, unrelated test-infra drift).
- `V-5` Not executed: a live re-check against a running desktop app + real Supabase instance — no backend/live environment available in this session; verified via targeted unit tests against the two pure mapping functions instead.

## 9. Regression Guard

- tests: the two new mapping tests lock in that a non-null `modelOverride`/`model`/`yoloMode` survives the mapping, which the pre-existing tests (both using `null`/default values) did not catch.
- alerts: none.
- audit checks: recorded in `change-audit/CA-229-navigator-catalog-model-yolo-mapping.md`.

## 10. Follow-Up Document Updates

- upstream docs updated as part of this fix: none required (mapping-layer bug only; `SS-05`/`SD-06` already correctly describe the intended resolution behavior).
- notes left unchanged on purpose: `npm run test:phase1`'s pre-existing, unrelated compile failures (`V-4`) are left as-is — out of scope for this fix; worth a separate cleanup pass if that pipeline is still relied on.
