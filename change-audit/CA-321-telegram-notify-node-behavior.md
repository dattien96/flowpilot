# CA-321: telegram.notify Deterministic (No-Agent) Node Behavior

## Scope

Implements Task-236: a new inline-scope flow-node behavior `telegram.notify` that sends a bound `telegram.v1` OUTPUT message directly from the Go runner — no agent spawn, no hub turn, no AI involvement at all — for the case where a fixed/already-templated message is enough. Reuses `executeTelegramLoopbackSend` (the same send path Task-233's MCP proxy already calls under loop-back mode), so credential resolution, the auto-approve gate, and real `message_id` verification are unchanged/shared, not reimplemented.

This slice was implemented earlier in the same working session as CA-320 (Task-235, `hub.notify`), in direct response to the user's chat request rather than through the `/add-new-task` skill at the time; this note backfills that gap.

## Changes

- `apps/local-runner/internal/runner/behavior_registry.go`: added `BehaviorTelegramNotify BehaviorID = "telegram.notify"`.
- `apps/local-runner/internal/runner/behavior_registry_builtin.go`: registered `telegram.notify` as `BehaviorScopeInline` with stub handler `behaviorTelegramNotify` (real work lives in the dispatch layer).
- `apps/local-runner/internal/agentpack/pack.go`: added `telegram.notify` / `notify.telegram` aliases to `NormalizeBehaviorID`.
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`:
  - added `resolveTelegramMessage(target, resultMessage)` — the artifact's Message Template verbatim if set, else a default header + the prior node's result (truncated 1500 chars). No template variable substitution (`{{status}}` etc.) in this slice.
  - added `runTelegramNotifyNode(ctx, parentRunID, edges, nodes, node, resultMessage)`: no binding → pass through to the forward `"done"` edge; binding present → send per target via `executeTelegramLoopbackSend`; failure/approval-required → escalate (`ask_user`), never a fabricated success; success → advance via `advanceToNextInlineOrDelegate`.
  - added `"telegram.notify"` cases to both `tryAdvanceFlowThroughInline`'s switch and `advanceToNextInlineOrDelegate`'s switch.
- `apps/local-runner/internal/runner/telegram_loopback.go`: added optional `ChatID` field to `telegramLoopbackSendRequest`, threaded through `executeTelegramLoopbackSend` so a node's own bound `chatId` overrides the connected integration's default channel; the provider-spawned MCP child path leaves it unset, so its behavior is unchanged.
- `apps/local-runner/internal/runner/behavior_registry_test.go`: added `telegram.notify` / `notify.telegram` to the known-alias list.
- `packages/flowpilot-client-core/src/domain/adminModels.ts`: added `telegram.notify` to `FLOW_BEHAVIOR_OPTIONS` (`requiresAgent: false`).
- New test `apps/local-runner/internal/runner/flow_telegram_notify_test.go`: message-resolver unit tests, a full-dispatch send test proving the per-node `chatId` override beats the connected default, a no-binding pass-through test, and an auto-approve-off escalate test.
- New Task doc: `requirements/08-Task/done/Task-236-Telegram-Notify-Deterministic-Inline-Node-Behavior.md`.

## Verification

- `go build ./...` (apps/local-runner) — passed. `go vet ./internal/runner ./internal/agentpack` — no issues.
- `go test ./internal/runner -run 'TelegramNotify|ResolveTelegramMessage'` — all passed (message resolver + dispatch tests).
- `go test ./internal/runner -run 'Flow|Telegram|Behavior|Validate|Audit|Advance|Inline'` — broad regression sweep, no new failures against the pre-change baseline.
- `npx tsc --noEmit` (apps/desktop-flowpilot) — clean.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`, no `gitnexus_*` tool resolved) — proceeded via careful manual inspection of every touched symbol instead of automated impact analysis. `gitnexus_detect_changes()` not run for the same reason.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-236
change_type: feature
summary: add telegram.notify deterministic inline node behavior that sends a bound telegram.v1 OUTPUT message directly from the Go runner with no agent turn, reusing executeTelegramLoopbackSend
# --->8---
