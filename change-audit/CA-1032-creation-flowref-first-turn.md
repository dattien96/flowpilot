# CA-1032 — creation-time FlowRef engages the flow engine on the first turn

Date: 2026-09-26 — review finding: a run created with `flowRef` never ran the flow

## Change

`apps/local-runner/internal/runner/flow_executor.go`:

- `resolveWorkflowFlowRef` (the single `handleStartTurn` resolver) now falls
  back to `rs.chatFlowRef` on a root run when `workflowID` is empty. The
  turn-level mounting rules are unchanged: first turn only (`turnCount==0`),
  restored runs still bail, the resolved ref still flows through
  `FlowAllowedForWorkingMode` and `startTurn`'s `flowStartOnly` /
  `startResolvedFlow` path.

## Why

`createRun` stamps `StartRunInput.FlowRef` on `rs.chatFlowRef` — that is the
creation-time mount — but `resolveWorkflowFlowRef` consulted only
`rs.workflowID`, so a run created via `POST /client/workflow-runs` with
`flowRef` and no `workflowId` resolved nothing: the first turn ran a plain
provider chat and the mounted flow never spawned a node.

Children are excluded explicitly (`parentRunID == ""`): their `chatFlowRef`
arrives via `FlowRefFallback` (BUG-506 propagation), which is documented as
"not a mount" — a child must never re-enter `startResolvedFlow`.

## Tests

- `TestFlowRefAtRunCreateStartsFlowOnFirstTurn` — real HTTP entry:
  `POST /client/workflow-runs {"flowRef": <ref>}` then
  `POST .../turns {"prompt": ...}` (no flowRef in the turn body); asserts the
  entry delegate child spawns and the run is marked flow-engine-driven.
  Verified RED pre-fix (`bailing, no workflowID` → plain chat turn).
- Existing resolver guards re-verified: BUG-315 restore bail, BUG-174
  workflow-picker adoption, BUG-261/BUG-270 invalid-ref surfaces,
  BUG-506 fallback propagation.

## Risk

Low: the fallback only widens resolution for root runs whose creation input
explicitly carried a flowRef; every prior bail condition (non-first turn,
restored runs, unresolvable/invalid definition, no spawnable entry node,
working-mode gate) is untouched.
