# CA-320: Hub Notify Node Behavior — Reinvoke Hub For AI-Composed Notification

## Scope

Implements Task-235: a new inline-scope flow-node behavior `hub.notify`, sibling to `hub.inline`, that lets a flow author place a node whose completion runs as ANOTHER TURN OF THE SAME HUB SESSION (no child agent spawn) — so the AI already driving the flow can freely compose a notification (e.g. fill a `telegram.v1` OUTPUT artifact's Message Template) rather than either a fixed/templateless Go-only send (`telegram.notify`, added earlier this session) or spawning a brand-new child agent (`agent.delegate`).

Also generalizes `advanceHubDoneThroughEdge` (added earlier this session, uncommitted-as-its-own-task, to let a node follow a synthesizer's "done" onto a real successor node instead of always settling the flow) to track WHICH hub-driven node is currently active, so a *second* hub-turn node resolves its own "done" against its own forward edge instead of re-resolving against the flow's first `hub.inline` node and looping forever.

## Changes

- `apps/local-runner/internal/runner/behavior_registry.go`: added `BehaviorHubNotify BehaviorID = "hub.notify"`.
- `apps/local-runner/internal/runner/behavior_registry_builtin.go`: registered `hub.notify` as `BehaviorScopeInline` with a thin stub handler `behaviorHubNotify` (real work lives in the dispatch layer, matching `telegram.notify`'s shape).
- `apps/local-runner/internal/agentpack/pack.go`: added `hub.notify` / `notify.hub` aliases to `NormalizeBehaviorID`.
- `apps/local-runner/internal/runner/interactive_service.go`:
  - added `interactiveRun.activeHubNodeID string` — the flow node id of whichever hub-driven inline node is currently awaiting the hub's own `flow_control("done")` call.
  - `advanceHubDoneThroughEdge` now resolves the hub node id from `activeHubNodeID` (falling back to `hubInlineNodeID(nodes)` when empty, reproducing pre-existing behavior for every current built-in flow); when the resolved forward "done" edge targets a `hub.notify` node, dispatches via the new `dispatchHubNotifyNode` instead of `advanceToNextInlineOrDelegate`; clears `activeHubNodeID` back to `""` when the edge resolves to a terminal.
  - added `dispatchHubNotifyNode(parentRunID, node)` — the single entry point for reaching `hub.notify`, shared with the dispatch-layer call site below.
  - added `maybeAutoReinvokeHubWithPrompt(parentRunID, prompt)` — an additive sibling of `maybeAutoReinvokeHubWithNote` that duplicates its single-flight/loop-status/round-cap guard verbatim but takes an arbitrary prompt instead of `cohortNote + autoReinvokePromptText()`. Deliberately NOT a refactor of the original (BUG-234/BUG-275-tagged, heavily exercised by every review-loop-shaped flow) function — GitNexus impact analysis was unavailable this session to safely verify a shared-function edit's blast radius, so a small additive duplicate carries zero risk to that path instead.
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`:
  - added `composeHubNotifyPrompt(node)` — reuses `appendTelegramOutputPrompt` (already softened this session against injection-pattern false-positive refusals) plus a generic "call the flow control tool with status=\"done\"" instruction.
  - added a `"hub.notify"` case to both `tryAdvanceFlowThroughInline`'s switch (reached as a single forward-done target from any completed node) and `advanceToNextInlineOrDelegate`'s switch (reached as the next hop after another inline node's own advance), both calling `dispatchHubNotifyNode` — so `hub.notify` behaves identically regardless of graph position.
- `apps/local-runner/internal/runner/behavior_registry_test.go`: added `hub.notify` / `notify.hub` to the known-alias list.
- `apps/local-runner/internal/agentpack/flow-pack/behaviors/registry.yaml`: added doc entries for `hub.notify` and (backfilled, was missing) `telegram.notify` (reference-only, not runtime-authoritative per the file's own header).
- `packages/flowpilot-client-core/src/domain/adminModels.ts`: added `hub.notify` to `FLOW_BEHAVIOR_OPTIONS` (`requiresAgent: false`) — selectable in Settings → Workflows → Steps.
- New test `apps/local-runner/internal/runner/flow_hub_notify_test.go`: `TestComposeHubNotifyPromptWithTelegramBinding`, `TestComposeHubNotifyPromptWithoutBinding`, `TestAdvanceHubDoneThroughEdgeDispatchesHubNotify`, `TestAdvanceHubDoneThroughEdgeResolvesSecondHubNodeAgainstOwnEdge` (the direct regression proof against the "loops forever re-finding synthesis" bug this design must avoid), `TestTryAdvanceFlowThroughInlineDispatchesHubNotify`.
- New Task doc: `requirements/08-Task/done/Task-235-Hub-Notify-Node-Behavior-Reinvoke-Hub-For-AI-Composed-Notification.md`.

## Verification

- `go build ./...` (apps/local-runner) — passed. `go vet ./internal/runner ./internal/agentpack` — no issues.
- `go test ./internal/runner -run 'HubNotify|ComposeHubNotify|ResolvesSecondHubNode'` — 5 passed (all new tests).
- `go test ./internal/runner -run 'Flow|Review|Orchestrat|Cohort|Hub|Reinvoke|Telegram|Audit|Validate|Behavior'` — 395 passed (broad regression sweep across every touched/adjacent path; run both before and after this change, no new failures).
- `npx tsc --noEmit` (apps/desktop-flowpilot) — clean.
- `go test -race` could not run in this environment (CGO disabled) — not claimed as verified; every read/write of `activeHubNodeID` was manually reviewed to confirm it always occurs inside `s.mu.Lock()/Unlock()`.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`, no `gitnexus_*` tool resolved) — proceeded via careful manual inspection of every touched symbol instead of automated impact analysis, per the add-new-task skill's fallback instruction. `gitnexus_detect_changes()` not run for the same reason.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-235
change_type: feature
summary: add hub.notify inline node behavior that reinvokes the hub's own session (no child spawn) to AI-compose and send a notification, tracking the active hub-driven node so a second hub turn resolves its own edge instead of looping
# --->8---
