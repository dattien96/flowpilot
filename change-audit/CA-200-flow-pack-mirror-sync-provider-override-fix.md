# CA-200: Flow-Pack Mirror-Sync Provider Override Fix

## Summary

Fixed `BUG-163`, diagnosed from a user-provided live Supabase CSV export: the built-in Review Loop flow's mirrored `workflow_steps` rows all had `provider_override='codex'` from a Postgres column default, silently overriding `LoadRunSteps`'s model-derived provider fallback (`BUG-160`) and showing `CODEX` instead of Claude even after the model correctly resolved to Claude Haiku (`BUG-162`).

## What Changed

- `apps/local-runner/internal/runner/supabase_workflow_flow_store.go`: `insertSteps` now explicitly sets `provider_override: nil` for every mirrored flow node, overriding the `workflow_steps.provider_override` column's `'codex'` default.

## Verification

- `go build ./...` + `go test ./internal/runner/... -run 'TestSupabaseWorkflowFlowStore'` in `apps/local-runner` — pass.
- Full `go test ./...` — 16 pre-existing failures, all unrelated (Windows paths, mocked Codex resume, skills-merge, history ordering); none touch `insertSteps`/`provider_override`.
- Not verified live — self-corrects on the next Review Loop run start since the mirror-sync always re-inserts on resolve; flagged in `BUG-163` (`V-4`).

## Notes

- Diagnosis required the user's live DB export — the read-side Go code (`LoadRunSteps`) was already correct; the write-side (`insertSteps`) silently relying on a schema default was invisible to static code review of the read path alone.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-163
change_type: bugfix
summary: Explicitly null provider_override in the flow-pack mirror-sync insert so mirrored nodes stop inheriting the workflow_steps column's codex default and resolve provider from their model like BUG-160 already does
# --->8---
