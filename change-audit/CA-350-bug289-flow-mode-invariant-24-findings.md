# CA-350: BUG-289 Flow Mode invariant audit — 24 findings + hub watchdog

## Summary

Implemented all 13 phases of BUG-289 (`F-0`..`F-12`) covering 5 CRITICAL silent-hang cases, 7 HIGH, 7 MEDIUM, and 5 LOW findings from the Flow Mode invariant re-audit.

## Key code changes

- **F-0** `hub_stall.go` — hub/root I-16 watchdog (`hub_stalled`)
- **F-1** `scheduleChildTurn` error path + M7 re-arm window for hub.notify
- **F-2/H2** pre-flight cohort `appendCohortResult(FAILED)` + join
- **F-3/H3** gate commit fail emit+escalate; atomic `SaveHead`; corrupt-tolerant `LoadHead`
- **F-4/H4** approval/question expiry and rehydrate clear WAITING
- **F-5/H5** `notifyTurnIdle` drains `pendingHubReinvoke`
- **F-6/A1** `linearizeSendStarted` storeErr → terminal turn
- **F-7/A2/A3** ListAttention cancel_required + OpenRepair; SettleDriver boot wiring
- **F-8** validation retry restore; reprompt reset; skipped_no_command rebuild
- **F-9** hub-less resume; lastFlowControlTurnID; inline depth cap
- **F-10** agentStatus normalize; reconcile filter; hub RUNNING; source DONE
- **F-11** disk-before-RAM commitTerminal; forRun store lock; Compact effects/releases/repairs
- **F-12** IsOverridden on failedTests; child GateOptions

## Tests

- `bug289_test.go` (H1, M7, H3, H4, A6, F-0, M1)
- `go test ./internal/changecontract/ ./internal/flowgate/` green
- Focused runner suites green; full package on Windows still has pre-existing `sh` PATH e2e failures unrelated to this change

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-289
change_type: bugfix
summary: Land BUG-289 F-0..F-12 — hub watchdog + 24 hang/correctness/durability point fixes across flow gate, cohort, reinvoke, resume, and dispatch store
# --->8---
