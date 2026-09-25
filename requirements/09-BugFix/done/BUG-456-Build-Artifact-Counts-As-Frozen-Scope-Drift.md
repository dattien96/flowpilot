# BUG-456: Build artifact counts as frozen-scope drift (bare-root `go build` binary parks the gate forever)

- status: done
- found: live run-6893 (post-rebase consolidated live test, task-harness on devin/swe-2-high)
- fixed_by: CA-953
- tests: internal/runner/bug456_build_artifact_drift_test.go

## Symptom (live)

Run-6893 (task-harness, cwd `/Users/tiendat/fp-beds/full`, frozen contract for
`str-utils` declaring `strutil.go` + `strutil_test.go`) reached
`test_signatures`. The scaffold-architect child wrote the stubs + RED tests,
then verified with a bare `go build` — which emitted a `livebed` binary at
repo root. Post-turn gate:

```
flow scope drift: wrote outside the frozen contract's declared paths: livebed
→ escalate → WAITING_USER_APPROVAL
```

Operator removed the binary and Continued; the re-invoked child ran `go build`
**again** (its normal verify habit), recreated `livebed`, and parked again.
`agent-loop/amend {paths:["livebed"]}` was correctly rejected — an
extension-less path is not a concrete code target — so the run had **no
contract-side resolution**: every verify pass re-creates the artifact, every
gate pass re-parks. Only an operator-side artifact deletion plus a steer
("never bare `go build`") broke the loop.

## Root cause

Regression from CA-427 Finding 2. The frozen-scope drift gate's
`codeOnlyWritten` filter (gate_hook.go) originally exempted via
`flowgate.IsDocOrAuditFile`, which covers `IsBinaryOrBuildArtifact`. CA-427
rightly replaced that broad filter with exact-path bookkeeping exemptions
(`.flowpilot/**`-wide exemption was a forge-the-gate hole) — but the binary
artifact clause was dropped with it.

Meanwhile the classifier exists precisely for this case
(flowgate.IsBinaryOrBuildArtifact: "bare root binaries like `gatesandbox`
from `go build`"), and the planner-side freeze guard
(`isFlowPlannerExcludedPath`) still exempts it via IsDocOrAuditFile —
asymmetric: planner tolerates artifacts, coder gate parks on them.

## Fix

Add `flowgate.IsBinaryOrBuildArtifact(p)` to the coder-side exemption chain in
`runChildArtifactOutputGateAtEpoch` (gate_hook.go), alongside the CA-427
exact-path exemptions. Security posture unchanged: the classifier only matches
binary extensions, build-output dirs, and extension-less files **at repo
root** — `.flowpilot/**` (contains `/` + `.`) and every source file still
drift; `TestGateDriftStillDetectsRewrittenGateRulesFile` /
`TestGateDriftStillBlocksExtensionlessSubdirFile` guard both sides.

## Tests

- `TestGateDriftIgnoresBareRootBuildBinary` — repro: declared `strutil.go` +
  root `livebed` binary → gate must not block. RED before fix.
- `TestGateDriftIgnoresBinaryExtensionsAndBuildDirs` — `.test` ext + `bin/` +
  `coverage.out`. RED before fix.
- `TestGateDriftStillBlocksExtensionlessSubdirFile` — `internal/payload`
  (extension-less but NOT root) must still drift. Guards the exemption stays
  narrow.

## Live evidence

- run-6893 transitions: `test_signatures` WAITING_USER_APPROVAL 02:47:32Z
  (drift `livebed`), re-invoked child rebuilt binary 09:47 local → park again,
  then after artifact deletion + feedback Continue: gate pass →
  `test_signatures` DONE → `implement` RUNNING 02:52:19Z. Implement leg hit
  the same park once more (child ran `go build` again) — the fix makes the
  artifact permanently invisible to the gate.
