# BUG-285: hub.notify Reinvoke Dropped When The Loop Becomes Blocked Mid-Defer

## Metadata

- Document ID: `BUG-285`
- Title: `hub.notify Reinvoke Dropped When The Loop Becomes Blocked Mid-Defer`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [Task-235: Hub Notify Node Behavior](../../08-Task/done/Task-235-Hub-Notify-Node-Behavior-Reinvoke-Hub-For-AI-Composed-Notification.md), [BUG-284: hub.notify Deferred Reinvoke Retried With The Wrong (Generic) Prompt](./BUG-284-Hub-Notify-Deferred-Reinvoke-Retried-With-Wrong-Generic-Prompt.md) (same defer/retry mechanism; this bug is what remains after `BUG-284`'s fix)
- Child Documents: `None`
- Related Documents: `.flowpilot/logs/features/agent-flow-engine/run-8774.ndjson` (evidence)
- Replaces: `None`
- Tags: `agent-flow-engine, node-behavior, hub-reinvoke, telegram, notification, concurrency, resume`

## AI Quick View

### Summary

- After `BUG-284`'s fix, a deferred hub.notify reinvoke correctly retried with its OWN prompt — but ONLY if the loop was still `running` by the time the retry fired. If the SAME turn that dispatched hub.notify also called `escalate()` (observed live, likely confused by the tool response — see `Q-1`), the loop was `blocked` by retry time, and the pre-existing guard silently dropped the reinvoke with no re-arm.
- `resumeFlowWithFeedback` (the "Continue" button's handler) had no knowledge of a pending hub.notify prompt at all — it always fired the generic synthesis reinvoke on unblock.
- Because `activeHubNodeID` stayed pointed at the notify node the whole time, the NEXT unrelated "done" (from the resumed synthesis turn approving again) was misattributed as the notify node's own completion: its forward edge resolved to the terminal, so the flow settled `done` and the node was swept to `DONE` (it was `RUNNING`, per `BUG-284`'s own fix) — LOOKING like success with no message ever actually sent.
- Root-caused live via `run-8774`: `hub_notify_reinvoke_deferred` → `flow_control_escalate` (same turn) → `hub_notify_reinvoke_blocked` (retry silently dropped) → later resume → generic `hub_reinvoke_scheduled` → final `flow_control_received{status:"done"}` settles the whole flow with `tele-step` marked `DONE` and no `flow_telegram_sent` anywhere in the log.

### Current Ask

- Re-arm the pending hub.notify prompt when blocked/cap-limited instead of dropping it, and make `resumeFlowWithFeedback` retry that prompt in preference to the generic reinvoke.

### Key Decisions

- `V-1` `maybeAutoReinvokeHubWithPrompt`'s `blocked`/`cap` branches now re-arm `pendingHubReinvoke`/`pendingHubReinvokePrompt` (previously only the `turnInFlight` branch did). The `done` sub-case does NOT re-arm — the flow already settled through some other path, so there is nothing to resume into.
- `V-2` `resumeFlowWithFeedback` checks for a pending custom prompt BEFORE firing the generic `maybeAutoReinvokeHubWithNote` reinvoke; if present, it clears it and calls `maybeAutoReinvokeHubWithPrompt` instead.
- `V-3` (contributing-cause fix, same session) `advanceHubDoneThroughEdge`'s successful hub.notify dispatch now reports `FlowControlResult{Status:"done", NextAction:"advancing"}` instead of `{Status:"continue", NextAction:"looping"}` — telling the AI its own "done" call was REJECTED ("continue"/"looping" reads as "keep working") is a plausible reason the same turn then called `escalate()`, which is what exposed this bug in the first place.

### Constraints

- Must not change `resumeFlowWithFeedback`'s existing cap-extension / generic-resume behavior for every flow that has no pending hub.notify prompt.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via direct inspection.

### Open Questions

- `Q-1` Why the synthesizer turn called `escalate()` immediately after `done()` in the same turn is not fully root-caused beyond "the `continue`/`looping` tool response plausibly read as a rejection" (`V-3`). This fix makes the SYSTEM robust to that sequence regardless of why the model does it, rather than relying solely on preventing it.

### Source Refs

- `Task-235`, `BUG-284`.
- `run-8774` diagnostic log: `hub_notify_reinvoke_deferred` (10:43:23.028) → `flow_control_received{status:"escalate"}` (10:43:26.130, SAME turn) → `hub_notify_reinvoke_blocked` (10:43:26.139, dropped, no re-arm) → resume → generic `hub_reinvoke_scheduled` (10:44:09.869) → `flow_control_received{status:"done"}` (10:44:14.979) → `tele-step` swept to `DONE` with no `flow_telegram_sent` anywhere in the log.
- Code anchors: `apps/local-runner/internal/runner/interactive_service.go` (`maybeAutoReinvokeHubWithPrompt`, `resumeFlowWithFeedback`, `advanceHubDoneThroughEdge`).

## 1. Issue Summary

Even after `BUG-284`'s fix, a `hub.notify` node could still silently never send its message: if the dispatching turn also escalated (paused the loop) before the deferred retry ran, the retry was dropped with no re-arm, and the eventual "Continue" resume had no way to know a notify send was still owed — it just resumed the generic review turn, whose next "done" got wrongly attributed to the (still stale) notify node and settled the flow as if it had run.

## 2. Parent Links

- impacted coding plan: none directly
- impacted tech design: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) (`F-3`, the settlement/awaiting-user contract this bug's resume path is part of)
- impacted system spec: none

## 3. Environment and Reproduction

- environment: Desktop app, same `synthesis --done--> tele-step(hub.notify) --done--> done` shape as `BUG-284`.
- reproduction steps:
  1. Synthesizer calls `submit_review_outcome(done)` mid-turn → hub.notify dispatch defers (per `BUG-284`, correctly re-armed with the right prompt this time).
  2. The SAME turn then also calls `escalate()` — loop becomes `blocked`, `WAITING_USER_APPROVAL`.
  3. Turn finishes → turn-completion retry fires → `maybeAutoReinvokeHubWithPrompt` sees `loop.Status == "blocked"` → drops the reinvoke (pre-fix: no re-arm).
  4. User clicks "Continue" → `resumeFlowWithFeedback` fires the GENERIC synthesis reinvoke (no knowledge of the dropped hub.notify prompt).
  5. That resumed turn re-approves and calls `done()` again → `advanceHubDoneThroughEdge` resolves against the STALE `activeHubNodeID` (still the notify node from step 1) → its edge is the terminal → flow settles, node marked `DONE` — no message ever sent.
- frequency: deterministic whenever the dispatching turn ALSO escalates/pauses before the retry fires.

## 4. Expected vs Actual

- expected: resuming after any blocked state that occurred while a hub.notify send was still owed retries that exact send; the flow does not settle until it has actually run.
- actual: the pending send was silently dropped; a later unrelated "done" call was misattributed as the notify node's own completion, marking it falsely `DONE`.

## 5. Impact

- users affected: anyone using `hub.notify` whose dispatching turn also escalates in the same turn (plausible whenever the AI's tool-call feedback is ambiguous — see `Q-1`).
- workflows affected: same as `BUG-284`.
- severity: high — same "silently never sent" outcome as `BUG-284`, but additionally reports a MISLEADING `DONE` status (as if the node ran successfully), which is worse for a user's trust in the step timeline than `BUG-284`'s `SKIPPED`.

## 6. Root Cause

- hypothesis: the defer/retry mechanism only handled ONE failure mode (`turnInFlight`, fixed by `BUG-284`) but silently dropped the pending action on a second, equally-reachable failure mode (`blocked`/`cap`).
- confirmed cause: `maybeAutoReinvokeHubWithPrompt`'s `case "paused","stopped","blocked","done":` branch unlocked and returned without touching `pendingHubReinvoke`/`pendingHubReinvokePrompt` at all; and `resumeFlowWithFeedback` (the sole path that reinvokes the hub after an unblock) always called `maybeAutoReinvokeHubWithNote` unconditionally, with no check for a pending custom prompt.
- evidence: `run-8774`'s diagnostic log shows exactly this sequence (§ Source Refs above) — the dropped defer, the generic resume reinvoke, and the final misattributed settle with `tele-step: DONE` and no send event anywhere in the log.

## 7. Fix Strategy

- `F-1` `maybeAutoReinvokeHubWithPrompt`'s `blocked`/`cap` branches now re-arm `pendingHubReinvoke = true` and `pendingHubReinvokePrompt = prompt` before returning (the `done` sub-case does not, since there is genuinely nothing left to resume).
- `F-2` `resumeFlowWithFeedback` reads and clears `pendingHubReinvokePrompt` under lock; if non-empty, it calls `maybeAutoReinvokeHubWithPrompt(parentRunID, pendingPrompt)` and returns, instead of falling through to the generic `maybeAutoReinvokeHubWithNote`.
- `F-3` (contributing-cause mitigation) `advanceHubDoneThroughEdge`'s hub.notify dispatch branch now returns `Status:"done"`/`NextAction:"advancing"` instead of `Status:"continue"`/`NextAction:"looping"`, so the AI's own accepted "done" verdict is not misreported back to it as a rejection.

## 8. Validation

- `V-1` `TestMaybeAutoReinvokeHubWithPromptReArmsOnBlockedStatus` (new): with the loop pre-set to `blocked`, calling `maybeAutoReinvokeHubWithPrompt` asserts `pendingHubReinvoke`/`pendingHubReinvokePrompt` are re-armed with the original prompt, not dropped.
- `V-2` `TestResumeFlowWithFeedbackRetriesPendingHubNotifyPromptInsteadOfGeneric` (new): with a pending stashed prompt and the loop blocked, `resumeFlowWithFeedback` is asserted (via a fake adapter capturing the next turn's prompt) to retry with the STASHED prompt, not the generic resume note text; both fields are cleared afterward.
- `V-3` `go build ./...`, `go vet ./internal/runner` clean.
- `V-4` Broad regression sweep including `Resume`/`Gate` filters — no new failures against the pre-fix baseline; existing resume/cap-extension tests (21 total) still pass unchanged.

## 9. Regression Guard

- tests: `TestMaybeAutoReinvokeHubWithPromptReArmsOnBlockedStatus`, `TestResumeFlowWithFeedbackRetriesPendingHubNotifyPromptInsteadOfGeneric` (`flow_hub_notify_test.go`).
- alerts: none.
- audit checks: `change-audit/CA-325` records this fix.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: `resumeFlowWithFeedback`'s cap-extension logic and its generic resume-note behavior for every flow without a pending hub.notify prompt are untouched.
