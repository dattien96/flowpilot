# Task-433 — Run-View Cache (Layer-2 Lite)

- Document ID: `Task-433`
- Title: `Run-View Cache — LRU`
- Phase: `task`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Parent Documents: `CP-84 (Realtime Multi-Lane Attention)`
- Child Documents: ``
- Related Documents: `Task-421 (project-scoped bootstrap), Task-425 (spectator), Task-429 (events mux)`
- Replaces: ``
- Tags: `project-nav, desktop-ui, cache`

## AI Quick View

### Summary

- Bounded/LRU hoá **existing `_runSnapshots` seam** để switch-back render
  tức thì rồi revalidate; không tạo `_runViews` registry song song.
- Cache là render hint, không authority. Pin snapshot cần cho active
  parent/child navigation; optional history snapshots mới bị LRU evict.

### Current Ask

- Extend `RunSnapshot` với access/freshness/scroll-anchor metadata; prune
  bounded entries với pin set; hydrate cache rồi always revalidate với
  stale-response guard. Task-429 update chỉ mark dirty, không xoá timeline.

### Key Decisions

- `T-1` Reuse `_runSnapshots` + `cacheRunSnapshot`/`restoreRunSnapshot`.
  Không có `_runViews`: parallel map sẽ duplicate timeline/status authority
  và trái engine rule "extend existing seams".
- `T-2` LRU cap áp dụng cho **evictable history views**. `runId`,
  `mainRunId` và snapshots cần cho active child-focus/back-to-main path
  được pin; nếu snapshot vẫn thiếu, navigation phải fetch/replay thay vì
  early-return/no-op.
- `T-3` Restore dùng stable scroll anchor (`timelineItemId + pixelOffset`),
  không raw `scrollTop` đơn thuần; timeline heights đổi sau revalidation.
- `T-4` Mọi cache hit hiển thị `revalidating/stale` rồi fetch authority.
  Task-429 upsert có revision mới → mark snapshot dirty; terminal không
  xoá cache (completed timeline vẫn hữu ích), chỉ buộc revalidation.
- `T-5` Async fetch/replay có generation guard: A→B→C nhanh không cho
  response A overwrite current C. Revalidated server state luôn thắng cache.

### Constraints

- Không chuyển toàn store thành `Record<runId>` interactive slices — CP-84
  R-5 non-goal. Chỉ bounded existing snapshot cache.
- Preserve tất cả fields hiện có (`artifacts`, pending arrays, token usage,
  replay seq, paging anchor, evicted ids); không tạo reduced duplicate shape.
- Snapshot capture phải clone mutable collections/arrays cần thiết — không
  giữ alias `Set`/array để focused reducer mutate cached lane.
- Memory bound đo theo entry count + timeline window hiện có; pin set có
  hard upper bound theo active focus ancestry. Delete chat/project prunes
  related snapshots.

### Open Questions

- SpectatorPane tiếp tục dùng attention/history projection; không đọc trực
  tiếp `_runSnapshots` trong task này để tránh biến internal focus cache
  thành public state authority.

### Source Refs

- `CP-84 P-5`, `store.ts` (`_runSnapshots`, `cacheRunSnapshot`,
  `snapshotRunState`, `openRunAtAttention`, `openHistoryRun`,
  `resetRun`), `SpectatorPane.tsx`

## 1. Goal

Switch giữa các lane trong phiên triage mượt: quay lại run đã mở thấy
ngay timeline + vị trí scroll, thay vì loading trắng + replay.

## 2. Parent Links

- coding plan: CP-84 (P-5)
- tech design: —
- system spec: SS-13
- specific upstream ids: Task-421, Task-425, Task-429

## 3. Trigger

Mỗi switch hiện tại: `resetRun()` → refetch → replay. Vibe sprint nhiều
legs → replay lâu dần; triage 5 lane nhân chi phí này ×5 mỗi vòng.

## 4. Exact Change

- `T-1` Extend `RunSnapshot` với `projectId`, `cachedAt`,
  `lastAccessedAt`, `dirtyRevision`, `scrollAnchor`; keep existing data fields.
- `T-2` `cacheRunSnapshot` clone snapshot + touch metadata rồi gọi
  `pruneRunSnapshots` với pin set `{runId, mainRunId, active focus ancestry}`.
- `T-3` `openHistoryRun`/`openRunAtAttention`/`backToMainRun`: cache hit →
  restore + stale/revalidating marker + generation-guarded replay; miss →
  fetch/replay fallback, never no-op chỉ vì LRU đã evict.
- `T-4` Task-429 upsert revision > cached revision → mark dirty. Remove/
  terminal không xoá useful timeline; revalidation settles final authority.
- `T-5` ChatWorkspace captures/restores stable item anchor after render;
  delete chat/project and app reset prune applicable snapshots.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/state/store.ts`,
  `src/components/ChatWorkspace.tsx` (scroll restore hook),
  `src/components/SpectatorPane.tsx` (optional read)
- modules: desktop state + components
- routes: none
- tables: none

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/state/store.ts
interface TimelineScrollAnchor {
  itemId: string;
  offsetPx: number;
}

interface RunSnapshot { // EXTEND existing seam — complete target shape
  timeline: TimelineItem[];
  artifacts: Artifact[];
  status: RunStatus;
  pendingApprovals: PendingApproval[];
  pendingQuestions: PendingQuestion[];
  latestTokenUsage?: TokenUsageSnapshot;
  lastTurnInput?: TurnInput;
  recoverable: boolean;
  _streamingAssistantId?: string;
  activeStepId?: string;
  lastEventSeq?: number;
  timelineHasOlder?: boolean;
  _timelineAnchorSeq?: number;
  _timelineEvictedIds?: Set<string>;
  projectId?: string;                 // NEW
  cachedAt: number;                   // NEW
  lastAccessedAt: number;             // NEW
  dirtyRevision?: number;             // NEW — mux says authority advanced
  scrollAnchor?: TimelineScrollAnchor;// NEW
}
const RUN_SNAPSHOT_LRU_CAP = 5; // evictable entries; pinned active ancestry excluded

function cacheRunSnapshot(state: AppState, runId?: string, anchor?: TimelineScrollAnchor): void
function restoreRunSnapshot(snapshot: RunSnapshot): Partial<AppState> // clone mutable values on restore
function touchRunSnapshot(state: AppState, runId: string): RunSnapshot | undefined
function pruneRunSnapshots(snapshots: Record<string, RunSnapshot>, pinnedRunIds: Set<string>): Record<string, RunSnapshot>
function markRunSnapshotDirty(state: AppState, runId: string, revision: number): void
```

```ts
// apps/desktop-flowpilot/src/state/store.ts — openHistoryRun/openRunAtAttention/backToMainRun
// cache hit: restore immediately + set revalidating, then authority fetch with request-generation guard.
// cache miss: fetch/replay fallback; never early-return solely because snapshot was evicted.
```

## 7. Test Signatures

- `test("openHistoryRun restores existing snapshot then always revalidates", ...)`
  — immediate render + authority refresh.
- `test("LRU prunes only evictable snapshots and pins current/main ancestry", ...)`
  — cap 5 không phá child-focus/back-to-main.
- `test("backToMainRun fetches when pinned snapshot is unexpectedly absent", ...)`
  — no silent no-op.
- `test("mux revision marks snapshot dirty without deleting completed timeline", ...)`.
- `test("rapid A-B-C switches ignore stale A and B revalidation responses", ...)`
  — generation guard.
- `test("stable item anchor restores after timeline height changes", ...)`.
- `test("cache capture and restore do not alias mutable arrays or Sets", ...)`.
- `test("delete chat/project prunes related snapshots", ...)`.
- `test("revalidated timeline and paging metadata override cached values", ...)`.
- `test("existing attention ingest via cacheRunSnapshot remains intact", ...)`
  — Task-404 regression guard.

## 8. Acceptance Check

- Live: mở run A (timeline dài) → switch sang B → quay lại A: timeline
  + scroll hiện ngay, status line báo "updating…" rồi settle; không
  flash trắng.

## 9. Out of Scope

- Full `Record<runId>` interactive state (timelines/composer/modal per
  run) — CP-84 R-5; persist cache qua app restart (session-scoped);
  đổi SpectatorPane sang đọc internal `_runSnapshots`.

## 10. Definition of Done

- [ ] All §6 signatures implemented exactly (or deviation documented in §11)
- [ ] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [ ] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [ ] Provider parity N/A — pure client view cache (noted in §11)
- [ ] No `_runViews`/parallel cache registry added; existing `_runSnapshots` semantics extended
- [ ] Pinned current/main/active ancestry cannot be evicted; cache miss has fetch fallback, never silent no-op
- [ ] Every cache hit revalidates; rapid-switch stale-response and mutable-alias tests green
- [ ] Stable scroll anchor, delete cleanup, memory cap, and Task-404 attention regression verified
- [ ] `feature_key` = `project-nav` (view-cache là navigation UX); CA entry written
- [ ] §8 acceptance checks verified by hand or test
- [ ] GitNexus impact run for `cacheRunSnapshot`, `restoreRunSnapshot`, `openHistoryRun`, `backToMainRun` before code
- [ ] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
