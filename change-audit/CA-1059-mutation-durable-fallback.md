# CA-1059 — mutation endpoints resolve durable runs post-restart

## What changed

CP live-test R.2 item 5: after a runner restart the mutation surfaces answered
`run_not_found` (or a 422 wrapping it) for runs whose durable session rows
still exist — the BUG-508 fallback covered only the GET snapshot path. Live
evidence: `run-1` was durably `blocked` pre-restart and
`POST agent-loop/continue` failed not-found.

- `interactive_resume.go` — new `ensureRunResident(runID)`: in-memory `s.runs`
  first, then `loadPersistedRun` (durable row → `reconstructRun`). Absent row
  → `run_not_found`; unreadable store → `workflow_state_unavailable`. Must not
  be called under `s.mu`.
- `interactive_handlers.go` — `handleSubmitFlowControl` and
  `handleContinueFlow` resolve through `ensureRunResident` instead of a bare
  map read. The continue handler resolves up front so `memberAction` and
  `captureDecisionChoice` operate on the reconstructed run too.
- `interactive_service.go` — `resumeFlowWithFeedback` resolves through the
  same helper; a truly missing run now surfaces the typed `run_not_found`
  apiErr (was a plain error mapped to `continue_flow_failed`).
- `gate_hook.go` — `SubmitGateDecision` releases `s.mu`, reconstructs via
  `ensureRunResident`, re-locks; only a durable-row miss still 404s.

## Red → green

`internal/runner/bug55x_mutation_durable_fallback_test.go` (new):

- `TestContinueResolvesDurablyBlockedRunPostRestart` — red: 422
  `continue_flow_failed` (run not found); green: 200, loop unblocked.
- `TestFlowControlResolvesDurableRunPostRestart` — red: 404 `run_not_found`;
  green: domain path reached, run resident.
- `TestGateDecisionResolvesDurableRunPostRestart` — red: 404; green: resolved.
- `TestMutationUnknownRunStill404s` — fail-closed pin: no durable row → 404.

`go test -count=1 ./internal/runner/` — 27 failures vs 29 on the HEAD run;
every diff verified flaky (suite-ordering/timing): the two tests absent from
this run fail intermittently on HEAD too, and the one new name
(`TestSendMessageClaudeRespawnsPrintCommandPerTurnAndResumesSession`,
provider-pipe race `read |0: file already closed`) fails ~50% isolated on a
tree without this change.
