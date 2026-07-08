# CA-257: Check Turn Status Before Bulk Progress

## Scope

Fixed a live regression (`run-9096`): a workflow-attached chat run whose turn failed (Codex usage-limit error) had every still-pending workflow step bulk-marked DONE, because the Codex adapter's `SendTurn` returns `nil` even when the terminal event was `EventTurnFailed`, and `runTurn`'s legacy bulk-progress gate trusted that `nil` at face value.

## Changes

- `interactive_service.go`: `runTurn` now takes a locked snapshot of `rs.status` right after `sendTurnWithRetry` returns and only calls the legacy `orchestrator.Progress()` bulk-completion fallback when `err == nil && rs.status != RunStatusFailed && !rs.flowEngineDriven` — previously it trusted `err == nil` alone, which the Codex adapter can report even for a genuinely failed turn.
- `interactive_service.go`: added `markStepFailed`, mirroring `markStepRunning`, and call it from `runTurn` when the turn failed on a non-flow-engine-driven run — otherwise the in-flight step (started by `markStepRunning`) hangs at `RUNNING` forever once the bulk-DONE fallback above is correctly skipped.
- `flow_step_runtime_test.go`: added `TestFlowEngineDrivenRunSkipsBulkProgress/failed_turn_with_nil_err_skips_despite_not_flow_engine_driven`, using a fake adapter that emits `EventTurnFailed` then returns `nil` (mirroring the real Codex adapter), asserting the in-flight step settles to `FAILED` and the untouched step stays `PENDING`.

## Verification

- `rtk go test ./internal/runner -run 'TestFlowEngineDrivenRunSkipsBulkProgress' -v -count=1` from `apps/local-runner` — 4 subtests passed; confirmed to fail against the pre-fix code via a temporary revert.
- `rtk go test ./internal/runner/... -count=1` from `apps/local-runner` — same 15 pre-existing unrelated failures on both baseline and fixed code; no new failures.
- Live confirmation: after rebuilding and restarting the server with the first fix only, `run-9325` correctly showed `Failed` with 3 of 4 steps `pending`, but `coder-gpt` was stuck at `running` — the second change (`markStepFailed`) settles that step to `FAILED` instead.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-259
change_type: bugfix
summary: gate the legacy bulk workflow-step Progress() fallback on the run's actual settled status, not just the adapter's (unreliable) nil-error return, so a failed turn on a workflow-attached run no longer bulk-marks every pending step DONE
# --->8---
