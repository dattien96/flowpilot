# Task-421 — Desktop Timeline Windowed Store (Long-Chat Memory Bound)

- Document ID: `Task-421`
- Title: `Desktop Timeline Windowed Store`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: `Task-405 (Desktop UI Consistency Revamp)`
- Child Documents: ``
- Related Documents: `CA-922 (Devin-UX polish session that identified this gap)`
- Replaces: ``
- Tags: `desktop, chat-ui, performance, chat-history`

## AI Quick View

### Summary

- `useStore.timeline` is an append-only in-memory array; a chat running
  continuously for a day+ accumulates thousands of items, and every streamed
  token currently triggers O(n) scans (`countPrompts`, `sliceTimelineFromPrompt`)
  plus a fresh array allocation per `set()`.
- Render cost is already bounded (prompt paging + `content-visibility` shipped
  in CA-922); what remains is **memory + GC pressure** from the unbounded array
  and `_runSnapshots` growth.
- Goal: keep only a tail window of timeline items in the store; "Load earlier"
  pages older events from the runner's persisted transcript/event log — the
  durable store is the source of truth, RAM is a cache.

### Current Ask

- Implement a windowed timeline: bounded `timeline` array + explicit older-event
  paging, without breaking replay (`openHistoryRun` → `consumeStream` seq
  dedup), scroll-back, or pending-approval restoration.

### Key Decisions

- `T-1` The runner's persisted chat transcript/event log is the source of
  truth; the in-memory `timeline` is a render cache, not the record.
- `T-2` Windowing is keyed on event **seq**, not array index — older pages are
  fetched by seq ranges so live-streamed events and paged history interleave
  without dedup drift.
- `T-3` Never drop items that carry unresolved pending state
  (`pendingApprovals`, `pendingQuestions`, decision cards) even if they fall
  outside the window — evict display-only, not actionable state.

### Constraints

- Additive tests only; replay/resume tests (`store.history-replay-*`,
  `store.chat-mode-persist`) must stay green.
- Provider-agnostic: windowing must not depend on Claude/Codex/Grok event
  shapes (cross-provider-parity).
- No behavior change when `timeline.length` is below the window threshold.

### Open Questions

- Does an existing endpoint already serve paged history? Candidates seen:
  `GET /client/chats/{chatId}/timeline` (`handleChatTimeline`) and
  `streamRun(runId, sinceSeq)`. Need to verify whether either supports
  *backward* paging (events before seq X) or whether a new
  `GET /client/chats/{chatId}/timeline?beforeSeq=` param is required.
- Should `_runSnapshots` get the same LRU bound in the same task?

### Source Refs

- `Task-405`, `CA-922`, `Task-404` (attention queue consumes the same store)

## 1. Goal

A chat can run continuously for 24h+ accumulating thousands of timeline items
without the renderer's memory footprint or per-token scan cost growing
linearly with total chat length.

## 2. Parent Links

- coding plan: — (none yet; spawn CP if the runner endpoint needs adding)
- tech design: — (SD-26 chat SSOT is adjacent: transcript store is source of truth)
- system spec: SS-13 document contract; SS-26 chat persistence intent
- specific upstream ids: Task-405, Task-404, BUG-146 (scroll pagination ordering)

## 3. Trigger

The CA-922 UI pass bounded the *render* path (6-prompt paging +
`content-visibility`) but left the store unbounded. User confirmed the real
use case: multiple vibe-flows per project running continuously — timelines
will grow far past what a resident array should hold.

## 4. Exact Change

- `T-1` Introduce a timeline window: `timeline` holds at most `N` tail items
  (proposed `N = 500`, tune after measuring item weight) plus a
  `timelineHasOlder: boolean` + `timelineOldestSeq?: number` cursor.
- `T-2` Add `loadEarlierTimeline()` store action: pages older events from the
  runner (new or extended endpoint per Open Questions), prepends them, and
  advances the cursor; "Load earlier" button in `Timeline.tsx` calls it when
  the in-memory window is exhausted instead of only paging prompt-count.
- `T-3` Eviction policy: drop items older than the window on ingest, but pin
  any item referenced by pending interactive state (approvals/questions/
  decision cards) and any `prompt` item needed to anchor the current window
  boundary.
- `T-4` Make `countPrompts`/`sliceTimelineFromPrompt` consume the window only;
  `hiddenPromptCount` becomes `windowHidden + serverSideRemainder` so the
  "Load earlier" affordance stays truthful.
- `T-5` Stream-path guard: `consumeStream` dedup/seq bookkeeping must not
  assume evicted items are present — seq cursors are the contract, not array
  position.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/state/store.ts`,
  `apps/desktop-flowpilot/src/components/Timeline.tsx`,
  `apps/desktop-flowpilot/src/state/timelineReducer.ts`,
  possibly `apps/desktop-flowpilot/src/types/contract.ts` +
  `apps/local-runner/internal/runner/interactive_handlers.go`
  (`handleChatTimeline` paging param)
- modules: desktop renderer store + runner chat-timeline API
- routes: `GET /client/chats/{chatId}/timeline` (likely `beforeSeq`/`limit`)
- tables: none (renderer-side only; runner transcript store read-only)

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/state/store.ts (interface excerpt)
interface AppState {
  timeline: TimelineItem[];
  /** true when persisted events older than the in-memory window exist. */
  timelineHasOlder: boolean;
  /** seq cursor of the oldest retained item — paging anchor for T-2. */
  timelineOldestSeq?: number;
  loadEarlierTimeline(): Promise<void>;
}
```

```ts
// apps/desktop-flowpilot/src/state/timelineReducer.ts
// Evict display-only items outside the window; keep pinned pending/prompt
// anchors. Returns the same array when under the threshold.
export function applyTimelineWindow(items: TimelineItem[], max: number, pinned: Set<string>): TimelineItem[] // T-1, T-3
```

```go
// apps/local-runner/internal/runner/interactive_handlers.go (if needed)
// handleChatTimeline gains optional ?beforeSeq=&limit= backward paging.
```

## 7. Test Signatures

- `test("window evicts oldest items but pins pending approvals", ...)` — asserts T-3 (AC-1)
- `test("loadEarlierTimeline prepends older events without seq gaps/dupes", ...)` — asserts T-2/T-5 (AC-2)
- `test("history replay on reopened chat rebuilds windowed timeline", ...)` — replay path regression (AC-3)
- `test("below-threshold timeline never pages/evicts", ...)` — no-op path (AC-4)
- `test("attention queue items survive window eviction", ...)` — T-3 + Task-404 parity (AC-5)
- `TestHandleChatTimeline_BeforeSeqPaging` (Go, if endpoint extended) — asserts deterministic page order (AC-2)

## 8. Acceptance Check

- A synthetic chat with 5k+ timeline items keeps `timeline.length` ≤ window
  bound while scrolling to the top repeatedly loads older pages.
- During a live streaming turn, memory does not grow unboundedly past the
  window; no visual gaps or duplicated rows at page seams.
- Reopening a long chat (history replay) lands at the latest window, not a
  full-array reload.

## 9. Out of Scope

- Sub-agent timelines, `_runSnapshots` sizing (Open Question), remote
  (Drive-synced) transcript paging, server-side transcript compaction.
- Any change to persisted event schemas.

## 10. Definition of Done

- [x] All §6 signatures implemented exactly (or deviation documented in §11)
- [x] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [x] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [x] Provider parity proven or evidenced where the change touches shared/provider paths (R2)
- [x] `feature_key` set; CA ledger entry written (CA-923); FEATURE-KEYS.md already contains the key
- [x] §8 acceptance checks verified by hand or test
- [ ] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: implemented — windowed timeline store (TIMELINE_WINDOW_MAX=500), pinned interactive rows, evicted-id replay dedup, beforeSeq backward paging end-to-end (runner local+supabase+fallback, client, store loadEarlierTimeline, Timeline UI). 25 new tests green; full suite = 14 pre-existing baseline failures, zero regressions.
- follow-ups: `_runSnapshots` LRU bound remains open scope; seam dedup is kind:text keyed (cosmetic collapse edge documented in CA-923).
- upstream docs updated: CA-923; CP-82/83 unaffected.
