# CA-271 — Resolved Question Replays Read-Only on Reconnect (not Hidden)

## Bug

After the user answered or skipped a `user_question_required` Drive question and
switched to a different chat view (causing the SSE stream to disconnect), returning
to the original run caused the QuestionCard form to re-appear as a fresh, interactive
form — even though the question had already been answered/skipped.

## Root Cause

`InteractiveService.subscribe()` builds a snapshot of all stored events in
`rs.events` and replays them to the reconnecting client. It did **not** account for
`EventUserQuestionRequired` events whose question record was already `resolved` or
`expired` — the client received the stale event with no answer attached and rendered
an interactive form again.

## Fix (iteration 2 — read-only replay, not hidden)

A first pass simply dropped resolved/expired question events from the reconnect
snapshot. Product feedback: the question should stay **visible** in the transcript
(so the history reads coherently) but must render **read-only** — not a fresh
interactive form. Final fix:

**`provider_event.go`**

Added `Answer []string` to `ProviderEvent` (`json:"answer,omitempty"`), populated
only when replaying an already-resolved question.

**`interactive_service.go` — `subscribe()`**

When building the reconnect snapshot: for a `user_question_required` event whose
question record status is `resolved`, stamp `e.Answer` with the recorded choice
before including it in the snapshot (the event is *not* dropped). For `expired`
questions (no answer to show, and replaying interactive would let the user submit
into an already-expired question and hit a 409), the event is still dropped.

**Client (`contract.ts`, `timelineReducer.ts`)**

- `ProviderEventDTO`'s `user_question_required` variant gained `answer?: string | string[]`.
- `statusFromEvent`: a `user_question_required` event carrying `answer` no longer
  flips run status to `waiting_question` (it's not a new pending state).
- `applyTimelineEvent`'s `user_question_required` case now forwards `answer` onto
  the pushed timeline item and skips adding the question to `pendingQuestions` when
  `answer` is already present — `QuestionCard` already has a built-in read-only
  "answered" rendering path (`resolved = answer !== undefined`) that this reuses
  without any component changes.

## Tests

**Go — `interactive_service_test.go`**

- `TestResolvedQuestionReplayedReadOnlyOnReconnect` (replaces
  `TestResolvedQuestionNotReplayedOnReconnect`): answers a question, reconnects via
  `subscribe(runID, 0)`, asserts the event is present with `Answer == ["__skip__"]`.
- `TestExpiredQuestionNotReplayedOnReconnect`: an expired (never-answered) question
  is still dropped from the reconnect snapshot.

**TypeScript — `timelineReducer.test.ts`**

- `"BUG-StaleQuestion: reconnect replay of an already-resolved question renders
  read-only and does not re-enter pendingQuestions"`: feeds a `user_question_required`
  event with `answer: ["__skip__"]` into `applyTimelineEvent` on an already-`completed`
  run and asserts the question card carries the answer, `pendingQuestions` stays
  empty, and run status is not flipped back to `waiting_question`.

## Files Changed

| File | Change |
|------|--------|
| `apps/local-runner/internal/runner/provider_event.go` | Add `Answer []string` to `ProviderEvent` |
| `apps/local-runner/internal/runner/interactive_service.go` | Stamp resolved-question `Answer` in `subscribe()` snapshot instead of dropping the event; still drop expired questions |
| `apps/local-runner/internal/runner/interactive_service_test.go` | Replace not-replayed test with read-only-replay test; add expired-question-still-dropped test |
| `apps/local-runner/internal/runner/phase5_test.go` | (unrelated, same commit) fix stale assertion in `TestWorkflowDrivenQuestionAnswerPersistsRunningStatus` |
| `apps/desktop-flowpilot/src/types/contract.ts` | Add `answer?: string \| string[]` to `user_question_required` DTO |
| `apps/desktop-flowpilot/src/state/timelineReducer.ts` | Forward `answer` onto the question timeline item; don't re-add to `pendingQuestions`; don't flip status to `waiting_question` for an already-answered replay |
| `apps/desktop-flowpilot/src/state/timelineReducer.test.ts` | Add read-only-replay regression test |

## Verification

- `go build ./...`, `go vet ./internal/runner/` — clean.
- `go test ./internal/runner/ -count=1` — full suite: same pre-existing
  environment-dependent failures as baseline (Codex CLI/account-home/skills-merge/
  auth-workspace/git-diff-wording), zero new failures.
- `apps/desktop-flowpilot`: `tsc --noEmit` clean; `.phase1-tests` recompiled and
  `node --test` run against the reducer test file (30/30 pass, including the new
  test), then `.phase1-tests` drift discarded via `git checkout -- .phase1-tests/`
  per established precedent (it's a committed compiled-output mirror, not meant to
  be regenerated wholesale in a session).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CA-271
change_type: bugfix
summary: resolved questions now replay read-only (with the recorded answer) instead of being hidden or re-shown as a fresh interactive form on client reconnect
# --->8---
