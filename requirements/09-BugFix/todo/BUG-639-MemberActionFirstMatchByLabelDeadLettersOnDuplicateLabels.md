# BUG-639 — `handleMemberAction` resolves the stalled member by first label match; on duplicate labels (every later round) Retry/Skip are dead letters and the hub re-parks forever

- **ID:** BUG-639
- **Severity:** Critical — member_stalled parks cannot be discharged by the
  card actions they render. Any parent whose flow re-uses a node label
  across rounds/cohorts (the normal sprint shape: `tdd`, `spec_align`,
  `reviewer` recur every round) dead-ends the operator; observed live for
  ~50 minutes on run-306526.
- **Status:** FIXED — memberAction resolution via open-cohort seat
  (CA-1224); tournament dedupe now ignores terminal/closed children so a
  fresh rescue escalation can spawn (CA-1226,
  `liveTournamentChildLocked`). Regression tests:
  `bug639_member_action_duplicate_label_test.go`,
  `bug639_tournament_dedupe_terminal_test.go`.
- **Found:** run-306526 (`vibe-tasks` CP-04, Task-044), 2026-10-06
  05:24–05:45. Member-stall parks fired serially on `tdd`, `spec_align`,
  `reviewer`.

## Defect

`handleMemberAction` (internal/runner/cohort_stall.go ~L355-367) locates the
actionable member by iterating `listChildren(parentRunID)` and taking the
**first** child whose `c.label == nodeID` — no status check, no cohort
check, no recency ordering:

```go
for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
    c := s.runs[childID]
    if c != nil && c.label == nodeID {
        child = c; cohortID = c.flowCohortId; break
    }
}
```

`listChildren` returns oldest-first insertion order, so the match is always
the earliest historical leg carrying that label — typically a leg from a
previous round that is `completed`/`failed`/`cancelled`, or a ghost leg
left `running` by an older dispatch. The truly stalled member (the one
`openCohortMemberRuns` flagged) is only ever reached when it happens to be
the first label bearer.

Observed live (run-306526 agents list): 7 `reviewer` children, 6
`spec-aligner`, 3 `scaffold-architect` — one fresh leg per cohort plus
historical terminals. Every `memberAction{node:"tdd"}` hit run-309960
(durable `failed`, later revived to `running` in-memory by the retry turn)
while the actual stalled member run-402250 stayed untouched; same for
`spec_align` (hit run-348298 instead of run-421405) and `reviewer` (would
have hit run-348293 instead of run-421410).

## Compounding observations (same live window)

1. **Retry resurrects dead duplicates.** The retry branch sends a fresh
   turn to the matched (wrong) leg; run-309960 flipped back to in-memory
   `running` and re-entered the stall pool.
2. **`tournament_escalation_deduped` refuses forever.** A tournament child
   `run-306526-tournament` persisted `running` from an earlier escalation,
   so every subsequent stall refused to spawn rescue. The dedupe check
   evidently does not require the existing tournament child to be
   non-terminal/live-progressing.
3. **`dead_dispatch_leg_reinvoked` works but is unreliable** — it revived
   `tdd` once (step transition RUNNING at 05:36:07) yet did not fire for
   spec_align/reviewer.
4. **Fresh legs die silently ~2 min after spawn.** spec-aligner run-421405
   and reviewer run-421410 (spawned 05:24:20 via
   `flow-auto-validate-round-5`) produced no events past ~05:26 — devin
   provider sessions appear DOA; worth its own investigation into the
   spawn path (provider-account admission? session start failure not
   surfaced as a leg event?).
5. **`flow_control_rejected_cohort_incomplete`** rejects plain
   `flow-control: continue` while any cohort is open — so once memberAction
   dead-letters, no supported surface remains except per-leg `agent-loop/
   stop`.

## Root cause direction

`openCohortMemberRuns`/`checkAndBlockStalledMembers` already know the real
stalled member identity (the child whose cohort seat is open and whose
`lastProviderEventAt` is stale). `handleMemberAction` ignores that and
re-derives the member by label. The selection must instead prefer, in
order:

1. The child in an **open cohort** holding the label's open seat
   (`openCohortSeatForLabelLocked(parentRunID, label)` already implements
   newest-first lookup — reuse it).
2. Among same-label children, the **non-terminal** one
   (`status not in {completed, failed, cancelled}`) that is currently
   unbuffered.
3. Fall back to first match only if no open-cohort member exists (keeps
   the error path for genuinely-missing members).

The skip path must also append the cohort entry to the **stalled member's**
`flowCohortId` — appending to a historical sibling's drained cohort is a
no-op that leaves the live seat unfilled.

Separately (same fix or follow-up): `maybeEscalateCapToTournament`'s dedupe
should treat a terminal/stalled existing tournament child as
non-blocking, and `member_stalled` should include the stalled member's
runID in `gateReason`/diag so cards and operators target legs, not labels.

## Fix direction

- `handleMemberAction`: resolve via open-cohort seat lookup + non-terminal
  filter before falling back to label match; keep the `ActiveNode` guard.
- Regression test (additive): parent with two same-label children — first
  `failed` in a drained cohort, second `running` unbuffered in the open
  cohort — `memberAction{skip}` must mark the second leg failed and append
  the entry to *its* cohortID, and a subsequent `checkAndBlockStalledMembers`
  must not re-park.
- Companion test: `retry` on the same shape must address the open-cohort
  member, not revive the terminal sibling.
- Follow-up candidates (log separately if not folded in): tournament dedupe
  liveness check; DOA devin legs emitting no events post-spawn;
  `agent-loop/stop` on a leg should also release its cohort seat if the
  terminal path does not already append an entry.

## DeclaredPaths

- `apps/local-runner/internal/runner/cohort_stall.go` (handleMemberAction,
  selection helper reuse of openCohortSeatForLabelLocked)
- `apps/local-runner/internal/runner/*_test.go` (new additive regression
  test file)
- `change-audit/CA-*.md` (runner change-audit entry)
