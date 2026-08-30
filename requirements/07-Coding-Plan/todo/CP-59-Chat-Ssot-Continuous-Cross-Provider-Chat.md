# CP-59: Chat SSOT — Continuous Cross-Provider Chat (chatId)

## Metadata

- Document ID: `CP-59`
- Title: `Chat SSOT — Continuous Cross-Provider Chat (chatId)`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-29`
- Last Updated: `2026-08-29`
- Parent Documents: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- Child Documents: [Task-312: Chat SSOT Design Freeze (SD-26)](../../08-Task/todo/Task-312-Chat-Ssot-Design-Freeze-SD26.md), [Task-313: ChatId Data Model, Transcript Store, Timeline](../../08-Task/done/Task-313-ChatId-Data-Model-Transcript-Store-Timeline.md), [Task-314: Chat Switch-Provider Endpoint And Chat Envelope](../../08-Task/todo/Task-314-Chat-Switch-Provider-Endpoint-And-Chat-Envelope.md), [Task-315: TUI Chat Switch Surface](../../08-Task/todo/Task-315-TUI-Chat-Switch-Surface.md), [Task-316: Desktop Chat Switch Surface](../../08-Task/todo/Task-316-Desktop-Chat-Switch-Surface.md), [Task-317: Chat-Level Sync Restore Reopen](../../08-Task/todo/Task-317-Chat-Level-Sync-Restore-Reopen.md)
- Related Documents: [BUG-330: Posture Tab applies foreign provider model on pinned run](../../09-BugFix/done/BUG-330-Posture-Tab-Applies-Foreign-Provider-Model-On-Pinned-Run.md) (this CP generalizes BUG-330 F-3 to chat-level switch; BUG-330's repro is the regression trigger), [Task-078: Cross-Provider Chat Handoff](../../08-Task/done/Task-078-Cross-Provider-Chat-Handoff.md) (envelope contract reused, not replaced), [CP-57: Opencode Provider Integration](../done/CP-57-Opencode-Provider-Integration.md), [BUG-329](../../09-BugFix/todo/BUG-329-Opencode-Midchat-Model-Switch-Session-Load-No-SessionId.md), [CA-679](../../../change-audit/CA-679-opencode-config-file-env-and-tui-model-restore.md)
- Replaces: `None` (Task-078 handoff stays the transport for context carry; this CP adds the chat-level continuity layer above it)
- Tags: `chat-ssot, chatId, cross-provider, chat-history, provider-switch, tui, desktop, durable-replay, severity-high`
- Feature Keys: `chat-history`, `cross-provider-handoff`

## AI Quick View

### Summary

- A chat today is **one run pinned to one provider** (`rs.providerKey` fixed at `startRun`, Task-078 T-1). Crossing providers mid-chat is impossible in place: `/provider` blocks (`app.go:3870`), Desktop chip switch resets the whole timeline (`store.ts:1058` `timeline: []`), and the posture Tab silently injects a foreign model into the current adapter (BUG-330: `grok-4.5` sent to opencode ACP → `set_config` 404 → Muse Spark answers while the footer claims Grok). The operator requirement is: **switch across codex/claude/grok/opencode mid-chat with zero visible discontinuity — same chat, full text history, no new chat, ever.**
- This CP introduces `chatId` — a **logical chat** that outlives runs. One chat = N ordered **legs** (runs), each leg = one provider session. FlowPilot's own durable **chat timeline** becomes the SSOT for rendering, replay, restore, and reopen; provider folders (`ses_*`, claude jsonl, codex rollout, grok chat_history) demote to per-leg engine session state. Provider switch = a runner-side operation that closes the current leg, seeds a new provider leg with a full-chat handoff envelope, and hands both UIs a new run handle **in place** — the visible transcript never resets.
- Honest limit (unchangeable): a provider cannot natively load another provider's session (claude cannot load an opencode `ses_*`). Continuity is therefore delivered on two layers: **display = 100%** (rendered from the FlowPilot chat store), **model memory = best possible** (full-chat envelope budgeted by the target model's context window + existing raw/hybrid/target_summary ladder + optional chat-recall tool).
- Storage ground truth (verified): `localFileSessionStore.AppendEvent` persists **only flow-sidecar event types** to disk (`local_file_session_store.go:661`, `isFlowSidecarEventType`) — normal-chat transcript survives restart today only via per-provider folder seeders (`interactive_resume.go:3008` `seedTranscriptFromDisk`, branches for claude/codex/grok/opencode/gemini) + turn log. A chat-level SSOT therefore needs its **own durable chat transcript store** — this CP adds it (P-3), it is not a free join of existing tables.

### Current Ask

- Detail the implementation plan for the Chat SSOT architecture (user-approved "Option B") so that: (1) provider switch mid-chat works across **all** provider pairs (codex↔claude↔grok↔opencode; gemini where transcript support is proven), (2) the chat view is continuous — no timeline reset, no chat creation, history text preserved verbatim, (3) sync / restore / re-open operate on the chat, not the run, and (4) BUG-330's cross-provider Tab failure class is eliminated at the same time. Plan only — no code in this doc.

### Key Decisions

- `P-1` **chatId is the user-facing continuity unit; the run stays the execution unit.** A chat is minted on the first `normal_chat` start and never changes; each provider switch appends a new **leg** (a real run, `runKind="chat"`) tagged `chatId` + `legSeq`. Run-level invariants (Task-078 T-1: `providerKey` pinned per run) are **preserved** — we never migrate a live provider session; we add a layer above.
- `P-2` **Two SSOTs with a hard split.** FlowPilot chat transcript/timeline = truth for **display, replay, reopen, restore, history**. Provider session folders = per-leg engine state for **turn execution and same-leg resume**. No code path may render chat text from provider folders once the chat store exists (seeders become fallback-only for a leg's own resume bootstrap).
- `P-3` **Provider switch is a runner-side three-phase operation** (`SD26-S-2`): `POST /client/chats/{chatId}/switch-provider` validates guards + stamps a durable switch intent (phase A, under `s.mu`), then **opens the target-provider leg first** (phase B — `createRun` takes `s.mu` itself, so the switch op never holds the lock across it; the new leg carries `switchFromRunId` as the durable intent), then **closes the old leg** and appends the `chat_provider_switch` record (phase C). The envelope is built from **all legs** (chat scope, generalizing `handoff_context.go:59 buildHandoffContext`), the seed turn is dispatched server-side fire-and-return (stats embedded in the seed's `turn_started` payload, prefix `handoffPromptPrefix` `handoff_context.go:22` preserved). Start-new-before-close-old guarantees every crash window heals (matrix in SD-26 §8, zero TBD cells).
- `P-4` **Envelope budget derives from the target model's context window** (`ProviderModel.ContextWindowTokens`, `types.go:34`) instead of the flat 64 KiB `handoffMaxBytes` (`handoff_context.go:15`); hard cap + existing summary ladder (raw → hybrid via chat summary ledger → target_summary) unchanged. Source gating `supportsHandoffSource` (`handoff_context.go:142`) becomes chat-transcript-based — provider-agnostic (opencode and, once proven, gemini stop being second-class).
- `P-5` **Same-provider model change stays in place** — no new leg: opencode `session/set_config_option`, grok `session/set_model` (BUG-329 path). The switch endpoint returns `409 handoff_same_provider` (existing semantics) so callers fall back to the in-place path.
- `P-6` **Posture profiles become honest `{provider, model}` pairs** (BUG-330 F-1): a bare-model pin (e.g. `plan={grok-4.5}`) derives its provider once via `providerKeyFromModel` (`provider_registry.go:202`) at apply time and **persists the derived provider** with a one-time system warning. Cross-provider Tab then routes through `P-3`; same-provider Tab keeps today's `session/load` + `set_config` behavior (`chat_posture.go:161 applyChatPostureProfile`).
- `P-7` **`/model <foreign>` and `/provider <key>` mid-chat join the switch flow** for chat runs (replacing the `Cannot change provider after a run has started` block at `app.go:3870`); workflow/flow/step runs keep the block — chat-kind gate is absolute (`runKind=="chat"` only, `interactive_service.go:161`).
- `P-8` **Cross-surface contract parity**: TUI and Desktop render the same `GET /client/chats/{chatId}/timeline` (the posture-doc SSOT pattern, `chat_posture.go:16-18`), both carry an in-flight switch guard (TUI `handoffInFlight` keyed by chatId; Desktop reuses `providerSwitchLoading`). **One divider per switch, single source of truth**: the seed turn itself is the divider — the runner embeds the handoff stats (`isHandoffSeed`, carried/omitted counts, mode) in the seed's `turn_started` payload; live streams render the seed as the divider, timeline replay renders the `chat_provider_switch` record (SD26-E-9) and suppresses the duplicate seed; renderers dedupe by `toRunId`. Clients never synthesize their own divider on switch success.
- `P-9` **Durable replay contract for switch**: `chat_provider_switch` is a first-class timeline event with stable id and a monotonic per-chat seq minted runner-side under the service lock; restart / resume / reconnect replay it identically (durable-replay-contracts skill; state×event matrix in §3.3).

### Constraints

- Additive-only: no pre-existing test edited (additive-tests-only); old single-run chats with **zero** provider switches must behave byte-identically (chatId transparent); workflow/flow runs untouched.
- Never write to provider session folders to "import" history — no session forging across CLIs.
- `rs.providerKey` pinned per run is kept (Task-078 T-1); the switch always mints a new run.
- Runner is the SSOT; TUI/Desktop are thin clients (same rule as chat-posture).
- Safe-fix contract + cross-provider-parity apply: parity across all four providers, no silent degradation — every unavailable capability is typed (`handoff_*`, `session_unavailable`), never masked.
- Legacy ledger: `feature_key` resolution per `change-audit/FEATURE-KEYS.md` (`chat-history` dominant, `cross-provider-handoff` secondary); CA note per change following audit-logging skill.

### Open Questions

> **All questions below are CLOSED by [SD-26 §13 Decision Register](../../06-System-Tech-Design/SD-26-Chat-Continuity-Ssot.md) (2026-08-30, Task-312).** They remain here for rationale history; SD-26 §13 is the decision of record.

- `Q-1` Chat transcript store shape: per-chat NDJSON sidecar under the local session dir (mirrors flow-events sidecar, `local_file_session_store.go:661`) + a `workflow_chat_events` Supabase table vs reusing the events table with an added `chat_id` column? Decide in SD-26 (P-1); default lean: dedicated store + `ChatTranscriptStore` interface mirroring the `localFileSessionStore`/`SupabaseWorkflowStore` duality.
- `Q-2` Chat seq model: global monotonic `chatSeq` stamped at append time vs `(legSeq, eventSeq)` tuple ordering? Tuple is migration-free; chatSeq is simpler for pagination/tail (TUI budgets, `chat_history_replay.go:6`). Decide in SD-26.
- `Q-3` Should switching close the old leg's provider process (opencode `acp` process reuse per BUG-329 lives per scopeKey, not per run) or keep it warm for a switch-back? Default: keep warm (process per scopeKey already shared), close only the leg record.
- `Q-4` Desktop run-history popover (`store.ts:308 runHistory`) regroups by chat — does the run list stay reachable as a secondary view (audit/debug) or collapse into chat-only? Default: chat is primary; keep a run detail drill-down.
- `Q-5` Gemini: `seedGeminiTranscriptFromState` exists (`interactive_resume.go:3016`) but gemini was excluded from handoff sources pending a proven extractor. Include gemini in the matrix as target-only first, source once proven — confirm.
- `Q-6` Token-usage display across legs: sum per chat or per leg? Default: per leg with a chat total line (matches `latestTokenUsage` reset semantics in `store.ts:1048`).
- `Q-7` chat-recall MCP tool (`flowpilot_chat_recall`) — in-scope for this CP as stretch P-9 or deferred to its own CP? Default: deferred; envelope budget + summary ladder first.

### Source Refs

- BUG-330 forensic (run-314536): posture pins `scan=opencode/muse-spark`, `code=opencode/deepseek`, `plan=grok-4.5` (bare) → cross-provider Tab sent `provider=opencode model=grok-4.5` → `session/set_config` 404 `model not found` → reply stayed Muse Spark while footer claimed grok-4.5.
- Runner: `interactive_service.go:161` (`runKind`), `:3565` (`persistenceStore`), `:3947` (`persistEvent` — skips `EventMessageDelta`), `:3667` (`RunKind` in session record); `local_file_session_store.go:661` (`AppendEvent` — flow-sidecar types only), `isFlowSidecarEventType`; `supabase_workflow_store.go:544` (`AppendEvent`), `:62` (`workflow_provider_sessions` column list — where per-run durable records live; `workflow_runs` table deprecated `:495`).
- Handoff: `handoff_context.go:15` (`handoffMaxBytes=64KiB`), `:22` (`handoffPromptPrefix`), `:59` (`buildHandoffContext`), `:67` (`handoff_same_provider` 409), `:73` (`handoff_run_busy` 409), `:76`/`:142` (`supportsHandoffSource` — claude/codex/grok only today), `:156` (`transcriptTurnsFromRun`), `:281` (`renderHandoffPrompt`), `:307` (`packConversationTurns`).
- Restart/rebuild: `interactive_resume.go:3008` (`seedTranscriptFromDisk` — claude/codex/grok/opencode/gemini branches), `:4784` (`loadPersistedRun`).
- TUI: `tui/app/chat_posture.go:146` (`postureModelPinWins`), `:161` (`applyChatPostureProfile`), `:231` (`setPostureProvider`), `:255` (`providerForModel`); `tui/app/app.go:1221` (`RunStartedMsg` — does NOT clear messages: in-place adopt is already the TUI default), `:3870` (`/provider` block), `:5937` (`cmdStartRun`); `tui/app/model.go:510` (`pendingPrompt` — first prompt waits for StartRun); `tui/app/chat_history_replay.go:6` (tail budgets); ca554 restore picker.
- Desktop: `store.ts:966` (`selectProvider` → `pendingProviderSwitch`), `:1010` (`confirmProviderSwitch`), `:1058` (`timeline: []` reset — the "new chat" UX this CP removes), `:308` (`runHistory`).
- Endpoints: `interactive_handlers.go:29` (`GET /client/projects/{projectId}/workflow-runs`), `:209` (`handleListProjectRunHistory`), handoff route `interative_handlers.go POST /client/workflow-runs/{runId}/handoff-context`.
- Sync: `chat_session_sync.go` (`ChatSessionSyncManifest` keyed by `SourceRunID`; BUG-316 sidecar semantics — events.jsonl/updates.jsonl/prompt_context.json per session dir).
- Model metadata: `types.go:34` (`ProviderModel.ContextWindowTokens`), `provider_registry.go:202` (`providerKeyFromModel`).

## 1. Goal

Make a **chat** — not a run — the unit the operator sees and keeps:

1. **Cross-provider switch, everywhere**: mid-chat switch across codex/claude/grok/opencode in every direction (gemini per proven support) via posture Tab, `/mode`, `/model`, `/provider`, or Desktop provider chips — no `/new`, no modal detour in the happy path.
2. **Zero visible discontinuity**: the transcript (user + assistant text, tool/question/approval cards) persists verbatim across switches; the UI appends one divider line (`⇄ switched to grok · grok-4.5 — carried 14 turns (raw)`) and continues. No timeline reset, no second chat in the list for what the user perceives as one conversation.
3. **Truthful memory**: the new provider leg receives the full prior conversation within its context budget (raw) or the best available ladder (hybrid/target_summary); the UI states what was carried. Optional later: a recall tool for turns beyond the envelope.
4. **Chat-level lifecycle**: reopen after restart, TUI restore picker, Desktop history, Drive sync/restore all operate on `chatId` and replay the full multi-provider timeline.
5. **BUG-330 dies here**: cross-provider posture Tab produces a real grok/claude/codex/opencode leg — never a foreign model inside the old adapter.

## 2. Input Documents

- [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md) (provider-selection invariant — gains the chat-switch note)
- [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md) (session/run lifecycle)
- [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md) (provider contract per run, `providerKeyFromModel`)
- [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md) (resume/sync precedent)
- [Task-078: Cross-Provider Chat Handoff](../../08-Task/done/Task-078-Cross-Provider-Chat-Handoff.md) (envelope contract: budget, ladder, feature blocks, prompt prefix)
- [BUG-330](../../09-BugFix/done/BUG-330-Posture-Tab-Applies-Foreign-Provider-Model-On-Pinned-Run.md), [BUG-329](../../09-BugFix/todo/BUG-329-Opencode-Midchat-Model-Switch-Session-Load-No-SessionId.md)
- [CP-57: Opencode Provider Integration](../done/CP-57-Opencode-Provider-Integration.md) (additive parity method, P-0 guard style)
- durable-replay-contracts skill (event contract for persisted chat events across restart/resume/reconnect/replay)
- New: `SD-26` (to be authored in P-1) — Chat SSOT data model, seq model, event contract, leg state machine.

## 3. Implementation Strategy

- **overall approach:**
  - Add the chat layer **above** runs, never inside them: `chatId`/`legSeq` ride on run records; the switch is a runner service operation composing existing primitives (`buildHandoffContext`-generalized, `StartRun`, envelope seed via `pendingPrompt`-equivalent runner path, leg close, timeline event).
  - Make the chat transcript durable first (P-3) — everything else (timeline endpoint, reopen, restore, switch stats) reads from it. Per-provider folder seeders stay exactly where they are for leg-level resume bootstrap.
  - Convert the four switch entry points one by one (posture Tab → `/model` → `/provider` → Desktop chips) onto the same endpoint so there is exactly one switch path to test and guard.
- **sequencing logic:**
  - SD-26 design freeze (P-1) → storage + timeline (P-2, P-3) → switch endpoint (P-4) → TUI (P-5) → Desktop (P-6) → sync/restore (P-8) → hardening/DOD (P-9). P-7 (same-provider no-op) is a guard inside P-4/P-5/P-6.
  - Each slice ships with its matrix row from §3.2 green before the next starts (additive tests only).
- **dependencies:**
  - Existing: handoff envelope contract (Task-078), provider folder seeders (`interactive_resume.go:3008`), chat summary ledger (hybrid mode), `providerKeyFromModel`, TUI in-place `RunStartedMsg` adopt, Desktop `pendingProviderSwitch` guard state.
  - New: chat transcript store (Q-1), chat seq (Q-2), switch endpoint, timeline endpoint, divider event contract.

### 3.1 Continuity Layers (what "continuous" means, precisely)

| Layer | Guarantee | Mechanism |
|---|---|---|
| Display (text history) | **100%** — every user/assistant turn ever sent in the chat is visible forever, across switches, restarts, reopen, restore | FlowPilot chat transcript store (P-3) + timeline endpoint (P-4); provider folders never consulted for display |
| Model memory | **Budget-bounded best effort** — full raw conversation up to the target model's context-derived cap; ladder (hybrid/target_summary) beyond; carried amount stated in the divider | Chat-scoped envelope (P-4) + summary ledger + (deferred) recall tool |
| Execution state | Per-leg only — tool permissions, provider session ids, approvals do not cross legs; switch happens only between turns | `handoff_run_busy` guard (409) on `turnInFlight`/`pendingApprovalID`/`pendingQuestionID` (`handoff_context.go:73`) |
| Identity | One chat row in history lists; runs remain visible as legs for audit | `chatId` grouping in run history (`Q-4`) |

### 3.2 Required Switch Matrix (executable DOD)

Every directed pair below must pass mid-chat with ≥3 prior turns on TUI **and** Desktop: text continuity 100%, envelope delivered (verify via runner log `handoff mode` + `includedTurnCount`), second prompt after switch answered by the target provider (identity check), footer/session panel truthful.

| From \ To | codex | claude | grok | opencode |
|---|---|---|---|---|
| codex | in-place (`set_model`) | switch | switch | switch |
| claude | switch | in-place | switch | switch |
| grok | switch | switch | in-place | switch |
| opencode | switch | switch | switch | in-place (`set_config`) |

Plus: first-turn switch (empty transcript → fresh leg, no envelope, no error — BUG-330 V-5), same-provider switch attempt → in-place fallback, and gemini rows per `Q-5` (target-only until source extractor proven).

### 3.3 Durable Replay: state × event matrix (per durable-replay-contracts)

| State \ Event | `chat_provider_switch` | turns (pre-switch legs) | turns (current leg) | pending approval/question |
|---|---|---|---|---|
| Runner live, UI attached | divider rendered; stream re-attached to new leg handle | replayed from chat store | live stream | live cards (switch blocked while pending — 409) |
| Runner restart, UI reopens chat | replayed identically (stable id + chat seq) | replayed from chat store | resumed leg (`seedTranscriptFromDisk` bootstraps provider session; display from chat store) | rehydrated via `rehydratePendingGatesLocked` |
| Drive restore on new machine | replayed from restored chat transcript | replayed | typed degradation: chat restores **detached** (all legs `closed(restored)`); first turn reattaches via `createRun(chatId, switchFromRunID=latest)` seeded by envelope (`SD26` §10) | cards not carried (leg-scoped) |
| Rapid double switch in flight | exactly one `chat_provider_switch` + one new leg (in-flight guard) | — | — | — |

## 4. Work Breakdown

**Namespace note:** `Key Decisions` `P-*` and `Work Breakdown` `P-*` are distinct namespaces — child Tasks cite **Work Breakdown §4 `P-*`** only (same rule as CP-57 §4).

Child Tasks allocated (this batch, post Grok review): Task-312 → P-1 (SD-26 design freeze, gate); Task-313 → P-2+P-3 (chatId/store/timeline); Task-314 → P-4 (switch endpoint, owns SS-05/SD-06 amendments + BUG-330 doc update); Task-315 → P-5 (TUI); Task-316 → P-6 (Desktop); Task-317 → P-8 (sync/restore); P-9 stretch/deferred.

- `P-1` **SD-26 design freeze (no code).** Author `SD-26: Chat Continuity SSOT` covering: chatId format + mint rules; leg state machine (`active → closed(provider_switch | chat_ended)`); chat seq model (Q-2); chat transcript store schema (Q-1) + retention; `chat_provider_switch` event contract (fields: from/to provider+model, leg ids, handoffMode, includedTurnCount, truncated); chat-kind gate; failure taxonomy (typed errors). Review gate: SD-26 approved before Task-A starts.
- `P-2` **chatId data model + run tagging.** Mint `chatId` at first `normal_chat` StartRun; add `chatId`, `legSeq`, `legState`, `legClosedReason` to the durable run record (`workflow_provider_sessions` additive columns `:62`; local store equivalent) and to `interactiveRun` (`interactive_service.go:161` area); `StartRunInput` accepts optional `chatId` (+`switchFromRunId`); `RunHandle` returns `chatId`; backfill: legacy chat runs self-tag (`chatId=runId`, `legSeq=0`).
- `P-3` **Durable chat transcript store + timeline read model.** New `ChatTranscriptStore` (local NDJSON per chat + Supabase table per Q-1); runner appends turn-level transcript records (user prompt, assistant final, switch events) on the existing persist path next to `persistEvent` (`interactive_service.go:3947`); `GET /client/chats/{chatId}/timeline` joins legs + events with stable ordering (Q-2), tail/chunked budgets mirroring `chat_history_replay.go:6`; replay-safe (idempotent, stable ids).
- `P-4` **Switch endpoint + chat-level envelope.** `POST /client/chats/{chatId}/switch-provider {targetProviderKey, model, reasoningEffort, yolo}`: guards (chat exists & active leg; `handoff_run_busy` 409 during turn/approval/question; same-provider → `handoff_same_provider` 409 so callers fall back in place; unknown/unavailable provider typed 4xx); generalize `buildHandoffContext` to chat scope (join all legs' transcripts through the chat store, fall back to per-run `transcriptTurnsFromRun` for legacy un-migrated chats); envelope gains an optional `<actions_summary>` block — **new vs Task-078** (whose envelope is text-only) — compiled from the chat store's tool/file/approval records so the target provider learns what previous legs **did** (turn-scoped tool calls, files touched, user approval decisions), not just what they said; the shared workspace `cwd` already exposes the on-disk effects, the digest adds the narrative; budget from target `ContextWindowTokens` with hard cap (keep `packConversationTurns` semantics, `handoff_context.go:307`); seed first turn on the new leg with the envelope (prefix `handoffPromptPrefix` keeps feature-resolution skip + per-turn injection seam intact; stats embedded in the seed payload); close old leg (`legClosedReason=provider_switch`) — **ordered start-new-first, close-old-second** under the three-phase linearization `SD26-S-2` (never hold `s.mu` across `createRun`, which locks it internally at `interactive_handlers.go:795` region); append `chat_provider_switch` (SD26-E-9) to the chat timeline; respond with new RunHandle + handoff stats — fire-and-return, `handle.LastEventSeq` is the client attach point. Open opencode (and gemini per proof) as sources — the chat store, not provider folders, is the source now.
- `P-5` **TUI switch surface.** Posture Tab / `/mode` cross-provider pin → switch flow (`applyChatPostureProfile` derives the pin's provider — bare model via `providerKeyFromModel`, derived once and persisted per P-6 Key Decision — and compares against the live leg's `providerKey`); `/model <foreign>` and `/provider <key>` mid-chat route to the same flow (P-7 Key Decision); in-place adopt of the new handle: keep `m.messages`, reset per-run stream state (`lastEventSeq`, turn stream, `NoteLastSeq`); the envelope seed turn is dispatched **server-side** by the switch endpoint (§4 `P-4`) — the TUI only adopts the returned handle and streams; `pendingPrompt` (`model.go:510`) stays the normal first-chat path only; `handoffInFlight` guard keyed by chatId (rapid Tab = one leg, matrix §3.3); divider rendering collapses any `handoffPromptPrefix` user message (live and replayed); restore picker (ca554) opens by chat and replays the full timeline.
- `P-6` **Desktop switch surface.** Provider chips + posture Tab in `normal_chat` call the switch endpoint through `confirmProviderSwitch`'s path but **stop resetting the timeline** (`store.ts:1058`): keep `timeline`, render the divider from the server-seeded turn (client never appends a divider — `SD26` single-source rule), reset only operational state (pending approvals/questions, gate, stream seq, `_runReplaySeq`/`_streamRunSeq`); `providerSwitchLoading` stays the double-fire guard; run history popover groups runs by `chatId` (Q-4); open-chat loads `/chats/{chatId}/timeline`; envelope prompt rendered as the divider, not a user bubble.
- `P-7` **Same-provider guard (inside P-4/P-5/P-6).** Same-provider model change never mints a leg: opencode `session/set_config_option` (BUG-329 path), grok `session/set_model`; opencode `acp` process stays warm per scopeKey across legs (Q-3); regression test proving scan↔code opencode flows are unchanged (run-314536 turns 1–3 as the lock).
- `P-8` **Sync / restore / reopen by chat.** `ChatSessionSyncManifest` gains a chat-level variant (chat transcript + per-leg sidecars; schema version bump); Drive restore rebuilds the full chat even when a leg's provider sidecars are unavailable — that leg resumes as a fresh provider session seeded by its envelope (typed `session_unavailable` per leg, chat still opens with full text — matrix §3.3 row 3); TUI reopen + Desktop open both go through the chat timeline.
- `P-9` **Stretch / deferred (explicitly out of first DOD unless re-scoped).** `flowpilot_chat_recall` MCP tool (Q-7); chat-total token usage line (Q-6); gemini source (Q-5).

## 5. Touched Areas

- **files (runner, extend):** `interactive_service.go` (`interactiveRun` chat fields; startRun mint/adopt; persist hook), `interactive_handlers.go` (routes: chats timeline, switch-provider), `handoff_context.go` (chat-scope build; budget derivation; source gate), `interactive_resume.go` (reopen-by-chat; leg bootstrap), `chat_session_sync.go` (chat manifest), `provider_registry.go` (nothing new expected — `providerKeyFromModel` reused), `local_file_session_store.go` + `supabase_workflow_store.go` (chat columns + chat transcript store), `runner.go` (chatId in StartRun plumbing).
- **files (runner, new):** `chat_ssot.go` (chat service: mint legs, switch op, leg close), `chat_timeline.go` (read model), `chat_transcript_store.go` (+ local/Supabase impls), `chat_provider_switch_event.go` (event contract), tests: `chat_ssot_switch_matrix_test.go`, `chat_timeline_replay_test.go`, `chat_transcript_store_test.go`, `bug330_posture_tab_switch_test.go`.
- **files (TUI):** `tui/app/chat_posture.go` (pin provider derivation + switch routing), `tui/app/app.go` (`/model`, `/provider`, adopt handling, divider render, restore), `tui/app/turn_stream.go` (per-run stream reset on adopt), `tui/app/chat_history_replay.go` (chat-scope replay), `tui/client/client.go` (`SwitchProvider`, `GetChatTimeline`).
- **files (Desktop):** `state/store.ts` (`confirmProviderSwitch` → switch endpoint, no timeline reset, chat grouping), `state/store.test.ts` (handoff tests extended: no-reset + divider), timeline components (divider render + envelope collapse), run-history popover (chat grouping), `client/*` (endpoint bindings).
- **modules:** runner interactive service + persistence; TUI chat; Desktop chat + history; Drive sync; (deferred) MCP tools.
- **database:** additive columns on `workflow_provider_sessions` (`chat_id`, `leg_seq`, `leg_state`, `leg_closed_reason`) + new chat-transcript table (Supabase) / NDJSON store (local) — no mutation of existing columns; backfill legacy rows (`chatId=runId`, `legSeq=0`, `legState=closed(chat_ended)` for terminal, `active` for resumable).
- **external systems:** none new; provider CLIs untouched (no session forging); Drive manifest schema bump (restore must accept v1 and v2).

### 5.1 Base-Regression Guard

| Area | Change type | Regression guard |
|---|---|---|
| Single-provider chat with zero switches | Behavior must be byte-identical | Existing chat suites stay green unmodified (additive tests only); new test proves `chatId` transparent when no switch occurs |
| Same-provider model switch (scan↔code opencode; grok set_model) | Untouched path | run-314536 turns 1–3 lock test (P-7) |
| Workflow/flow/step runs | Untouched (`runKind` gate) | `/provider` block text + 409 for non-chat kinds asserted in tests |
| Task-078 handoff endpoint (`runId`-scoped) | Kept (chat switch composes it) | Existing handoff suite unchanged + new chat-scope tests separate |
| Per-provider folder seeders | Untouched (fallback-only role) | Existing `interactive_resume` suites green |
| Desktop timeline semantics elsewhere | Audit all `timeline: []` call sites | `store.test.ts` additions prove provider-switch keeps timeline; other resets (new chat, mode change) unchanged |

## 6. Data or Migration Steps

- **schema:** additive columns on the durable run record + new chat-transcript store (Q-1). No existing column reordered or repurposed (Supabase column list `supabase_workflow_store.go:62` extended at the end).
- **data backfill:** legacy chat runs self-tag on first touch (load or switch): `chatId=runId`, `legSeq=0`. Chat transcript backfill for legacy chats happens lazily from per-run `transcriptTurnsFromRun` when a legacy chat is first opened/switched (best effort, raw mode) — never blocks the turn.
- **config updates:** none mandatory; optional `FLOWPILOT_CHAT_SSOT` feature flag (default off → on after E2E, §8) and optional envelope cap override per deployment.

## 7. Validation Plan

- **automated tests (new files only):**
  - `CS-01` Switch matrix §3.2: every directed pair mid-chat — text continuity 100% on the timeline; envelope delivered (mode + `includedTurnCount` logged); post-switch identity answer from the target provider; footer/session panel truthful.
  - `CS-02` Same-provider Tab/model change → no new leg, `session/load`+`set_config`/`set_model` path (run-314536 turns 1–3 lock).
  - `CS-03` BUG-330 repro lock: posture pins `scan=opencode/muse-spark`, `code=opencode/deepseek`, `plan=grok-4.5` (bare) → Tab to plan mints a real `provider=grok` leg; second prompt answered by Grok; no `model not found` anywhere in logs.
  - `CS-04` Bare-model pin derivation: `{provider:"", model:"grok-4.5"}` → derives + persists `provider=grok` once with a warning system message; second apply does not re-derive.
  - `CS-05` In-flight guard: two rapid Tab/chip events within the switch window → exactly one new leg + one `chat_provider_switch` (mirror Desktop double-click test).
  - `CS-06` Busy guard: switch during in-flight turn / pending approval / pending question → `handoff_run_busy` 409, no partial leg; queued retry after turn completes succeeds.
  - `CS-07` First-turn switch: empty transcript → fresh leg, no envelope, no error (BUG-330 V-5).
  - `CS-08` Truncation ladder: transcript beyond the target-derived budget → `hybrid` or `target_summary` mode, `[Earlier conversation omitted…]` marker, divider states the carried count (BUG-330 V-6 analog).
- `CS-09` Restart mid-chat: kill runner after a switch, reopen → full timeline replayed (both legs), continue on the current leg; provider session resumed via existing seeder; display from chat store. (Owner: Task-315 — TUI `/open` path; Task-317 covers the Drive-restore variant via CS-10.)
  - `CS-10` Drive sync/restore: chat-level manifest round-trip; restore on a clean machine shows full text; leg with missing sidecars resumes fresh via envelope (typed `session_unavailable`), chat still opens.
  - `CS-11` Cross-surface: same chat opened in TUI and Desktop shows identical timeline (same endpoint).
  - `CS-12` Legacy transparency: pre-CP chat with zero switches behaves identically (chatId self-tag invisible); workflow runs cannot hit the switch endpoint (`runKind` gate 4xx).
  - `CS-13` Envelope collapse: `handoffPromptPrefix` message renders as a divider in TUI live stream, TUI replay, and Desktop timeline — never as a raw user bubble.
  - `CS-14` Restore with a missing provider: chat holding codex+claude legs restored on a machine with claude only → full timeline renders from the chat transcript; continuing on claude (in-place or one Tab switch) works with a full-chat envelope; anything codex-specific returns typed `provider_unavailable` + install hint — the chat is never dead, and no codex session is forged.
- **manual checks:** the full operator walk: `just chat-dev <project>` → scan(opencode) → code(opencode, same session) → plan(grok) switches visibly and answers as Grok → continue on grok → switch to codex → restart runner → reopen → full history → Drive restore on a second checkout. Footer, history list (one chat), and session panel checked at every step.
- **failure cases:** target provider unavailable → typed error, original leg still active, chat usable; chat store write failure → turn proceeds, transcript marked degraded (typed), no silent gap; switch while offline runner → client-side typed error, no local leg mutation.

## 8. Rollout and Fallback

- **rollout order:** SD-26 approved → P-2/P-3 storage+timeline (flag off, inert) → P-4 endpoint + runner tests → P-5 TUI (flag on in dev) → P-6 Desktop → P-8 sync/restore → E2E matrix green → flag default on. BUG-330's todo doc is updated to point at this CP as the implementing plan the moment P-4 lands (its minimal `Use /new`-block guard, if shipped earlier, is superseded by the switch flow).
- **fallback path:** with the flag off, today's behavior is preserved exactly — including the `/provider` block (message updated to mention the coming switch, no behavior change). If the switch endpoint fails typed (busy/unavailable), clients fall back to the old block message. The switch linearizes in three phases (SD-26 `SD26-S-2`: validate/intent under `s.mu` → `createRun` + seed without the lock → close old + record under `s.mu`), ordered start-new-first so **every** crash window heals on load — heal matrix (intent-no-new-leg / two-active / closed-no-record / seed-failed) is closed in SD-26 §8 with zero TBD cells; a seed failure surfaces typed `switch_seed_failed` and the chat stays continuable.
- **monitoring:** runner log lines for switch lifecycle (`switch begin/seed/close`, handoff mode + counts); chat-store append failures; flag state; leg counts per chat (anomaly: >0 failed switches).

## 9. Risks

- `R-1` **Dual-store drift**: provider folders vs chat transcript diverge (e.g. assistant text edited upstream). Mitigation: chat store is display truth by definition; divergence only affects leg resume quality, which the envelope already bounds.
- `R-2` **Ordering/concurrency**: turns streaming on leg N while a switch commits must not interleave into leg N+1's timeline. Mitigation: switch blocked while `turnInFlight` (409); chat seq minted under the service lock.
- `R-3` **Budget overflow on target**: context-derived cap misestimated (provider lies or `ContextWindowTokens` nil) → fall back to 64 KiB + ladder; overflow degrades to `target_summary`, never a hard turn failure.
- `R-4` **Migration risk** on `workflow_provider_sessions`: additive columns + lazy backfill only; no rewrite of existing rows at startup beyond self-tagging on touch.
- `R-5` **Desktop reset blind spots**: other `timeline: []` call sites could regress the continuity feel. Mitigation: audit list in §5.1 + store tests.
- `R-6` **Process lifecycle across legs** (opencode `acp` warm reuse, BUG-329): switch-back to opencode must reuse, not duplicate, the process; teardown remains scopeKey-bound (Q-3).
- `R-7` **UX confusion**: two runs in audit views for one perceived chat. Mitigation: chat grouping in history (Q-4); leg drill-down stays for debugging.
- `R-8` **Scope creep into workflow runs**: any non-`chat` runKind reaching the switch endpoint is a defect — gate asserted in CS-12.
- `R-9` **Gemini asymmetry** (target ok, source unproven): matrix rows typed explicitly; no silent "works sometimes".

## 10. Definition of Done

- `chatId` exists end-to-end: minted on first normal_chat, carried on run records and handles, self-tagged backfill for legacy runs.
- A durable chat transcript store backs `GET /client/chats/{id}/timeline`; TUI and Desktop render from it; replay is idempotent across restart/reconnect (matrix §3.3 green).
- `POST /client/chats/{id}/switch-provider` performs the full §4 P-4 operation atomically with typed guards; the full §3.2 matrix passes on real providers (CS-01) — including BUG-330's exact pin set (CS-03).
- Provider switch in TUI (Tab, `/mode`, `/model`, `/provider`) and Desktop (chips, posture Tab) keeps the transcript, appends one truthful divider, and never creates a second visible chat; in-flight guard proves single-leg under rapid fire (CS-05).
- Same-provider model changes remain in-place with no new leg (CS-02); workflow runs cannot switch (CS-12).
- Reopen-after-restart and Drive restore operate per chat with full text continuity and typed per-leg degradation (CS-09, CS-10).
- Zero pre-existing test edited; all suites listed in §5.1 stay green; every new behavior covered by the CS-* files.
- SD-26 exists, approved, and linked; SS-05 carries the chat-switch invariant note; CA notes filed per merged slice (`chat-history` dominant key).
- Feature flag `FLOWPILOT_CHAT_SSOT` defaults on only after the E2E walk is recorded; fallback path (§8) verified once with the flag off.
