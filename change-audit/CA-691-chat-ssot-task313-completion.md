# CA-691 — Chat SSOT Task-313 completion: Supabase store, legacy backfill, integration tests, migration

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-59
change_type: feature
summary: Supabase workflow_chat_events ChatTranscriptStore + chat columns on workflow_provider_sessions (null-when-empty keys + additive migration), one-shot marker-guarded legacy backfill on timeline read, and real-createRun mint/adopt integration tests closing Task-313
# --->8---

## What changed (worktree `cp59-chat-ssot`, CP-59 Task-313 completion slice)

- `supabase_chat_transcript_store.go` (new): `*SupabaseWorkflowStore` implements `ChatTranscriptStore` over `workflow_chat_events` — append with `Prefer: resolution=ignore-duplicates` + `on_conflict=chat_id,chat_seq` (idempotent upsert per SD26-D-1), ascending `chat_seq` reads with `chat_seq=gt.` pagination, `LatestChatSeq` desc-limit-1 seed; plus `ListProviderSessionsByChat` (ChatSessionReader) with a dedicated `providerSessionSelectChat` column list so recovery reads never require the new columns.
- `supabase_workflow_store.go`: `dbProviderSessionRow` + `providerSessionFromDBRow` gain the chat leg fields; `UpsertProviderSession` writes them **null-when-empty** (`nilIfEmpty`/`nilIfZeroInt`) so pre-migration deployments are untouched while the flag is off; `nilIfZeroInt`/`derefString` helpers.
- `chat_timeline.go`: `ensureChatTranscriptWriter` prefers the Supabase store when `s.workflowStore` is `*SupabaseWorkflowStore` (local NDJSON otherwise); `backfillLegacyChatTranscript` — one-shot raw synthesis from the newest resident leg's in-memory events via `transcriptTurnsFromRun`, guarded by the `chat_backfilled` marker record; invoked on timeline reads.
- Migration `supabase/migrations/20260830080000_chat_ssot_chat_columns_and_events.sql`: 5 additive `workflow_provider_sessions` columns + `workflow_chat_events` with PK `(chat_id, chat_seq)` + leg index.
- Tests: `TestChatIDMintedOnFirstNormalChat` / `TestChatIDAdoptedFromSwitchFromRunID` (real `createRun`: mint `cht_<12hex>`, explicit-seq leg, implicit max+1, SwitchFromRunID adoption, workflow-run gate, timeline 4-leg ordered join), `TestLegacyChatBackfillRawOneShot` (5 records first read — 2 turns × prompt/final + marker; identical second read).

## Prior claims honored

- CA-690 (substrate slice) — extended, not reworked; `recordChatTranscript`/writer/store contracts unchanged.
- CA-688 (opencode resume precheck) — no resume-path edits.
- CA-679 / BUG-329 — untouched paths.

## R1 evidence

- Runner full suite after slice 2: **exactly the 13 pre-existing baseline failures** (identical name set to the stashed-clean baseline).
- TUI suite: 7 failures, **identical count on the stashed clean tree** (spinner-glyph/YouBox border render assertions — pre-existing).
- Flaky probes: `TestTryAdvanceSpawnsAgentCodeWriterWithWriterPrompt` (1/5 fail clean), `TestMultiWorkspaceRunsIndependent` (1/3 fail clean) — both pre-existing on the branch/machine, not attributable to this change.

## Falsifiable expectations locked

1. A Supabase-backed runner (post-migration, flag on) records chat timelines durably in `workflow_chat_events`; a local runner uses NDJSON under `~/.flowpilot/chat-transcripts`.
2. Pre-migration Supabase deployments see zero behavior change with the flag off (chat keys write as SQL NULL).
3. A legacy chat's first timeline read backfills its raw turns exactly once (marker present; second read adds nothing).
