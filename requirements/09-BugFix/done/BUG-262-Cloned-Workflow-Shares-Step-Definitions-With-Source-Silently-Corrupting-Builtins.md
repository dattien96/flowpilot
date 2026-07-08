# BUG-262: Cloned Workflow Shares Step Definitions With Source, Silently Corrupting Built-Ins

## Metadata

- Document ID: `BUG-262`
- Title: `Cloned Workflow Shares Step Definitions With Source, Silently Corrupting Built-Ins`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (Scenario 13, Scenario 14), [BUG-236: Builtin Flow Mirror Stores Node Definition On Workflow Steps Instead Of Step Definitions](./BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md) (established the `workflow_steps`/`step_definitions` split this bug's corruption exploits)
- Child Documents: `none`
- Related Documents: [BUG-249: Corrupted Builtin Mirror Recreate Duplicates Row And Drops Overrides](./BUG-249-Corrupted-Builtin-Mirror-Recreate-Duplicates-Row-And-Drops-Overrides.md) (same family: built-in mirror row corruption), [BUG-261: Chat-Mode Explicit FlowRef Resolve Failure Silently Suppresses Hub Turn Forever](./BUG-261-Chat-Mode-Explicit-FlowRef-Resolve-Failure-Silently-Suppresses-Hub-Turn-Forever.md) (this bug's corrupted `review-loop` row is what caused BUG-261's live symptom), [CA-260](../../../change-audit/CA-260-clone-workflow-copies-independent-step-definitions.md)
- Replaces: `none`
- Tags: `settings, workflows, clone, data-integrity, regression, agent-flow-engine`

## AI Quick View

### Summary

- Found while diagnosing BUG-261's live trigger: the built-in `review-loop` workflow's `synthesis` node had `depends_on_json` corrupted to `["claude-review-fake-model","my-reviewer"]` — node names from a completely unrelated custom test flow (`flow-claude-2-review-1failed`), not review-loop's own `["reviewer_correctness","reviewer_security"]`.
- Root cause: `cloneWorkflow` (`supabaseAdminRepository.ts`) creates a new `workflows` row for the clone, but inserts the clone's `workflow_steps` referencing the **exact same `step_type` values as the source** — it never creates independent `step_definitions` rows for the clone. Since `step_definitions` is a de-duplicated catalog keyed by `step_type` (BUG-164 — "a step type has exactly one configured model"), the clone and its source end up pointing at the literal same row.
- The user cloned the built-in Review Loop, added nodes to build a custom "1 reviewer fails" test flow, and saved. `persistEdgeDerivedDependsOn` (`WorkflowsSettings.tsx`) computed `dependsOn` from the clone's own local edges and called `saveStepDefinition`, which unconditionally upserts by `step_type` — silently overwriting the shared row, corrupting the built-in `review-loop`'s real `synthesis` node in the process.
- This contradicts the Clone Workflow dialog's own stated promise (CP-36 Scenario 13): "Creates an editable copy of ... The original stays unchanged." That promise held for the `workflows` row itself but not for its steps.
- `listWorkflowsUsingSteps` already exists and is already used to block deleting a step still referenced by another workflow (a "shared step" concept the team was already aware of) — but it was never consulted before the `dependsOn` overwrite this bug fixes.

### Current Ask

- Cloning a workflow must produce a fully independent copy — including its steps — so editing the clone can never write into the source's (or any other workflow's) step definitions.
- As defense in depth, saving a workflow's edge-derived `dependsOn` must never silently overwrite a step definition that is still shared with another workflow.

### Key Decisions

- `V-1` Fix at the source: `cloneWorkflow` now deep-copies each source step into a brand-new `step_definitions` row with a fresh, workflow-scoped `step_type` (`${clonedWorkflowId}__${sourceStepType}`), and points the clone's `workflow_steps` at those new rows. `node_id`/`behaviorId`/`agentRef`/`dependsOn`/etc. are copied verbatim — only the storage-level `step_type` key changes, since `node_id` (what edges actually reference) only needs to be unique within one flow's own graph, which a copied graph already satisfies.
- `V-2` Defense in depth: `persistEdgeDerivedDependsOn` now calls the existing `listWorkflowsUsingSteps` before writing a computed `dependsOn`, and skips (reporting back to the caller) any step_type still referenced by a workflow other than the one currently being saved — covering the residual case of a brand-new workflow deliberately picking an *existing* step from the "add step" dropdown (not just Clone), and any already-shared row left over from before this fix shipped.
- `V-3` Skipped steps are surfaced to the user in the save confirmation message ("Dependencies for X were NOT updated because it is shared with another workflow...") rather than failing the whole save or silently doing nothing — the workflow itself still saves.

### Constraints

- Does not retroactively repair any `step_definitions` row already corrupted by this bug before the fix (e.g. the live `review-loop` `synthesis` row) — that is a one-time data fix (`UPDATE step_definitions SET depends_on_json = ... WHERE step_type = ...`), out of scope for this code change.
- Does not change the `step_definitions` schema or the fact that a step_type can be legitimately shared by design for some fields (e.g. `model`, per BUG-164) — this fix only stops `dependsOn` (and, as a side effect of cloning fully, every other field) from being silently cross-written; a workflow that *intentionally* shares a step via the dropdown still can, it just can no longer silently corrupt it via edge-derived `dependsOn`.
- No backend (Go) changes — `cloneWorkflow`/`saveWorkflow`/`saveStepDefinition` are frontend-direct-to-Supabase calls (`supabaseAdminRepository.ts`), a separate write path from the Go `SupabaseWorkflowFlowStore` BUG-261 touched.

### Open Questions

- None.

### Source Refs

- `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` `cloneWorkflow` (~line 635), `saveStepDefinition` (~line 764+30 after this change), `listWorkflowsUsingSteps` (~line 708, pre-existing).
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` `persistEdgeDerivedDependsOn` (~line 700), `computeDependsOnByStepType` (~line 670), `openDeleteStepConfirm`/`stepTypesUsedByBuiltin` (~line 1020, the pre-existing analogous guard on the delete path that this bug's fix mirrors for the save path), `saveWorkflow`/`saveNewWorkflow` call sites (~line 830, ~893).
- Live evidence: user-provided Supabase CSV exports of `workflows_rows` and `step_definitions_rows` (this session) — the `flowpilot_core_flow_pack_review_loop_synthesis` row's `depends_on_json` was `["claude-review-fake-model","my-reviewer"]` (from the unrelated test flow `flow-claude-2-review-1failed`, `step_type` created/updated `2026-07-08 00:44:5x`), instead of `["reviewer_correctness","reviewer_security"]` matching the built-in's own `reviewer_correctness`/`reviewer_security` step rows.
- New regression test: `cloneWorkflow gives every cloned step its own step_definitions row, not the source's` (`tests/phase1/workflowFlowEngineAttrs.test.ts`).

## 1. Issue Summary

The user cloned the built-in **Review Loop** workflow in Settings to build a custom test flow with an intentionally-broken reviewer node. After saving their custom flow's edges, the **original built-in Review Loop's own `synthesis` node** silently ended up with a `dependsOn` list naming the custom flow's node ids instead of its own reviewers — discovered only because it later caused Review Loop's `flowRef` to fail resolution entirely (BUG-261).

## 2. Parent Links

- impacted coding plan: [CP-36](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), Scenario 13 ("Clone A Built-in Flow In Settings; Original Stays Read-Only") — its own checklist already asserts "the edit did not leak back into the built-in row," verified at the time only for the `workflows` row's own columns (`edges_json`/`policy_*`/`model_override`), not for the step-level data this bug found leaking.
- related: CP-36 Scenario 14 (built-in mirror recreation) and BUG-249 (mirror row corruption) are the same general class of "built-in row integrity" issue, via a different mechanism (pack_flow_id renaming vs. shared step_definitions).

## 3. Environment and Reproduction

- environment: Desktop app → Settings → Workflows, Supabase-backed workspace.
- reproduction steps:
  1. Clone a workflow that has at least one step (e.g. the built-in Review Loop).
  2. In the clone, add new nodes/edges but leave at least one cloned step (e.g. `synthesis`) attached as-is.
  3. Save the clone. Observe (via a `step_definitions` export or the Settings step editor) that the **source** workflow's step for that same node now shows the clone's edge-derived `dependsOn`, not its own.
- frequency: deterministic — every clone-then-edit-edges-then-save sequence that doesn't first replace every inherited step with a brand-new one hits this.

## 4. Expected vs Actual

- expected (per the Clone Workflow dialog's own copy): "Creates an editable copy of [workflow]. The original stays unchanged."
- actual: the `workflows` row itself was independent, but every cloned step silently continued to share its `step_definitions` row with the source. Any edge-derived `dependsOn` write on the clone also mutated the source.

## 5. Impact

- users affected: anyone using Settings' Clone Workflow feature and then editing the clone's edges — a normal, expected part of Scenario 13's own documented flow.
- workflows affected: the source workflow of any clone, up to and including built-ins (Review Loop in the observed incident) — a built-in's `editable=false` protection on the `workflows` row does **not** protect its steps, since the corruption writes to `step_definitions` directly, a table with no built-in/editable gate of its own.
- severity: high — silently corrupts a workflow the user isn't even looking at (the clone's source), with no error, no warning, and a delayed, confusing downstream symptom (BUG-261's "run gets stuck" was a full flow-resolution failure, not an obviously-related step edit).

## 6. Root Cause

- confirmed cause: `cloneWorkflow` (`supabaseAdminRepository.ts:682-693` prior to this fix) inserted the clone's `workflow_steps` rows with `step_type: step.stepType` copied verbatim from the source's steps — never creating new `step_definitions` rows. `step_definitions` is upserted/read by `step_type` alone (a de-duplicated catalog, not a per-workflow table — see BUG-164's "a step type has exactly one configured model" precedent), so the clone's `workflow_steps` row and the source's both resolve to the identical `step_definitions` row. `persistEdgeDerivedDependsOn` (`WorkflowsSettings.tsx:700`) then computes `dependsOn` purely from the CURRENTLY-open workflow's local edges and calls `saveStepDefinition`, an unconditional `upsert(..., { onConflict: "step_type" })` — with no check for whether other workflows (via `listWorkflowsUsingSteps`, already used elsewhere for the analogous delete-guard) also reference that step_type.
- evidence: user-supplied `step_definitions_rows.csv` shows `step_type=flowpilot_core_flow_pack_review_loop_synthesis` (review-loop's real, pack-prefixed synthesis step) with `depends_on_json=["claude-review-fake-model","my-reviewer"]` — node ids belonging only to the separate custom flow `flow-claude-2-review-1failed` — and an `updated_at` timestamp within one second of that same custom flow's `claude-review-fake-model` step's own `updated_at`, confirming a single save operation touched both.

## 7. Fix Strategy

- `F-1` `cloneWorkflow` (`supabaseAdminRepository.ts`) now: (a) loads the source's steps' full `step_definitions` rows (`select("*").in("step_type", sourceStepTypes)`), (b) for each, inserts a new `step_definitions` row with a fresh `step_type` (`${clonedWorkflow.id}__${sourceStepType}`) and every other field copied verbatim (including `depends_on_json`, `node_id`), and (c) inserts the clone's `workflow_steps` referencing the new step_types, never the source's.
- `F-2` `persistEdgeDerivedDependsOn` (`WorkflowsSettings.tsx`) now calls `listWorkflowsUsingSteps` for every step_type it's about to write a computed `dependsOn` for, and skips (collecting for the caller) any step_type also referenced by a workflow other than the one being saved. `saveWorkflow`/`saveNewWorkflow` append a note to the success message naming any skipped steps and suggesting cloning/dedicated steps instead of silently doing nothing.

## 8. Validation

- `V-1` `npm --prefix apps/desktop-flowpilot run typecheck` — clean.
- `V-2` New test `cloneWorkflow gives every cloned step its own step_definitions row, not the source's` (`tests/phase1/workflowFlowEngineAttrs.test.ts`): seeds a source workflow with one step (`flowpilot_core_flow_pack_review_loop_synthesis`, `dependsOn=["reviewer_correctness","reviewer_security"]`), clones it, and asserts the newly-inserted `step_definitions` row has a fresh `step_type` (`<clonedId>__<sourceStepType>`, not equal to the source's), preserves `node_id`/`depends_on_json` verbatim, and that the clone's `workflow_steps` insert references the new step_type. Passes.
- `V-3` Existing `cloneWorkflow creates an editable, non-builtin copy referencing the source` test (same file) still passes unchanged — it clones a workflow with zero steps, so it never exercised (and isn't affected by) this fix's new code path.
- `V-4` `npx tsx --test tests/phase1/*.test.ts`: 48 passed, 2 failed — both pre-existing and unrelated (`desktopRunnerMode.test.ts`/`importBoundary.test.ts` failing on absolute-path resolution — `ENOENT ... 'C:\working\apps\desktop-flowpilot\src\main.tsx'` / `'C:\packages\flowpilot-client-core\src\domain'` — a path-joining bug in those two test files' own setup when run via a multi-file glob from the repo root, confirmed by stack trace to be unrelated to any file this bug touches).
- `V-5` `F-2` (`persistEdgeDerivedDependsOn`'s new guard) has no dedicated unit test — `WorkflowsSettings.tsx` has no existing test file/harness in this repo (verified: no `WorkflowsSettings*.test.*` exists), and introducing one was judged out of scope for this fix; it was verified by code review and typecheck only. A live manual re-test of the Clone dialog (Scenario 13) picking up an already-shared step and confirming the new save-time warning appears was not performed this session.
- `V-6` GitNexus MCP tools were unavailable in this thread (same as BUG-261); proceeded via direct code inspection and by tracing every call site of `cloneWorkflow`/`persistEdgeDerivedDependsOn`/`saveStepDefinition` manually.
- `V-7` The already-corrupted live `review-loop` `synthesis` row (`depends_on_json=["claude-review-fake-model","my-reviewer"]`) was **not** repaired as part of this change — see Constraints; the user was given the direct `UPDATE step_definitions ... WHERE step_type = 'flowpilot_core_flow_pack_review_loop_synthesis'` statement separately to unblock their own testing.

## 9. Regression Guard

- tests: `cloneWorkflow gives every cloned step its own step_definitions row, not the source's` (`tests/phase1/workflowFlowEngineAttrs.test.ts`).
- audit checks: `gitnexus_detect_changes()` was not run — GitNexus MCP tools were unavailable in this thread; see `V-6`.

## 10. Follow-Up Document Updates

- upstream docs that must change: [CP-36 Scenario 13](../../07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) should get a note that its "original stays unchanged" checklist item is now also proven at the step-definitions level, not just the `workflows` row — added alongside this bug's filing.
- notes left unchanged on purpose: `step_definitions` remains a shared catalog by design for fields like `model` (BUG-164) when a workflow *deliberately* reuses an existing step via the "add step" dropdown — this fix only stops the *silent, edge-derived* `dependsOn` overwrite; it does not forbid intentional step sharing outright.
