# BUG-641 — A flow node can transition to `WAITING_USER_APPROVAL` while no question/approval record exists anywhere server-side: the run soft-locks on a phantom prompt the user can never answer, and the desktop paints "2 questions pending" / "Waiting: question" chrome with nothing behind it

- **ID:** BUG-641
- **Severity:** High — the node parks mid-round with zero actionable
  surface: `GET /admin/workflow-runs/{id}/questions` returns `[]` for
  the parent and every leg, no gate-decision record is open, and the
  only recovery is an operator `agent-loop/continue` with feedback.
  Autonomous runs therefore stall indefinitely unless watched.
- **Status:** FIXED — both observed arms are fixed (2026-10-06):
  CA-1228 un-stamps the mirrored flow step on both wait-heal paths, and
  CA-1229 stops `reinvokeMatchingFlowChild` resurrecting explicitly-ended
  legs (Cancelled / LegStateClosed) that callers matched by label. The
  remaining `waiting_user_approval`-rebuild arm below is the last open
  thread — watch the next live run before closing the file.

## Evidence chain (all live)

1. Round-9 loop-back after `spec_align=changes_requested`:
   `step_status_transition {node_id: coder, RUNNING}` then immediately
   `step_status_transition {node_id: coder, WAITING_USER_APPROVAL}` —
   no `flow_parked_awaiting_user`, no gate, no child spawn.
2. Desktop simultaneously painted: coder row `waiting_question` +
   "2 questions pending" chip + header "Waiting: question" +
   "Waiting for the current turn…" composer lock.
3. Server ground truth at the same instant:
   `GET /admin/workflow-runs/run-306526/questions` → `[]`
   (also `[]` on run-429281, run-433214, run-434917); loopState carried
   no pendingQuestion/pendingApproval fields.
4. The wait discharged only via operator `agent-loop/continue` with
   feedback "[adj-23] coder node went WAITING_USER_APPROVAL but no
   question record exists" → `running` resumed.

## Root cause direction

Two candidate seams, both worth instrumenting:

1. The flow engine sets `WAITING_USER_APPROVAL` when a leg emits an
   `ask_user`-style provider event, but the question store write and
   the step transition are not atomic — if the leg's turn ends without
   the runner ever seeing a question payload (DOA-session family, see
   BUG-639 observations), the node is left parked on nothing.
2. The coder step here was executed **inline by the hub leg** (no
   `child_spawn_*` for the round-9 coder — the hub absorbed the node
   after the dispatch dead-end). An inline-absorbed node may reach the
   "await user" posture through a code path that never mints a
   `questions` record.

Whichever it is, the invariant violation is the same:
`WAITING_USER_APPROVAL` must be unreachable without a resolvable
question/approval id. The transition should either carry the id in the
same write, or the read path should fail closed back to RUNNING/blocked
with an explicit reason instead of an unanswerable wait.

### Resolved arm (CA-1228)

The emit path was already card-gated (`s.questions[ev.QuestionID]` must
be pending before `rs.status` flips — interactive_service.go emitLocked).
The live phantom persisted because **heal paths repaired `rs.status`
only**: `healOrphanedWait` (BUG-581 sweep) and
`healMirroredQuestionWaitLocked` (CA-642/CA-1207) returned the leg to
`running` but never un-stamped the durable step row that
`settleFlowChildStepAwaitingUserLocked` had stamped
`WAITING_USER_APPROVAL`. The step row then blocked hub reinvokes
("children running or waiting") forever and kept painting the phantom
question chip. CA-1228 adds `unstampHealedFlowChildStepLocked` — the
mirror-image helper called from both heal sites; it touches only a step
still in `WAITING_USER_APPROVAL`. Regression:
`bug641_orphaned_step_wait_test.go` (4 tests, 2 red-first).

## Fix direction

- Gate the `WAITING_USER_APPROVAL` transition on an existing pending
  question/approval record (or mint one atomically), same transaction.
- Surface `pendingQuestionId` on the loop/node snapshot so the desktop
  can render the question or explicitly render "no question — recover"
  instead of a phantom chip.
- Add a watchdog assertion: `WAITING_USER_APPROVAL` with empty pending
  set for >N seconds auto-escalates (reuses the tournament/escalate
  surface) rather than sitting silently.

## Compounding observations (same session)

- `cohort_incomplete` rejections stopped once `flow-auto-context-round-1`
  joined — proving the only remaining wedge was the phantom wait.
- Leg `run-429281` (tdd bookkeeping stand-in) showed
  `waiting_user_approval` in the graph long after its real completion —
  same "wait state never cleared" family, cosmetic layer.
- **Ghost resurrection (FIXED — CA-1229):** legs terminalized via
  `agent-loop/stop` flipped back to `running`/`waiting_user_approval` in
  the in-memory graph within minutes (run-348382 stopped → answered
  `stopped` → shown `running` again ~4 min later; same for
  run-309960/run-402250). Root cause found: `reinvokeMatchingFlowChild`
  scanned children newest-first and unconditionally re-stamped
  `status=running` on the first label/agent match — every caller's match
  predicate was label-only, so a review-loop continue back-edge (or any
  re-drive) resurrected the operator-stopped leg *and* stamped its
  orchestrator summary running. `hasActiveFlowChild` then read `running`
  and hub reinvokes deferred as `hub_parked` forever. Fix: the match
  loop now skips `RunStatusCancelled` and `LegStateClosed` children —
  explicitly-ended legs are never re-driven in place (the designed
  re-drive is a fresh spawn which re-binds the open cohort seat, CA-645).
  Completed legs stay matchable (BUG-Rnd2 round-2+ re-entry) and Failed
  legs keep retry-in-place (redriveDeadDelegateLeg). Regression:
  `bug641_cancelled_leg_reinvoke_test.go` (4 tests, 3 red-first).
- The phantom wait reproduced **three times** in one session
  (coder round-9, coder round-10, synthesis round-12) — each discharges
  via operator `agent-loop/continue` or an escalate+continue pair; the
  node then flips to FAILED/RUNNING without any user input ever having
  existed.

## DeclaredPaths

- `apps/local-runner/internal/runner/interactive_service.go` (step
  transition + question minting)
- `apps/local-runner/internal/runner/agent_orchestrator.go` (node
  posture transitions)
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx` /
  question-chip rendering (display side only if server semantics
  change)
