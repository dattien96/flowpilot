# CA-1122 — BUG-578: debate_synthesis retry re-attaches the joined cohort note

## Why

Live run-100368: a `debate_synthesis` escalate retried via Continue/Retry
fell into the generic inline dispatch (`flowNodeInlineDispatchable` →
`dispatchHubNotifyNode` → `composeHubNotifyPrompt`) — a bare "reached step"
prompt. The owners' joined result note had been consumed by the first
synthesis turn, so the retry hub turn had no verdicts to synthesize and
re-escalated identically forever.

## What changed

`apps/local-runner/internal/runner/interactive_service.go`:

- In `resumeFlowWithFeedback`'s escalated-node dispatch, when the escalated
  node is `debate_synthesis`, the hub is re-driven via
  `dispatchHubNotifyNodeWithPrompt` with a composed prompt: the last joined
  cohort note (`lastCohortNote`, which carries the owner verdicts) followed
  by the standard hub-notify instruction, with operator feedback prepended
  when present. When no cohort note exists the behavior degrades to the
  previous generic prompt — same as before.

## Invariant

A retried synthesis turn must carry the evidence it is supposed to
synthesize; a hub turn without its cohort note can only re-escalate.

## Tests

`bug570_578_debate_reprompt_test.go` (red → green):

- `TestBUG578DebateSynthesisRetryCarriesJoinedNote` — a Retry on an
  escalated debate_synthesis schedules a hub turn whose prompt contains the
  owners' joined result note
