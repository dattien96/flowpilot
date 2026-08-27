# CA-647 — live amend trigger for frozen contracts (CP-43 F3 / CP-55 P-4 wiring)

## What

CP-43 F3 (planner declares narrow scope → coder drifts → amend → retry) had no
live path for its third leg: `changecontract.AmendFrozenContract` existed and
was unit-tested (Task-266, CP-55 P-4) but **no runner endpoint or flow-control
action ever called it**. A flow parked on `FrozenContractScopeDrift` could only
Continue (re-block, same drift) or Stop — the operator could not widen the
frozen contract and let the retry pass.

F2 live (run-153698) proved the park works end-to-end; F3 additionally needs
the amend action.

## Fix

`apps/local-runner/internal/runner/interactive_handlers.go`:

- New route `POST /client/workflow-runs/{runId}/agent-loop/amend`,
  body `{"paths": ["user.go"]}`.
- `handleAmendFlow`:
  - rejects non-parked runs (409 `flow_not_blocked`), blank/missing paths
    (400), runs not found (404);
  - opens the run's `FrozenStore`, then for EVERY `agent.code` writer node of
    the run calls `AmendFrozenContract` with the additional paths — union
    semantics, mints version+1 with `Supersedes` on the prior, no-op (no new
    version) when the paths are already declared;
  - 404 `no_frozen_contract` when nothing needed widening;
  - `AmendFrozenContract`'s own non-concrete-path rejection surfaces as 422;
  - on success resumes the parked flow (`resumeFlowWithFeedback`) so the
    retried writer runs against the widened contract.

Provider-agnostic (Case 1): no `ProviderKey` branch in the handler.

Will not undo: CA-427 frozen-scope enforcement (amend widens the contract
explicitly, it does not weaken the gate), CA-640/645 exclusion sets,
Task-266 amendment unit contract.

## Tests

Additive only — legacy suites untouched.

- `apps/local-runner/internal/runner/amend_flow_endpoint_test.go` (new):
  - `TestAmendFlow_WidensContractAndResumesParkedFlow` — parked flow + frozen
    v1 `[src/calc.go]` → amend `["src/user.go"]` → 200, v2 minted with
    `Supersedes`, both paths declared, flow unparked.
  - `TestAmendFlow_RejectsWhenFlowNotParked` — running flow → 409.
  - `TestAmendFlow_RejectsEmptyAndMissingPaths` — 400.
  - `TestAmendFlow_RejectsNonConcretePath` — `Makefile` → 422.
  - `TestAmendFlow_NoWideningNeededReturns404` — already-declared path → 404,
    no version minted.
  - `TestAmendFlow_UnknownRunReturns404`.

## Verification

- `go test ./internal/runner -run 'TestAmendFlow|TestRun151954|TestRun243681|TestRunContractFreezeNode|TestFlowScopeDriftBlocks|TestFlowCoderUsesFrozenScope|TestContractFreeze|TestFlowFrozen|TestFlowCoderComputesWrittenPaths|TestAgentGraphRoutes|TestNormalChat' -count=1` → green.
  - note: `TestRunContractFreezeNodeBindsContractToCoderStep` flaked once in a
    batch (passed isolated + on reruns; pre-existing timing flake, unrelated).
- `go vet ./internal/runner` clean.

## Manual (operator tick — CP-43 F3)

1. Run /flow rag-harness with the F3 prompt; when the coder drifts the flow
   parks WAITING_USER_APPROVAL naming the drifted path.
2. `Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:4317/client/workflow-runs/<runId>/agent-loop/amend" -ContentType "application/json" -Body '{"paths":["user.go"]}'`
3. Flow resumes; retried coder passes; frozen contract v2 + Supersedes visible
   in `frozen_contracts.ndjson`.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-43
change_type: bugfix
summary: wire live amend trigger for frozen contracts — POST /client/workflow-runs/{runId}/agent-loop/amend widens every active frozen contract of a parked run (version+1, Supersedes, union paths) then resumes the flow, so CP-43 F3 (block → amend → retry) works end-to-end instead of only Continue/Stop
# --->8---