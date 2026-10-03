# CA-1152 — BUG-620: frozen-contract resolution scoped to the sprint's task doc

Live trigger: run-150388 rewind (BUG-616/619 surgery). The fabricated
sprint-3 preflight froze a Task-033 contract at 11:25Z; when sprint 2 was
honestly rewound to Task-032, the re-driven `tdd` leg still read "the frozen
contract" — i.e. the ledger head — and appended a Task-033 ProtectedKeyGuard
signature block into `tdd-signatures.md` while sitting in Task-032's sprint.

Root cause: every frozen-contract resolution path picked the **newest
active record per step key**. Vibe sprints reuse the same node ids
(`tdd`/`coder`/…) every task, so sprint N+1's freeze shadows sprint N's
under `(runID, "coder")` — the same keying hole as BUG-616's
session→node projection and BUG-619's boundary evidence, one layer deeper.

## Fix (selection semantics, not schema)

- `FrozenStore.ListActiveForStep` (new, changecontract): active versions
  newest-first — lets callers reach *past* the newest active record to an
  older doc-matching one.
- `latestContractForRunScoped(workspace, runID, wantDocID)`: when
  `wantDocID` names a doc and the newest active record for a step belongs
  to another doc, scan that step's active versions and prefer the newest
  **matching** record across all steps. No match → legacy newest-active
  (fail-open: sprint-1-era records and non-sprint runs unchanged).
  `latestContractForRun` delegates with `""` — byte-identical behavior.
- `InteractiveService.frozenContractForRun`: same preference applied at the
  writer-node loop and the restart `ListForRun` fallback, keyed by
  `vibeSprintCurrentTaskDocID(runID)` (new helper: `Task-NNN` from
  `vibeTaskPlan[vibeSprintIndex-1]` under sprint topology).
- Leg-visible contract sections get the same scope:
  `FlowContextHints.PreferredContractDocID` → `change.contract` /
  `source.dependence` sources resolve via the scoped variant; populated at
  the delegate-spawn hints, the freeze-chain package builder, the handoff
  slow path, and the inline-entry dispatch (payload key
  `preferredContractDocId`).

Provider-agnostic: read-path selection only; no adapter/event code.

## Tests (additive — red before fix)

- `TestBUG620_LatestContractPrefersSprintTaskDoc` — scoped resolution
  returns the sprint's own doc-matched record despite a newer foreign
  version; unscoped call keeps newest-active.
- `TestBUG620_FrozenContractForRunPrefersSprintTaskDoc` — armed sprint-2
  run resolves Task-032's record over the newer Task-033 one.
- `TestBUG620_NoSprintDocMatchFallsBackToNewest` — no matching doc keeps
  legacy fallback.
- CA-1151/1092/1096/1087 + BUG-616/617/618/619 + VibeSprintBoundary families
  re-run green; vet clean. The 6 gate-mode failures in the broad run
  (`gateMode="warn"` where fixtures expect enforce) reproduce identically
  at HEAD `82a6c90e` in a clean worktree — same pre-existing baseline
  breakage family as `TestRun200816FlowHubGateSkipsTaskDocReprompt`, not a
  regression of this change.

## Files

- `apps/local-runner/internal/changecontract/preflight.go`
- `apps/local-runner/internal/runner/contract_for_context.go`
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`
- `apps/local-runner/internal/runner/flow_context_package.go`
- `apps/local-runner/internal/runner/context_source_change_contract.go`
- `apps/local-runner/internal/runner/context_source_dependence.go`
- `apps/local-runner/internal/runner/flow_context_handoff.go`
- `apps/local-runner/internal/runner/flow_executor.go`
- `apps/local-runner/internal/runner/behavior_registry_builtin.go`
- `apps/local-runner/internal/runner/bug620_frozen_contract_sprint_scope_test.go` (new)
