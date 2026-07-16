# CA-329: BUG-288 Vòng 13 residual close (R13-01…R13-27)

## Scope

Close all Vòng 13 re-review findings on Phase A/B gate + change-contract re-entry: production-store restart intent, stall-retry cohort, poisoned baseline cancel, observation/baseline fail-closed, durable marker secret, validate RAM fail-open, stall/gate window races, and P3/test/doc residual.

## Changes

- Session durability: `pending_restart_*` + `flow_context_injected` in NDJSON store; Supabase migration + upsert/get fields; durable `run_marker_secret` file via `InitRunMarkerSecretFromDir`.
- Stall lifecycle: `stalledRetryCause` suppress cohort fail; skip gateEpoch/settle clear + child persist; retry parks when gate busy; re-arm stall timer.
- Gate/observe/baseline: child observation escalate; non-repo carve-out only when no turn-start repo signal; LC_ALL=C git; second ObserveGitDiff fail-closed; corrupt baseline fail-closed; cancel aborts baseline write (atomic rename).
- Validate: commit `flowValidationRetryState` only after persist; unify escalate return true + awaiting-user.
- Marker: `isFlowContextHandoff(..., expectedIDs)` run-bound; FlowContextInjected restore.
- Tests: `bug288_round13_test.go` + Windows junction/absolute path tests; observe_test comment R13-24.
- Doc: BUG-288 §11 Vòng 13 Fixed inventory.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-288
change_type: bugfix
summary: close Vòng 13 R13-01..R13-27 — durable restart intent/marker secret, stall-retry cohort, baseline/observe fail-closed, validate RAM channel, gate lifecycle window
# --->8---
