# BUG-338 — TUI /open and /history still split one chat into 3 rows

## Metadata

- Document ID: `BUG-338`
- Title: `TUI history still splits switched chat into separate rows`
- Phase: `bugfix`
- Status: `done`
- Owner: `codex`
- Reviewers: `codex`
- Created: `2026-08-31`
- Last Updated: `2026-09-02`
- Parent Documents: `CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat`, `SD-26-Chat-Continuity-Ssot`, `SS-05-Workflow-Ai-Provider`
- Child Documents: ``
- Related Documents: `CP-59-Test-Steps`, `CA-693..CA-701`, `fcd70d5`
- Replaces: ``
- Tags: `chat-history, TUI, regression, CP-59`

## AI Quick View

### Summary

- TUI `/open` and `/history` still list each provider-switch leg as its own chat (`run-198151`, `run-197970`, `run-197929`) instead of one grouped chat `cht_a8d253c2fe6f`.
- New grouping code `history_group.go` is live but has no effect because `GET /client/projects/{id}/workflow-runs` returns `chatId=""` for those legs.
- Transcript SSOT `~/.flowpilot/chat-transcripts/chats/cht_a8d253c2fe6f/transcript.ndjson` already proves they belong to one chat (switch records with `legSeq` 1 and 2).

### Current Ask

- Make `GET /client/projects/{id}/workflow-runs` return stamped `chatId`/`legSeq` for persisted legs whose `workflow_provider_sessions` row was written before `fcd70d5`, by joining the durable transcript index.

### Key Decisions

- `V-1` History API must stamp missing `chatId`/`legSeq` from the transcript store at read time; the TUI stays pure view-grouping.
- `V-2` Persisted-only legs after restart must also be stamped, and the store row is backfilled on first stamp so the next restart needs no scan.

### Constraints

- No break to old persisted rows; file and Supabase stores both must handle stamped reads.
- Provider-agnostic: join via transcript `legRunId → chatId`, not per-provider adapters.

### Open Questions

- Whether to add `ListChatIDs` to the transcript store or scan `chats/*/transcript.ndjson` directly in the stamp helper.

### Source Refs

- Runs: `run-198151`, `run-197970`, `run-197929` ( Gate-sandbox `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363` )
- Chat: `cht_a8d253c2fe6f` (also `cht_3810173c6b36` with `run-197689→197698` is expected to have same class)
- Transcript: `C:\Users\dat.nguyen\.flowpilot\chat-transcripts\chats\cht_a8d253c2fe6f\transcript.ndjson` lines 1..40 (`chat_provider_switch` at seq 22 leg 1 and 29 leg 2)
- Live evidence 2026-08-31 15:03: `curl http://127.0.0.1:4317/client/projects/db51ec26-1a0f-4b92-8ceb-b03dc8e9b363/workflow-runs` → those 3 rows have `chatId=""`
- Prior fix: `fcd70d5` added `chatId`/`legSeq` to `local_file_session_store.ndjsonSessionRecord` and `supabase_workflow_store.providerSessionSelectRecovery`, but does not backfill rows written before it
- TUI grouping: `apps/local-runner/internal/tui/app/history_group.go:15`, `helpers.go:1507`, `history.go:163` already live

## 1. Issue Summary

After the TUI grouping fix `fcd70d5`, a switched chat created in section A still shows as 3 separate rows in `/open` and `/history`. The picker dump lists `#1 run-198151`, `#2 run-197970`, `#3 run-197929` each as `chat · completed` with separate titles, instead of one row `#1 … 3 legs` headed by `run-198151`.

## 2. Parent Links

- impacted coding plan: `CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat` Q-4 "history unit is chat" and P-5 TUI grouping
- impacted tech design: `SD-26-Chat-Continuity-Ssot` §5.1 chat identity, §5.3 transcript SSOT
- impacted system spec: `SS-05-Workflow-Ai-Provider` chat history contract

## 3. Environment and Reproduction

- environment: `cp59-chat-ssot` at `fcd70d5` + `373edf3`, TUI grouping code present, runner on `http://127.0.0.1:4317` started via `just chat-dev D:\working\gate-sandbox` (always-ON, flag removed)
- reproduction steps:
  1. In the Gate-sandbox project, run the section-A script: 4 turns on `run-197929` (opencode), `/model grok-4.5` → `run-197970` (leg 1), `/model opencode-go/longcat-2.0` → `run-198151` (leg 2) — all under `cht_a8d253c2fe6f` (timeline line 22 and 29 in transcript).
  2. Exit TUI, restart `just chat-dev` (so `projectRunHistory` reads persisted `workflow_provider_sessions` only).
  3. In TUI type `/open ` and observe picker dump, or `curl /client/projects/db51ec26-1a0f-4b92-8ceb-b03dc8e9b363/workflow-runs` and inspect `chatId`.
- frequency: 100% for chats whose legs were persisted before `fcd70d5`; new chats created after the fix group correctly.

## 4. Expected vs Actual

- expected: `GET .../workflow-runs` returns those 3 rows with `chatId="cht_a8d253c2fe6f"` and `legSeq` 0,1,2; TUI `groupRunsByChatId` collapses to one row `#1 … 3 legs` headed by `run-198151`; `/open 1` or `/open cht_a8d253c2fe6f` opens the head and backfills prior legs via `GetChatTimeline`.
- actual: those 3 rows have `chatId=""` (`legSeq` 0), so `groupRunsByChatId` treats each as its own chat; picker shows 3 rows and numeric open opens the wrong leg without full timeline.

## 5. Impact

- users affected: operator doing gate-sandbox validation of CP-59 Task-314/315; any long-lived switched chat after restart.
- workflows affected: TUI `/open`, `/history`, `/resume` picker, and `formatChatList` dump; Desktop already groups via same field so it would also desync if it read persisted rows without stamp (currently Desktop history is re-fetched from same endpoint).
- severity: medium — chat continuity (envelope, timeline) still works when opened by leg `runId`, but history UX regresses and restart loses the "one chat" invariant.

## 6. Root Cause

- hypothesis: persisted `workflow_provider_sessions` rows for legs `run-198151/197970/197929` were written before `fcd70d5` added `chat_id`/`leg_seq` to `ndjsonSessionRecord` and to `providerSessionSelectRecovery`; the file `sessions.ndjson` on disk for those runs contains no `chat_id` key, and `projectRunHistory`'s persisted branch (`interactive_handlers.go:1090`) copies `sess.ChatID` verbatim (empty), so the API cannot group.
- confirmed cause: `curl .../workflow-runs` on `db51ec26` shows empty `chatId` for those 3 runIds while `readAll("cht_a8d253c2fe6f")` proves the durable transcript already binds `legRunId` to that chat. `git log --oneline` shows `fcd70d5` landed after those legs were created (leg timestamps 14:56,14:57,15:03 on 2026-08-31; `fcd70d5` build only later in TUI, runner still on old session file).
- evidence:
  - Transcript lines 1..40 for `cht_a8d253c2fe6f` show `run-197929` seq 1..20, `chat_provider_switch` at seq 22 `toRunId run-197970 legSeq 1`, seq 29 `toRunId run-198151 legSeq 2`.
  - `GET /client/projects/.../workflow-runs` dump (2026-08-31 15:06) → 3 rows `chatId=""`.
  - Code: `local_file_session_store.go:76` new fields `chat_id` etc added in `fcd70d5` but existing lines lack the key; `projectRunHistory` persisted branch (`interactive_handlers.go:1108`) does not synthesize missing identity.

## 7. Fix Strategy

- `F-1` Add transcript-index helper `legRunId → (chatId, legSeq)` on `localFileChatTranscriptStore` (and `SupabaseWorkflowStore` via `ReadChatRecords` scan): enumerate `chats/*/transcript.ndjson` (or `workflow_chat_events`), parse `legRunId` and `chat_provider_switch.payload.legSeq`, cache `runId → chatId/legSeq` in `InteractiveService`.
- `F-2` In `projectRunHistory(projectID)` stamp missing `ChatID`/`LegSeq` on both the live and persisted branches when empty, using that index; if a persisted session is newly stamped, `UpsertProviderSession` with `ChatID`/`LegSeq` so the next restart needs no rescan.
- `F-3` Keep TUI grouping pure (no transcript join in TUI); it will then receive correct `chatId` and collapse to one row. Old untagged chats stay 1:1.

## 8. Validation

- `V-1` After fix, `curl /client/projects/db51ec26-1a0f-4b92-8ceb-b03dc8e9b363/workflow-runs` shows `run-198151/197970/197929` all with `chatId="cht_a8d253c2fe6f"` and `legSeq` 2,1,0 and TUI `/open ` dump shows one row `#1 … 3 legs` headed by `run-198151`.
- `V-2` Numeric open ` /open 1` and ` /open cht_a8d253c2fe6f` both resolve to `run-198151`; opening an old leg `run-197929` still resolves to head.
- `V-3` `go test ./internal/runner -run TestProjectHistoryPersistedBranchCarriesChatId` and `TestLocalFileSessionStoreRoundTripsChatId` pass with a fixture that seeds persisted sessions without `chatId` but with transcript containing those `legRunId`s — history then carries stamped `chatId`.
- `V-4` Untagged legacy chats (`chatId=""`, no transcript entry) remain separate rows.

## 9. Regression Guard

- tests: extend `project_history_chatid_test.go` with a transcript-stamped fixture (persisted rows without `chatId` + transcript with `legRunId`s → stamped history); keep `history_chat_group_test.go` as view-grouping guard; no edits to already-green old tests (`additive-tests-only`).
- alerts: manual re-run of `CP-59-Test-Steps` S and A sections must still PASS.
- audit checks: verify both `localFileSessionStore` and `SupabaseWorkflowStore` paths (provider-agnostic join, not per-adapter logic).

## 10. Follow-Up Document Updates

- upstream docs that must change: none — SD-26 already defines transcript as SSOT; this fix is the missing derived read.
- notes left unchanged on purpose: `CP-59-Test-Steps` S/A PASS marks remain; `/delete` stays run-level this slice (deleting a grouped chat is a separate UX).
