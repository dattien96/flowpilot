# CA-1228 — runner: BUG-641 wait heals un-stamp the mirrored flow step

- **Area**: apps/local-runner/internal/runner (flow_step_runtime.go,
  wedge_sweep.go, interactive_service.go)
- **Evidence**: live run-306526 (vibe-tasks CP-04, Task-045), 2026-10-06 —
  a labeled child's wait stamps its parent flow step
  `WAITING_USER_APPROVAL` via `settleFlowChildStepAwaitingUserLocked`.
  When the backing wait later resolved or proved phantom, the leg healed
  (`orphaned_wait_healed` fired 4×; mirrored-question heal cleared sibling
  legs) but the **step row stayed parked** — desktop kept painting
  "Waiting: question" with no card and the hub kept answering
  `hub_reinvoke_start_failed("children waiting")` long after every leg was
  running. `GET /questions` was `[]`; operator had to discharge via
  `agent-loop/continue` each time.
- **Bug**: both heal paths repaired only `interactiveRun.status` /
  `agentStatus`; nothing un-stamped the durable step row the wait had
  mirrored onto. `WAITING_USER_APPROVAL` became reachable without a
  resolvable question/approval — the doc's stated invariant violation.
- **Fix**: new `unstampHealedFlowChildStepLocked(rs)` — the mirror-image
  of `settleFlowChildStepAwaitingUserLocked` (same guards: labeled child
  of a flow-engine-driven parent). Called from:
  1. `healOrphanedWait` (wedge sweep) inside the `waitOrphan` arm under
     `s.mu`;
  2. `healMirroredQuestionWaitLocked` when a mirror target returns to
     running (under `s.mu`).
  Only a step still in `WAITING_USER_APPROVAL` is un-stamped, so a
  re-stamp by a genuine decision between scan and heal is preserved.
  Heals emit `orphaned_step_wait_healed` diag on the parent run.
  Designed parks are untouched: card-backed waits exit before the arm,
  and `waiting_user_approval` under a `blocked` parent loop still counts
  as owner-owned.
- **Deferred**: the doc's second arm — stopped legs rehydrating to
  `running` in memory (ghost legs re-blocking "children settled"). Separate
  mechanism (resume/reconcile rebuild), tracked in the BUG-641 doc.
- **Provider parity**: provider-agnostic — status bookkeeping only; no
  adapter, event stream, or session code touched.
- **Tests (additive, reproduce-first)**:
  `bug641_orphaned_step_wait_test.go` —
  - `TestBug641_OrphanedWaitHealClearsMirroredStepStamp`: live repro —
    stale card-less wait heals the leg AND the step (red before fix).
  - `TestBug641_MirroredWaitHealClearsStepStamp`: question-resolve mirror
    heal un-stamps the sibling leg's step (red before fix).
  - `TestBug641_SweepKeepsStepWaitBackedByCard`: pending question keeps
    leg + step parked (guard).
  - `TestBug641_SweepKeepsOwnerParkedWaitAndStep`: `waiting_user_approval`
    under blocked parent loop stays (guard).
- **Verification**: `go test -count=1 -run 'TestBug641'` green; regression
  batch `-run 'Wedge|Mirror|CA1207|Orphan|FlowStep|SettleFlow|StepStatus|
  Phantom|Heal'` green. Impact: `healOrphanedWait` /
  `healMirroredQuestionWaitLocked` unresolved by GitNexus (index 128+
  commits stale; Go walk returned UNKNOWN) — callers confirmed by text
  search (sweep + 4 question-resolve sites, all additive).
- **Worktree**: n/a (runner-only change).

## Review hardening (post-review pass)

- **Reviewer I-2**: the step stamp keys on node id (= label) — with
  duplicate labels (BUG-639 class) a different sibling leg may own the
  live park. `unstampHealedFlowChildStepLocked` now scans same-label
  siblings before un-stamping: any sibling carrying a pending
  approval/question/card or a waiting_* status owns the stamp and blocks
  the heal. Regression: `TestBug641_HealKeepsStepStampOwnedBySiblingLeg`.
  Residual: the store write path is last-write-wins with no CAS — a
  nanosecond-scale window between lookup and write remains vs. unlocked
  step stampers; self-heals on the next step transition.
- **Reviewer I-3**: `healMirroredQuestionWaitLocked` flipped mirror
  targets' `rs.status` in RAM only — a restart could rehydrate the stale
  `waiting_question` while the step row already read running. Healed
  targets now persist via detached `persistProviderSession` (the funnel
  is documented safe under s.mu for child runs).
