---
id: CA-954
title: BUG-457 frozen drift gate exempts runner .flowpilot/logs/** diag output
type: bugfix
status: done
date: 2026-09-24
---

## Summary

run-13080's implement gate parked on
`.flowpilot/logs/features/agent-flow-engine/run-13080.ndjson` — the runner's
own per-feature diag log appended mid-turn. Append-only → fingerprint
subtraction never matches → every frozen writer self-parks. Added
`IsRunnerLogsBookkeepingPath` (narrow `.flowpilot/logs/` prefix) to the
exemption chain — same class as CA-649's gate-metrics fix.

## Files

- `internal/changecontract/frozen_scope.go` — new `IsRunnerLogsBookkeepingPath`.
- `internal/runner/gate_hook.go` — added to `codeOnlyWritten` filter.
- `internal/runner/bug456_build_artifact_drift_test.go` — additive
  `TestGateDriftIgnoresRunnerDiagLog` (RED before fix).

## Verify

- `go test -run TestGateDrift` green incl. negative guards (gate-rules rewrite
  and extension-less subdir file still drift).
