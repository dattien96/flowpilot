# BUG-405: `reconstructRunInternal` drops legState/legClosedReason/switchFromRunID — switch-provider 409, durable rows clobbered

## Metadata

- Document ID: `BUG-405`
- Title: `Post-restart reconstructed chat legs lose legState/legClosedReason/switchFromRunID → chat_no_active_leg + persistSessionSnapshot omitempty clobbers durable rows`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-59-Test-Steps](../../07-Coding-Plan/done/CP-59-Test-Steps.md), [CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat](../../07-Coding-Plan/done/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md)
- Feature Keys: `chat-ssot`, `multi-leg`, `provider-switch`, `flow-resume`

## AI Quick View

### Summary

- CP59-1 (chat `cht_40a2a29ee70c`, legs run-1→run-14→run-189→run-301→run-312): after a runner SIGKILL/restart, `reconstructRunInternal` restored `chatID`/`legSeq` but not `legState`/`legClosedReason`/`switchFromRunID` — every reconstructed leg is resident with `legState==""`.
- Three verified consequences: timeline renders `legState:""`/`legClosedReason:""` for all reconstructed legs; `switch-provider` → `409 chat_no_active_leg` (chat orphaned until a reattach mints a fresh active leg); post-restart `persistSessionSnapshot` writes omit the three fields (`omitempty`), so last-wins rows permanently erase previously persisted values (`sessions.ndjson` rows 38–39 vs 31–37).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** after restart, `GET /client/chats/{id}/timeline` shows `legs: [(0,devin,""),(1,grok,""),(2,opencode,""),(3,opencode,""),(4,devin,"")]` while `sessions.ndjson` still holds correct values; `POST /client/chats/{id}/switch-provider` → `409 chat_no_active_leg` ("chat has no active leg — reattach via startRun with chatId+switchFromRunId"); any post-restart turn on a reconstructed leg rewrites its durable rows **without** `leg_state`/`switch_from_run_id`/`leg_closed_reason`.
- **Expected:** reconstructed legs restore all three fields from persisted `ProviderSessionState` (same class of gap as the `chatID`/`legSeq` restoration fixed for BUG-330 — residual comment at `interactive_resume.go:890-893`); timeline shows `closed/provider_switch` for old legs, `active` for the survivor; `switch-provider` keeps working post-restart; persisted rows never regress to missing fields they previously had.
- **Actual:** legs come back with the three fields empty; the chat silently demotes to "detached" semantics after every restart; durable history is progressively destroyed by subsequent writes.
- **Impact:** functional (switch-provider dead post-restart until a manual reattach) + durable-store data loss (last-wins per `run_id` clobbers good rows). Recoverable only via reattach (`POST /client/workflow-runs` with `chatId`+`switchFromRunId` minted leg-5 run-584 → switch-provider worked again for leg-6 run-591, `includedTurnCount=11`).

## Reproduction

1. `runner serve` with a multi-leg chat (≥2 switch-provider hops; here legs 0–4 on `cht_40a2a29ee70c`).
2. `pkill` the runner mid-turn on the active leg (turn-504 on run-312).
3. Restart with identical env/cwd.
4. `GET /client/chats/cht_40a2a29ee70c/timeline` → all reconstructed legs `legState:""` while `sessions.ndjson` shows correct values.
5. `POST /client/chats/cht_40a2a29ee70c/switch-provider {"targetProviderKey":"grok"}` → `409 chat_no_active_leg` (pre-reattach; a later same-provider probe returned `handoff_same_provider`, proving the endpoint itself works).
6. Send a turn on a reconstructed leg (turn-575 `RESUMED_OK` on run-312) → post-restart `persistSessionSnapshot` writes rows 38–39 without the three fields, erasing what rows 31–37 wrote.

## Root cause

- `reconstructRunInternal` (`apps/local-runner/internal/runner/interactive_resume.go` ~L890-895): restores `chatID`/`legSeq` from `ProviderSessionState` but not `legState`/`legClosedReason`/`switchFromRunID` — even though `local_file_session_store.go` persists all three (`ndjsonSessionRecord` `leg_state`/`leg_closed_reason`/`switch_from_run_id`, ~L81-83 `omitempty`) and `sessionStateFromRecord` reads them back.
- Timeline: `handleChatTimeline` (`chat_timeline.go:106-115`) prefers resident runs and copies `rs.legState`/`rs.legClosedReason` verbatim; persisted rows with correct values are skipped by the resident-preferred merge (`chat_timeline.go:127-130`).
- Switch: `chat_switch.go:243` requires a resident leg with `legState=="active"` → none exists → 409 `chat_no_active_leg`.
- Clobber: `persistSessionSnapshot` emits rows with the fields absent (`omitempty` drops empty strings); the ndjson store is last-wins per `run_id`.

## Evidence

- `~/fp-beds/lt-evidence/cp59/BUG-LIVE-59-1.md` — full repro writeup.
- `~/fp-beds/lt-evidence/cp59/l6_timeline_after_restart.json` — all legs `legState:""` post-restart.
- `~/fp-beds/lt-evidence/cp59/l6_timeline_reattached.json` / `l6_timeline_endstate.json` — legs 0–4 `""`, fresh leg-5 `active`, leg-6 `active` after reattach+switch.
- `~/fp-beds/lt-evidence/cp59/l6_switch_after_restart_error.json` (409 `chat_no_active_leg`), `l6_switch_after_reattach.json` (works again, `includedTurnCount=11`), `l6_reattach.json` (run-584 legSeq 5).
- `~/fp-beds/lt-evidence/cp59/bug59_ndjson_excerpt.txt` — rows 38–39 lack `leg_state`/`switch_from_run_id`/`leg_closed_reason` that rows 31–37 had.
- `~/fp-beds/lt-evidence/cp59/RESULT.md` (L-59-6 PARTIAL, BUG-LIVE-59-1).

## Severity

`high` — functional regression (switch-provider dead) plus durable data loss on every post-restart write for reconstructed legs.

## Completion Notes (implemented 2026-09-23, CA-922)

- Fix: `reconstructRunInternal` restores `legState`/`legClosedReason`/`switchFromRunID` from `ProviderSessionState` (same class as BUG-330's chatID/legSeq restore). Reconstructed legs render correctly and switch-provider works post-restart; `persistSessionSnapshot` no longer rewrites durable rows with the fields `omitempty`-dropped.
- Files: `internal/runner/interactive_resume.go`.
- Tests: `TestBug405_ReconstructRestoresLegFields` (assertion-red on baseline).
