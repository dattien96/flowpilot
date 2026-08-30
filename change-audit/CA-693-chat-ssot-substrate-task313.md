# CA-693 — Chat SSOT substrate: chatId, transcript store, timeline (Task-313 slice 1)

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-59
change_type: feature
summary: chatId mint/adopt/self-tag on chat runs, durable NDJSON chat transcript store with idempotent (chatId,chatSeq) append, capture hook beside persistEvent, and the flag-gated GET /client/chats/{chatId}/timeline read model
# --->8---

## What changed (worktree `cp59-chat-ssot`, CP-59 Task-313 substrate slice)

- `chat_ssot.go` (new): leg state/reason constants (`SD26-S-1`), `FLOWPILOT_CHAT_SSOT` gate (default off), `newChatID` (`cht_<12-hex>`), `ensureChatTagging` (legacy self-tag, caller holds s.mu), `resolveChatIdentity` (mint/adopt resolved BEFORE `createRun` takes `s.mu` — `stampAccount` pattern), `chatRunRegistry` (runID→chatID + per-run degraded flag, own mutex — capture never touches `s.mu`), `recordChatTranscript` (hooked into `persistEvent`, same delta skip, never fails the turn — SD26-X-6), `chatRecordsFromProviderEvent` mapping to `SD26-E-1..E-9` records.
- `chat_transcript_store.go` (new): `ChatTranscriptRecord`, `ChatTranscriptStore` (append idempotent on `(chatId, chatSeq)` — restore upsert semantics), `localFileChatTranscriptStore` (NDJSON per chat, O_APPEND, 1 MiB-line reader, corrupt-tail skip, path-traversal-sanitized ids), `chatTranscriptWriter` (monotonic per-chat seq, seeds from `LatestChatSeq` across restart, dedicated mutex — never `s.mu`, SD26-S-2 lock rule).
- `chat_timeline.go` (new): `ChatSessionReader` optional store capability, `GET /client/chats/{chatId}/timeline` (legs = resident + persisted, chat-kind gate absolute, legacy self-tag on read, `afterSeq`/`limit` pagination + `truncated`, `degraded`, consecutive-duplicate-final collapse), `ensureChatTranscriptWriter` (flag-gated lazy init, dir `~/.flowpilot/chat-transcripts` via `FLOWPILOT_CHAT_STORE_DIR` override).
- Wiring: `interactiveRun` + `ProviderSessionState` + runner `RunHandle`/`StartRunInput` + TUI-client mirrors gain additive chat fields (omitempty — old payloads byte-identical, pinned by `TestChatFieldsAdditiveJSON`); `createRun` resolves/stamps/persists chat identity for `normal_chat` when the flag is on and echoes `ChatID`/`LegSeq` on the handle; route registered, handler 404s typed when flag off.
- Out of this slice (remaining Task-313 DOD): Supabase `workflow_chat_events` impl + upsert keys, legacy backfill from `transcriptTurnsFromRun`, full-`startRun` mint integration test, desktop store mirror. Flag stays off; zero behavior change when off.

## Prior claims honored (context-discipline)

- CA-679 (TUI restore keeps user model) — untouched; chat fields do not interact with model restore.
- BUG-329/CA-680 (opencode mid-chat switch, session adopt) — `createRun` chat stamping is additive before the existing flow; no adapter change.
- CA-688 (opencode resume precheck) — no resume-path change in this slice.
- Task-078 (handoff contract) — untouched; `supportsHandoffSource`/`buildHandoffContext` unmodified (Task-314 scope).

## Tests (additive, new files only)

- 19 new tests, all green: identity format/self-tag/adopt-precedence, flag default-off/opt-in, additive-JSON pins, NDJSON round-trip/idempotent-dup/corrupt-tail/traversal-guard, writer concurrency + restart seq seed, capture delta-skip/mapping/degraded/flag-off-noop/restart continuation, timeline 404 (unknown + flag-off)/leg-join order + self-tag + workflow exclusion/tail pagination.
- R1 evidence: baseline run (clean tree, 91643075) has 13 pre-existing failures (documented WIP). Full-suite after-run ×2: same 13 + one flake per run. Both flakes proven pre-existing on the **stashed clean tree**: `TestTryAdvanceSpawnsAgentCodeWriterWithWriterPrompt` (1/5 fail — git gate "exit status 128 failing closed"), `TestMultiWorkspaceRunsIndependent` (1/3 fail). Zero new deterministic failures.

## Falsifiable expectations this CA locks

1. Flag off ⇒ no chat keys in any payload, capture no-op, timeline 404 `chat_ssot_disabled`.
2. `chatSeq` strictly increasing per chat across writer restarts; duplicate `(chatId, chatSeq)` never duplicates a record.
3. Workflow runs never join a chat timeline (chat-kind gate absolute — CS-12 substrate).
