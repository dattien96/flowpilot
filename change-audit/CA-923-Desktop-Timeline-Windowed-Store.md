# CA-923 — Desktop windowed timeline store (Task-421)

## Context

`useStore.timeline` was an append-only array: every prompt, message, tool
call, and file-change row accumulated for the life of a chat. A run that
loops for 24h+ (auto vibe) accumulated thousands of items; every streamed
token re-rendered and several O(n) scans ran per event. The DOM was already
bounded (prompt paging + `content-visibility`), but the store itself was not.

Task-421 makes the persisted transcript/event log the source of truth and
the in-memory `timeline` a bounded render cache.

## Changes

### Runner (backward paging)

- `handleChatTimeline` + `handleGetRunTimeline` accept optional `beforeSeq`:
  `chatSeq < beforeSeq`, ascending, tail-bounded by `limit`; `beforeSeq=-1`
  resolves to `MaxInt64` = latest page. `truncated` on a backward page means
  "older records exist" (full-page heuristic; the following empty page
  converges the client).
- `ChatTranscriptBackwardReader` optional capability + `readChatRecordsBefore`
  shared helper. `localFileChatTranscriptStore` implements it natively
  (sorted slice); `SupabaseWorkflowStore` implements it as
  `chat_seq=lt.N&order=desc&limit=M` + reverse. Stores without the capability
  fall back to a forward read + `tailBefore` in-memory slice — same contract.

### Contract / client

- `RunnerClient.chatTimeline(chatId, afterSeq?, limit?, beforeSeq?)` —
  fourth param is additive; all prior call sites unchanged.
- New optional `RunnerClient.runTimeline?(runId, {beforeSeq, limit, afterSeq})`
  hitting `/client/workflow-runs/{id}/timeline` — workflow runs persist
  transcript records under the run id. Optional so legacy test doubles still
  compile; callers fall back to the in-memory window.
- `HttpWsRunnerClient` sends the params; `MockRunnerClient` mirrors the shape.

### Renderer store

- `TIMELINE_WINDOW_MAX = 500` resident cap. `applyTimelineWindow(timeline,
  evictedIds, max)` evicts oldest non-pinned rows and records their ids into
  `_timelineEvictedIds` (capped at 50k) so a replayed/re-delivered event never
  re-appends a dropped row.
- `isPinnedTimelineItem`: never evicts unresolved approvals/questions/
  decision-cards (they gate `settleHistoryReplayPendingState`), `thinking`,
  the in-flight assistant bubble, or rows the user paged back (`pinned`).
- Every `applyTimelineEvent` return path runs the window via `finalize()`;
  `wasEvicted(id)` guards added to `turn_started`/`message_delta`/
  `message_completed`/`tool_started`/`tool_completed`/`file_changed`/
  `agent_*`/`flow_*` dedup checks.
- New state: `timelineHasOlder`, `_timelineAnchorSeq` (smallest materialized
  chatSeq), `_timelineLoadingEarlier`, `_timelineEvictedIds`. All are part of
  `RunSnapshot` so agent-focus round-trips preserve paging state.
- `openHistoryRun` now fetches the tail page (`beforeSeq=-1`,
  `TIMELINE_TRANSCRIPT_PAGE=400`) instead of the full transcript, windows the
  result, and seeds anchor/hasOlder.
- `loadEarlierTimeline()`: backward page from the anchor (or `-1` first),
  `includeCurrentLeg=true` build (the window may have evicted current-leg
  rows), seam dedup by `kind:text|id` against retained rows, paged rows
  pinned, anchor/hasOlder advanced. Stale-write guarded by runId.
- `Timeline.tsx` Load-earlier button: in-memory prompt pages first, then
  falls through to `loadEarlierTimeline` while `timelineHasOlder`.
- Optimistic prompt id switched to a module counter (`prompt-local-N`) — the
  old `prompt-${timeline.length}` scheme collides forever once the array
  stops growing.

## Safe-fix compliance

- Zero existing tests edited. `runTimeline` made optional precisely because
  several legacy `makeClient` fixtures don't implement it — they still
  compile unchanged.
- Full desktop suite: 14 failures = exactly the recorded HEAD baseline
  (history-replay order ×3, selectProject localStorage, openHistoryRun
  historyOpenError, sendPrompt abort, workflow handoff, stop flowRef,
  terminal-replay ×3-providers, findDuplicateJiraIntegration, client-core
  boundary, HttpRunnerRepository health, TestSupervisor ×2). No new failures.
- New tests: `timelineWindow.test.ts` (9: eviction, pinning, dedup-after-
  eviction, below-bound no-op), `store.timelineWindow.test.ts` (8: page
  prepend/anchor/pin, seam dedup, stale-run guard, open tail page, bounded
  hydrate, below-window), `task421_timeline_beforeseq_test.go` (8: backward
  page order/bounds, latest page, exhaustion, exact-boundary convergence,
  forward-compat, run-keyed parity, local store, forward-only fallback).
- Provider parity: paging is transcript/seq-keyed — provider-agnostic; legs
  are provider-labeled but the window never branches on providerKey. The
  3-provider pre-existing replay-order failures were re-verified byte-identical
  to baseline (ordering drift, not windowing).

## Known edges (accepted, logged)

- Seam dedup keys on `kind:text` — two adjacent *identical* prompt texts
  across a page boundary collapse to one visible row. Cosmetic only; seq
  anchors stay correct.
- `truncated = len(recs) == limit` is a heuristic at exact boundaries — the
  client pays one extra empty fetch to learn exhaustion (tested).
- `_runSnapshots` LRU growth is unchanged (per-task scope); per-entry size is
  now bounded, which is the part that mattered for memory.

## Validation

- `tsc -p tsconfig.phase1-tests.json` clean except pre-existing
  `adminLogic.test.ts` `inputImage` drift.
- New: 25 tests green. Full `src/**` + `tests/phase1/**`: 492 tests, 14 fail
  = baseline set, zero regressions.
- `go test ./internal/runner -run 'BeforeSeq|Forward'` 8/8 pass; package
  compiles clean (full `internal/runner` suite has a pre-existing 600s fake-
  provider hang — baseline, untouched).

## GitNexus

MCP unreachable — manual blast-radius: `chatTimeline` callers = store +
history tests (additive param); `applyTimelineEvent` signature unchanged;
`RunSnapshot`/`snapshotRunState`/`restoreRunSnapshot` extended with optional
fields, all call sites (`focusAgentRun`, `backToMainRun`) verified.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: Task-421
change_type: feature
summary: Windowed timeline store — bounded render cache over the persisted transcript with seq-keyed backward paging (runner beforeSeq + client loadEarlierTimeline), pinned interactive rows, evicted-id replay dedup
# --->8---
