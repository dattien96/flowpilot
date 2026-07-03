# CA-233: Escalate / Awaiting-User Made Actionable

## Summary

User reported the Review Loop's `synthesis` step and the main chat hanging indefinitely after a mixed-verdict review (one reviewer approve, one changes-requested) led the synthesizer to submit a `blocked` outcome. Investigation found this was NOT a bug in escalate's semantics — `escalate`/cap-reached correctly pause the loop, awaiting the human — but the "awaiting user" state had no actionable representation anywhere: the step timeline showed `RUNNING` (reads as a hang) and the desktop derived the run status as `running` from a `blocked` loop, locking the composer with no way for the very user the flow was waiting on to respond. Co-designed the fix with the user across several rounds (recovery affordance modeled on `ask_user`; Continue lets the hub re-decide rather than hard-routing; auto-extend the cap for both block reasons; retire `ExtendMax` as a hard limit on the human). Implemented across both the Go runner and the desktop app.

## What Changed

### Runner (`apps/local-runner`)

- `provider_event.go`: `AgentLoopState` gains `BlockReason` (`"cap"` | `"escalate"`).
- `flow_step_runtime.go`: new `setFlowStepAwaitingUser` — settles the flow's hub inline node to `WAITING_USER_APPROVAL` (not `RUNNING`, not `FAILED`).
- `interactive_service.go`:
  - `applyFlowControl`'s `escalate` case and the `continue` case's cap-reached branch now set `BlockReason` and call `setFlowStepAwaitingUser`.
  - New `resumeFlowWithFeedback(parentRunID, feedback)`: the "Continue" action. Clears the block (auto-raises the cap by `ExtendBy` only when `BlockReason=="cap"`), settles the hub node back to `RUNNING`, and re-invokes the hub synthesis turn via `maybeAutoReinvokeHubWithNote` with the feedback embedded directly in the prompt (the same reliable-delivery pattern CA-226 established for cohort-join notes). Idempotent no-op if the loop isn't currently blocked.
  - `extendCap` no longer rejects once `ExtendCount >= ExtendMax` — that limit only ever bounded this user-triggered action (never any auto path), so its only remaining effect was to eventually block the human, reproducing the exact wedge this fix resolves. `Cap` (the auto-loop bound) is unchanged.
- `interactive_handlers.go`: new `POST /client/workflow-runs/{runId}/agent-loop/continue` route + `handleContinueFlow`.
- Tests: `TestApplyFlowControlEscalateSettlesHubToWaitingUser`, `TestApplyFlowControlCapReachedSettlesHubToWaitingUser`, `TestResumeFlowWithFeedbackAutoExtendsOnlyForCap`, `TestResumeFlowWithFeedbackNoopWhenNotBlocked`, `TestResumeFlowWithFeedbackReinvokesHubSynthesisTurn`, `TestExtendCapNoLongerRejectedPastFormerMax`.

### Desktop (`apps/desktop-flowpilot`)

- `types/contract.ts`: new `RunStatus` value `"blocked"`; `AgentLoopState.blockReason`; `RunnerClient.continueFlow`.
- `client/HttpWsRunnerClient.ts` / `client/MockRunnerClient.ts`: `continueFlow` implementations (mock mirrors the runner's auto-extend-only-for-cap semantics).
- `state/store.ts`: `deriveOrchestrationRunStatus` now returns `"blocked"` (exported for direct testing) instead of folding it into `"running"`; new `continueFlow(feedback)` store action.
- `components/RunStatus.tsx`, `components/Navigator.tsx`: added the `"blocked"` label to the two exhaustive `Record<RunStatus, string>` maps the new enum value requires.
- `components/RunToast.tsx` / `app/runToastGrouping.ts`: a blocked loop now fires a native "Flow paused — needs your input" notification, same as approval/question.
- `components/FlowAwaitingUserCard.tsx` (new): the inline Continue/Stop-with-feedback card, modeled on `QuestionCard`'s ask_user shape per the user's explicit direction. Mounted in `ChatWorkspace.tsx` between the timeline and the composer.
- `components/FlowStepTimeline.tsx`: no change needed — `WAITING_USER_APPROVAL` already renders as a distinct "waiting" state, not the RUNNING spinner.
- `components/OrchestrationBoard.tsx`: the blocked banner now uses `continueFlow` (feedback textarea + Continue); the standalone "Extend cap +2" button was removed (redundant/inconsistent with Continue's auto-extend).
- Tests (`state/store.test.ts`): `deriveOrchestrationRunStatus` blocked/running/paused/done/stopped cases plus an actively-running-child-wins-over-blocked case; `continueFlow` calls the client with trimmed feedback and applies the snapshot; `continueFlow` no-ops safely when unimplemented.

### Docs

- `requirements/06-System-Tech-Design/SD-19-Agent-Flow-Engine.md` §8 `F-3`: documented the settlement contract (waiting step status, `BlockReason`, non-`running` client status, hub self-routing on resume).
- `requirements/07-Coding-Plan/inprogress/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md` `P-4`: noted that this fix makes the pause deterministic at the engine layer rather than solely relying on the skill calling `ask_user` (the same model-reliability gap CA-226 found for `submit_review_outcome`), and that `ExtendMax` is retired as a hard reject.
- `requirements/09-BugFix/done/BUG-231-...md`: full investigation, design decisions (`D-1`..`D-8`), Definition of Done, and per-item code change plan.

## Verification

- `go build ./...` / `go vet ./internal/runner/...` — clean. `go test ./internal/runner/...` — 1065 passed, 15 failed (identical pre-existing, environment-specific baseline established this session), 14 skipped.
- The 5 new escalate/resume Go tests re-run 15x each (75 runs total) — 0 failures.
- `npm run typecheck` / `npm run build` in `apps/desktop-flowpilot` — clean. Full desktop unit suite — 116 passed, 2 failed (identical pre-existing baseline: `localStorage` unavailable in the Node test runner, one timing-sensitive test).
- Not executed: a live click-through in the running desktop app. The Vite dev-server preview's bootstrap gate requires a reachable local-runner backend (`127.0.0.1:4317/supabase-config`), unavailable in this sandbox — confirmed instead that every new/changed module (including `FlowAwaitingUserCard.tsx`) loads with zero console/network errors, ruling out an import/syntax crash but not confirming the rendered/interactive UI.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-231
change_type: bugfix
summary: Make escalate/cap-reached a visible, actionable awaiting-user pause instead of an indistinguishable hang, with a Continue/Stop recovery form in the main chat
# --->8---
