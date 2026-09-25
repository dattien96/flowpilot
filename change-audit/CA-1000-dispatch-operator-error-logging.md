# CA-1000 — dispatch operator durable-failure logging

## Scope
`apps/local-runner/internal/runner/dispatch_operator.go` — `dispatchOperatorErr`.

## Finding
BUG-499's live drill (write-fault → `POST /client/workflow-runs/{runId}/dispatches/{turnId}/resolve`)
surfaced a visibility gap: the failed durable commit reached the HTTP caller as
`502 dispatch_operator_error` but left **no server-side log line**. Operator-visible
durability failures were diagnosable only by the client's report.

## Change
Log `[dispatch] operator mutation failed: %v` in the `default` (502) branch of
`dispatchOperatorErr` only. The 404 (not-found) and 409 (CAS/state conflict —
stale revision, illegal transition, repair-state mismatch) branches are expected
client-facing races and stay log-free to avoid noise.

## Verification
Live on build b500: write-fault drill repeated — `POST .../resolve` returns 502
and `/tmp/runner-b500.log` gains the `operator mutation failed` line.

## Risk
None — additive log line, no behavior change.
