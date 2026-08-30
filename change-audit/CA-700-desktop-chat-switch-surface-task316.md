# CA-700 — Desktop chat switch surface (Task-316): chat-scoped switch keeping the timeline + grouping

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-59
change_type: feature
summary: Desktop confirmProviderSwitch routes the chat-scoped switch endpoint (runner mints the leg, seeds server-side) keeping the timeline with one divider and kept artifacts (D-7); legacy Task-078 path preserved verbatim; client bindings + DTO mirrors; run-history grouped one-row-per-chat with legs chip; close-out 7b91ced3 fixes legacy collision + clamp cwd; typecheck green, store.chatSwitch 6/6 + chatHistory 2/2, phase1 363/375 (12 pre-existing fails)
# --->8---

## What changed

- `types/contract.ts`: `StartRunInput`/`RunHandle` chat fields; `HANDOFF_PROMPT_PREFIX`; `ChatSwitch*`/`ChatTimeline*` DTOs; `RunnerClient` gains `switchChatProvider` + `chatTimeline`; `RunHistoryItem` gains `chatId`/`legSeq` (mirrors the runner's extended runHistoryItem, CA-699 follow-up on the runner side landed in this slice).
- `HttpWsRunnerClient` + `MockRunnerClient`: bindings (Mock encodes `handoff_same_provider` refusal + per-chat leg counting).
- `store.ts`: `chatId` state rides every run adoption and clears on reset; `confirmProviderSwitch` — when `chatId` exists → `switchChatProvider` → operational-state reset with **timeline KEPT** + one divider (`seed-divider-<runId>`, D-7 single source; the runner-side seed turn is not double-rendered because Desktop `turn_started` prompts are not rendered as user items — surface nuance vs TUI) → **legacy Task-078 path verbatim** when `chatId` unknown (flag off / old runner).
- `chatHistory.ts` (new): `groupRunsByChatId`/`flattenGroupedHistory` — one navigator row per logical chat (latest leg is the face, legs keep insertion order), untagged items 1:1; `Navigator` groups `activeHistory` through it and renders a legs-count chip.

## R1 / verification status

- `npm run typecheck`: PASS (apps/desktop-flowpilot, `tsc --noEmit` green)
- Tests: `chatHistory.test.ts` 2/2 PASS; `store.chatSwitch.test.ts` 6/6 PASS (99-104) via `tsc -p tsconfig.phase1-tests.json` + `node --require scripts/phase1-runtime.js --test` (was blocked by runId collision + alias debt — fixed in 7b91ced3); `phase1` 363/375 PASS (12 fails pre-existing, documented, down from 15 — 3 fixed: DOD-4 + 2 clamp cwd-robust)
- Close-out: `store.ts:1065` artifacts kept (remove `artifacts:[]` on chat-scoped path); `store.chatSwitch.test.ts:99` run-legacy-1 collision fixed; `timelinePromptClamp.test.ts:1` + `timelineSkillSummary.test.ts:1` cwd-robust repoRoot helper; `.phase1-tests` mirrors synced
- Runner: `runHistoryItem` chat fields additive; runner suite unchanged (baseline 12 fails + flakes)

## Falsifiable expectations locked

1. Desktop chip switch with a known chatId hits `/client/chats/{id}/switch-provider` and keeps every prior timeline message.
2. Without chatId the Task-078 two-step path runs verbatim (timeline reset included).
3. Grouped navigator renders one row per chat with a legs chip; untagged history unchanged.
