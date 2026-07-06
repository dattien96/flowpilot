# BUG-241: Generic Flow Dispatch Rows Shadow Per-Node Model Resolution

## Metadata

- Document ID: `BUG-241`
- Title: `Generic Flow Dispatch Rows Shadow Per-Node Model Resolution`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-06`
- Last Updated: `2026-07-06`
- Parent Documents: [BUG-239: Flow Step Timeline Ignores Node-Specific Step Definition Model](../todo/BUG-239-Flow-Step-Timeline-Ignores-Node-Specific-Step-Definition-Model.md), [BUG-236: Builtin Flow Mirror Stores Node Definition On workflow_steps Instead Of step_definitions](../todo/BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md)
- Child Documents: `none`
- Related Documents: `change-audit/CA-241-clear-generic-flow-dispatch-node-identity.md`, `change-audit/CA-238-flow-node-definition-step-definitions.md`, `change-audit/CA-239-node-specific-flow-step-model-resolution.md`, `change-audit/CA-230-agent-delegate-node-per-role-model.md`
- Replaces: `none`
- Tags: `agent-flow-engine, model-resolution, step-definitions, supabase, migration, regression`

## AI Quick View

### Summary

- In the Review Loop flow, the running agents' models did not match the workflow's configured intent: `main`/orchestrator ran `gpt-5.4-mini`, the two reviewer nodes — identical in the flow graph, both `agents/reviewer.md` — diverged (`reviewer_correctness` = `claude-sonnet`, `reviewer_security` = `gpt-5.4`), and `main` disagreed with the `coder` child for the *same* node.
- Confirmed root cause is **stale node identity on the generic dispatch-category `step_definitions` rows**. An early BUG-236 draft migration copied `node_id`/`behavior_id`/`agent_ref` onto shared generic rows (`flow-agent-delegate-coder`, `flow-agent-delegate-reviewer`, `flow-hub-inline`, …); the BUG-239 repair created the correct per-node rows and repointed `workflow_steps.step_type`, but never cleared that contamination.
- `resolveConfiguredModelForAgent` (behind `resolveFlowNodeModel`) scans **all** `step_definitions` ordered by `name.asc` and returns the first `node_id` match. The contaminated `"Flow: …"`-named generic rows sort before the real `"<pack>: …"` per-node rows, so they won the match — but only one generic reviewer row exists (aliased to `reviewer_correctness`), so `reviewer_security` fell through to its own per-node row, producing the divergence. Meanwhile `createRun`/`ListWorkflowSteps` reads the per-node row via the `workflow_steps` FK, so `main` and the `coder` child disagreed for the same node.
- Two-part fix: a code guard so the `node_id` match ignores generic dispatch rows (making every path resolve the same authoritative per-node row regardless of residual data), plus a cleanup migration that strips node identity back off the generic seed rows.

### Current Ask

- Make a flow node's model resolve from exactly one authoritative source (its own per-node `step_definitions` row / the `workflow_steps` relation), so the run's own model, the child-spawn model, and the timeline display all agree, and two graph nodes sharing an agent file never diverge.

### Key Decisions

- `D-1` Code guard in `resolveConfiguredModelForAgent`: skip any `step_definitions` row whose `step_type` begins with the literal `flow-` prefix during the `node_id` match. That prefix uniquely identifies the hand-seeded generic dispatch categories — real per-node mirror step_types come from `flowNodeStepType` → `sanitizeStepType`, which rewrites `-` to `_`, so they can never start with `flow-`. The role fallback (exact match on `flow-agent-delegate-<role>`) is deliberately preserved for manually-built workflows.
- `D-2` Cleanup migration `20260706120000_clear_node_identity_from_generic_flow_dispatch_rows.sql`: nulls `node_id`/`behavior_id`/`agent_ref` (and the other node-definition fields) back to seed defaults on every `flow-%` generic row, restoring the BUG-236 domain contract (generic rows are dispatch categories with no node identity). Idempotent; leaves `model`/`yolo_mode` intact so the role rows keep serving the role fallback.
- `D-3` Model *values* are intentionally NOT set by this fix — they are user configuration set via Settings > Workflows (which edits the per-node relation row). Once shadowing is gone, those edits take effect on the run's own model, the child spawns, and the display consistently.

### Constraints

- Live Supabase data is not mutated by this change (per the maintainer's instruction); the cleanup ships as a migration to be applied through the normal migration path, and per-node model values are set in the Settings UI.
- The wider `task/flow-agents` working tree carries in-flight BUG-234/235/236/239 changes; this fix is scoped to model resolution and does not touch step-status/escalation machinery.

### Open Questions

- None. (Longer term, `resolveFlowNodeModel` could resolve strictly via the `workflow_steps` relation given the run's `workflowID`, removing the last dependence on scanning `step_definitions` by `node_id`; the guard makes that optional rather than urgent.)

### Source Refs

- Live DB evidence: two rows share `node_id="coder"` (`flow-agent-delegate-coder` model `claude-haiku` vs `flowpilot_core_flow_pack_review_loop_coder` model `gpt-5.4-mini`); `flow-agent-delegate-reviewer` carries `node_id="reviewer_correctness"`; `flow-hub-inline` carries `node_id="synthesis"`.
- `apps/local-runner/internal/runner/flow_executor.go` `resolveConfiguredModelForAgent` / new `isGenericFlowDispatchStepType`.
- `apps/local-runner/internal/runner/supabase_catalog_store.go` `ListSteps` (`order=name.asc`) — the ordering that made generic rows win.
- `apps/local-runner/internal/runner/interactive_handlers.go` `createRun` / `supabase_catalog_store.go` `ListWorkflowSteps` — the relation path that read the per-node row, disagreeing with the scan.
- `supabase/migrations/20260706120000_clear_node_identity_from_generic_flow_dispatch_rows.sql`.
- `supabase/migrations/20260704153000_repair_flow_node_step_definition_relations.sql` — the repair that omitted the generic-row cleanup.

## 1. Issue Summary

Running the built-in Review Loop, the displayed/executed agent models did not match the workflow's configured models: the orchestrator (`main`) ran `gpt-5.4-mini` instead of the workflow's `claude-haiku`, and the two structurally-identical reviewer nodes ran on different models/providers (`claude-sonnet` vs `gpt-5.4`). Editing a step's model in Settings appeared to have no effect on what actually ran.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/todo/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md` (via BUG-236)
- impacted tech design: none identified
- impacted system spec: none identified

## 3. Environment and Reproduction

- environment: built-in Review Loop flow, Supabase-backed catalog, desktop Flow Mode; DB previously migrated through the BUG-236 draft + BUG-239 repair.
- reproduction steps:
  1. Start the Review Loop in Flow Mode.
  2. Observe the Agents panel / step timeline: `main` = `gpt-5.4-mini`, `coder` = `claude-haiku`, `reviewer_correctness` = `claude-sonnet`, `reviewer_security` = `gpt-5.4`, `synthesis` inherits `main`.
  3. Note that `main` (gpt-5.4-mini) and the `coder` child (claude-haiku) disagree for the same node, and the two reviewers diverge despite identical flow-graph definitions.
- frequency: deterministic on any DB where the generic dispatch rows carry stale `node_id`s.

## 4. Expected vs Actual

- expected: each node resolves its model from a single authoritative source; `main`, the child spawn, and the display agree per node; nodes sharing an agent file resolve identically unless their own per-node rows differ.
- actual: `resolveFlowNodeModel` matched contaminated generic rows first (by `name.asc`), while `createRun` read the per-node relation row — the two paths disagreed, and only one reviewer had a contaminated generic row to alias onto, so the reviewers diverged.

## 5. Impact

- users affected: anyone running flow-pack flows (Review Loop, RAG Harness) on a migrated DB.
- workflows affected: model/provider selection for every agent.delegate node; Settings model edits silently not taking effect at runtime.
- severity: high for correctness of what model each agent runs on (cost/behavior), though the flow still executes.

## 6. Root Cause

- hypothesis (initial, wrong): the per-node mirror rows never carried a model, so resolution fell through to the Flow/Project default. (Disproved: the per-node rows DO carry models.)
- confirmed cause: the generic dispatch-category `step_definitions` rows retained `node_id` (and `behavior_id`/`agent_ref`) copied by an early BUG-236 draft migration and never cleared by the BUG-239 repair. `resolveConfiguredModelForAgent` scans `ListSteps()` (ordered `name.asc`) and returns the first `node_id` match, so a `"Flow: …"` generic row shadowed the real `"<pack>: …"` per-node row for any node it was contaminated with. Only one generic reviewer row exists (aliased to `reviewer_correctness`), so `reviewer_security` fell through to its own per-node row — hence the divergence — and `createRun`/`ListWorkflowSteps` (relation FK) always read the per-node row, disagreeing with the scan for the same node.
- evidence: direct Supabase queries showing the duplicate `node_id` rows and their conflicting models (see Source Refs); the displayed values match exactly what first-by-name resolution over that data produces.

## 7. Fix Strategy

- `F-1` Add `isGenericFlowDispatchStepType(stepType)` (`flow-` prefix test) and skip such rows in `resolveConfiguredModelForAgent`'s `node_id` match loop, so only genuine per-node rows can win. Preserve the exact-match role fallback.
- `F-2` Ship `20260706120000_clear_node_identity_from_generic_flow_dispatch_rows.sql` to null node-definition fields off every `flow-%` generic row (idempotent), restoring the BUG-236 domain contract for all databases.
- `F-3` Leave per-node model *values* to Settings (per-node relation row); do not hardcode them in code or migration.

## 8. Validation

- `V-1` `go build ./...` (apps/local-runner) — clean.
- `V-2` New test `TestResolveFlowNodeModelIgnoresContaminatedGenericDispatchRow` — a contaminated `flow-agent-delegate-reviewer` (node_id=`reviewer_correctness`, model `claude-sonnet`) plus two real per-node rows; asserts `reviewer_correctness` → its own `gpt-5.4` and `reviewer_security` → its own `claude-haiku`, i.e. the generic row no longer shadows.
- `V-3` Existing resolution tests still green: `TestResolveFlowNodeModelPrefersNodeSpecificStepDefinition`, `TestResolveFlowNodeModelIgnoresHubInlineAgentRef`, `TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel`, `TestWorkflowStepsRuntimeReflectsPerNodeModelOverride`, and the `TestCreateRun*` model-resolution suite.
- `V-4` Migration is `update … where step_type like 'flow-%'`, idempotent (guarded by an `is not null`/non-default predicate); verified it targets only generic seed rows (per-node step_types never begin with `flow-`).
- `V-5` Not run: the equivalent live-DB correction (maintainer opted for migration-only, no direct Supabase writes) and an in-app end-to-end re-run. Pre-existing suite failures in this WIP branch (Codex-CLI/Google-Drive/home-dir/interactive-auth environment tests, and `TestE2EReviewLoopSynthesisFallbackEscalates`, a synthesis step-status escalation assertion in BUG-234/235 WIP code paths this fix does not touch) are unrelated to model resolution.

## 9. Regression Guard

- tests: `TestResolveFlowNodeModelIgnoresContaminatedGenericDispatchRow` plus the retained resolution suite above.
- alerts: none.
- audit checks: `change-audit/CA-241-*`; watch that no future migration re-adds node-definition fields onto `flow-%` generic rows.

## 10. Follow-Up Document Updates

- upstream docs: BUG-236's "generic rows are dispatch categories with no node identity" contract is now enforced by both a runtime guard and a cleanup migration; no further schema doc change required.
- notes left unchanged on purpose: `resolveConfiguredModelForAgent` still resolves by scanning `step_definitions` (now guarded) rather than strictly via the `workflow_steps` relation; a relation-only resolution is a possible future simplification, not required for correctness.

## 11. Definition of Done

- `[x]` **DOD-1 — Guard implemented.** `resolveConfiguredModelForAgent` skips generic `flow-` dispatch rows in the `node_id` match.
- `[x]` **DOD-2 — Cleanup migration.** `20260706120000_*` nulls node identity off generic seed rows, idempotently.
- `[x]` **DOD-3 — Regression test.** `TestResolveFlowNodeModelIgnoresContaminatedGenericDispatchRow` green; existing resolution tests still green.
- `[x]` **DOD-4 — Build.** `go build ./...` clean.
- `[x]` **DOD-5 — Docs.** This doc in `done/`; `change-audit/CA-241-*` written.

## 12. Code Change Plan (per DoD item)

### DOD-1 / DOD-3 — resolver guard
- `flow_executor.go`: new `isGenericFlowDispatchStepType`; `resolveConfiguredModelForAgent` skips those rows in the `node_id` match loop, keeping the exact-match role fallback intact. Corrected the surrounding doc comment (the earlier "per-node rows never carry a model" premise was wrong).
- `flow_executor_test.go`: `TestResolveFlowNodeModelIgnoresContaminatedGenericDispatchRow`.

### DOD-2 — cleanup migration
- `supabase/migrations/20260706120000_clear_node_identity_from_generic_flow_dispatch_rows.sql`: idempotent `update` nulling node-definition fields on `flow-%` rows.

### Prior-attempt note
- The earlier `createRun` entry-step role-fallback (`resolveConfiguredModelForAgent` when `steps[0].Model` is empty) and the `ListWorkflowSteps` `node_id`/`agent_ref` additions remain — they are correct and benign, but were aimed at the initial (wrong) "missing model" hypothesis and are not what resolves this bug; the guard + migration are.
