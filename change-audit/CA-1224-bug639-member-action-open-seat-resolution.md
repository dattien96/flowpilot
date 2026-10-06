# CA-1224 — runner: BUG-639 memberAction resolves the open-cohort member instead of first label match

- **Area**: apps/local-runner/internal/runner (cohort_stall.go)
- **Evidence**: live run-306526 (vibe-tasks CP-04, Task-044), 2026-10-06
  ~05:24–05:45 — `member_stalled` parks fired serially on `tdd`,
  `spec_align`, `reviewer`. Every `memberAction` Retry/Skip dead-lettered:
  the run had 3 scaffold-architect, 6 spec-aligner and 7 reviewer children
  from earlier rounds, and the first label match always picked a
  historical sibling. The operator could only unblock by stopping the live
  leg via `agent-loop/stop` per leg.
- **Bug**: `handleMemberAction` resolved the actionable member by
  iterating `listChildren` oldest-first and taking the first
  `c.label == nodeID` — no status, cohort, or recency check. The entry
  was appended to that stale sibling's (often drained) cohort, so the
  real open seat stayed unfilled and the hub re-parked within seconds.
  Retry additionally revived dead duplicates (run-309960 flipped back to
  running).
- **Fix**: resolve via `openCohortSeatForLabelLocked(parentRunID, nodeID)`
  (the BUG-1194 seat lookup — newest-first, only cohorts whose barrier
  still holds an unconsumed seat for the label). Selection order:
  1. newest non-terminal same-label leg seated in the open cohort;
  2. newest terminal leg in that cohort (skip still appends the result to
     consume the released seat);
  3. fallback: newest live same-label leg anywhere, else any match
     (legacy error path preserved for genuinely missing members).
  The skip entry appends to the seat-holding cohort (`cohortID =
  seatCohortID`) so the barrier can actually join. Skip mutation is
  guarded by `childTerminal` — a terminal seated member keeps its
  terminal status (no Completed→Failed clobber) while the cohort append
  still consumes the seat.
- **Provider parity**: provider-agnostic — the change is member
  resolution + cohort-seat targeting inside the orchestrator; no adapter,
  event stream, session, or gate-hook behavior is touched. The devin legs
  observed live (claude/codex unaffected paths unchanged) and both added
  tests use Codex; no provider-specific branch exists in the edited code.
- **Tests (additive, reproduce-first)**:
  `bug639_member_action_duplicate_label_test.go` —
  - `TestMemberActionSkipTargetsLiveOpenCohortMember`: two `tdd` legs,
    old one failed in a drained cohort, live one running+silent in the
    open cohort. Pre-fix: live member stayed running and the open seat
    never filled (both assertions red). Post-fix: live member Failed,
    round-5 cohort drained, no open cohort remains.
  - `TestMemberActionRetryTargetsLiveOpenCohortMember`: retry refreshes
    the live member's `lastProviderEventAt` instead of the completed
    sibling's.
- **Verification**: `go test -count=1 -run
  'TestMemberAction(Skip|Retry)TargetsLiveOpenCohortMember'
  ./internal/runner/` — 2/2 pass; both red before the fix. Full
  `./internal/runner/` suite: 19 failures, all confirmed pre-existing by
  re-running the cohort/resume-related ones (TestReconstructResume*,
  TestRestartBetweenFreezeAndCoderPreservesContract) on the stashed
  pre-change tree — identical failures. Others are env-dependent
  (provider session/registry/gitnexus-bootstrap noise), untouched by this
  change.
- **Follow-ups captured in BUG-639 doc**: tournament dedupe refuses on a
  stale `run-*-tournament` child (tournament child liveness not checked);
  fresh devin legs produce zero events ~2 min post-spawn (suspected
  session-start failure, separate investigation); `agent-loop/stop` on a
  leg should release its cohort seat when the terminal path does not
  append.
- **Worktree**: n/a (runner-only change; no worktree mode involvement).
