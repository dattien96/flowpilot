# CP-84 Test Steps — Realtime Multi-Lane Attention

- Document ID: `CP-84-Test-Steps`
- Title: `CP-84 Test Steps`
- Phase: `coding-plan`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-23`
- Last Updated: `2026-09-23`
- Parent Documents: `CP-84 (Realtime Multi-Lane Attention)`
- Child Documents: ``
- Related Documents: `Task-429, Task-430, Task-431, Task-432, Task-433, Task-434, CP-82-Test-Steps`
- Replaces: ``
- Tags: `verification, attention-queue, event-plane, multi-lane`

## AI Quick View

### Summary

- Kế hoạch verify CP-84: automated (Go + Vitest), manual desktop trên
  build thật, live HTTP trên `local_runner` thật. Trọng tâm: lane nền
  vào waiting <1s lộ inbox, inline action không switch, draft/view-cache
  sống qua navigation, modal nền không hijack.

### Current Ask

- Chạy sau khi Task-429 → Task-434 land. §2 phải xanh trước khi tick
  manual; §4/§5 cần `local_runner` + desktop build thật, **không mock**.

### Key Decisions

- Mọi scenario bắt đầu từ **state thật**: run thật park ở gate/waiting —
  không inject fake item vào store.
- Reconnect + degraded-input cases là first-class (D-10), không optional.
- Fail-closed behavior (payload thiếu → Open-only) cần 1 case riêng.

### Constraints

- Baseline suite fail khớp `main` HEAD — fail mới = STOP + report.
- Không edit test cũ để xanh; additive only.
- Live test giữ runner chạy; kill runner chỉ ở scenario được nói.

### Open Questions

- `<1s` đo tay bằng mắt + timestamp log; nếu muốn máy đo: so `At` trong
  envelope với timestamp item xuất hiện trong store — để manual trước.

### Source Refs

- `CP-84 §10 DoD`, `CP-82-Test-Steps` (format)

## 1. Goal

Chứng minh model "1 focus + lane nền realtime, triage qua inbox" hoạt
động end-to-end: event plane đủ nhanh, inbox đủ context để quyết, focus
không bị đánh cắp.

## 2. Automated Verification

```bash
cd apps/local-runner && go test ./internal/runner/ -run 'TestRunUpdates|TestDecisionPayloads' -count=1
cd apps/local-runner && go test -race ./internal/runner/ -run 'TestRunUpdates' -count=1
cd apps/desktop-flowpilot && npx vitest run src/state src/components --reporter=verbose
cd apps/desktop-flowpilot && npx vitest run   # full suite — zero failures
cd apps/local-runner && go test ./internal/runner/ -count=1   # regression rộng
```

| Nhóm | Test bắt buộc (đủ tên) | Pass criteria |
|---|---|---|
| Mux endpoint | `TestRunUpdates_InitialSnapshotIsAtomicWithSubscription`, `TestRunUpdates_ChunkedSnapshotCommitsOnlyOnComplete`, `TestRunUpdates_DirtyOverflowClosesRetryableAndBoundsMemory`, `TestRunUpdates_ExcludesDelegatedChildRuns`, `TestRunUpdates_CoalescesSlowSubscriberWithoutLosingWaitingState`, `TestRunUpdates_DoesNotEmitMessageDeltaOrToolNoise`, `TestRunUpdates_ReconnectSnapshotReconcilesMissedTerminalRemove`, `TestRunUpdates_ProviderSwitchLegAppearsWithoutReattach`, `TestRunUpdates_TurnCompletedBeforeGatePassIsNotTerminal`, `TestRunUpdates_UnsubscribeOnCancelAndWriteFailure`, `TestRunUpdates_PerRunEventStreamUnchanged`, `TestRunUpdates_ClaudeCodexGrokSameProjection` | level snapshot + coalesced latest state; child/noise excluded; focused stream unchanged |
| Decision payload | `TestDecisionPayloads_MultiplePendingRecordsAreStableAndOrdered`, `TestDecisionPayloads_RecoverSameIDsAndRevisionsAfterRestart`, `TestDecisionPayloads_GateCarriesBoundedContext`, `TestDecisionPayloads_MissingSSQuickViewIsNonActionable`, `TestDecisionPayloads_MergeUsesDurableBindingRevision`, `TestDecisionPayloads_QuestionKeepsNativeQuestionOptions`, `TestDecisionPayloads_RedactsSecretsAndCapsPayload`, `TestDecisionPayloads_ProjectorPerformsNoIOWhileLocked` | durable versioned decisions; bounded/redacted; invalid → Open-only |
| Ingest/routing | `test("snapshot reconciliation replaces mux-owned active lane set")`, `test("upsert ignores older revision and remove is idempotent")`, `test("resetRun does not abort the app-lifetime mux stream")`, `test("disconnect backs off, polls independently, and resets backoff after snapshot")` | full reconcile; stale-safe; stream app-lifetime; poll independent |
| Inbox controls | `test("gate item renders radio + custom text + Fix/Suggest/Open")`, `test("ss_lock item expand shows quickView; no project switch")`, `test("worktree_merge item shows apply_patch/keep_branch/discard")`, `test("quota item 'Switch to X' calls account-switch confirm with candidate id")`, `test("payload-absent item renders Open only")`, `test("batch approve submits eligible items only, reports partial failure")`, `test("kind filter and project filter narrow the list")`, `test("stale submit (409) shows toast, refreshes histories, keeps item")`, `test("inline submit does not change selectedProjectId/runId")` | control đúng kind; không switch focus |
| Drafts | `test("draft survives selectProject + resetRun")`, `test("send success clears only that chat's draft")`, `test("send failure keeps draft")`, `test("drafts persist to localStorage and reload on store init")`, `test("new-chat draft keyed '<projectId>:new' survives project switch")`, `test("deleting a chat prunes its draft keys")`, `test("draft map prunes oldest beyond DRAFT_CAP")`, `test("empty draft is not persisted")` | key đúng; persist; clear đúng phạm vi |
| View cache | `test("openHistoryRun restores existing snapshot then always revalidates")`, `test("LRU prunes only evictable snapshots and pins current/main ancestry")`, `test("backToMainRun fetches when pinned snapshot is unexpectedly absent")`, `test("mux revision marks snapshot dirty without deleting completed timeline")`, `test("rapid A-B-C switches ignore stale A and B revalidation responses")`, `test("stable item anchor restores after timeline height changes")`, `test("cache capture and restore do not alias mutable arrays or Sets")`, `test("delete chat/project prunes related snapshots")`, `test("revalidated timeline and paging metadata override cached values")`, `test("existing attention ingest via cacheRunSnapshot remains intact")` | extend existing `_runSnapshots`; pinned-safe; authority giữ server; bounded LRU |
| Modal routing | `test("non-focused quota event creates inbox item, modal state stays null")`, `test("focused-run quota event still opens AccountSwitchModal")`, `test("non-focused gate event lands in inbox as gate kind; gateBlock untouched")`, `test("modal-source event without resolvable runId goes to inbox, no modal")`, `test("duplicate non-focused quota event dedupes to one item")`, `test("quota item action calls same account-switch confirm path as modal")`, `test("non-focused provider-switch prompt → inbox item")` | hijack-free; focused path giữ nguyên |

## 3. Manual Test Prep

1. `local_runner` thật đang chạy (`:4317`), desktop build thật
   (`pnpm dev` / packaged).
2. Ít nhất **2 project thật** (tốt nhất 3–5 — scenario 5-app); mỗi
   project có 1 chat/run có thể park ở gate hoặc chờ approval.
3. DevTools console mở để đọc `[attention-queue]`/mux logs; terminal
   `curl` sẵn cho L-*.

## 4. Manual UI Verification (desktop app)

| ID | Steps | Expected |
|---|---|---|
| `M-1` | Focus run A ở project A; start run B ở project B, để B park ở approval/gate | Inbox badge + item xuất hiện **<1s** (không đợi 30s); toast/notification bắn |
| `M-2` | Từ lane A, expand item B trong inbox (`▸`) | Thấy decision payload (tests/quickView/conflicts tuỳ kind) **mà không rời chat A** |
| `M-3` | Inline act trên item B (Approve/option/Fix…) | Run B unblock; `selectedProjectId`/`runId` focus không đổi; composer A giữ nguyên draft đang gõ |
| `M-4` | Gõ dở draft ở A → switch sang B → quay lại A | Draft còn nguyên (text + mentions); gửi A xong chỉ draft A bị clear |
| `M-5` | Mở run C có timeline dài → switch sang A → quay lại C | Timeline + scroll hiện ngay từ cache, "updating…" settle; không trắng màn |
| `M-6` | Lane nền D hết quota (hoặc hit provider-switch prompt) | Inbox có quota item; **không** AccountSwitchModal nào pop lên trên chat A |
| `M-7` | Repeat M-6 nhưng **focused run** hết quota | `AccountSwitchModal` pop đúng như cũ |
| `M-8` | Inbox nhiều item: filter theo kind, rồi theo project; batch-approve 2+ approval/question item | Filter đúng; chỉ eligible kinds được batch; failure báo từng item |
| `M-9` | Click OS notification của lane nền | Deep-link mở đúng run đó (`openRunAtAttention`) |
| `M-10` | Thu hẹp cửa sổ / pane hẹp | Inbox + controls wrap sạch, không vỡ layout |
| `M-11` | Spectator một run nền | Spectator cập nhật nhanh (mux-driven), promote-to-focus vẫn hoạt động |

## 5. Live REAL Tests — qua HTTP, không mock

```bash
# Attach mux stream (để chạy nền trong lúc drive các request khác)
curl -N http://127.0.0.1:4317/client/events/stream
```

| ID | Steps | Expected |
|---|---|---|
| `L-1` | `POST /client/workflow-runs` cho project A rồi project B; quan sát mux stream | Envelope của cả hai run trên **1 stream**, đủ `runId/projectId/kind` |
| `L-2` | Attach mux **sau khi** B đã park waiting | Frame snapshot đầu tiên chứa B với status waiting (catch-up) |
| `L-3` | Provider/model switch trên B (leg mới, runId mới) | Event của leg mới đến trên cùng stream — client không reattach |
| `L-4` | Kill connection mux giữa chừng; để run C vào waiting trong lúc disconnect | Client reconnect + backoff; poll fallback hoặc snapshot-on-reconnect lấy lại C; không duplicate item |
| `L-5` | Trong lúc B waiting: submit exact `decisionId + revision` qua endpoint hiện có | Stream upsert/remove phản ánh state mới; run tiếp tục; stale revision trả 409 + refresh |
| `L-6` | Park một run có payload thiếu/oversized/secret-shaped | Projection giữ marker `actionable=false`, context bounded/redacted; desktop "Open" only — không guess/leak |
| `L-7` | Hoàn tất B rồi reconnect stream | Live `remove` hoặc reconnect snapshot vắng B xoá stale inbox item; completed timeline cache vẫn mở được; per-run stream format cũ |

## 6. Log & Audit Evidence

- `change-audit/` có CA entry cho từng slice (429–434) với `feature_key`
  đúng (`event-plane`, `chat-drafts` mới append; `attention-queue`,
  `project-nav`, `decision-card-ui` reuse).
- `gitnexus_detect_changes` trước commit: chỉ expected symbols (realtime
  projection/broadcaster + handler, `attentionQueue`, `drafts`, existing
  `_runSnapshots`, inbox components).
- Log desktop: mux connect/reconnect/snapshot-reconcile + revision drops —
  không payload text, không reconnect error loop.

## 7. Verification Complete When

- [ ] §2 automated xanh đủ bảng (cả Go lẫn Vitest); baseline fail
      byte-identical HEAD nếu có.
- [ ] `M-1` → `M-11` ticked bởi operator trên build thật.
- [ ] `L-1` → `L-7` ticked trên runner thật.
- [ ] D-* trong CP-84 §10 map đủ sang checklist này — mỗi D có ít nhất
      một M/L/auto proof.
- [ ] Không test cũ bị sửa; không silent fallback; payload-absent →
      Open-only được chứng minh (L-6 + auto test).
