---
id: CA-953
title: BUG-456 frozen drift gate exempts build artifacts (bare-root go build binary)
type: bugfix
status: done
date: 2026-09-24
---

## Summary

Live run-6893 parked `test_signatures` on `flow scope drift: livebed` — a
bare-root binary the scaffold child emitted via `go build`. The frozen-scope
drift gate's exemption chain (exact paths per CA-427 Finding 2) had lost the
build-artifact clause that `flowgate.IsDocOrAuditFile` used to provide via
`IsBinaryOrBuildArtifact`. Restore it narrowly: exempt
`flowgate.IsBinaryOrBuildArtifact` in the coder-side filter.

## Why safe (CA-427 Finding 2 stays closed)

`IsBinaryOrBuildArtifact` matches only: binary extensions (.exe/.so/.test/
.o/.out/...), build-output dirs (bin/, dist/, build/, out/, target/, tmp/),
and extension-less files at repo root. It can never match `.flowpilot/**`
(always has `/` + a dotted filename) or any source file — gate-rules rewrites
and forged contract stores still drift.

## Files

- `internal/runner/gate_hook.go` — added `flowgate.IsBinaryOrBuildArtifact(p)`
  to the `codeOnlyWritten` exemption chain with BUG-456 comment.
- `internal/runner/bug456_build_artifact_drift_test.go` — 3 additive tests
  (2 RED before fix, 1 negative guard).

## Verify

- `go test -run 'TestGateDrift|TestFlowScopeDrift|TestFlowCoder|...'` green.
- Live: run-6893 unparked via artifact deletion + steer; the fix removes the
  need for that workaround permanently.
