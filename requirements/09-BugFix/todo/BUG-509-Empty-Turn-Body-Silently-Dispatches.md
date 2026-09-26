# BUG-509 — Turn endpoint silently accepts unknown/misspelled JSON fields, dispatching a no-op turn

## Status
FIXED + LIVE-VERIFIED — 2026-09-26, fixed build, runner on :19400.

- Fix (CA-1006): `handleStartTurn` fails closed (`400 invalid_request
  "prompt is required"`) when the decoded body carries nothing actionable:
  empty prompt AND no attachments AND no `flowRef`/`subMode`/
  `sourceDocId`/`changeType`. Strict `DisallowUnknownFields` is not viable
  (clients legitimately send `runId`/`idempotencyKey`), so the content
  check is the guard. Attachment-only turns and flow-launch turns are
  preserved.
- Unit: `bug509_empty_turn_body_test.go` (green).
- Live re-verify (fixed build): `POST
  /client/workflow-runs/run-23515/turns` with
  `{"stepId":"chat-run-23515","text":"misspelled"}` → `400 prompt is
  required`; `{"stepId":"chat-run-23515"}` → `400` same. A valid prompt
  body still dispatches 200.

## Live-found during
Full CP live-test rerun, 2026-09-26 — operator error surfaced as flow
noise.

## Observed

- A turn POST sent `{"text": "Implement a garbage-collection…"}` instead
  of `{"prompt": …}` (the real field). The decoder accepted the body
  cleanly (unknown field ignored, `Prompt` stays `""`), and `startTurn`
  dispatched a turn with an empty prompt. The provider answered a no-op
  reply, the post-turn gate fired on zero output, and the run entered
  vibe owner-debate — real downstream churn caused by one misspelled key.
- Symmetric risk: ANY body composed only of unknown fields dispatches a
  meaningless provider turn while the caller's intent is silently lost.

## Root cause

`json.Decoder.Decode(&body)` ignores unknown fields by default and
`Prompt` was not validated before `startTurn`. The pre-existing
`stepId is required` check only covers the step id — an empty prompt on a
valid step id passed straight through.

## Fix applied

Fail-closed content check in `handleStartTurn` before orchestration
selection — see Status above.

## Related
- BUG-503: same launch path — empty-field tolerance let a project-bound
  run silently bind the wrong workspace.
