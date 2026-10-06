# CA-1229 — runner: BUG-641 reinvokeMatchingFlowChild never re-drives explicitly-ended legs

- **Area**: apps/local-runner/internal/runner (flow_executor.go)
- **Evidence**: live run-306526 (vibe-tasks CP-04, Task-045), 2026-10-06 —
  legs terminalized via `agent-loop/stop` re-appeared `running` /
  `waiting_user_approval` in the in-memory graph minutes later
  (run-348382 stopped → `stopped` → `running` ~4 min on; same for
  run-309960/run-402250). Each ghost blocked every hub reinvoke with
  `hub_reinvoke_start_failed("hub is parked while flow children are
  running or waiting")` — durable said stopped, memory said live, and
  `hasActiveFlowChild` reads memory.
- **Bug**: `reinvokeMatchingFlowChild` scans children newest-first and
  unconditionally re-stamps `status=running` (+ running summary) on the
  first match — and every caller's match predicate is label/agent-only.
  A continue back-edge or any re-drive therefore resurrected a stopped
  leg in place. Only `redriveDeadDelegateLeg` carried a local
  `Cancelled` skip; the other seven callers had none.
- **Fix**: inside the match loop, skip `child.status == RunStatusCancelled`
  and `child.legState == LegStateClosed`. Both marks mean "explicitly
  ended" (operator stop / member skip / dispatch-failed close / claim
  reclaim) — the designed re-drive for those is a fresh spawned leg that
  re-binds the open cohort seat (CA-645), which callers already do on a
  non-match. Completed legs remain matchable — the BUG-Rnd2 round-2+
  coder re-entry IS a designed completed→running transition — and Failed
  legs keep retry-in-place semantics (redriveDeadDelegateLeg).
- **Provider parity**: provider-agnostic — in-memory run bookkeeping only.
- **Tests (additive, reproduce-first)**:
  `bug641_cancelled_leg_reinvoke_test.go` —
  - `TestBug641_ReinvokeNeverResurrectsCancelledLeg`: live repro —
    operator-stopped leg must not match a label re-drive (red).
  - `TestBug641_ReinvokeNeverResurrectsClosedLeg`: member-skipped leg
    (Failed + LegStateClosed) must not match either (red).
  - `TestBug641_ReinvokePicksNewerLiveLegOverCancelledSibling`: a
    cancelled newest sibling must not shadow the older live same-label
    leg — scan skips to it (red).
  - `TestBug641_ReinvokeStillMatchesCompletedLeg`: BUG-Rnd2 contract —
    a Completed leg still re-drives in place (guard).
- **Verification**: `go test -count=1 -run 'TestBug641'` green; regression
  batch `-run 'Reinvoke|BackEdge|Continue|DeadDispatch|Redrive|
  SpawnContinue|BUG559|BUG569|BUG588|Cancel'` green (14.7s).
- **Worktree**: n/a (runner-only change).

## Review hardening (post-review pass)

- **Reviewer C-1 (Critical)**: the terminal-leg skip originally ran AFTER
  `match(child)` — but several callers' predicates have side effects:
  `rearmCohortIfDrainedLocked` re-opens a drained cohort
  (`preRegisterCohort` deletes the `cohortDrained` tombstone, sets
  expected=1), and another caller pre-registers + re-tags flowCohortId.
  A predicate firing on a leg the skip then refused to drive left a
  phantom open cohort — `hasOpenCohort` stayed true with nobody to fill
  it, blocking continue/done (`rejected_cohort_incomplete`) and
  suppressing dead-dispatch re-drives — the same zombie-barrier class as
  BUG-640. The terminal check now runs BEFORE `match()`.
- **Adjacent**: `resumeVerdictDeficientMembers`' `seen[label]` scan also
  counted cancelled/closed legs — a deficient member whose only leg was
  dead was marked seen, skipped by the re-drive, AND excluded from the
  fresh-spawn fallback → re-park loop. Dead legs now fall through to
  spawn. Regression: `TestBug565_ContinueSpawnsReviewerWhenOnlyLegIsClosed`.
- New regressions: `TestBug641_ReinvokeSkipsTerminalBeforeMatchNoPhantomCohort`,
  `TestBug641_ReinvokeSkipsCancelledBeforeMatchNoPhantomCohort` —
  predicate never runs on terminal legs and `hasOpenCohort` stays false.
