# CA-352: Task-251 production SettleDriver + Task-255 settle barriers/model/CI

## Summary

Close Task-251 and Task-255: wire SettleDriver with fail-closed EvaluateGate, boot/live/retry callers, structured effect payloads, outage tests, B8a–e settle barriers, seeded model suite, and CI race workflow.

## Code

- `dispatch_settle.go` — stable effect payloads; test barrier hooks
- `dispatch_settle_wire.go` — evaluateSettleGate, scheduleSettleDrive, gate pass/block hooks
- `dispatch_live.go` — drivePendingSettlesOnBoot, maybeScheduleSettleAfterTerminal
- `interactive_service.go` — schedule settle after gate pass/block
- `gate_checkpoint_outage_test.go`, `dispatch_settle_barrier_test.go`, `dispatch_model_test.go`
- `.github/workflows/local-runner-race.yml`

## Explicit architecture choice

`resumePendingFlowGate` keeps gate evaluation (BUG-288 guards). SettleDriver owns durable SettlePhase + effect ledger after disposition is known. No full in-place rewrite of the 350-line gate resume body.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-251
change_type: feature
summary: Wire production SettleDriver EvaluateGate+retry; close Task-251/255 with barriers model CI
# --->8---
