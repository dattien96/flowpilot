# CA-1089: runner-owned `workflow_drift_events.json` counted as coder scope drift — flow gate self-parked on its own observability file

Date: 2026-10-01
Refs: live production run-3362 (vibe-tasks, PrivateVault) — repeated
`flow gate block: flow scope drift: wrote outside the frozen contract's
declared paths: .flowpilot/workflow_drift_events.json` → escalate →
owner-debate mount loop.

## Symptom

On the gated `tdd` child (run-15514, sprint 5/5), the post-turn flow gate
blocked with a scope-drift verdict whose only out-of-contract write was
`.flowpilot/workflow_drift_events.json`. The escalate mounted
`vibe-owner-debate` — which then wedged on the CA-1088 mount/stall race —
and every later unblock re-triggered the same false positive because the
file stays in the git diff window.

## Root cause

`internal/runner/gate_hook.go` (`driftEventsFileName =
"workflow_drift_events.json"`, Task-336 drift recorder) appends to
`.flowpilot/workflow_drift_events.json` **inside the same gate pass** that
diffs the working tree for coder writes. The file was not listed in the
frozen-scope runner-owned exclusion set, so the scope check classified the
runner's own bookkeeping append as an unscoped coder write.

Identical defect class as CA-649 (`gate-metrics.ndjson` self-park): the
gate parks itself on its own observability file.

## Fix (`internal/changecontract` only)

- `frozen_scope.go` `RunnerOwnedConfigPaths()`: added
  `.flowpilot/workflow_drift_events.json` to the set of paths the
  runner/gate harness itself writes mid-flow. Real out-of-scope product
  files still block — the exemption is exact-path only.

## Verification

- Red→green: `ca1089_drift_events_self_drift_test.go` —
  (a) the drift-events file alone does not produce a scope-drift verdict,
  (b) the path is recognized runner-owned,
  (c) a real unscoped code file still blocks.
- Live: the false-positive gate block was the trigger for the run-3362
  owner-debate wedge; with the exemption the gate no longer escalates on
  its own artifact.

## Files

- `apps/local-runner/internal/changecontract/frozen_scope.go`
- `apps/local-runner/internal/runner/ca1089_drift_events_self_drift_test.go`
