# CA-1230 — runner: BUG-637 agent-loop/amend enumerates the run's active contracts, not the mounted flow's writer nodes

- **Area**: apps/local-runner/internal/runner (interactive_handlers.go)
- **Evidence**: live run-306526 (vibe-tasks CP-04, Task-044 sprint, round
  3), 2026-10-06 — owner debate resolved RESCOPE ("declare
  CMakeLists.txt in scope"), but the run was parked INSIDE the mounted
  debate sub-flow, so `activeFlowNodes` was `owner_1/owner_2/
  debate_synthesis` — zero `agent.code` writers. `handleAmendFlow`
  iterated `flowAgentCodeWriterNodes(activeFlowNodes)`, found no
  contracts, and 404'd `no_frozen_contract` while two active v6 records
  (tdd + coder) sat in `frozen_contracts.ndjson`. Operator had to
  hand-append v7 records to the ndjson to unblock.
- **Bug**: contract enumeration was keyed to the currently-mounted flow
  topology instead of the run — the same mux-lane visibility class:
  state keyed to one active node list while a nested sub-flow owns the
  park.
- **Fix**: enumerate `store.ListActiveForRun(runID)` — the store's
  designed per-step-active enumeration (same record set
  `GetFrozenForStep` returns per writer node, deduplicated by step,
  superseded/abandoned excluded). Amend semantics unchanged: every
  active contract of the run is union-widened, and a record whose
  declared paths already cover the request produces no new version
  (amended counter unchanged → honest 404 persists for the
  nothing-to-widen case).
- **Provider parity**: provider-agnostic — endpoint/store logic only.
- **Tests (additive, reproduce-first)**:
  `bug637_amend_nested_debate_test.go` —
  - `TestBug637_AmendReachesParentContractDuringNestedDebatePark`: live
    repro — parked with debate-graph activeFlowNodes while coder+tdd
    contracts are active; amend must widen both and resume (red before
    fix: 404 no_frozen_contract).
  - `TestBug637_AmendStill404WhenNoContractExists`: a contractless park
    keeps the honest 404 (guard).
- **Verification**: `go test -count=1 -run 'TestBug637|TestAmendFlow'`
  green — all existing amend tests pass unchanged (union semantics,
  unamendable-path reporting, not-parked/empty-path rejections
  preserved). Full `./internal/runner/` suite: only the known
  pre-existing/env classes fail (BUG-334 opencode PATH, Firebase/
  Supabase env, ReconstructResume deterministic pre-existing — all
  confirmed identical on the untouched tree).
- **Worktree**: n/a (runner-only change).
