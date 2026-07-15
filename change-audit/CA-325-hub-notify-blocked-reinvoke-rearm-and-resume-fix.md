# CA-325: hub.notify Reinvoke Re-Arms On Blocked/Cap And Resume Retries The Right Prompt

## Scope

Fixes BUG-285: after CA-324's fix, a deferred hub.notify reinvoke still silently dropped if the SAME dispatching turn also escalated (loop became `blocked`) before the retry ran — the guard unlocked and returned without re-arming, and `resumeFlowWithFeedback` (the "Continue" handler) had no knowledge of the dropped prompt, always firing the generic synthesis reinvoke instead. Because `activeHubNodeID` stayed pointed at the notify node, the next unrelated "done" got misattributed as that node's own completion — settling the flow and marking the node falsely `DONE` with no message ever sent.

Also includes a contributing-cause mitigation: `advanceHubDoneThroughEdge`'s successful hub.notify dispatch now reports `Status:"done"`/`NextAction:"advancing"` instead of `Status:"continue"`/`NextAction:"looping"` — the prior wording told the AI its own accepted "done" verdict looked rejected, a plausible reason the same turn then called `escalate()` in the first place.

## Changes

- `apps/local-runner/internal/runner/interactive_service.go`:
  - `maybeAutoReinvokeHubWithPrompt`'s `blocked`/`cap` branches now re-arm `pendingHubReinvoke`/`pendingHubReinvokePrompt` before returning (previously only the `turnInFlight` branch did); the `done` sub-case does not re-arm (nothing left to resume).
  - `resumeFlowWithFeedback` now reads and clears any pending custom prompt under lock BEFORE deciding how to reinvoke; if present, calls `maybeAutoReinvokeHubWithPrompt` with it and returns, instead of always falling through to the generic `maybeAutoReinvokeHubWithNote`.
  - `advanceHubDoneThroughEdge`'s hub.notify dispatch branch now returns `Status:"done"`/`NextAction:"advancing"`.
- New tests (`flow_hub_notify_test.go`): `TestMaybeAutoReinvokeHubWithPromptReArmsOnBlockedStatus`, `TestResumeFlowWithFeedbackRetriesPendingHubNotifyPromptInsteadOfGeneric`.

## Verification

- `go build ./...` — passed. `go vet ./internal/runner` — no issues.
- `go test ./internal/runner -run 'HubNotify|ComposeHubNotify|ResolvesSecondHubNode'` — all passed (9 tests at time of this fix).
- `go test ./internal/runner -run 'ResumeFlowWithFeedback|Resume.*Hub|AutoReinvokeHub'` — 21 passed, confirming no regression to the existing resume/cap-extension path.
- Broad regression sweep — 468 passed, 7 pre-existing/unrelated codex-binary-dependent failures (confirmed baseline), 0 new failures.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via direct inspection of the diagnostic log (`run-8774`) and the named code paths.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-285
change_type: bugfix
summary: re-arm the deferred hub.notify prompt when the loop becomes blocked instead of dropping it, make resumeFlowWithFeedback retry that exact prompt on Continue, and stop telling the AI its own accepted done verdict looks rejected
# --->8---
