# CA-1125 — BUG-579 + BUG-581: orphaned / phantom waiting_* states healed in-session

## Why

Live run-100368 (leg run-124283): a child leg sat `waiting_user_approval`
with `pending_resume_approval_id = q-120784` — a card id that no longer
existed in approvals or questions. Nobody could answer it; the parent's
child-turn fence then refused new work — a circular fence until operator
intervention. Separately (BUG-581), `user_question_required` events
appeared on the parent stream (seq 4903) while `/admin/questions` was empty
— a phantom `waiting_question` with no durable card.

The reconstruct path already heals a durable-free `waiting_question`
(BUG-470 precedent), but nothing did the same in-session for live runs.

## What changed

`apps/local-runner/internal/runner/wedge_sweep.go`:

- `sweepWedgedFlowWork` heals `waiting_question` / `waiting_approval` /
  `waiting_user_approval` past a grace window (`wedgedWaitGrace`, 60s) when
  no card backs the wait: `pendingApprovalID`, `pendingQuestionID`, and
  `decisionCard` all empty. The grace covers the emit-then-stamp window,
  which is sub-second (both writes happen under `s.mu` in one code path).
- `waiting_user_approval` on a child is a legitimate park while the parent
  loop is `blocked` awaiting a human decision — the parent owns the
  decision surface. It is healed only when the park has no owner (no
  parent, or a parent loop that is not blocked). Healing a parent-owned
  park would be a silent auto-allow; this check prevents it.
- A `pendingResumeApprovalID` pointing at a card record absent from both
  `s.approvals` and `s.questions` is cleared even while a real card backs
  the wait — the reconcile it references can never fire.

`healOrphanedWait` re-checks every condition under `s.mu` before mutating,
persists the session snapshot, logs `orphaned_wait_healed` to the flow diag
log, and drains via `notifyTurnIdle`.

## Invariant

A `waiting_*` status must be backed by a live, answerable surface. A wait
with no card is a phantom (BUG-470 contract generalized in-session).
Conversely, a parent-owned park is never healed — no silent auto-allow.

## Tests

`bug571_wedge_sweep_test.go` (red → green):

- `TestBug581_SweepHealsPhantomWaitingQuestion` — card-free
  `waiting_question` past grace returns to `running`.
- `TestBug579_SweepHealsOrphanedResumeApproval` — dangling
  resume-approval id cleared and the orphaned wait healed.
- `TestBug581_SweepKeepsCardBackedWait` — a wait with a live
  `pendingQuestionID` is never touched.
