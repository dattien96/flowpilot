# CA-618: CA-617 follow-up I2/I3/M2/M3/M6

## What

Fix CA-617 residuals that affect `/open` and delegate Continue: persist test lock, chat first-poll, banner truncate, label-empty child.

## Why

- I2: `TestRun135037_DelegateFailPersistsAcrossReconstruct` gán tay field, không gọi `reconstructRun`/`ndjsonSessionRecord` → restart claim chưa lock (local NDJSON + Drive sync path).
- I3: `/open` run cũ `prev=[]` emit mọi `FAILED+note`; `PENDING→FAILED+note` im khi poll nhảy mất RUNNING.
- M2: `gate[:120]` cắt byte → vỡ tiếng Việt.
- M3: `reason==""` + `gate!=""` ra `"end. — gate"`.
- M6: child pre-adapter `label==""` → `reinvoke` miss → spawn planner thứ 2.
- I1 Supabase blob out of scope (local + Drive sync only).
- M4/M5 không phải bug runtime.

## Fix

- **M6** `interactive_service.go:1853`: `reinvokeMatchingFlowChild` match `label==id || (label=="" && agentName==flowNodeAgentName(node))`; sau `reinvoke` false kiểm tra `listChildren` đã có `Running` cùng label/agent thì `setFlowStepStatus(RUNNING)` và return, không `spawnChildRun`.
- **M2/M3** `tui/app/step_runtime.go:showBlockedBanner`: dùng `truncateRunes(gate,120)` (skill.go, rune-safe); gộp base `blocked` rồi ` — gate` (không `end. —`), trimSuffix.
- **I3** `formatStepChatNotices`: `len(prev)==0` cap tối đa 1 line — last `FAILED+note` không có `RUNNING/WAITING` sau; `else` late-note thêm `PENDING→FAILED+note` (old != RUNNING/WAITING/FAILED), bỏ `if !ok emit` vô hạn, giữ `RUNNING→FAILED` 1 line.
- Provider-agnostic: `reconstructRun`, `sessionRecordFrom`, `formatStepChatNotices`, `showBlockedBanner`, resume spawn không nhánh `ProviderKey` — grep xác nhận, test mới 1 representative (không loop 3 provider giả).

Will not undo: CA-616 park/inherit; CA-617 stepID fill + NDJSON fields + banned Thinking; CA-355 hub reinvoke; `step_chat_notice_test.go` unchanged.

## Tests (additive)

- Runner `run135037_delegate_fail_restart_fallback_test.go` thêm: `TestRun135037_DelegateFailRoundTripsViaReconstructRun` (reconstructRun), `TestRun135037_DelegateFailRoundTripsViaNDJSON` (sessionRecordFrom/StateFromRecord), `TestRun135037_ContinueEmptyLabelDoesNotSpawnSecondChild` (M6, label rỗng → same RunID, len 0).
- TUI `run135037_ca618_open_pending_banner_test.go` thêm: `TestCA618_FirstPollManyFailedHistoryCapsToOne` (prev rỗng nhiều fail → 1 line last), `TestCA618_PendingToFailedEmitsNote` (PENDING→FAILED), `TestCA618_FirstPollWithLaterRunningEmitsNothing` (first-poll có RUNNING sau → không dump old fail), `TestCA618_BannerTruncatesRunesNotBytes` (gate 130 "á" → có …, không \ufffd), `TestCA618_BannerReasonEmptyGatePresent` (reason rỗng gate có → có gate, không "end. —").
- Old: `go vet`, `go test ./internal/runner -run TestRun135037|TestHubStall|TestPark|TestFlow`, `go test ./internal/tui/app -run TestRun135037|TestCA618|TestFormatStepChatNotices|TestBlockedBar` pass. Không sửa test cũ.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-43
change_type: bugfix
summary: CA-617 follow-up I2/I3/M2/M3/M6 — reconstruct/NDJSON lock, /open cap, PENDING emit, banner rune truncate, empty-label no-spawn
# --->8---
