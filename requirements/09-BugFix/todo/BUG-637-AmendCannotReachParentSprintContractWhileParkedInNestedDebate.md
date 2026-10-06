# BUG-637 — `agent-loop/amend` iterates only `activeFlowNodes` writer steps, so a park inside the nested owner-debate sub-flow cannot widen the parent sprint's frozen contract → `no_frozen_contract` 404 even though the contract exists

- **ID:** BUG-637
- **Severity:** High — RESCOPE debate verdicts are un-actionable through the
  only operator surface that can widen scope; the run dead-ends on a
  decision-form park whose correct resolution is a contract amendment.
- **Status:** FIXED — CA-1230 (2026-10-06): the endpoint enumerates
  `ListActiveForRun(runID)` instead of writer-nodes-of-mounted-flow, so
  the sprint's contracts are reachable while parked in the nested
  debate. Regression: `bug637_amend_nested_debate_test.go`.
  `FrozenContractRecord` + `superseded` status event to
  `.flowpilot/contracts/*.ndjson` (byte-compatible with
  `AmendFrozenContractForAllow`'s own write shape), then
  `flow-control:continue`.
- **Found:** run-306526 (`vibe-tasks` CP-04, Task-044 sprint, round 3),
  2026-10-06 ~04:40. Owner debate resolved RESCOPE — contract v6 declared
  only `core/security-rasp/src/main/cpp/src/RaspBridge.cpp` but the task's
  own intent requires wiring it into `rasp_native` via
  `core/security-rasp/src/main/cpp/CMakeLists.txt`. The leg literally
  cannot satisfy the contract without editing a path the contract forbids.

## Symptom

```
POST /client/workflow-runs/run-306526/agent-loop/amend
{"paths":["core/security-rasp/src/main/cpp/CMakeLists.txt"]}
→ 404 {"code":"no_frozen_contract",
      "message":"no frozen contract of this run needed widening"}
```

…while `frozen_contracts.ndjson` holds TWO active v6 records for the run
(`coder_step_id: tdd` `74171e55…` and `coder_step_id: coder`
`2e748f4f…`), both missing the path.

## Defect

`handleAmendFlow` (interactive_handlers.go:2503) iterates
`flowAgentCodeWriterNodes(rs.activeFlowNodes)` →
`store.GetFrozenForStep(runID, wn.ID)`. When the run is parked **inside
the mounted owner-debate sub-flow**, `activeFlowNodes` is the debate
graph (`owner_1`, `owner_2`, `debate_synthesis`…) — no code-writer node —
so the loop finds zero contracts to amend and returns the misleading
`no_frozen_contract` 404. The sprint writer contracts for the parent flow
are one scope-level up and unreachable from the endpoint.

Same visibility gap class as the mux-lane work: state keyed to *one*
active node list while a nested sub-flow owns the park. Any verdict whose
fix is "the contract was wrong" (the canonical RESCOPE outcome) is
unactionable until the park unwinds to a sprint node — which never
happens, because the park is the decision surface.

## Fix direction

- The amend target set should be **all active frozen contracts for the
  run** (`store` lookup by runID across step keys — the contract rows
  already carry `run_id`), not writer-nodes-of-the-current-flow. Union
  semantics already exist; only the record enumeration is wrong.
- Alternatively walk the flow stack: if `activeFlowNodes` has no writers,
  fall back to the enclosing sprint flow's nodes.
- The 404 message should distinguish "no contract exists" from "no
  writer node in the currently-mounted flow" — the current text sent the
  operator down a wrong rabbit hole (contract store corruption? wrong
  workspace?) when the store was fine.

## Evidence

- `tail -3 .flowpilot/contracts/frozen_contracts.ndjson` (worktree
  feat-cp04): v6 `tdd`/`coder` records, declared = `[RaspBridge.cpp]`.
- Gate reason (agent-graph `loopState.gateReason`, 04:40): "resolving in
  favor of owner_1's RESCOPE … contract v6: its own intent requires
  wiring RaspBridge.cpp into rasp_native (CMakeLists.txt edit) but
  declared_paths contain ONLY RaspBridge.cpp with no allowed_extra_paths".
- Amend call → `no_frozen_contract`; after hand-appended v7 records
  (coder `0071e167…`, tdd `33eb7194…`) + `flow-control:continue` the loop
  resumed (`running`, round 4) — proving the contract itself was the only
  blocker.
