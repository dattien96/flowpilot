# CA-197: Workflow Step Model Override And Identity

## Summary

Fixed `BUG-160`, found via live testing of `BUG-158`/`BUG-159`: newly-added workflow steps got a forced model override at creation with no way to clear it, no fallback to the step type's own configured default once cleared, and the `Settings > Workflows` step list still showed the generic `BUG-155` label instead of real per-step identity.

## What Changed

- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`: `addWorkflowStep` now defaults `modelOverride` to `null`; the `Model override` select gains a `No override (use run's model)` option; the step list title now prefers `step.nodeId || step.agentRef` over the shared generic `step_definitions.name`.
- `apps/local-runner/internal/runner/supabase_workflow_store.go`: `LoadRunSteps` now falls back to `step_definitions.model` when `model_override` is unset, and derives `Provider` from the resolved model via `providerKeyFromModel` when `provider_override` is also unset.
- `apps/local-runner/internal/runner/phase5_test.go`: added `TestSupabaseStoreLoadRunStepsFallsBackToStepDefinitionModel`; updated the existing select-string assertion for the added `model` field.

## Verification

- `go build ./...` + `go test ./internal/runner/... -run 'TestSupabaseStoreLoadRunSteps|TestWorkflowStepsRuntime'` in `apps/local-runner` — pass.
- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Not verified live (no backend/Supabase available in this environment) — flagged in `BUG-160` (`V-4`).

## Notes

- Deliberately did not add new `step_definitions` catalog rows for "Coder" vs "Reviewer", and did not change `step_definitions.model`'s actual default value away from the schema default (`gpt-5.5`) — both are content/catalog decisions, not code bugs; `nodeId`/`agentRef` already exist for per-step identity, this change just surfaces them in the settings list too.
- `apps/desktop-flowpilot/src/styles.css` also carries pre-existing, already-uncommitted `.workflow-step-card-toggle`/`.workflow-step-card-chevron` rules for the collapsible step-card UI this fix's `WorkflowsSettings.tsx` changes render inside of — not authored as part of `BUG-160`, but included here since it was already sitting in the same working tree with no separate commit of its own.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-160
change_type: bugfix
summary: Stop forcing a model override on new workflow steps, add a way to clear it, fall back to the step type's own configured model, and show real per-step identity in Settings > Workflows
# --->8---
