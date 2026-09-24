# BUG-470: CA-642 question mirror stamps root `waiting_question` — resolution/expiry only heals the owner run

- status: done
- found: live run-102429 → slicer question q-103744 on child run-103606 (fp-beds/full, A-60-4 R-TK delete-demotion drill)
- fixed_by: CA-967
- tests: internal/runner/bug470_root_question_status_heal_test.go

## Symptom (live)

The vibe-cp-ingest run's task_slicer asked the operator a question
(q-103744, correctly surfaced on the ROOT run-102429's stream per CA-642).
The operator answered it; the child resumed and completed; the zero-task
park fired correctly. Yet the parent run stayed `status: waiting_question`
forever — no pending question anywhere on the run or its children.
`GET questions` returned `[]`, but the run snapshot read as still awaiting
an answer, and later `agent-loop/continue` / `resume` calls could not drive
the flow back to a working state (validator step flapped RUNNING →
WAITING_USER_APPROVAL → RUNNING with no delegate ever dispatched).

## Root cause

`turnBridge.askQuestion` (interactive_service.go) mirrors the child's
`EventUserQuestionRequired` onto the root flow run's stream (CA-642) so the
operator sees the card. `emitLocked(root, ...)` runs the event through
`applyRunEventLocked`, which sets `root.status = waiting_question` and
stamps the child step `WAITING_USER_APPROVAL` (BUG-288 #22 — intended).

But every resolution path only heals `s.runs[rec.runID]` — the question's
OWNER run (the child):

- `AnswerQuestion` clears `rs.pendingQuestionID` and restores
  `running`/`waiting_approval` on the owner only; `resumeStepTurn` stamps
  the STEP back to RUNNING but never touches the root run's status.
- `expireQuestion` heals the owner only (BUG-289 H4/F-4 mirror).
- `clearPendingQuestion` (ctx-abandon) cleared only the owner field.

Nothing ever reverts the mirrored stamp on the root. The mirrored event
also does NOT set `root.pendingQuestionID`, so there is no tracked wait to
reconcile — the status is a durable lie (survives restart via session
snapshot restore, which preserves waiting states per V10R4 P0-01).

## Fix

New helper `healMirroredQuestionWaitLocked(owner, resolvedQuestionID)`:
walks `flowRootIDLocked(owner)` to the root flow run and resets
`waiting_question` → `running` (or `waiting_approval` when the root has its
own pending approval) — only when it was the last pending wait mirrored
there. Keeps the wait when:

- `root.pendingQuestionID != ""` (root-owned pending question), or
- another `pending` question record is owned by a run whose
  `flowRootIDLocked` resolves to the same root (sibling question still
  open).

Called from all three resolution paths: `AnswerQuestion` (fresh arm and the
BUG-288 P1-01 resolving-replay arm), `expireQuestion`, and
`clearPendingQuestion`.

`rehydratePendingGatesLocked` additionally heals the already-persisted lie
at boot: `waiting_question` with `pendingQuestionID == ""` and no durable
pending card on that run normalizes back to `running` (a legitimate wait is
always backed by an owner-run pending record, which `gotPendingQuestion`
already re-stamps). Gated on `questionScanOK` so a failed durable read can
never clear a possibly-real wait.

## Verification

- `TestBUG470_AnswerClearsRootMirroredWaitingQuestion` — child question
  mirrored on root → root is `waiting_question` → `AnswerQuestion` → root
  back to `running`. Red before the fix.
- `TestBUG470_ExpiryClearsRootMirroredWaitingQuestion` — `expireQuestion`
  heals the mirrored wait too. Red before the fix.
- `TestBUG470_RehydrateHealsPersistedPhantomWaitingQuestion` — boot
  rehydrate clears a persisted phantom wait with no durable card.
- `TestBUG470_AnswerKeepsRootWaitingWhileSiblingQuestionPending` — a second
  pending sibling question keeps the root wait (no over-heal).
- Full question/approval suite (`-run 'Question|Approval|CA642|ChildAsk|
  GateRepair|Rehydrate'`) green — CA-642 mirror behavior and BUG-288/289
  owner-run semantics unchanged.

## Live evidence

run-102429 session row: `status = waiting_question`,
`agent_status = waiting_question`, `pendingQuestionID` empty, zero pending
questions in `questions.ndjson` — while `loop_state.status = running`.
Direct match to the leak.
