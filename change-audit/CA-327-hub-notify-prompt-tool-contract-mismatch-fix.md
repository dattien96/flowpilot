# CA-327: hub.notify Prompt Now Names The Actual Callable Tool/Status Value

## Scope

Fixes BUG-287: `composeHubNotifyPrompt` instructed the model to "call the flow's control tool with status=\"done\"", but the only tool actually exposed to the session (`submit_review_outcome`) has no such enum value — its `status` field is strictly `approved | changes_requested | blocked`, and its own description says "This is the only flow-control tool — do not use flow_control directly." Unable to literally comply, the model resubmitted an unrelated, stale verdict (`blocked`, with the earlier reviewer's feedback text), re-escalating a flow that had already sent its Telegram notification successfully.

## Changes

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`: `composeHubNotifyPrompt`'s closing instruction now explicitly names `submit_review_outcome` and `status="approved"` (which maps to `done` via `submit-review-outcome.yaml`'s own `statusMap`), with an added sentence clarifying this is a technical formality to advance the flow, not a real code-review judgment. Scoped fix — hardcoded to the one control-tool name every flow that can currently reach `hub.notify` declares; not generalized to a hypothetical future flow with a different tool schema (flagged as an open question in BUG-287).
- Updated tests (`flow_hub_notify_test.go`): `TestComposeHubNotifyPromptWithTelegramBinding` / `TestComposeHubNotifyPromptWithoutBinding` now assert the prompt names `submit_review_outcome` / `status="approved"` instead of the uncallable `status="done"`.

## Verification

- `go build ./...` — passed. `go vet ./internal/runner` — no issues.
- `go test ./internal/runner -run 'HubNotify|ComposeHubNotify|ResolvesSecondHubNode|ForwardReachable|ApplyFlowControlContinue|ApplyFlowControlLooping'` — 17 passed.
- Broad regression sweep (`Flow|Review|Orchestrat|Cohort|Hub|Reinvoke|Telegram|Audit|Validate|Behavior|Resume|Gate|Continue|Loop`) — 478 passed, 7 pre-existing/unrelated codex-binary-dependent failures (confirmed baseline), 0 new failures.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via direct inspection of the diagnostic log (`run-9664`) and `claude_mcp_server.go`'s actual tool schema.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-287
change_type: bugfix
summary: fix composeHubNotifyPrompt to instruct the model to call submit_review_outcome with status=approved (the only tool/value it can actually invoke) instead of a nonexistent generic status=done, which was causing the model to resubmit a stale verdict and re-escalate an already-completed flow
# --->8---
