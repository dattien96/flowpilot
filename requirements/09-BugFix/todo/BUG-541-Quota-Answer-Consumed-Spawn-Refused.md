# BUG-541 — Quota-route answer consumed at the API layer but apply fails on blocked parent; candidate permanently stranded

Status: **CAPTURED — live reproduction, not yet fixed**
Severity: High — a one-shot operator decision is destroyed on a transient
failure. The question is consumed (removed from the pending list, cannot be
re-answered), the respawn never happens, and the candidate is silently lost
from the cohort — the tournament proceeds with a missing member.

## Symptom (live, `/tmp/fp-live5`, run-12520)

1. run-14292 (candidate-a) bound grok → organic `quota_exhausted` (real 402,
   0% headroom) → `user_question_required` `q-14305` with
   `quota_route_required` options (`use_for_run|devin|...` / `use_once` /
   `stop`). Step stamped `WAITING_USER_APPROVAL` → `FAILED`.
2. While the card was pending, sibling run-14297 (candidate-b) hit the
   reprompt cap → `applyFlowControl(escalate)` → parent loop went
   `blocked(escalate)`.
3. Operator answered `q-14305` with `use_for_run|devin|...`:
   `POST /client/questions/q-14305/answer` → **`{"status":"accepted"}` (200)**.
4. Inside `AnswerQuestion` → `applyQuotaDecision` → `respawnChildOnRoute` →
   `spawnChildRun` → the spawn guard refused:
   `[quota-gate] answer apply failed run=run-14292 err=quota_gate: respawn
   child: parent run "run-12520" loop is blocked (escalate)`.
5. The answer was already consumed: `GET
   /admin/workflow-runs/run-12520/questions` → `[]`. The card cannot be
   re-answered; there is no pending intent to retry the respawn.
6. run-14292 remains `running` with `leg: closed` (zombie), the flow's
   `candidate-a` step is `FAILED`, and the tournament continues one member
   short — no path re-arms the question.

## Root cause

`handleAnswerQuestion` → `AnswerQuestion` resolves the question record
(consumes it) before/independent of the downstream effect succeeding. The
respawn path (`respawnChildOnRoute`) runs the synchronous spawn guard, which
correctly refuses while the parent loop is `blocked`. The failure is logged
(`answer apply failed`) but:

- not surfaced to the caller (HTTP already returned 200), and
- not parked as a durable intent (unlike the BUG-538 parked-successor fix,
  which covers `flow_awaiting_user` on first-turn dispatch — this failure is
  one frame earlier, inside the answer-apply itself, before a child exists
  to park).

So the quorum of "answered → applied" is broken at the consume-vs-apply
boundary: the single decision input is lost on a transient refusal.

## Evidence

- `fp-live5-server.log` 00:44:15: `answer apply failed ... loop is blocked
  (escalate)`.
- `run-12520-flow-events.ndjson` seq 14: `user_question_required` q-14305
  (only event; no follow-up `quota_route_applied`/`spawned` event).
- Admin questions endpoint returns `[]` after the answer — consumed, not
  re-armed.
- sessions.ndjson: run-14292 `status=running, leg_state=closed`
  (updated_at frozen at the veto); run-12520 `loop.blockReason=escalate`.

## Why it matters

- The decision is unrecoverable: operator intent ("use devin for this run")
  is dropped, and the flow loses a candidate with no second chance.
- Same defect class the BUG-538 fix addressed one frame later — the fix
  covers spawn→dispatch, but the answer-apply→spawn boundary still swallows
  failures.

## Expected fix shape (for the fixer)

- `AnswerQuestion` must not consume the question when the apply fails —
  re-surface/retain it (or return an error status to the caller instead of
  200).
- Alternatively, park the respawn decision as a durable intent and apply it
  on the next unblock — mirror the BUG-538 `pendingResume*` mechanism at the
  answer-apply seam.
- The stranded child (run-14292) should settle to a terminal status
  (`failed`) rather than `running` once its leg is closed.
