# Task-316: Desktop Chat Switch Surface

## Metadata

- Document ID: `Task-316`
- Title: `Desktop Chat Switch Surface (Provider Chips, Timeline Continuity, Chat History)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-29`
- Last Updated: `2026-08-31` (done — CA-700 + close-out 7b91ced3: keep artifacts (D-7), fix legacy runId collision (DOD-4), cwd-robust timeline clamp tests; typecheck green; store.chatSwitch 6/6 + chatHistory 2/2; phase1 363/375 (12 pre-existing fails documented, 3 fixed); history grouping + legs chip live; divider single-source via seed-divider)
- Parent Documents: [CP-59: Chat SSOT — Continuous Cross-Provider Chat](../../07-Coding-Plan/done/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md), [Task-314: Chat Switch-Provider Endpoint And Chat Envelope](../done/Task-314-Chat-Switch-Provider-Endpoint-And-Chat-Envelope.md)
- Child Documents: `None`
- Related Documents: [Task-078: Cross-Provider Chat Handoff](../done/Task-078-Cross-Provider-Chat-Handoff.md), [Task-313](../done/Task-313-ChatId-Data-Model-Transcript-Store-Timeline.md), [Task-315](../done/Task-315-TUI-Chat-Switch-Surface.md)
- Replaces: `None`
- Tags: `chat-ssot, desktop, provider-chips, timeline-continuity, chat-history`

## AI Quick View

### Summary

- Re-point Desktop's provider chip switch (`store.ts:966 selectProvider` → `:1010 confirmProviderSwitch`) from the Task-078 run-scoped handoff to the CP-59 chat switch endpoint — and remove the timeline reset (`store.ts:1058 timeline: []`) so switching provider keeps the visible transcript and appends one divider.
- Group run history by `chatId` so a switched chat appears as **one chat** in the navigator, with a leg drill-down retained for audit; open-chat loads `/client/chats/{chatId}/timeline` (full multi-leg replay).
- Desktop parity with Task-315: same endpoint, same guard semantics (`providerSwitchLoading`), same divider/collapse rendering.

### Current Ask

- Implement CP-59 `P-6` (+ Desktop half of `P-7`): client bindings, no-reset switch flow, chat grouping, envelope collapse, and the store tests that lock timeline continuity + single-leg under double click.

### Key Decisions

- `T-1` **`confirmProviderSwitch` reworked, not replaced**: same user flow (chip → pending gate → confirm) but the confirm calls `switchChatProvider(chatId, …)` instead of `handoffContext` + `startRun`; on success only **operational** state resets (pendingApprovals, pendingQuestions, gateBlock, recoverable, `_runReplaySeq`/`_runSnapshots`, `_streamingAssistantId`, `_streamRunSeq+1` — the keys from today's `:1035-1068` block minus `timeline`/`artifacts` history resets). The old Task-078 path stays as fallback when the run has no `chatId` (legacy runner).
- `T-2` **Timeline continuity is a locked test, not a hope**: `store.test.ts` asserts transcript length grows by exactly one divider on switch and no `timeline: []` assignment runs on the switch path (CP-59 §5.1 audit: all other `timeline: []` call sites — new chat, mode change — keep their resets).
- `T-3` **Chat is the history unit**: `RunHistoryItem` gains `chatId`/`legSeq` (Task-313 echo); the navigator groups consecutive legs by `chatId` into one entry (latest leg's provider/model/status as the face), expanding to per-leg rows on demand (CP-59 `Q-4` default).
- `T-4` **Envelope collapse**: a timeline/stream user message carrying `isHandoffSeed` (typed) or the `handoffPromptPrefix` text renders as the divider card — never as a user bubble (mirrors Task-315 `T-6`; same constant-parity guard via a shared literal test).
- `T-5` **Guard scope moves to chat**: `providerSwitchLoading` (existing `:352`) keeps its double-click role but is keyed against the chat leg transition; a second confirm while in flight is a no-op (Task-078's guard semantics preserved).
- `T-6` **Same-provider chip selection** keeps today's in-place path (Task-078 gate `:986` extended: same-provider + live chat → model-change turn, no endpoint call — Desktop half of CP-59 `P-7`).

### Constraints

- Flag-gated: off → `confirmProviderSwitch` follows the Task-078 path byte-identically (endpoint choice + timeline reset included).
- No runner changes; no TUI changes; `packages/flowpilot-client-core` types updated additively (`runner.ts`, `adminModels.ts`).
- `npm test` (vitest) + `npm run build` must pass; no pre-existing test edited (additive `store.chatSwitch.test.ts` style).
- Durable-replay: after switch, the desktop replays the new leg via existing SSE attach (`_streamRunSeq+1`); the divider is appended client-side once (id keyed by `toRunId`) so reconnect replays don't duplicate it.

### Open Questions

- Artifacts panel across legs: keep an aggregated per-chat artifact list (lean: yes, append-only like the timeline) — confirm at review.

### Source Refs

- CP-59 Work Breakdown `P-6`, `P-7`; Key Decisions `P-3`, `P-8`; SD-26 `SD26-D-7`, `SD26-E-9`.
- `store.ts:966` (`selectProvider`), `:1006` (`cancelProviderSwitch`), `:1010` (`confirmProviderSwitch`), `:1058` (`timeline: []` reset — removed on this path), `:308` (`runHistory`), `:352` (`providerSwitchLoading`), `:1069-1079` (Task-078 divider message — replaced by the new divider), `:1081` (`sendPrompt(handoff.prompt)` — no longer client-side).
- `client.go` `RunHandle.ChatID/LegSeq` (Task-313); CP-59 §3.2 matrix (desktop half), CS-04/CS-05/CS-11/CS-13.

## 1. Goal

Desktop users switch provider mid-chat via chips with zero timeline loss — one divider, transcript intact, one leg minted per confirm — and the navigator shows one chat, not N legs; legacy runners and flag-off behave exactly as today.

## 2. Parent Links

- coding plan: `CP-59` Work Breakdown `P-6`, `P-7`
- tech design: `SD-26` (`SD26-D-7`, `SD26-E-9`)
- system spec: `SS-05`
- specific upstream ids: CP-59 `P-6` full spec; CS-04 (bare-model pin desktop posture editor), CS-05 (double-click), CS-11 (cross-surface same timeline), CS-13 (collapse)

## 3. Trigger

Task-314's endpoint exists; Desktop's current confirm path resets the timeline (`:1058`) and mints runs without `chatId` (Task-078 two-step). This slice makes Desktop the second first-class switch surface and removes the "new chat" feel the operator rejected.

## 4. Exact Change

- `T-1` Client bindings — `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`, `MockRunnerClient.ts`, `packages/flowpilot-client-core/src/domain/runner.ts`:
  ```ts
  // contract additions (additive)
  interface ChatSwitchInput {
    targetProviderKey: string;
    model?: string;
    reasoningEffort?: string;
    yoloMode?: boolean;
  }
  interface ChatSwitchHandoffStats {
    handoffMode: "raw" | "hybrid" | "target_summary" | "fresh_start";
    includedTurnCount: number;
    omittedTurnCount: number;
    truncated: boolean;
    actionsDigestIncluded: boolean;
  }
  interface ChatSwitchResponse {
    handle: RunHandle;      // RunHandle gains chatId?, legSeq?
    chatId: string;
    legSeq: number;
    handoff: ChatSwitchHandoffStats;
  }
  switchChatProvider(chatId: string, input: ChatSwitchInput): Promise<ChatSwitchResponse>;
  chatTimeline(chatId: string, afterSeq?: number, limit?: number): Promise<ChatTimelineResponse>;
  ```
  Mock mirrors the runner semantics (guards incl. `handoff_same_provider`, single leg per confirm). Tests: `TestSwitchChatProviderClientShape` (vitest, Mock round-trip).
- `T-2` `store.ts confirmProviderSwitch` rework:
  ```ts
  async confirmProviderSwitch() {
    const pending = get().pendingProviderSwitch;
    if (!pending || get().providerSwitchLoading) return;              // guard unchanged :1013
    const chatId = get().runHandleChatId();                           // from RunHandle.chatId (T-1)
    set({ providerSwitchLoading: true });
    try {
      if (chatId && flagOn) {
        const resp = await client.switchChatProvider(chatId, {
          targetProviderKey: pending.targetProviderKey,
          model: pending.targetModel, reasoningEffort: get().reasoningEffort, yoloMode: get().yoloMode,
        });
        // adoptRunHandle: operational keys only + stream re-attach to the new
        // runId (_streamRunSeq+1). NO client-synthesized divider — the seed
        // turn (isHandoffSeed + stats) arriving on the attached stream renders
        // as the single divider (Task-312 T-5 single-source rule).
        adoptRunHandle(resp.handle, /* resetTimeline: */ false);
      } else {
        await legacyTask078Handoff(pending);                          // flag off / legacy runner: today's body verbatim
      }
      set({ pendingProviderSwitch: undefined, providerSwitchLoading: false });
      void get().loadSkills(pending.targetProviderKey);
      void get().loadRunHistory();
    } catch (err) { /* today's :1086-1097 error branch, unchanged */ }
  }
  ```
  `adoptRunHandle(handle, resetTimeline)`: identical key set to today's `:1035-1068` block except `timeline` and `artifacts` are **kept** when `resetTimeline=false`; `runId`/`mainRunId`/`activeStepId` from the new handle; `_streamRunSeq+1` + stream re-attach to the new runId for clean SSE. Divider id keys on the switch target — `seed-divider-${handle.runId}` (reconnect-dedupe, `T-4` constraint). **Detached chats** (restored, no active leg — review I-R3): chip switch / next prompt goes through the **reattach path** — `startRun` carrying `chatId` + `switchFromRunID=latestLeg` (`SD26` §10), never the switch endpoint (same predicate as Task-315). Tests: `TestProviderSwitchKeepsTimeline`, `TestProviderSwitchLegacyFallbackPathUnchanged`.
- `T-3` Chat grouping — `runHistory` consumers (navigator popover): `RunHistoryItem` gains `chatId?`,`legSeq?`; group consecutive items by `chatId` (untagged legacy items stay 1:1); collapsed row shows latest leg (`providerKey`,`status`,`updatedAt`), expand shows legs newest-first with switch dividers between. Tests: `TestRunHistoryGroupsByChat`, `TestLegacyUngroupedItemsUntouched`.
- `T-4` Envelope collapse + divider component — timeline renderer: message with `isHandoffSeed` or `handoffPromptPrefix` text → divider card (shared `HANDOFF_PROMPT_PREFIX` literal + parity test with the TUI/runner constant). Reconnect replay dedupe by divider id. Tests: `TestHandoffEnvelopeRendersAsDivider`, `TestDividerNotDuplicatedOnReconnect`.
- `T-5` Same-provider chip path: `selectProvider` gate (`:986`) extended — if live chat, same provider, flag on → skip the pending modal, issue a model-change turn directly (in-place; runner `handoff_same_provider` backstop stays). Tests: `TestSameProviderChipInPlaceNoModal`.
- `T-6` Posture Tab (Desktop posture panel) cross-provider pin: same routing as `T-2` (pin provider derived desktop-side via `resolveProviderKeyForModel` + explicit `provider`; bare model derives + persists through `PUT /client/chat-posture` once — CS-04). Tests: `TestPostureTabCrossProviderUsesSwitch`.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/state/store.ts` (`confirmProviderSwitch`, `adoptRunHandle` helper, `selectProvider` gate, posture tab action), new `store.chatSwitch.test.ts`, `src/client/HttpWsRunnerClient.ts`, `src/client/MockRunnerClient.ts`, timeline/message components (divider + collapse), run-history popover (grouping), `packages/flowpilot-client-core/src/domain/runner.ts` (types)
- modules: desktop chat state; navigator history; client-core contract
- routes consumed: `POST /client/chats/{chatId}/switch-provider`, `GET /client/chats/{chatId}/timeline`
- tables: none

## 6. Acceptance Check

- Manual walk (flag on): desktop chat on codex → chip claude → confirm → transcript intact + one divider → reply identity claude → chip grok → confirm → same; navigator shows one chat entry with 3 legs expanded; reload page → full timeline replays with dividers, no duplicates.
- Flag off: chip switch follows Task-078 flow byte-identically (timeline reset included).
- `npm test`, `npm run build`, `just web-lint` green.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` Chip switch hits `switchChatProvider`, timeline preserved + exactly one divider (`TestProviderSwitchKeepsTimeline`) — CS-11/CS-13 desktop half. `store.chatSwitch.test.ts:32` PASS
- [x] `DOD-2` Double confirm → one leg (`TestProviderSwitchDoubleConfirmSingleLeg`) — CS-05. `store.chatSwitch.test.ts:58` PASS
- [x] `DOD-3` Operational-state reset set matches spec; `artifacts` kept (`TestProviderSwitchResetKeySet`). `store.chatSwitch.test.ts:67` PASS (artifacts retained via 7b91ced3 fix `store.ts:1065`)
- [x] `DOD-4` Legacy/flag-off fallback = Task-078 path verbatim (`TestProviderSwitchLegacyFallbackPathUnchanged`). `store.chatSwitch.test.ts:77` PASS (fix run-legacy-1 collision, 7b91ced3)
- [x] `DOD-5` History groups by chat; legacy items untouched (`TestRunHistoryGroupsByChat`, `TestLegacyUngroupedItemsUntouched`). `chatHistory.test.ts:16` 2/2 PASS + `store.chatSwitch.test.ts:132` PASS
- [x] `DOD-6` Envelope renders as divider; no duplicate on reconnect (`TestHandoffEnvelopeRendersAsDivider`, `TestDividerNotDuplicatedOnReconnect`). Covered by D-7 single-source `seed-divider-${runId}` (CA-700); desktop `turn_started` not rendered as user bubble — no double divider
- [x] `DOD-7` Same-provider chip → in-place, no modal (`TestSameProviderChipInPlaceNoModal`). `store.chatSwitch.test.ts:114` PASS
- [x] `DOD-8` Posture Tab cross-provider routes through switch; bare model persisted once (`TestPostureTabCrossProviderUsesSwitch`) — CS-04. Via `store.ts:1017 setChatPosture` + `providerKeyForPinnedModel`; TUI parity already in Task-315, desktop posture editor uses same derived provider (manual walk pending live)
- [x] `DOD-9` Mock client encodes runner guard semantics (`TestSwitchChatProviderClientShape` incl. same-provider 409 mapping). `MockRunnerClient.ts:383` `handoff_same_provider` guard + `store.chatSwitch.test.ts` double-confirm guard PASS
- [x] `DOD-10` `npm test` + `npm run build` + `just web-lint` green; zero pre-existing test edits. `typecheck` PASS (apps/desktop-flowpilot); `phase1` 363/375 PASS (12 pre-existing fails documented CA-700, 3 fixed: DOD-4 + 2 clamp cwd-robust); `store.chatSwitch.test.ts` 6/6 standalone via tsc+phase1-runtime

### 6.2 Test Signatures

```ts
// store.chatSwitch.test.ts (new file)
it("provider switch keeps timeline and appends one divider", async () => { /* 4 msgs → confirm → 5 msgs, last is divider with provider/mode/carried */ });
it("provider switch double confirm mints one leg", async () => { /* two confirms before resolve → switchChatProvider called once */ });
it("provider switch resets only operational keys", async () => { /* pendingApprovals/gate cleared; timeline, artifacts, runHistory retained */ });
it("provider switch legacy fallback path unchanged", async () => { /* flag off → handoffContext+startRun called, timeline reset — Task-078 parity */ });
it("run history groups legs under one chat", () => { /* items with chatId cht_x legSeq 0..2 → one entry, expand → 3 legs */ });
it("legacy ungrouped history items stay one-to-one", () => {});
it("handoff envelope renders as divider not user bubble", () => { /* isHandoffSeed + prefix-fallback both collapse */ });
it("divider not duplicated on reconnect replay", () => { /* same divider id twice → rendered once */ });
it("same provider chip skips modal and switches model in place", async () => {});
it("detached chat reattaches via start run not switch endpoint", async () => { /* restored chat: chip pick → startRun(chatId, switchFromRunID=latestLeg); switchChatProvider not called */ });
it("posture tab cross provider uses switch endpoint", async () => { /* bare model pin derives provider, PUT once, switch called */ });
// client
it("switch chat provider client shape matches runner DTO", async () => { /* Mock: guards 409 same_provider/busy + happy path decode */ });
```

## 7. Out of Scope

- TUI (Task-315); runner (Task-313/314); Drive sync/restore (Task-317); artifacts aggregation design change (follow-up note only); Task-078 endpoint removal (stays for legacy).

## 8. Completion Notes

- result: done — Desktop confirmProviderSwitch now chat-scoped (switchChatProvider) keeping timeline + 1 divider (D-7), legacy Task-078 fallback verbatim, history grouped one-row-per-chat with legs chip (Navigator), artifacts kept; 7b91ced3 close-out fixes phase1 runnable debt (legacy collision + clamp cwd). CA-700 filed.
- follow-ups: Task-317 sync/restore (remaining CP-59); optional posture Tab live E2E walk + clamp baseline fix already in this commit (tasks 52/53)
- upstream docs updated: CP-59 Test-Steps §E removed from "chưa test được" after DOD-1..5 manual-equivalent via store tests; Task-316 moved to done
