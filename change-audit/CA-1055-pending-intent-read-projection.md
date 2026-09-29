# CA-1055 — pending-intent read projection asymmetry (Agents panel / node status)

## What changed

Live anomaly found during the 2026-09-29 closure campaign: a durable child row
carrying `pending_resume_*` (BUG-538 parked-successor shape) reconstructed as
`running` via `normalizeResumedFlowStatus`, but the `listAgentRunSummaries`
disk-fallback projected the same row as `cancelled` — the Agents panel (and the
flow-node step projection) disagreed with the durable record's semantics.

- `interactive_service.go` — `listAgentRunSummaries` disk-fallback branch now
  projects `Status` via `normalizeResumedFlowStatus(session)` (pending-aware:
  resume/gate-reprompt intents keep the row non-terminal, same as
  reconstructRun) and `AgentStatus` via the new `agentStatusFromSession`.
- `interactive_resume.go` — new `agentStatusFromSession`: preserves the
  persisted agent status when a resume/reprompt intent is armed (falls back to
  `running` when empty); unchanged `normalizeResumedStatus` otherwise.
  `resumedChildRunStepStatus` now takes the full session row and keeps the
  node `pending` while an intent is armed instead of projecting `canceled`.
- Drive manifest / tombstone projections (`chat_session_sync.go`)
  intentionally keep the pending-blind normalizer — synced children carry no
  local durable intents, so collapsing to terminal stays correct there.

## Red → green

`internal/runner/bug55x_pending_intent_projection_test.go` (new):
`TestDiskFallbackKeepsPendingIntentChildRunning`,
`TestDiskFallbackKeepsPendingRepromptChildRunning` — red before the fix
(`cancelled`), green after; `TestDiskFallbackStillCancelsPlainInterruptedChild`
pins the BUG-251 stale-spinner guardrail (no intent → still cancelled).

`go test -count=1 ./internal/runner/` — package suite run: the only failures
reproduce identically on HEAD (gate-warn baseline, provider/env deps); the
one resume-path failure (`TestResumeRunReconstructsWorkflowRunFromDisk`)
passes with the change in isolation — suite-ordering flake, not a regression.
