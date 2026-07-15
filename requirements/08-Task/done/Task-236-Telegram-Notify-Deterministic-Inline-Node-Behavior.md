# Task-236: `telegram.notify` — Deterministic (No-Agent) Telegram Send Node Behavior

## Metadata

- Document ID: `Task-236`
- Title: `telegram.notify — Deterministic (No-Agent) Telegram Send Node Behavior`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (`P-3`, `P-5`), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) (`D-1`, `D-7`), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md)
- Child Documents: [Task-235: Hub Notify Node Behavior](./Task-235-Hub-Notify-Node-Behavior-Reinvoke-Hub-For-AI-Composed-Notification.md) (builds on this task's `runTelegramNotifyNode`/`resolveTelegramMessage`/dispatch wiring as its Go-inline sibling)
- Related Documents: [Task-233: Telegram Output Write-Contract, Verify Gate, And Approval](./Task-233-Telegram-Output-Write-Contract-Verify-Gate-And-Approval.md) (source of `requiredTelegramOutputTargets`/`executeTelegramLoopbackSend` this task reuses), [Task-232: Telegram Output Artifact Type And Bot-API Proxy MCP](./Task-232-Telegram-Output-Artifact-Type-And-Bot-API-Proxy-MCP.md), [CP-05-05: Telegram MCP As An Output Notification Artifact](../../07-Coding-Plan/done/CP-05-05-Tele-Mcp.md), [Task-237: Generalize Post-Node "Done" Edge-Walking (Audit And Hub-Inline Successor Chaining)](./Task-237-Generalize-Post-Node-Done-Edge-Walking-Audit-And-Hub-Inline-Successor-Chaining.md) (this task's `runTelegramNotifyNode` is the first consumer of the successor-chaining Task-237 later generalizes)
- Replaces: `None`
- Tags: `agent-flow-engine, node-behavior, telegram, notification, flow-pack, no-agent`

## AI Quick View

### Summary

- New inline-scope node behavior `telegram.notify`: sends a bound `telegram.v1` OUTPUT message **directly from the Go runner**, with **no AI turn at all** (not even a hub reinvoke) — for the case where a fixed or already-fully-formed message is enough and the token/turn cost of any AI involvement is unwanted.
- Reuses `executeTelegramLoopbackSend` (the same in-runner send path `Task-233`'s MCP tool proxies to under `BUG-281`'s loop-back mode) directly, so it inherits credential resolution, the auto-approve gate, and real `message_id` verification with zero new Telegram-API code.
- Message text is deterministic: the artifact's configured Message Template verbatim if set, else a default `"[FlowPilot] Run finished.\n\n" + <prior node's result, truncated>` — no template variable substitution in this slice (documented as `Q-1`).
- Motivated by the user's minimal test flow (`notify(telegram.notify, bind telegram.v1 OUTPUT+chatId) --done--> done`) reached mid-flow after an `agent.delegate` entry node — proved out the "no-agent send" path before `Task-235`'s AI-composed sibling (`hub.notify`) was built for the "needs template-variable filling" case.

### Current Ask

- Add `telegram.notify` end-to-end (constants, registry, alias, dispatch from both existing inline-behavior entry points, desktop authoring dropdown) so a flow node bound to `telegram.v1` OUTPUT can send without any agent turn.

### Key Decisions

- `T-1` `telegram.notify` is `BehaviorScopeInline`; the registry handler (`behaviorTelegramNotify`) is a thin stub — the real send logic lives in the executor dispatch layer (`runTelegramNotifyNode`), matching how `command.validate`/`artifact.audit_draft` already split "registry entry" from "dispatch implementation".
- `T-2` Reuses `executeTelegramLoopbackSend` (not a new Telegram Bot API call) — this is the exact function the provider-spawned MCP proxy already calls under loop-back mode, so behavior (credential resolution, `autoApprove` gate, `message_id` extraction) is identical whether an AI or the flow engine triggers the send.
- `T-3` `telegramLoopbackSendRequest` gained an optional `ChatID` field so a flow node's own bound `chatId` can override the connected integration's default channel — the provider-spawned MCP child path never sets it, so existing behavior for that path is unchanged.
- `T-4` A send that does not complete (auto-approve off, API error) escalates the flow (`ask_user`) rather than settling `done` — never a fabricated success.
- `T-5` A node with no `telegram.v1` OUTPUT binding at all is a no-op pass-through to its own forward `"done"` edge (mirrors `command.validate`'s "no command configured" degrade).

### Constraints

- No AI composition and no template-variable substitution — a fixed/deterministic message only; `Task-235`'s `hub.notify` is the sibling for when AI composition is needed.
- Do not duplicate `executeTelegramLoopbackSend`'s credential/gate logic — call it directly.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via careful manual inspection instead of automated impact analysis.

### Open Questions

- `Q-1` No template variable substitution (e.g. `{{status}}`) — a Message Template is sent verbatim. Follow-up if a real per-run status value needs to be filled deterministically without an AI turn.
- `Q-2` No flow-gate write-contract rule is needed for this path specifically because Go both performs and verifies the send (`message_id` extraction) in the same function — unlike the AI-driven `agent.delegate` + MCP path, there is nothing for a gate to independently verify.

### Source Refs

- `CP-42` `P-3`, `P-5`; `SD-19` `D-1` (domain-free engine — a new behavior ID needs no engine-level use-case code), `D-7`.
- `Task-233` (`requiredTelegramOutputTargets`, `executeTelegramLoopbackSend`, `telegramCredential.AutoApprove` this task reuses unmodified).
- Code anchors: `behavior_registry.go`, `behavior_registry_builtin.go`, `pack.go` (alias table), `flow_validate_audit_dispatch.go` (`runTelegramNotifyNode`, `resolveTelegramMessage`, `tryAdvanceFlowThroughInline`/`advanceToNextInlineOrDelegate` switch cases), `telegram_loopback.go` (`telegramLoopbackSendRequest.ChatID`, `executeTelegramLoopbackSend`), `adminModels.ts` (`FLOW_BEHAVIOR_OPTIONS`).

## 1. Goal

Let a flow author bind a `telegram.v1` OUTPUT artifact to a node whose behavior is `telegram.notify`, so that reaching that node sends the notification deterministically from the Go runner — no agent spawned, no hub turn consumed — verified by a real `message_id`, then advances the node's own forward edge.

## 2. Parent Links

- coding plan: [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) — `P-3` (behavior selected by node definition), `P-5` (declared tool faces map onto generic control)
- tech design: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) — `D-1` (domain-free engine: adding this behavior required zero engine-level use-case branching), `D-7` (`FlowNode.run: inline|delegate` vocabulary — this is a `run: inline` instance with no provider call at all)
- system spec: [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md)
- specific upstream ids: `CP-42 P-3`, `P-5`; `SD-19 D-1`, `D-7`

## 3. Trigger

The user asked how to build the smallest possible flow to send a Telegram notification — a single `agent.delegate` entry node followed by a `notify` step — and specifically asked whether a node could send "inline" without spawning any agent for it. Prior art (`Task-233`) only wired Telegram OUTPUT through an `agent.delegate` node's AI-driven MCP tool call; there was no path for a flow node to send deterministically with zero AI involvement.

## 4. Exact Change

- `T-1` Add constant `BehaviorTelegramNotify = "telegram.notify"` (`behavior_registry.go`) and register it as `BehaviorScopeInline` with a stub handler `behaviorTelegramNotify` (`behavior_registry_builtin.go`).
- `T-2` Add aliases `telegram.notify` / `notify.telegram` to `agentpack.NormalizeBehaviorID` (`pack.go`).
- `T-3` Add `resolveTelegramMessage(target, resultMessage)` (`flow_validate_audit_dispatch.go`) — the artifact's Message Template verbatim if set, else a default header + the prior node's result truncated to 1500 chars.
- `T-4` Add `runTelegramNotifyNode(ctx, parentRunID, edges, nodes, node, resultMessage)` (`flow_validate_audit_dispatch.go`): no binding → pass through to the forward `"done"` edge; binding present → for each target, call `executeTelegramLoopbackSend`; on failure/approval-required, escalate (`ask_user`) instead of settling; on success, advance via `advanceToNextInlineOrDelegate`.
- `T-5` Add optional `ChatID` field to `telegramLoopbackSendRequest` (`telegram_loopback.go`) and thread it through `executeTelegramLoopbackSend` so a node's own bound `chatId` can override the connected integration's default channel; the provider-spawned MCP child path leaves it unset, so its behavior is unchanged.
- `T-6` Add `"telegram.notify"` cases to both `tryAdvanceFlowThroughInline`'s switch (reached as a single forward-done target from a completed node) and `advanceToNextInlineOrDelegate`'s switch (reached as the next hop after another inline node's advance).
- `T-7` Add `telegram.notify` to `FLOW_BEHAVIOR_OPTIONS` (`adminModels.ts`, `requiresAgent: false`) — selectable in Settings → Workflows → Steps.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/behavior_registry.go`, `behavior_registry_builtin.go`, `behavior_registry_test.go`, `flow_validate_audit_dispatch.go`, `telegram_loopback.go`; `apps/local-runner/internal/agentpack/pack.go`; `packages/flowpilot-client-core/src/domain/adminModels.ts`; test `apps/local-runner/internal/runner/flow_telegram_notify_test.go`
- modules: flow-engine behavior registry, flow executor dispatch, Telegram loop-back send path, desktop step-authoring UI
- routes: none
- tables: none

## 6. Acceptance Check

- A `telegram.notify` node bound to a `telegram.v1` OUTPUT with an explicit `chatId` (different from the connected integration's default channel) sends to ITS OWN bound `chatId`, not the default — proves the per-node `ChatID` override.
- A `telegram.notify` node with no OUTPUT binding at all passes through to its own forward `"done"` edge without attempting any send.
- Auto-approve OFF on the connected integration: no send occurs, and the flow escalates (`ask_user`) instead of settling done.
- `go build ./...`, `go vet ./internal/runner ./internal/agentpack` clean; `npx tsc --noEmit` (desktop) clean.
- `go test ./internal/runner -run 'TelegramNotify|ResolveTelegramMessage'` and the wider `Flow|Telegram|Behavior|Validate|Audit|Advance|Inline` regression sweep both pass with no new failures.

## 7. Out of Scope

- AI-composed messages / Message Template variable substitution — that is `Task-235`'s `hub.notify` (or the pre-existing `agent.delegate` + MCP path from `Task-233`).
- Flow-gate write-contract verification for this path — unnecessary since Go both sends and verifies (`message_id`) in the same function.
- Wiring this behavior by default into the built-in pack YAML (`rag-harness.yaml` / `context-coding-review-synthesis.yaml`) — stays a user-authored addition in Settings → Workflows, matching `Task-233 T-5`'s "manual workflow wiring is the supported path" decision.

## 8. Completion Notes

- result: done. `T-1`–`T-7` implemented; `go build`/`go vet`/`tsc --noEmit` clean; targeted and broad regression suites pass with no new failures against the pre-change baseline.
- This document backfills a slice implemented earlier in the same working session as `Task-235`, in direct response to the user's chat request rather than through the `/add-new-task` skill at the time it was built; it is written now, after the fact, to close that traceability gap (flagged as a follow-up in `Task-235`'s Completion Notes).
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`, no `gitnexus_*` tool resolved) — proceeded via manual inspection of every touched symbol instead of automated impact analysis.
- follow-ups: `Q-1` (template variable substitution), `Q-2` noted above.
- upstream docs updated: none — `telegram.notify` is a new instance within the `run: inline` vocabulary `SD-19 D-7` already defines; it does not contradict or change upstream meaning.
