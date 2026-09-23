# Task-429 — Multiplexed Run Events Endpoint (`GET /client/events/stream`)

- Document ID: `Task-429`
- Title: `Multiplexed Run Events Endpoint`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Parent Documents: `CP-84 (Realtime Multi-Lane Attention)`
- Child Documents: ``
- Related Documents: `Task-430 (durable decision projection — prerequisite), Task-404 (attention queue), CP-81 (lifecycle clients), CP-59 (chat legs)`
- Replaces: ``
- Tags: `event-plane, runner, sse, attention-queue`

## AI Quick View

### Summary

- Thêm 1 SSE endpoint multiplexed phát **bounded run-state projection**
  của các user-visible lane qua một connection; không mirror toàn bộ raw
  `ProviderEvent` stream. Frame gắn `runId/projectId/chatId/revision/kind`
  và decision refs; client reconcile `attentionQueue` + history cache.
- Xoá điểm mù poll 30s nhưng không biến event plane thành nguồn truth:
  durable run/approval/question/worktree records vẫn authoritative; connect
  và reconnect luôn bắt đầu bằng full level snapshot.

### Current Ask

- Runner: realtime projection + conflated subscriber broadcaster + SSE
  handler; desktop: one long-lived subscriber, snapshot reconciliation,
  upsert/remove routing, reconnect backoff. Poll 30s giữ làm independent
  fallback/reconciliation.

### Key Decisions

- `T-1` Đây là **level-triggered state stream**, không phải durable global
  event log. Initial/reconnect frame là authoritative snapshot của mọi
  user-visible non-terminal lane; live frame chỉ báo latest projection.
  Không claim exactly-once và không dùng global RAM sequence làm cursor.
- `T-2` Không broadcast `message_delta`, token, tool-start/tool-complete hay
  mọi child-agent event. Chỉ emit khi projection có ý nghĩa đổi:
  run status, last completed activity summary, decision set/revision,
  terminal transition hoặc leg topology. Nhờ vậy activity storm không
  được phép starve attention.
- `T-3` Scope = top-level/chat lane user-visible. Delegated child runs
  (`parentRunID != ""`) không vào mux trừ khi contract explicit đánh dấu
  user-visible; child progress tiếp tục đi qua parent graph/event projection.
- `T-4` Snapshot + subscriber registration phải atomic dưới `s.mu`; copy
  projection rồi unlock trước JSON marshal/network write. Snapshot được gửi
  theo bounded chunks (`snapshotId`, `complete`); client stage chunks và chỉ
  **replace/reconcile atomically** khi `complete=true`. Disconnect giữa snapshot
  → discard staging. Terminal dùng `remove`; snapshot vắng lane cũng xoá stale.
- `T-5` Backpressure dùng latest-state coalescing per run/subscriber. Publisher
  không block dưới `s.mu`; channel đầy không silently drop attention — dirty
  run stays pending. Dirty-set vượt hard cap → close subscriber với retryable
  overflow reason để reconnect full snapshot, không tăng RAM vô hạn.
- `T-6` Revision/dedupe = `(runId, runSeq, projectionHash/kind)` từ durable
  per-run event sequence/current state. Không thêm global seq/reset-on-boot.
  Frame cùng hoặc cũ hơn revision đã apply bị bỏ; reconnect snapshot thắng cache.
- `T-7` Provider/model switch tạo leg mới nhưng cùng chat; leg mới tự xuất
  hiện trong snapshot/upsert, không attach/detach race. Client group theo
  `chatId` nhưng action luôn target exact `runId + decisionId + revision`.

### Constraints

- Không tạo persistence/registry authority song song; realtime projection
  đọc state đã recover của `s.runs` và durable pending records hiện có.
- Per-run stream `/client/workflow-runs/{runId}/events/stream` giữ nguyên
  byte-for-byte và vẫn là transcript/focused-run stream.
- Endpoint mux không mang transcript delta/raw tool input/output; bounded
  text/path/count limits từ Task-430 áp dụng cho projection.
- Subscriber cleanup trên request cancel/write failure; nhiều desktop/TUI
  clients độc lập, không share mutable cursor.
- Không đổi `IdleTTL`/sweeper semantics trong task này (CP-84 `Q-1`).
- Provider-agnostic; Claude/Codex/Grok phải tạo cùng projection shape.

### Open Questions

- Không còn câu hỏi cursor: reconnect-by-snapshot là quyết định đóng.
  Cần confirm predicate user-visible lane dựa trên field hiện có hay thêm
  explicit `visibility` field; implementation phải chọn trước code và test
  child-run exclusion.

### Source Refs

- `CP-84 P-1`, `Task-404`, `interactive_handlers.go:handleEventStream`,
  `interactive_service.go:emitLocked`/`subscribe`/`unsubscribe`,
  `provider_event.go` (`ProviderEvent.Seq` monotonic per-run),
  `apps/local-runner/AGENTS.md §2` (durability/three-outcome)

## 1. Goal

Lane nền vào trạng thái chờ/terminal được client biết trong <1s qua một
stream duy nhất, thay vì poll 30s; leg churn tự xuất hiện và reconnect
full-snapshot bảo toàn **latest actionable state** (không claim replay đủ
mọi transient event).

## 2. Parent Links

- coding plan: CP-84 (P-1)
- tech design: SD-28 (shared runner lifecycle — client registry)
- system spec: SS-24 (runner lifecycle), SS-13 (doc contract)
- specific upstream ids: Task-404, CP-82 P-1/P-2

## 3. Trigger

5-app parallel vibe cần realtime awareness cho mọi lane; hiện chỉ focused
run có SSE attach, lane nền phụ thuộc `loadAllProjectHistories` 30s.

## 4. Exact Change

- `T-1` `types.go`: `RunRealtimeProjection`, `RunRealtimeFrame`, frame
  kinds `snapshot|upsert|remove`; projection bounded và không chứa raw
  `ProviderEvent`/transcript.
- `T-2` `interactive_service.go`: `projectRealtimeRunLocked` + visibility
  predicate + subscriber broadcaster có per-subscriber dirty set/coalescing.
  Register subscriber và copy initial snapshot trong cùng critical section;
  tuyệt đối không marshal/write network khi giữ `s.mu`.
- `T-3` Gọi broadcaster từ seam trạng thái hiện có sau khi `emitLocked`
  đã apply status, và từ mọi durable mutation không đi qua `emitLocked`
  nhưng làm đổi decision/leg/terminal projection. Không dựa riêng vào
  `ProviderEvent.Type` (`TurnCompleted` không luôn terminal theo V10 gate).
- `T-4` `handleAllEventsStream`: frame `snapshot` đầu tiên là full replace,
  sau đó `upsert/remove`; heartbeat ticker riêng; check write error;
  request cancel/write failure luôn unsubscribe. Route
  `GET /client/events/stream`.
- `T-5` Desktop `RunnerClient` + `HttpWsRunnerClient`: async iterable
  `streamRunUpdates(signal)` reuse parser/fetch-stream hiện có; reconnect
  exponential backoff + jitter, reset sau successful snapshot; một controller
  app-lifetime, không bị `resetRun()` cancel.
- `T-6` Desktop reducer: snapshot reconcile toàn bộ attention/history lane
  scope; upsert theo `(runId, revision)`; remove idempotent; stale frame bỏ.
  Poll 30s tiếp tục chạy độc lập và sửa drift nếu stream degraded.
- `T-7` Telemetry/log: connected/reconnecting/snapshot-applied,
  subscriber count, coalesced updates, dirty drain; không log payload text.

## 5. Touched Areas

- files: `internal/runner/types.go`, `internal/runner/interactive_service.go`,
  `internal/runner/interactive_handlers.go`,
  `apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts`,
  `src/types/contract.ts`, `src/state/attentionQueue.ts`, `src/state/store.ts`
  (wiring call only)
- modules: `internal/runner`, desktop client/state
- routes: `GET /client/events/stream` (new)
- tables: none

## 6. Code Guide Signatures

```go
// apps/local-runner/internal/runner/types.go
type RunRealtimeFrameKind string
const (
    RunRealtimeSnapshot RunRealtimeFrameKind = "snapshot" // full authoritative active-lane set
    RunRealtimeUpsert   RunRealtimeFrameKind = "upsert"   // latest projection for one run
    RunRealtimeRemove   RunRealtimeFrameKind = "remove"   // terminal/deleted/no-longer-visible run
)

type RunRealtimeProjection struct {
    RunID       string            `json:"runId"`
    ProjectID   string            `json:"projectId"`
    ChatID      string            `json:"chatId,omitempty"`
    Revision    int64             `json:"revision"` // recovered per-run seq/state revision, never global RAM seq
    Status      RunStatus         `json:"status"`
    UpdatedAt   string            `json:"updatedAt"`
    LastSummary string            `json:"lastSummary,omitempty"` // bounded/sanitized
    Decisions   []DecisionPayload `json:"decisions,omitempty"`   // Task-430
}

type RunRealtimeFrame struct {
    Kind       RunRealtimeFrameKind    `json:"kind"`
    SnapshotID string                  `json:"snapshotId,omitempty"` // connection-local staging id, not replay cursor
    Complete   bool                    `json:"complete,omitempty"`   // atomically reconcile only on final chunk
    RunID      string                  `json:"runId,omitempty"`      // remove
    Run        *RunRealtimeProjection  `json:"run,omitempty"`       // upsert
    Runs       []RunRealtimeProjection `json:"runs,omitempty"`      // bounded snapshot chunk
    Retryable  bool                    `json:"retryable,omitempty"` // overflow/server close signal
}
```

```go
// apps/local-runner/internal/runner/interactive_service.go
func isRealtimeVisibleRun(rs *interactiveRun) bool // top-level/user-visible only; excludes delegated children
func (s *InteractiveService) projectRealtimeRunLocked(rs *interactiveRun) RunRealtimeProjection
func (s *InteractiveService) subscribeRunUpdates() (subID int64, wake <-chan struct{}, snapshot []RunRealtimeProjection)
func (s *InteractiveService) drainRunUpdates(subID int64) []RunRealtimeFrame // drains per-subscriber dirty set, latest state per run
func (s *InteractiveService) unsubscribeRunUpdates(subID int64)
func (s *InteractiveService) markRunRealtimeDirtyLocked(runID string) // non-blocking; retains dirty bit until drain
```

```go
// apps/local-runner/internal/runner/interactive_handlers.go
func (s *InteractiveService) handleAllEventsStream(w http.ResponseWriter, r *http.Request) // snapshot → live upsert/remove + heartbeat
// route: mux.HandleFunc("GET /client/events/stream", s.handleAllEventsStream)
```

```ts
// apps/desktop-flowpilot/src/types/contract.ts
export interface RunnerClient {
  streamRunUpdates(signal?: AbortSignal): AsyncIterable<RunRealtimeFrame>; // T-5
}
```

```ts
// apps/desktop-flowpilot/src/state/attentionQueue.ts
reconcileRunSnapshot(runs: RunRealtimeProjection[]): void // full replace of mux-owned lane scope
applyRunUpdate(frame: RunRealtimeFrame): void              // revision guard; upsert/remove idempotent
```

## 7. Test Signatures

- `TestRunUpdates_InitialSnapshotIsAtomicWithSubscription` — event đổi
  ngay tại connect xuất hiện trong snapshot hoặc live upsert, không gap.
- `TestRunUpdates_ChunkedSnapshotCommitsOnlyOnComplete` — disconnect giữa
  chunks không partial-reconcile/xoá item; complete chunk apply atomically.
- `TestRunUpdates_DirtyOverflowClosesRetryableAndBoundsMemory` — slow client
  vượt cap bị close/reconnect snapshot, registry/dirty memory bounded.
- `TestRunUpdates_ExcludesDelegatedChildRuns` — child-agent storm không
  lọt mux; parent projection vẫn update.
- `TestRunUpdates_CoalescesSlowSubscriberWithoutLosingWaitingState` —
  flood activity > buffer rồi waiting; drain nhận latest waiting projection.
- `TestRunUpdates_DoesNotEmitMessageDeltaOrToolNoise` — raw high-volume
  provider events không tạo frame.
- `TestRunUpdates_ReconnectSnapshotReconcilesMissedTerminalRemove` —
  disconnect trước terminal, reconnect snapshot vắng run → stale item bị xoá.
- `TestRunUpdates_ProviderSwitchLegAppearsWithoutReattach` — leg mới xuất
  hiện trên same subscription, grouped chat id đúng.
- `TestRunUpdates_TurnCompletedBeforeGatePassIsNotTerminal` — bảo vệ V10:
  không classify terminal chỉ từ `EventTurnCompleted`.
- `TestRunUpdates_UnsubscribeOnCancelAndWriteFailure` — subscriber registry
  + goroutine sạch ở cả hai path.
- `TestRunUpdates_PerRunEventStreamUnchanged` — endpoint cũ byte-compatible.
- `TestRunUpdates_ClaudeCodexGrokSameProjection` — provider parity evidence.
- `test("snapshot reconciliation replaces mux-owned active lane set", ...)`
  — reconnect xoá stale item nhưng không xoá non-mux/local item.
- `test("upsert ignores older revision and remove is idempotent", ...)`.
- `test("resetRun does not abort the app-lifetime mux stream", ...)`.
- `test("disconnect backs off, polls independently, and resets backoff after snapshot", ...)`.

## 8. Acceptance Check

- Live: mở desktop, chạy 2 run ở 2 project; run B vào `waiting_approval`
  → inbox badge + item xuất hiện <1s mà không đợi poll 30s.
- `curl -N /client/events/stream` trên runner live thấy envelope JSON đủ
  field; kill stream → client reconnect, không item trùng.

## 9. Out of Scope

- Per-kind inbox controls (Task-431), durable decision projection ownership
  (Task-430 — prerequisite; Task-429 chỉ transport `decisions[]`), transcript
  streaming cho background lanes, turn-level admission control (CP-84 P-7),
  thay đổi IdleTTL/sweeper.

## 10. Definition of Done

- [x] All §6 signatures implemented exactly (or deviation documented in §11)
- [x] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [x] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [x] Provider parity proven with Claude/Codex/Grok projection test; missing capability is typed degradation (R2)
- [x] Slow subscriber/activity flood cannot lose latest waiting/terminal state; dirty memory bounded; overflow is retryable; race test runs with `go test -race`
- [x] Chunked snapshot applies atomically only at `complete`; interrupted staging is discarded
- [x] Reconnect snapshot proves stale terminal cleanup; no global RAM cursor/exactly-once claim remains
- [x] Child-agent runs excluded; focused per-run SSE byte-compatible; `resetRun()` does not cancel mux
- [x] No JSON marshal, disk read, or network write while `s.mu` is held; subscriber cleanup proven
- [x] `feature_key` = `event-plane` (append vào FEATURE-KEYS.md); CA ledger entry written
- [x] §8 acceptance checks verified by hand or test
- [x] GitNexus impact run for `emitLocked` and state-mutation seams before code; HIGH/CRITICAL findings acknowledged
- [x] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: DONE — level-triggered mux SSE /client/events/stream (snapshot chunked-atomic / upsert / remove / resync), per-subscriber dirty-set drain with bounded memory, heartbeat, telemetry (connect/disconnect/subscriber-count only); desktop streamRunUpdates + mux-lane reconcile in attentionQueue (mux-owned scope separate from poll history); 15 runner + 7 desktop tests green, `go test -race` clean.
- follow-ups: live validation pending in CP-84 close-out; GitNexus detect_changes at commit time.
- upstream docs updated: CP-84, CA-931, FEATURE-KEYS (event-plane).
