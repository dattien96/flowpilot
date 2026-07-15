# BUG-284: hub.notify Deferred Reinvoke Retried With The Wrong (Generic) Prompt

## Metadata

- Document ID: `BUG-284`
- Title: `hub.notify Deferred Reinvoke Retried With The Wrong (Generic) Prompt`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [Task-235: Hub Notify Node Behavior](../../08-Task/done/Task-235-Hub-Notify-Node-Behavior-Reinvoke-Hub-For-AI-Composed-Notification.md)
- Child Documents: [BUG-285: hub.notify Reinvoke Dropped When The Loop Becomes Blocked Mid-Defer](./BUG-285-Hub-Notify-Reinvoke-Dropped-When-Loop-Blocked-Mid-Defer.md) (same defer/retry mechanism, second failure mode)
- Related Documents: run diagnostic log `.flowpilot/logs/features/agent-flow-engine/run-8059.ndjson` (evidence — `telegram.notify` used instead of `hub.notify`, unrelated to this bug but the same investigation session), `.flowpilot/logs/features/agent-flow-engine/run-8491.ndjson` (evidence for this bug)
- Replaces: `None`
- Tags: `agent-flow-engine, node-behavior, hub-reinvoke, telegram, notification, concurrency`

## AI Quick View

### Summary

- `hub.notify`'s "done" is observed via `SubmitFlowControl` — i.e. from INSIDE the very turn that is finishing — so `turnInFlight` is essentially ALWAYS true at that moment; deferring the reinvoke is the common case, not a rare race.
- The turn-completion retry site that drains `pendingHubReinvoke` unconditionally called the GENERIC `maybeAutoReinvokeHub` (the review-loop synthesis reinvoke prompt), never the hub.notify node's own write-contract prompt — so the deferred notify dispatch was silently replaced by an unrelated synthesis re-prompt, and the actual Telegram send never happened.
- Confirmed live via `run-8491`: `hub_notify_reinvoke_deferred` fires, then the retry produces another `hub_reinvoke_scheduled` (the generic event name, not a hub.notify-specific one) — the node was left `activeHubNodeID`-tracked but the real send-composing turn never ran.

### Current Ask

- Make the turn-completion retry remember and re-fire the EXACT prompt a deferred hub.notify dispatch needed, instead of falling back to the generic synthesis reinvoke.

### Key Decisions

- `V-1` Add `interactiveRun.pendingHubReinvokePrompt string` — stashed alongside `pendingHubReinvoke` specifically for a hub.notify defer; empty means "no custom prompt pending", preserving the existing generic-reinvoke retry behavior for the cohort-join case untouched.
- `V-2` The turn-completion retry site checks `pendingHubReinvokePrompt` first: non-empty → `maybeAutoReinvokeHubWithPrompt(rs.id, prompt)`; empty → the pre-existing `maybeAutoReinvokeHub(rs.id)` unchanged.

### Constraints

- Must not change `maybeAutoReinvokeHubWithNote`'s own behavior or its existing BUG-275/BUG-234 guarantees — purely additive.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via direct inspection of the diagnostic log and the code paths it named.

### Open Questions

- `Q-1` (resolved by `BUG-285`) What happens if the loop transitions to `blocked` before this retry actually fires — see the follow-up bug.

### Source Refs

- `Task-235` (`dispatchHubNotifyNode`, `maybeAutoReinvokeHubWithPrompt`).
- `run-8491` diagnostic log: `hub_notify_reinvoke_deferred` at `10:26:18.982` followed by a plain `hub_reinvoke_scheduled` (not a hub.notify-specific retry) rather than the stashed prompt ever being used.
- Code anchors: `apps/local-runner/internal/runner/interactive_service.go` (`maybeAutoReinvokeHubWithPrompt`, the turn-completion `pendingHubReinvoke` drain site, ~line 3350).

## 1. Issue Summary

After `Task-235` shipped `hub.notify`, a flow wired `synthesis --done--> notify(hub.notify) --done--> done` never actually sent its Telegram notification: the node was tracked as the run's active hub node and briefly showed `RUNNING`, but no second hub turn with the notify write-contract ever ran — the flow eventually settled `done` with the node marked `DONE` (from a later, unrelated synthesis completion) without ever composing or sending anything.

## 2. Parent Links

- impacted coding plan: none directly — `Task-235` is the parent Task, not a `CP`
- impacted tech design: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) (`D-6`, the hub-owns-the-decision model this reinvoke mechanism is part of)
- impacted system spec: none

## 3. Environment and Reproduction

- environment: Desktop app, `context-coding-review-synthesis`-shaped flow with `synthesis --done--> tele-step(hub.notify) --done--> done`, auto-approve on Telegram integration enabled.
- reproduction steps:
  1. Run the flow through coder → reviewer cohort → synthesis.
  2. Synthesizer calls `submit_review_outcome(status="done")` as its own tool call, mid-turn.
  3. `SubmitFlowControl` → `advanceHubDoneThroughEdge` dispatches `hub.notify`, calling `maybeAutoReinvokeHubWithPrompt` — which defers (this run's OWN turn is still `turnInFlight=true`, since we are inside it).
  4. The synthesis turn finishes normally.
  5. Observe: the turn-completion retry fires, but the SUBSEQUENT hub turn's prompt is the generic "Synthesize the join note above..." text, not the notify write-contract — no `send_message` call happens.
- frequency: deterministic — every `hub.notify` dispatch reached via `advanceHubDoneThroughEdge` (i.e. chained directly after a `hub.inline`/synthesis "done") defers on its first attempt, since it is always observed mid-turn.

## 4. Expected vs Actual

- expected: the deferred reinvoke retries with the hub.notify node's own write-contract prompt once the current turn clears, and the AI composes + sends the Telegram message.
- actual: the retry fired the generic synthesis reinvoke prompt instead, silently dropping the hub.notify dispatch — no message sent, no error surfaced.

## 5. Impact

- users affected: anyone using `hub.notify` chained directly after a `hub.inline`/synthesis node's own "done" (the primary intended use case).
- workflows affected: any flow with `synthesis --done--> <hub.notify node>`.
- severity: high — the feature silently never worked for its main use case; no error, no log distinguishing "dropped" from "working" without reading the diagnostic log directly.

## 6. Root Cause

- hypothesis: the deferred-reinvoke retry mechanism, originally built only for the generic cohort-join case, does not carry enough state to know a DIFFERENT, node-specific prompt was actually pending.
- confirmed cause: `maybeAutoReinvokeHubWithPrompt`'s defer branch only set the boolean `pendingHubReinvoke = true` (no custom prompt was remembered anywhere); the turn-completion retry site unconditionally called `s.maybeAutoReinvokeHub(rs.id)` (equivalent to `maybeAutoReinvokeHubWithNote(rs.id, "")`), which builds its OWN generic prompt from `autoReinvokePromptText()` — entirely unrelated to what `composeHubNotifyPrompt` had produced for the notify node.
- evidence: `run-8491`'s diagnostic log shows `hub_notify_reinvoke_deferred` (10:26:18.982) followed only by generic `hub_reinvoke_scheduled` events for the rest of the run — no hub.notify-specific retry event ever appears, and no `flow_telegram_sent` event appears anywhere in the run.

## 7. Fix Strategy

- `F-1` Add `interactiveRun.pendingHubReinvokePrompt string`, set alongside `pendingHubReinvoke` in `maybeAutoReinvokeHubWithPrompt`'s defer branch.
- `F-2` At the turn-completion retry site, drain both fields together: if `pendingHubReinvokePrompt != ""`, call `maybeAutoReinvokeHubWithPrompt(rs.id, prompt)`; otherwise call the pre-existing `maybeAutoReinvokeHub(rs.id)` unchanged.

## 8. Validation

- `V-1` `TestHubNotifyDeferredReinvokeRetriesWithStashedPromptNotGenericOne` (new): a fake adapter's first turn calls `SubmitFlowControl(done)` mid-turn (simulating the exact live defer condition), asserts a second hub turn is scheduled whose prompt contains the hub.notify write-contract text and does NOT contain the generic "Synthesize the join note above" text.
- `V-2` `go build ./...`, `go vet ./internal/runner` clean.
- `V-3` Broad regression sweep (`Flow|Review|Orchestrat|Cohort|Hub|Reinvoke|Telegram|Audit|Validate|Behavior`) — no new failures against the pre-fix baseline.

## 9. Regression Guard

- tests: `TestHubNotifyDeferredReinvokeRetriesWithStashedPromptNotGenericOne` (`flow_hub_notify_test.go`) is the direct regression proof; keep it passing on any future change to the hub-reinvoke transport.
- alerts: none.
- audit checks: `change-audit/CA-324` records this fix.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this is an internal concurrency fix within `Task-235`'s own mechanism; no upstream `SS`/`SD`/`CP` meaning changes.
- notes left unchanged on purpose: `maybeAutoReinvokeHubWithNote`'s own defer/retry semantics for the cohort-join case are untouched.
