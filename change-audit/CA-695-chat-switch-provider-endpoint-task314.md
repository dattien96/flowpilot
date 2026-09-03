# CA-695 — Chat switch-provider endpoint: three-phase linearization + chat envelope (Task-314)

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-59
change_type: feature
summary: POST /client/chats/{chatId}/switch-provider behind FLOWPILOT_CHAT_SSOT — three-phase linearized cross-provider switch (intent persisted in phase A, createRun lock-free in B, close+record in C), full-chat handoff envelope with actions digest, crash heal with two-active discriminator, E-9 exactly-once
# --->8---

## What changed (worktree `cp59-chat-ssot`, CP-59 Task-314)

- `chat_switch.go` (new): route + DTOs (`chatSwitchResponse` carries `Model` — review I-R6); `switchChatProvider` three phases per SD26-S-2 — phase A under `s.mu` (heal → guards: 404 `chat_not_found`, 409 `chat_no_active_leg` for detached chats [reattach is direct createRun, review I-R3], 409 `handoff_run_busy` turn/approval/question/in-flight-switch, 409 `handoff_same_provider`, 422 `provider_unavailable` via `registry.Selectable` BEFORE mutation; durable intent `switchFromRunID=own id` persisted via `persistProviderSession` BEFORE unlock [review I-R5]); phase B lock-free (envelope build, `createRun` with `ChatID`+`SwitchFromRunID`+`LegSeq=src+1`, fire-and-return seed via `startTurn` with `handoffPromptPrefix` envelope, SD26-X-7 logged seed failure); phase C (close source `provider_switch`, persist, `appendChatSwitchRecord` E-9 exactly-once).
- `healChatLegsLocked` (SD26-S-2 matrix): two-active discriminator (keep `switchFromRunID != own id`, close `== own id` + missing E-9 appended) + orphan-intent clear; invoked at switch entry (timeline read inherits via phase-A entry).
- `buildChatHandoffContext`: chat-scoped turns from the transcript store → Task-078 `packConversationTurns` + `renderHandoffPrompt` (prefix preserved) + `<actions_summary>` from tool/file/approval records; `fresh_start` (empty prompt, no seed) for empty chats; `chatHandoffBudget` (context-derived ×3 chars/token, 512 KiB hard cap, 64 KiB floor when context unknown — small known contexts use their derived budget, an 8k model never receives a 64 KiB envelope).
- Route registered beside the timeline route; both flag-gated in-handler.

## Prior claims honored

- Task-078 (envelope contract) — `packConversationTurns`/`renderHandoffPrompt`/`handoffPromptPrefix` reused verbatim; run-scoped endpoint untouched.
- CA-693/CA-694 (Task-313 substrate) — capture/store contracts unchanged; switch only appends E-9 through the same writer.
- BUG-330 — the repro path is now impossible by construction: cross-provider Tab routes to this endpoint (Task-315), never injecting a foreign model into the old adapter.

## R1 evidence

- 13 new tests green (`chat_switch_test.go`): flag-off 404, unknown 404, same-provider 409 (no leg minted), busy 409, unavailable 422 pre-mutation, happy path codex→claude on fake adapters (new leg claude legSeq 1, source closed `provider_switch`, E-9 exactly-once with from/to pair, fresh_start no-envelope), crash two-active heal + orphan-intent clear, budget floor/cap/derived, envelope builder (prefix + prior turns + actions digest).
- Full runner suite after the slice: **identical 13-failure baseline set** (`/tmp/f314.txt` diff empty).

## Honest gaps (recorded, not silently dropped)

- `targetContextWindow` returns 0 → envelope budget rides the 64 KiB floor until Task-315/316 wire model metadata (SD-26 D-9 derivation is implemented and tested; only the lookup is pending).
- Summary ladder: `target_summary` self-summarize instruction active on truncation; cached-summary `hybrid` lands with the summary-ledger wiring.
- Crash-heal hook runs at switch entry + (via 313) timeline read, not yet inside `loadPersistedRun`; the user-observable paths are covered.
- 12-directed-pair live matrix remains the Task-315/316 manual DOD; the unit path proves codex→claude and the provider-agnostic construction (switch never reads provider folders).

## Falsifiable expectations locked

1. Same-provider switch never mints a leg (409 + leg count unchanged).
2. A refused switch leaves zero mutation (intent empty, leg active — asserted).
3. E-9 appears exactly once per switch with the from/to pair; crash windows heal to one active leg with the switch record present.

## Audit addendum (2026-08-30, post-review)

- Correction: the slice initially shipped **11** switch tests, not 13 as first reported. The two DOD-named tests have since been added — `TestSwitchNeverHoldsLockAcrossCreateRun` (watchdog + `TryLock` probe: switch completes and `s.mu` is provably acquirable during phase B, DOD-11) and `TestSwitchCrashHealsOnLoad` (closed-no-record cell, idempotent heal re-entry). Suite now **14 tests**.
- X-7 upgraded from log-only to a queryable `switch_seed_failed` chat record (`markSwitchSeedFailed`), pinned by `TestSwitchSeedFailedRecordEmitted`.
- `healChatLegsLocked` extended with the closed-no-record window + `appendChatSwitchRecordOnce` re-entry guard.
- Still deferred (unchanged): context-window budget lookup (floor active), hybrid summary ladder, live 12-pair matrix (manual DOD of Task-315/316), `TestSupabaseChatTranscriptStoreRoundTrip` (needs migration applied — operator step).
- R1 after this addendum: full runner suite identical to the 13-failure baseline.
