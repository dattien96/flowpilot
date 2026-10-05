# CA-1190 — flow children blocked on their own human decision had no surface

## Evidence (live run-204891, CP-03 Task-039 sprint)

- `run-220224` (spec-aligner child) sat `waiting_question` ~7min — a
  `spawn_agent` permission question found only by polling the admin
  questions API; nothing in the desktop surfaced it.
- `run-215331` (scaffold child) parked `waiting_user_approval` ~95min —
  no lane, no history row, no inbox item; release required an operator
  `POST /turns` dispatch with no UX equivalent.

## Root cause

`isRealtimeVisibleRun` (decision_payload.go) admits only `parentRunID==""`
runs (T-3: child progress rides the parent projection), and
`projectRunHistory` filters `isLiveAgentHistoryRun`. A child's pending
approval/question records are keyed `rec.runID == child.id`, so they never
aggregate onto the parent lane either — child decisions were invisible in
every client surface: no inbox item, no lane, no history row.

## Fix

`isRealtimeVisibleRun` now also admits children that OWN a human decision:
`pendingApprovalID != "" || pendingQuestionID != ""` or status
`waiting_approval`/`waiting_question`. Park-frozen children
(`waiting_user_approval` with no own record) intentionally stay lane-less —
the parent's decision card owns their unblock, and surfacing every frozen
member would flood the inbox on each park. Desktop needs zero changes:
`submitAttentionDecision` is ID-scoped (`answerQuestion(questionId)`,
`submitGateDecision(runId,…)`), so a child lane is actionable end-to-end.

## Regression

`decision_payload_test.go`: `TestRunUpdates_ChildLanesOnlyForOwnDecision`
replaces `TestRunUpdates_ExcludesDelegatedChildRuns` — quiet and park-frozen
children still excluded; a child with its own pending approval produces a
lane carrying the actionable decision payload.
