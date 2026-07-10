# CA-272 — Flow Run Shows "Cancelled" on App Start Instead of "Completed"

## Bug

After running a flow that includes a Google Drive question (context source node),
restarting the desktop app shows the run as "Cancelled" in history. After switching
between chat views a few times, it eventually shows "Completed".

## Root Cause

Two separate defects combined to cause this:

### Defect 1: History list used wrong normalizer (primary)

`projectRunHistory()` in `interactive_handlers.go` used `normalizeResumedStatus(sess.Status)`
for persisted-only sessions. This function only knows about raw run status and converts
any in-flight status (`running`, `waiting_question`, etc.) to `cancelled`.

It should use `normalizeResumedFlowStatus(sess)` which also checks
`LoopState.Status == "done"` — returning `completed` for flows that finished
even if the raw status is stale.

### Defect 2: Race between AnswerQuestion persist and flow-done persist

The `flowStartOnly` path (when a flow is started without a provider turn) persists
`status = completed` synchronously. But when the user answers the Drive question,
`AnswerQuestion` persists `status = running` (overwriting). The final
`applyFlowControl("done")` persist was launched as `go persistParentSession(...)` —
an async goroutine that could fail to execute before the process stopped.

Result: last NDJSON record = `{status: "running"}` → `normalizeResumedStatus` → `"cancelled"`.

## Fix

### 1. `interactive_handlers.go` — `projectRunHistory()`

Changed:
```go
Status: normalizeResumedStatus(sess.Status),
```
To:
```go
Status: normalizeResumedFlowStatus(sess),
```

`normalizeResumedFlowStatus` correctly returns `completed` for any session where
`ActiveFlowNodes` is non-empty and `LoopState.Status == "done"`, regardless of the
raw persisted status field.

### 2. `interactive_service.go` — `applyFlowControl("done")`

Changed `go s.persistParentSession(parentRunID)` to a synchronous call so the
completed state is guaranteed written to disk before any subsequent goroutine
can observe a stale status.

## Tests

- **`TestProjectRunHistoryShowsCompletedForDoneFlowWithStalePersistStatus`** (new):
  seeds a persisted session with `status=running` + `LoopState.Status="done"`,
  calls `projectRunHistory` WITHOUT resuming the run, and asserts the result is `completed`.

## Files Changed

| File | Change |
|------|--------|
| `apps/local-runner/internal/runner/interactive_handlers.go` | Use `normalizeResumedFlowStatus` in `projectRunHistory` |
| `apps/local-runner/internal/runner/interactive_service.go` | Make flow-done persist synchronous |
| `apps/local-runner/internal/runner/interactive_service_test.go` | Add `TestProjectRunHistoryShowsCompletedForDoneFlowWithStalePersistStatus` |

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CA-272
change_type: bugfix
summary: flow run history uses normalizeResumedFlowStatus (checks LoopState.Status=="done") instead of normalizeResumedStatus, and persists the flow-done state synchronously, so a completed flow no longer shows as "cancelled" after an app restart
# --->8---
