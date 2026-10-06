# CA-1233 — runner+desktop: second-pass review hardening (verdict flags on driven leg, closed-leg ghosts, amend scope, phantom pre-register)

- **Area**: apps/local-runner/internal/runner (review_done_verdict.go,
  cohort_stall.go, wedge_sweep.go, flow_executor.go,
  interactive_handlers.go, gate_hook.go),
  apps/desktop-flowpilot/src/components/FlowAwaitingUserCard.tsx
- **Evidence**: independent second-pass review of ffc3d129 (CA-1224..1232)
  — no Criticals, four Importants verified against live code paths.
- **Findings & fixes**:
  - **I-1**: `resumeVerdictDeficientMembers` armed
    `verdictRepromptInFlight` in its scan loop; `reinvokeMatchingFlowChild`
    then cleared the flag as a stale marker before the turn ran — the
    BUG-559 draft shield was dead code on the deficient-member path
    (run-60899 wedge class). The marker now re-arms on the driven leg
    after the helper returns.
  - **I-2**: the scan armed `reinvokeInFlight`/`verdictRepromptCount` on
    the FIRST live leg in spawn order while the drive scans NEWEST-first
    — duplicate live labels put the flags on a stale sibling (same
    first-vs-newest class as BUG-639). All flag writes moved inside the
    match predicate, which runs under s.mu on the leg actually driven.
  - **I-3**: `openCohortMemberRuns`, the memberAction `pick` liveness
    check, and `deadDispatchNonTerminalChildExists` filtered terminal
    *statuses* only — `status=running + legState=closed` ghosts (quota-veto
    respawn, claim reclaim) counted as live. The BUG-643 DOA arm made
    them stall-visible for the first time: a ghost could park
    `member_stalled` while its live successor worked, or suppress the
    dead-dispatch spawn that would fill the seat. All three sites now
    exclude `LegStateClosed`.
  - **I-4**: `handleAmendFlow`'s BUG-637 enumeration (ListActiveForRun)
    sprayed the path union into EVERY active contract of the run —
    sibling steps' frozen scopes silently widened. New
    `filterAmendTargets` scopes to contracts for gated steps (a child's
    post-turn gate raised the card), then live steps, then the full set —
    the last tier preserves BUG-637 nested-debate reachability.
  - **M-1**: `reinvokeExistingFlowChild` pre-registered the cohort before
    knowing a live leg existed — a no-match + failed spawn fallback left
    a phantom open barrier (same class as C-1 one layer up).
    preRegisterCohort moved inside the match predicate.
  - **M-2 desktop**: `handleAllow` had no catch (rejected amend = silent
    dead click — now surfaces `role=alert`); `unamendablePaths` was
    decoded but never rendered (now shown under the amend row); amend
    error/input state persisted across hide→re-block cycles (reset when
    blockReason|gateReason changes).
  - **M-3**: `unchangedSinceTurnStart` lacked `filepath.Clean` — "./x"
    aliases missed the fingerprint arm; canonicalized with a fail-closed
    guard for empty/root results.
- **Tests**: review2_regression_test.go — 8 cases covering every finding
  (flags on driven leg + marker survives, ghost stall/member-action/
  dead-dispatch exclusion, no-match → no phantom cohort, live match →
  registers + re-tags, filterAmendTargets tiers, alias/empty path).
- **Provider parity**: provider-agnostic bookkeeping/UI; no adapter,
  event stream, or gate-hook provider branch touched.
