# CA-198: Add Coder/Reviewer Step Types And Reasoning Override Fix

## Summary

Fixed `BUG-161`, a direct follow-up to `BUG-160`: added distinct Coder/Reviewer step types to the manual workflow builder's catalog, and closed the identical "forced value, no way to opt out" gap on the per-step Reasoning override that `BUG-160` had already fixed for Model.

## What Changed

- `supabase/migrations/20260702120000_split_flow_agent_delegate_coder_reviewer.sql`: adds `flow-agent-delegate-coder`/`flow-agent-delegate-reviewer` step_definitions rows; best-effort reassigns existing `workflow_steps` rows by `agent_ref` pattern; does not delete the generic `flow-agent-delegate` row (still needed by other flows/ambiguous rows).
- `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`: `addWorkflowStep` now defaults `reasoningEffortOverride` to `null`; the Reasoning select gains a `No override (use run's reasoning)` option and correctly reflects an unset override instead of always showing a concrete level.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Migration reviewed for FK safety (additive inserts, updates only move rows to step_types inserted in the same migration).
- Not verified live (no Supabase/backend available in this environment) — flagged in `BUG-161` (`V-3`).

## Notes

- Direct user follow-up after live-testing `BUG-160`.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-161
change_type: bugfix
summary: Add distinct Coder/Reviewer step types to the manual workflow builder catalog and fix the Reasoning override to support true "no override" like Model already does
# --->8---
