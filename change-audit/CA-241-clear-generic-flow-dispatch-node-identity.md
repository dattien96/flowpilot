# CA-241: Stop Generic Flow Dispatch Rows From Shadowing Per-Node Model Resolution

## Summary

BUG-241: in the built-in Review Loop, agent models didn't match the workflow's intent — `main` ran `gpt-5.4-mini`, the two structurally-identical reviewer nodes diverged (`claude-sonnet` vs `gpt-5.4`), and `main` disagreed with the `coder` child for the same node. Root cause: the generic dispatch-category `step_definitions` rows (`flow-agent-delegate-coder`, `flow-agent-delegate-reviewer`, `flow-hub-inline`, …) still carried stale `node_id`/`behavior_id`/`agent_ref` copied by an early BUG-236 draft migration and never cleared by the BUG-239 repair. `resolveConfiguredModelForAgent` scans `step_definitions` ordered by `name.asc` and returns the first `node_id` match, so those `"Flow: …"` generic rows shadowed the real `"<pack>: …"` per-node rows — but only one generic reviewer row exists (aliased to `reviewer_correctness`), so `reviewer_security` fell through to its own per-node row, and the relation-based `createRun` path read the per-node row, disagreeing with the scan.

## What Changed

### Runner (`apps/local-runner`)

- `flow_executor.go`: added `isGenericFlowDispatchStepType(stepType)` (`flow-` prefix test) and made `resolveConfiguredModelForAgent` skip those generic dispatch rows in its `node_id` match loop, so only genuine per-node `step_definitions` rows can win — making the run's own model, the child-spawn model, and the timeline display resolve the same authoritative row regardless of any residual contamination. The exact-match `flow-agent-delegate-<role>` role fallback (for manually-built workflows) is unchanged. Corrected the stale doc comment on the function (the earlier "per-node rows never carry a model" premise was wrong).
- Test: `TestResolveFlowNodeModelIgnoresContaminatedGenericDispatchRow` — proves a contaminated generic reviewer row no longer shadows either reviewer node's own per-node row.

### Supabase

- `migrations/20260706120000_clear_node_identity_from_generic_flow_dispatch_rows.sql`: idempotent `update` that nulls `node_id`/`behavior_id`/`agent_ref` (and the other node-definition fields) back to seed defaults on every `flow-%` generic row, restoring the BUG-236 contract (generic rows are dispatch categories with no node identity). `model`/`yolo_mode` are left intact so the role rows keep serving the role fallback. Complements the BUG-239 repair migration, which created per-node rows and repointed `workflow_steps` but never cleared the generic-row contamination.

### Docs

- `requirements/09-BugFix/done/BUG-241-Generic-Flow-Dispatch-Rows-Shadow-Per-Node-Model-Resolution.md`.

## Verification

- `go build ./...` (apps/local-runner) — clean.
- `TestResolveFlowNodeModelIgnoresContaminatedGenericDispatchRow` green; existing resolution suite still green (`TestResolveFlowNodeModelPrefersNodeSpecificStepDefinition`, `TestResolveFlowNodeModelIgnoresHubInlineAgentRef`, `TestCoderCompletionAutoSpawnsReviewerCohortWithOwnModel`, `TestWorkflowStepsRuntimeReflectsPerNodeModelOverride`, `TestCreateRun*`).
- Not run: direct live-Supabase correction (maintainer opted for migration-only) and an in-app end-to-end re-run. Per-node model *values* (coder=haiku, reviewers=sonnet, synthesis=haiku) are set by the maintainer in Settings > Workflows, not by this change.
- Unrelated pre-existing WIP/environment test failures on this branch (Codex-CLI/Drive/home-dir/auth env tests; `TestE2EReviewLoopSynthesisFallbackEscalates`, a synthesis step-status escalation assertion in BUG-234/235 code paths this fix does not touch) are not caused by this change.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-241
change_type: bugfix
summary: Stop stale-node_id generic flow dispatch rows from shadowing per-node step_definitions model resolution, so every path resolves the same authoritative per-node model
# --->8---
