# CA-1150 — BUG-619: sprint boundary armed on unverifiable audit settle

Live trigger: BUG-616 rewind of run-150388. After restoring
`vibe_sprint_index: 2` and truncating the fabricated transition block, the
next boot still logged `audit -> DONE` +
`vibe_sprint_boundary_autostart`, re-seeded the sprint-3 topology and parked
a `contract-planner` spawn — the second time Task-032 was skipped while its
coder leg had never run.

Root cause: `maybeAutoAdvanceVibeSprintBoundary` armed the boundary
unconditionally. The CA-1096 evidence gate
(`vibeSprintEvidenceComplete` + `hasRunningSprintStep`) only wrapped the
`blocked_missing_feature_key` draft branch in the audit dispatch; the
general settle path — and the repark fallback at
`vibe_sprint_boundary.go:400` — called the advance with no proof the current
sprint's own legs finished. Any path that re-evaluates or re-settles the
audit node during resume could therefore stamp `audit DONE` and start the
next sprint on unverifiable work.

## Fix

The evidence gate moved *inside* `maybeAutoAdvanceVibeSprintBoundary`, ahead
of the lock: every advance now requires `vibeSprintEvidenceComplete` (no
open issues, every declared `agent.code` leg DONE across active+parked
topology — BUG-616) and no other sprint step still RUNNING. DONE/open-issue
state is terminal, so the pre-lock check cannot race into a false pass.
Callers keep their existing semantics; only unproven advances are refused.

## Tests (additive)

- `TestVibeSprintBoundaryRefusesUnfinishedLegs` — red before fix: with the
  sprint `coder` row still PENDING the boundary armed, stamped `audit DONE`
  and attempted the next-sprint `preflight_contract_plan` spawn. Green
  after: advance refused, `audit` stays untouched.
- Full `TestVibeSprintBoundary_*` family re-run — all existing boundary
  tests pass unchanged (their fixtures carry complete evidence).

## Files

- `apps/local-runner/internal/runner/vibe_sprint_boundary.go`
- `apps/local-runner/internal/runner/bug619_boundary_requires_evidence_test.go` (new)
