# BUG-299: Flow/Workflow launches allowed YOLO=off, exposing missed approvals and hub stall-timeouts

## Metadata

- Document ID: `BUG-299`
- Title: `Flow/Workflow-mode launches (including the built-in Review Loop) could run with YOLO=off, exposing real gate/approval robustness gaps (a missed approval render while a sibling child was focused, the hub's own stall-timeout firing while a sibling was still legitimately working) — product decision: Flow mode must always run YOLO=true; only Normal Chat supports YOLO=off`
- Phase: `bugfix`
- Status: `fixed`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-20`
- Last Updated: `2026-07-20`
- Parent Documents: `-`
- Child Documents: `-`
- Related Documents: `BUG-296 (same-session Claude YOLO permission-wiring investigation), BUG-297/BUG-298 (same-session Review Loop chat-mode investigations), supabase/migrations/20260610160000_add_workflow_yolo_mode.sql (the migration this bug's fix builds on)`
- Replaces: `-`
- Tags: `desktop-flowpilot, chat-ui, agent-flow-engine, yolo, review-loop, product-decision, database-default`

## AI Quick View

### Summary

- User ran test case A3 (YOLO=off) against the built-in "Review Loop" workflow: `coder` completed, `reviewer_correctness` completed, but while the user stayed on `reviewer_correctness`'s own transcript to approve it, `reviewer_security`'s own approval-required tool call never rendered a visible approval card in the UI — it sat "Waiting · approval" indefinitely, then eventually failed with a transient `API Error: 529 Overloaded`. The hub's own subsequent retry round then hit its own stall-timeout ("hub has made no progress for 2m0s") while other legitimate work was arguably still in flight.
- User's conclusion: Flow/Workflow-mode's gate/approval/reinvoke machinery is not robust enough against YOLO=off's paused-approval races; YOLO=off should only be offered in Normal Chat, which has no hub/cohort/gate layer to race against. Decision: lock Flow/Workflow launches to always run YOLO=true, and remove the ability to configure YOLO=off for them anywhere in the desktop Settings UI.
- Traced the actual root cause of "a built-in workflow like Review Loop can even reach yoloMode=false in the first place": `SyncBuiltins` mirrors a built-in workflow's row from its flow-pack YAML (`review-loop.yaml`), but `yolo_mode` is intentionally NOT part of the pack schema (a pure per-installation admin setting, so a resync never overwrites an admin's own choice) — so a first-time mirror INSERT falls through to the column's own Postgres default, which was `false` (`supabase/migrations/20260610160000_add_workflow_yolo_mode.sql`). The same applies to `step_definitions.yolo_mode`.
- Separately found (and fixed) a real, independent inconsistency in the desktop Settings UI: the workflow-edit form's YOLO checkbox was the ONE field that did not respect `workflowDetailReadOnly` (every sibling field — Cap, reasoning effort, etc. — correctly disables when a built-in/non-editable workflow is open) — so a built-in's YOLO could be toggled even though the rest of its form was locked.
- Fix has two layers: (1) desktop Settings UI — all three yoloMode checkboxes (create-workflow, edit-workflow, step-definition) are now shown checked and unconditionally disabled, and the three draft-construction functions (`createEmptyWorkflowDraft`, `mapWorkflowToDraft`, and the existing-step-select path) all force `yoloMode: true` regardless of what is loaded/defaulted; (2) a new Supabase migration flips the `workflows.yolo_mode` and `step_definitions.yolo_mode` column defaults from `false` to `true`, so a future first-time mirror-sync insert (or any other row created without an explicit override) starts YOLO=true without requiring an admin to open Settings first.
- Confirmed the ChatInput composer's own YOLO toggle does NOT need separate locking: it is already wrapped in `{isChatMode && (...)}` (`isChatMode = chatMode === "normal_chat"`), so it is already fully hidden whenever the user is in Workflow launch mode — the Settings-page default is the only remaining lever for a Workflow-mode run's YOLO value.

### Current Ask

- None — both layers implemented and additive-tested. See "7. Fix Strategy" and "8. Validation".

### Key Decisions

- `V-1` Flow/Workflow launches always run YOLO=true; YOLO=off is only configurable in Normal Chat mode (product decision, made by the user after this investigation).
- `V-2` The new column-default migration does NOT retroactively update already-persisted rows (e.g. an existing "Review Loop" mirror row currently stored with `yolo_mode=false`) — only future inserts get the corrected default. Retroactive correction was explicitly declined by the user for this pass.

### Constraints

- additive-tests-only: existing tests (`WorkflowsSettings.step-list-model-visibility.test.ts`, `Timeline.live-agent-card.test.ts` and siblings) were left unmodified; only new test files/cases were added.
- Migration only changes the column DEFAULT clause — it does not touch existing row data, and must be applied by the user against their actual Supabase instance (this sandbox has no live Supabase/local Postgres to run it against).
- This is a desktop (TypeScript) + database (SQL migration) change; no Go runner code was touched.

### Open Questions

- Whether the currently-persisted "Review Loop" (and any other already-mirrored built-in) row's stored `yolo_mode=false` should be corrected now via an explicit `UPDATE` (declined for this pass — user chose default-only) or left to self-correct only if/when an admin opens it in Settings and saves (which, per the Settings UI fix, would now correctly submit `yoloMode=true`).
- `workflow_steps.yolo_mode` (a separate, nullable column with no default clause — distinct from `step_definitions.yolo_mode`) was not investigated or touched; out of scope for the approved fix.

### Source Refs

- Desktop screenshots: run-14217 (`reviewer_security`, Round 1/3) — top bar "Waiting · approval" with the transcript panel showing only "Thinking..." (no approval card rendered) while the sidebar shows it as "running"; then `run-14217` "Failed" with `API Error: 529 Overloaded`; then a bash "Approval required" card correctly showing on MAIN's own synthesis step; then Round 2/3 with `reviewer_correctness` now "failed" and a hub stall-timeout card ("hub has made no progress for 2m0s").
- User: "Tôi nghĩ k nên cho YOLO = off ở flow mode nữa... Muốn yolo off thì dùng normal chat đi -> ở trong Flow Setting page luôn enable YOLO option và k cho edit."
- `apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml` (no yolo field at all).
- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go:341-349` (comment confirming `yolo_mode` "aren't part of FlowDefinitionRecord or the pack schema at all... a genuine INSERT falls through to the column's own Postgres default").
- `supabase/migrations/20260610160000_add_workflow_yolo_mode.sql` / `20260611120000_restore_step_definition_single_step_yolo.sql` (the `default false` migrations this fix's new migration corrects).

## 1. Issue Summary

While testing a Review Loop flow with YOLO=off, an approval-required tool call on a second, concurrently-progressing reviewer child never surfaced a visible approval card in the desktop UI (it stayed "Waiting · approval" while the user was looking at a sibling child's transcript), eventually failed with a transient provider error, and the hub's subsequent retry round hit its own stall-timeout. The user concluded that Flow/Workflow mode's approval/gate machinery is not reliable enough under YOLO=off and decided Flow mode should always run YOLO=true, with YOLO=off reserved for Normal Chat only.

## 2. Parent Links

- impacted coding plan: `CP-51-PhaseAB-Timeline-And-Verification-Log`
- impacted tech design: desktop Workflow/Step Settings forms (`WorkflowsSettings.tsx`), built-in workflow mirror-sync (`supabase_workflow_flow_store.go: SyncBuiltins`), `workflows`/`step_definitions` schema (Supabase migrations)
- impacted system spec: YOLO posture contract (04-04) — this bug narrows its UI-configurable surface, not the posture resolution logic itself

## 3. Environment and Reproduction

- environment: FlowPilot Desktop, runner `127.0.0.1:4318`, built-in "Review Loop" workflow launched via the Workflow picker, YOLO explicitly set off for the test.
- reproduction steps: launch "Review Loop" with YOLO=off; let `coder` and `reviewer_correctness` complete; stay on `reviewer_correctness`'s transcript while `reviewer_security` (a cohort sibling) reaches its own approval-required tool call — observe no approval card renders for it; it eventually fails.
- frequency: reproduced once, deliberately, by the user; not independently re-verified as deterministic (a real transient `529 Overloaded` error was also involved, muddying a clean signal) — the product decision to disallow YOLO=off in Flow mode was made without requiring full reproduction of every step, given the broader pattern this session already surfaced (BUG-296/297/298).

## 4. Expected vs Actual

- expected (per the product decision this bug implements): Flow/Workflow-mode launches cannot be configured with YOLO=off anywhere in the desktop Settings UI, and any future first-time built-in mirror row starts YOLO=true.
- actual (before this fix): three Settings checkboxes allowed setting/persisting `yoloMode=false` for workflows and step definitions, one of them (the workflow-edit form's checkbox) even bypassing the read-only lock every other field in that same form respected; and the underlying database columns defaulted new rows to `false`.

## 5. Impact

- users affected: anyone using Flow/Workflow-mode launches (any provider) with YOLO=off configured, whether deliberately or because a built-in workflow's mirror row happened to default to `false`.
- workflows affected: any multi-child flow (review-loop or similar) under YOLO=off.
- severity: medium-high — not silent data loss, but a confusing, hard-to-diagnose stall/failure experience that this fix eliminates by removing the unsupported configuration entirely rather than trying to harden the gate machinery against it.

## 6. Root Cause

- confirmed cause (why a built-in workflow could be `yoloMode=false` at all): `SyncBuiltins` (`flow_definition_resolver.go:326-391`) builds each built-in workflow row from its flow-pack YAML via `builtinRecordFromFlow`, then `Upsert`s it. `yolo_mode` is deliberately NOT part of `FlowDefinitionRecord` or the pack schema (per the code's own comment at `supabase_workflow_flow_store.go:341-349`) — it is treated as a pure per-installation admin setting that mirror-sync must never overwrite. Consequently, the FIRST time a built-in row is ever inserted, this column is omitted from the INSERT entirely and Postgres applies its own column default — which was `false` (`supabase/migrations/20260610160000_add_workflow_yolo_mode.sql`, and correspondingly for `step_definitions.yolo_mode` via `20260611120000_restore_step_definition_single_step_yolo.sql`).
- confirmed secondary cause (why an admin could ALSO explicitly set it to false, or a built-in's YOLO stayed editable despite the rest of its form being locked): the workflow-edit form's YOLO checkbox (`WorkflowsSettings.tsx`, around the `workflowDraft.yoloMode` checkbox) lacked the `disabled={workflowDetailReadOnly}` guard that every sibling field in the same form (Cap, reasoning effort, etc.) already had — an inconsistency that let a built-in/non-editable workflow's YOLO be toggled from the UI even though nothing else on its form could be.
- evidence: `apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml` (no yolo field); `supabase_workflow_flow_store.go:341-349` (comment confirming the INSERT-falls-through-to-default behavior); `supabase/migrations/20260610160000_add_workflow_yolo_mode.sql` / `20260611120000_restore_step_definition_single_step_yolo.sql` (`default false`); `WorkflowsSettings.tsx` (the workflow-edit YOLO checkbox missing `disabled={workflowDetailReadOnly}` next to the Cap field, which had it).

## 7. Fix Strategy (APPLIED)

- `F-1` (applied, desktop Settings UI) — in `WorkflowsSettings.tsx`: all three yoloMode checkboxes (step-definition form, create-workflow form, edit-workflow form) now render `checked` + `disabled` unconditionally (ignoring the underlying draft value entirely, stronger than the pre-existing `workflowDetailReadOnly` guard other fields use), with a `title` tooltip explaining Flow mode always runs YOLO and to use Normal Chat for gated runs. `createEmptyStepDraft`, `mapWorkflowToDraft`, `createEmptyWorkflowDraft`, and the existing-step-select path (`selectStepDefinition`) all now force `yoloMode: true` on the constructed/loaded draft, so a legacy row loaded with `yoloMode: false` is normalized the moment it enters the edit form, ensuring any future explicit save persists `true` regardless of what was previously stored.
- `F-2` (applied, database) — new migration `supabase/migrations/20260720150000_default_flow_yolo_mode_true.sql` flips the `workflows.yolo_mode` and `step_definitions.yolo_mode` column defaults from `false` to `true`, so a future first-time mirror-sync insert (or any other row created without an explicit override) starts YOLO=true without requiring an admin to open Settings first. Deliberately does NOT touch already-persisted rows (user's explicit choice) — an existing "Review Loop" mirror row currently stored with `yolo_mode=false` self-corrects only once someone opens it in Settings (now forced to `true` via `F-1`) and saves, or via a separate future `UPDATE` if the user later decides to run one.
- Confirmed the ChatInput composer's own YOLO toggle needs no change: it is unconditionally wrapped in `{isChatMode && (...)}` (`isChatMode = chatMode === "normal_chat"`), already fully hidden for Workflow-mode launches — verified by reading the surrounding JSX after an initial, corrected misread.
- additive-tests-only: existing `WorkflowsSettings.step-list-model-visibility.test.ts` and `Timeline.live-agent-card.test.ts` were left unmodified. New coverage added in `WorkflowsSettings.yolo-locked.test.ts` (see Validation).

## 8. Validation

- New additive test file `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.yolo-locked.test.ts`, 5 tests, all pass:
  - `mapWorkflowToDraft` forces `yoloMode=true` for a built-in workflow persisted with `yoloMode=false` (the exact Review Loop repro shape).
  - `mapWorkflowToDraft` stays `true` for an already-correct workflow (non-regression).
  - `mapWorkflowToDraft` returns `null` for a null workflow (no-selection state unaffected).
  - `createEmptyWorkflowDraft` defaults `yoloMode=true` for a brand-new workflow.
  - `createEmptyStepDraft` defaults `yoloMode=true` for a brand-new step definition.
- Verification method: this environment's `npm run test:phase1` is blocked by pre-existing, unrelated stale fixtures in `tests/phase1/**` (same gap noted in BUG-297); worked around with the same temporary, uncommitted `--require` alias hook + local node_modules copy used for BUG-297's verification (neither committed, both cleaned up after) to get a real signal from the actual project test suite.
- `go test ./internal/runner -run "TestBug29"` / broader battery: not applicable — no Go runner code was touched by this fix.
- `tsc --noEmit`: no new type errors from either changed file (`WorkflowsSettings.tsx`, the new test file) — the same 10 pre-existing, unrelated errors persist unchanged.
- Migration: syntax-reviewed against the existing `20260610160000_add_workflow_yolo_mode.sql` migration's conventions; not executed against a live database in this sandbox (none available) — must be applied by the user against their actual Supabase instance.

## 9. Regression Guard

- tests: added, all pass (see Validation) — pins that every draft-construction path (new workflow, new step, loading an existing workflow regardless of its persisted value) always yields `yoloMode=true`.
- alerts: none proposed for this pass.
- audit checks: CA note `CA-378` (feature_key `chat-ui`) accompanies this fix.

## 10. Follow-Up Document Updates

- upstream docs that must change: the YOLO posture contract (04-04) should eventually note that Flow/Workflow-mode launches are constrained to YOLO=true by product decision, not just by UI default — flagged here, not authored as a separate SS/SD edit in this pass.
- notes left unchanged on purpose: the Open Questions (retroactive row correction, `workflow_steps.yolo_mode`) were deliberately left unresolved per the user's explicit scope choice.
