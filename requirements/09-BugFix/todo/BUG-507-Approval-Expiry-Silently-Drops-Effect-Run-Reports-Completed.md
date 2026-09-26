# BUG-507 — Provider-side approval expiry silently drops the requested effect; run surfaces `completed` while its flow loop stays open

## Status
FIXED — unit-verified, 2026-09-26, fixed build.

- Fix (CA-1010): (a) `expireApproval`/`expireQuestion` now emit durable,
  broadcast run events (`approval_expired` / `question_expired` with the
  requested details) — a dropped effect leaves a trace instead of a clean
  run. The existing hub-stall escalation path still fires via
  `notifyTurnIdle`/`maybeScheduleHubStallCheck`. (b) `emitLocked`'s
  turn-completion path withholds `completed` while the run hosts an open
  flow loop (`loopSt` not done/stopped): the run reports `running` until
  the loop's own seal path (`settleParentRunOnFlowDone`) or escalation
  card publishes the honest terminal. `signalChild` still fires — it feeds
  the loop the completion that drives the next hub dispatch.
- Unit: `bug507_approval_expiry_test.go` (4 tests, green): both expiry
  events durable+broadcast; open loop never reads completed; sealed loop
  still completes normally.

## Live-found during
Full CP live-test rerun, 2026-09-26.

`run-22241` (vibe-ingest, grok hub, lt-full bed):

- `turn-22243` (ingest turn): model requested file-write approval
  `appr-22375` (event `permission_required`, 19:22 UTC). Operator was not
  watching; the approval **expired** provider-side. Nothing was written —
  no wordfreq artifacts exist in the bed. Post-turn gate fired (no
  artifacts) → `vibe-owner-debate` spawned (`run-22381`, `run-22389`).
- `turn-22554` (synthesis turn): model tried to record the `blocked`
  verdict via a self-invented `Write /tmp/submit_review_outcome.json`
  (BUG-504 mechanism 2 — no such ingestion path exists). Approval
  `appr-22618` raised 19:33 UTC, expired unanswered. Verdict lost.
- Operator answered `appr-22618` ~1 h late → `409 question_expired`.
  Confirmed: `question_expired` is thrown when the bridge already expired
  the question (interactive_service.go:10806) — i.e., provider-side
  permission requests carry their own timeout and late operator answers
  are rejected.
- Final state: `status: completed`, `agentStatus: completed` on the run
  row, while the flow-engine log ends at `hub_reinvoke_scheduled` with
  `loop_status=running`, round 0, cap 5. No card, no escalate, no
  attention item tells the operator the verdict was dropped.

## Why this matters

- Human gates that expire silently are the opposite of the durability
  contract: a pending decision is itself state that must survive or be
  explicitly resolved — not vanish on a provider-side timer.
- A "completed" run whose loop is open is a dishonest terminal. Any
  dashboard/history consumer sees success; the vibe loop actually
  dead-stopped with an unrecorded `blocked` verdict.
- Same mechanism silently dropped the ingest turn's file write — so this
  is not verdict-specific; ANY write/command approval that expires
  unattended loses its effect with no trace at run level.

## Suggested fix directions

1. On approval expiry, resolve the pending question as an explicit
   recorded outcome (`expired_unanswered`) and surface it — park the run
   with a card or at minimum an attention item + durable event. Never let
   the run settle `completed` with an open loop.
2. Consider runner-side re-raise: when the permission bridge sees the
   provider timeout, re-issue the approval card on the runner's own
   durable store so it survives provider-side expiry (and restarts).
3. Flow status honesty: a chat run should not report `completed` while
   its attached flow loop is `running`/`open` — project the loop state
   into the run row or park the run.
4. Separately (BUG-504 follow-up): if a turn ends with an attempted
   verdict tool-call that never landed, treat it as `missing machine
   verdict` (the known escalate path) rather than closing the turn clean.

## Related

- BUG-504 — the missing `submit_review_outcome` exposure that forced the
  file-drop workaround in this run.
- BUG-503 — the wrong-workspace binding found in the same rerun.
- BUG-505 — freeze-escalate undocumented feedback-as-draft resolution.
- Design question (unfiled, see CP-Full-Live-Test notes): parked-at-gate
  runs normalize to `cancelled` on restart.
