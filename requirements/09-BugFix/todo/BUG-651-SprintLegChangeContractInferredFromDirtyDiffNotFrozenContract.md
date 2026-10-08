# BUG-651 — `prepareChangeContract` falls through to `InferFromDiff` for sprint legs: a dirty worktree mints a bogus `app-bootstrap`/`scope-none` contract under `enforce`, blocking every declared-path write — the step's FROZEN contract is never consulted

- **ID:** BUG-651
- **Severity:** Critical — root defect of the whole run-523131 Task-113
  wedge: 3 debate rounds, a wrong-contract park loop, operator
  re-stamp required. Any sprint after task 1 runs in a dirty worktree,
  so every leg can bind the wrong contract.
- **Status:** OPEN (captured live, run-523131; `contracts.ndjson` tail
  shows `inferred app-bootstrap paths:0` shadowing frozen `9c2fb307`)

## Evidence chain (all live)

1. Task-113 coder/scaffold legs parked `flow_gate_violation` — change
   contract resolved to feature `app-bootstrap`, intent
   `inferred docs/audit-only`, `declared_paths` EMPTY.
2. With `gate_mode: enforce` + empty scope, all 33 declared sprint paths
   were out-of-scope → every write blocked → park → debate →
   flag-requirement-change → resume → rebind SAME wrong contract → loop.
3. Frozen contract `9c2fb307` (feature-vault, 33 declared paths) existed
   and was ACTIVE the whole time — the binder simply never looked at it.
4. Operator repair: append a `declared` contract row (feature-vault, 33
   paths) to `contracts.ndjson`; `GetLatestForRun` is append-order
   last-wins → next turn bound correctly and writes unblocked.
5. Related family: BUG-425/BUG-439 (inferred contract metadata quality).
   This defect is different: inference should not run AT ALL when a
   frozen contract governs the step.

## Root cause

Contract binding order is `declared-in-prompt → InferFromDiff`; the
sprint's frozen contract (the authoritative scope for the leg) is absent
from the chain. `InferFromDiff` sees `.flowpilot/` bookkeeping + prior
task output and synthesizes a plausible-but-wrong contract.

## Fix direction

- `F-1` `prepareChangeContract`: if the run/step has an active frozen
  contract, the change contract IS the frozen scope — inference is for
  unfrozen work only.
- `F-2` If a frozen contract exists but is unbindable, fail closed with
  a `contract_bind_error` event — never silently degrade to inference.

## Regression coverage

- `TestBug651_FrozenContractBeatsInference` — dirty worktree + frozen
  scope → turn contract = frozen, `confidence: declared`.
- `TestBug651_InferenceOnlyWhenUnfrozen` — no frozen contract →
  inference path still works (near-miss guard).
