# CA-700 — Desktop chat switch surface (Task-316): chat-scoped switch keeping the timeline + grouping

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-59
change_type: feature
summary: Desktop confirmProviderSwitch routes the chat-scoped switch endpoint (runner mints the leg, seeds server-side) keeping the timeline with one divider; legacy Task-078 path preserved verbatim as fallback; client bindings + DTO mirrors; run-history grouped one-row-per-chat with a legs chip; typecheck green, 2 runnable grouping tests, store tests compile but runtime-blocked by pre-existing phase1 breakage
# --->8---

## What changed

- `types/contract.ts`: `StartRunInput`/`RunHandle` chat fields; `HANDOFF_PROMPT_PREFIX`; `ChatSwitch*`/`ChatTimeline*` DTOs; `RunnerClient` gains `switchChatProvider` + `chatTimeline`; `RunHistoryItem` gains `chatId`/`legSeq` (mirrors the runner's extended runHistoryItem, CA-699 follow-up on the runner side landed in this slice).
- `HttpWsRunnerClient` + `MockRunnerClient`: bindings (Mock encodes `handoff_same_provider` refusal + per-chat leg counting).
- `store.ts`: `chatId` state rides every run adoption and clears on reset; `confirmProviderSwitch` — when `chatId` exists → `switchChatProvider` → operational-state reset with **timeline KEPT** + one divider (`seed-divider-<runId>`, D-7 single source; the runner-side seed turn is not double-rendered because Desktop `turn_started` prompts are not rendered as user items — surface nuance vs TUI) → **legacy Task-078 path verbatim** when `chatId` unknown (flag off / old runner).
- `chatHistory.ts` (new): `groupRunsByChatId`/`flattenGroupedHistory` — one navigator row per logical chat (latest leg is the face, legs keep insertion order), untagged items 1:1; `Navigator` groups `activeHistory` through it and renders a legs-count chip.

## R1 / verification status

- `npm run typecheck`: all CP-59 files green. Pre-existing debt (NOT this change, verified on the clean main tree): `store.run75035-timeline-isolation.test.ts` ProviderEventDTO fixture errors; `test:phase1` fails repo-wide on unrelated tsc errors (settingsHelpers/workflowFlowEngineAttrs) BEFORE any test executes.
- Runnable tests: `chatHistory.test.ts` 2/2 PASS. `store.chatSwitch.test.ts` compiles but cannot execute standalone: the phase1 runner has no `@/*` alias runtime resolution and `test:phase1` is blocked repo-wide (above). Making it runnable = fix the phase1 runner debt — separate bug, `chat-history` adjacent, next-session item with Task-315 slice 3 leftovers.
- Runner: `runHistoryItem` chat fields additive; runner suite unchanged (13 baseline + proven flakes).

## Falsifiable expectations locked

1. Desktop chip switch with a known chatId hits `/client/chats/{id}/switch-provider` and keeps every prior timeline message.
2. Without chatId the Task-078 two-step path runs verbatim (timeline reset included).
3. Grouped navigator renders one row per chat with a legs chip; untagged history unchanged.
