# BUG-287: hub.notify Prompt Told The AI To Call status="done" — Its Only Tool Has No Such Value

## Metadata

- Document ID: `BUG-287`
- Title: `hub.notify Prompt Told The AI To Call status="done" — Its Only Tool Has No Such Value`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [Task-235: Hub Notify Node Behavior](../../08-Task/done/Task-235-Hub-Notify-Node-Behavior-Reinvoke-Hub-For-AI-Composed-Notification.md)
- Child Documents: `None`
- Related Documents: [BUG-284](./BUG-284-Hub-Notify-Deferred-Reinvoke-Retried-With-Wrong-Generic-Prompt.md), [BUG-285](./BUG-285-Hub-Notify-Reinvoke-Dropped-When-Loop-Blocked-Mid-Defer.md), [BUG-286](./BUG-286-Continue-Round-Reset-Used-Wrong-Entry-Node-Stuck-Context-Coder.md) (same `hub.notify` debugging session, prior fixes in this exact mechanism — this bug's evidence run (`run-9664`) shows all three of those fixes working correctly, isolating this as a fourth, independent defect), `.flowpilot/logs/features/agent-flow-engine/run-9664.ndjson` (evidence)
- Replaces: `None`
- Tags: `agent-flow-engine, node-behavior, telegram, notification, tool-contract, prompt`

## AI Quick View

### Summary

- After `BUG-284`/`BUG-285`/`BUG-286`'s fixes, `hub.notify` correctly reinvoked the hub, correctly re-armed and retried through a blocked/resume cycle, and correctly sent the Telegram message (user-confirmed) — but the hub session then re-escalated ("Needs your decision" reappeared) with the SAME stale reviewer summary as before, instead of settling the flow.
- Root cause: `composeHubNotifyPrompt` instructed the model to "call the flow's control tool with status=\"done\"" — but the ONLY tool actually exposed to this session, for every flow that can reach `hub.notify` today, is `submit_review_outcome`, whose `status` enum is strictly `approved | changes_requested | blocked` (`claude_mcp_server.go`'s `sharedReviewOutcomeSchema`), and whose own tool description explicitly states "This is the only flow-control tool — do not use flow_control directly." There is no literal "done" value the model can invoke.
- Unable to comply literally, the model instead re-submitted an unrelated, stale verdict (`blocked`, with the earlier reviewer feedback text) — re-triggering the exact same escalate/approval card the user had already resolved.

### Current Ask

- Instruct the model to call the ACTUAL tool with a value its schema accepts (`submit_review_outcome` / `status="approved"`, which maps to `done` via the tool's own `statusMap`), with a clarifying note that this is a formality and not a real code-review judgment.

### Key Decisions

- `V-1` Hardcode the instruction to `submit_review_outcome`/`approved` rather than generalize — every flow that can currently reach `hub.notify` (review-loop.yaml, context-coding-review-synthesis.yaml) declares this exact tool; a fully generic fix would need to resolve the correct tool name/enum value from the flow's OWN declared tool face at compose time, which `composeHubNotifyPrompt` does not currently have access to (only the node, not the flow definition's `tools:` list).

### Constraints

- Scoped, not fully generic — flagged explicitly as a follow-up (`Q-1`) rather than silently assumed to generalize.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via direct inspection of the diagnostic log and `claude_mcp_server.go`'s actual tool schema.

### Open Questions

- `Q-1` If a future flow declares a DIFFERENT control-tool name/schema for its `hub.inline` node and also uses `hub.notify`, this hardcoded instruction would be wrong for it. Resolving the correct tool name/enum dynamically (from the flow's own `tools:` declaration and `statusMap`) would need `composeHubNotifyPrompt` to receive the flow definition, not just the node — deferred until such a flow actually exists.

### Source Refs

- `Task-235` (`composeHubNotifyPrompt`).
- `run-9664` diagnostic log: `tele-step: RUNNING` (11:31:16 and again 11:31:33, both dispatches correct per `BUG-284`/`BUG-285`'s fixes) → `hub_notify_reinvoke_scheduled` (11:31:33.537, the actual notify-send turn, user-confirmed the message arrived) → `flow_control_received{status:"escalate", summary_len:827}` (11:32:25.341) — an IDENTICAL byte length to the FIRST escalate's summary (line 38 vs line 44 of the log), strongly indicating the model resubmitted its earlier stored verdict rather than producing a fresh one for this turn.
- Code anchor: `apps/local-runner/internal/runner/claude_mcp_server.go` (`sharedReviewOutcomeSchema`, `claudeMCPToolDefs` — tool description "This is the only flow-control tool — do not use flow_control directly"), `apps/local-runner/internal/agentpack/flow-pack/tools/submit-review-outcome.yaml` (`statusMap: approved: done`).

## 1. Issue Summary

After three prior fixes to the `hub.notify` reinvoke/defer/reset mechanism, a live run showed the Telegram message being sent successfully, but the flow then re-displayed the "Needs your decision" approval card with the same stale reviewer content instead of settling — reported directly by the user as "gửi mes done xong vẫn lại show form need decision" (sends the done message, then still shows the need-decision form again).

## 2. Parent Links

- impacted coding plan: none directly
- impacted tech design: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) (`D-4`, the generic `flow_control` / declared-tool-face contract this bug's mismatch violates)
- impacted system spec: none

## 3. Environment and Reproduction

- environment: Desktop app, `context-coding-review-synthesis`-shaped flow, `synthesis --done--> tele-step(hub.notify) --done--> done`.
- reproduction steps:
  1. Run the flow through a full review round to synthesis's `submit_review_outcome(done)`.
  2. Confirm `BUG-284`/`BUG-285`'s fixes work: the hub.notify reinvoke defers, re-arms through any blocked cycle, and eventually sends a real Telegram message (user-confirmed the message arrived).
  3. Observe: the SAME notify-send turn (or its continuation) then calls `submit_review_outcome` again with `status="blocked"` and the ORIGINAL reviewer feedback text (not anything about the notify step) — re-escalating the flow and reopening the "Needs your decision" card.
- frequency: deterministic for any `hub.notify` dispatch, since the underlying tool-schema mismatch is unconditional (the model is NEVER given a way to literally comply with "status=done").

## 4. Expected vs Actual

- expected: after sending the notification, the hub turn calls a tool/value combination its own schema actually supports, settling the flow as done.
- actual: the model could not comply with "status=\"done\"" (no such enum value exists on its only available tool) and fell back to resubmitting an unrelated stored verdict, re-escalating instead of settling.

## 5. Impact

- users affected: every user of `hub.notify` — this is not conditional on anything, so it affected 100% of runs that reached the notify-send turn.
- workflows affected: any flow using `hub.notify`.
- severity: high — the flow never actually settles after a hub.notify dispatch; the user must repeatedly re-approve a stale, unrelated escalate card, and the flow's true completion is masked.

## 6. Root Cause

- hypothesis: the write-contract's instruction assumed a generic `flow_control` tool with a literal `done` status value was available to call.
- confirmed cause: `claude_mcp_server.go`'s `claudeMCPToolDefs`/`sharedReviewOutcomeSchema` expose ONLY `submit_review_outcome` for a hub turn, with `status` restricted to the enum `approved | changes_requested | blocked` — the tool's own description states "This is the only flow-control tool — do not use flow_control directly," directly contradicting `composeHubNotifyPrompt`'s instruction to call "the flow's control tool with status=\"done\"."
- evidence: `run-9664`'s escalate summary at `flow_control_received` (11:32:25.341, `summary_len:827`) exactly matches the FIRST escalate's summary length (11:31:19.947, also `summary_len:827`) — the same reviewer_correctness verdict text, strongly indicating the model reused its earlier stored verdict rather than generating anything new for the notify-completion turn, consistent with having no valid way to express "I'm just done with the notify step."

## 7. Fix Strategy

- `F-1` Change `composeHubNotifyPrompt`'s final instruction to explicitly name the real, callable tool and value: `submit_review_outcome` with `status="approved"` (which maps to `done` via `submit-review-outcome.yaml`'s own `statusMap`), with an added clarifying sentence that this is a technical formality to advance the flow, not a real code-review judgment — so a notify-only turn does not feel compelled to relate the call to reviewing code.
- `F-2` (rejected for this pass, see `Q-1`) Dynamically resolving the correct tool name/enum from the flow's own declared `tools:` face — deferred as a generalization not needed by any flow that exists today.

## 8. Validation

- `V-1` `TestComposeHubNotifyPromptWithTelegramBinding` / `TestComposeHubNotifyPromptWithoutBinding` (updated): assert the prompt names `submit_review_outcome` and `status="approved"` instead of the uncallable `status="done"`.
- `V-2` `go build ./...`, `go vet ./internal/runner` clean.
- `V-3` Broad regression sweep (`Flow|Review|Orchestrat|Cohort|Hub|Reinvoke|Telegram|Audit|Validate|Behavior|Resume|Gate|Continue|Loop`): 478 passed, 7 pre-existing/unrelated codex-binary-dependent failures (confirmed baseline), 0 new failures.

## 9. Regression Guard

- tests: the two updated `composeHubNotifyPrompt` tests (`flow_hub_notify_test.go`) guard the exact tool name/enum value used.
- alerts: none.
- audit checks: `change-audit/CA-327` records this fix.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: `submit-review-outcome.yaml`'s schema and `statusMap` are unchanged — this fix only corrects the PROMPT that tells the model how to use the existing, correctly-designed tool contract.
