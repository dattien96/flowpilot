# CA-342: CP-51 P1 — live Stop wired to durable RunStopState fence (Task-249)

## Scope

Close the headline Task-249 gap found in the 2026-07-17 audit (CA-341): the durable Stop path (`requestRunStopV2`) had zero callers, so the root `ErrRunStopFence`/parent `ErrParentStopFence` (INV-3) never fired in production — only tests triggered it.

## Changes

- `apps/local-runner/internal/runner/interactive_service.go` — `stopAgentLoop` now calls `requestRunStopV2` for the parent run and every child at the top of the handler, before any RAM cancel, so a concurrent `send_claimed→send_started` CAS is fenced in revision order.
- `apps/local-runner/internal/runner/dispatch_live.go` — `requestRunStopV2` now no-ops for runs without V2 activation (`GetRunProtocolVersion < V2`) so stopping a never-dispatched run does not spuriously create activation/stop state.
- `apps/local-runner/internal/runner/stop_race_barrier_test.go` (new) — `TestLiveStop_AdvancesRunStopState_FencesRootSend`, `TestLiveStop_ParentStopFencesChildSend`, `TestLiveStop_NoOpForNonV2Run`.
- `apps/local-runner/internal/runner/interactive_service_test.go` — `TestStopAgentLoopCancelsParentTurn` updated to correct CP-51 zero-send-on-Stop semantics: a Stop that wins the pre-send linearization means the turn is never sent (adapter never runs). The test now accepts both correct outcomes — adapter observed cancellation OR run settled cancelled/not-in-flight — instead of requiring the adapter to run. (Test encoded the pre-CP-51 spec; updated to match the deliberately-changed INV-3 spec.)

## Verification

- `go build ./...` clean; `go vet ./internal/runner` clean.
- `TestLiveStop_*`, `TestStopAgentLoopCancelsParentTurn` (now passing, previously failing), `TestStopCASBeforeSendStarted`, `TestPreSendStop*` green.
- Pre-existing unrelated failures remain (Windows `dispatch.lock` flock not released before `t.TempDir` cleanup in `TestCrashMatrix_MultiProjectShardAndExportImport` / `TestDispatchStore_ContractSuite`) — test-hygiene gap, not caused by this change; noted for Task-248/258 cleanup.

## Not in this change (remaining Task-249 work)

- T-6 removal of the RAM clear path (`durableIdemReplaySafe`/`durableIntentClearOK`) — deferred until Task-254 authority-inversion (`FindActiveByOuterIntent` wired into `startTurn`) lands, else R16/R20 fixes regress.
- Remaining §4.2 seam tests; fence coverage on local NDJSON + real-PG for `RSF`/`PS`.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-249
change_type: bugfix
summary: Wire live Stop (stopAgentLoop) to durable RunStopState so root/parent send-fence (INV-3) fires in production; add stop_race_barrier_test
# --->8---
