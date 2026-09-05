# BUG-355: TUI history picker kẹt cache cũ + workflow run mở lại mất transcript

## Metadata

- Document ID: `BUG-355`
- Title: `TUI history picker kẹt cache cũ + workflow run mở lại mất transcript`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-05`
- Last Updated: `2026-09-05`
- Feature Keys: `cli-tui, chat-history`
- Parent Documents: [CP-58-Test-Steps](../../07-Coding-Plan/inprogress/CP-58-Test-Steps.md)
- Child Documents: `none`
- Related Documents: [BUG-351](../../09-BugFix/todo/BUG-351-TUI-Flow-Picker-Tab-Ket-Cache-Cu.md) (same stale-cache pattern, `/flow` picker — fixed with background-refresh-on-open), [BUG-328](../done/BUG-328-TUI-Input-Dies-With-Zero-Key-Events-And-Cannot-Self-Recover.md) (same live session hit "Input stalled")
- Replaces: `none`
- Tags: `cli-tui, chat-history, stale-cache, history-picker, transcript-backfill, workflow-run, live-2026-09-05`

## AI Quick View

### Summary

- F1 — symptom (live 2026-09-05, TUI `just chat-dev`, runner :4317, process start 13:04): sau khi chạy 2 flow runs mới (`run-547025`, `run-548341`), picker `/history|/open|/resume` chỉ hiện list cũ tới `run-540927` (Sep 4 22:58) — 2 runs Sep-5 vắng mặt dù server trả đủ 23 runs, 2 runs mới đứng đầu.
- F1 — expected: picker luôn thấy runs mới như server. Actual: picker render từ cache lần fetch đầu, kẹt cả session.
- F2 — symptom (cùng session): `/open run-548341` (gõ tay full id, bypass picker) khôi phục steps panel đầy đủ (preflight → audit DONE) nhưng main transcript trống — chỉ còn prompt + dòng "Opened flow …", toàn bộ turn messages live trước đó biến mất.
- F2 — expected: mở lại run thấy lại transcript như cũ. Actual: steps có, messages không.
- Impact: operator không switch được sang runs mới (F1) và không review lại nội dung runs cũ (F2); evidence duy nhất còn lại là `runner.log` + diag ndjson (operator tự mò, không phải UX).

### Current Ask

- ~~F1: picker `/history|/open|/resume` lúc nào mở cũng tự tươi~~ — DONE (code + unit 2026-09-05, live-verify pending rebuild).
- ~~F2: `/open <workflow-run>` khôi phục được main transcript~~ — DONE (code + unit 2026-09-05, live-verify pending rebuild).
- Q-1 answered: option (b) — persist keyed by runId, KHÔNG cấp chatId cho workflow runs (tránh chạm chat lifecycle/sync).

### Key Decisions

- D-1: Một doc cho cả 2 findings (cùng session repro, cùng vùng open/history, cùng operator) thay vì tách 2 BUG.
- D-2: Capture trước, fix sau — phiên live CP-58 đang cần runner ổn định, không sửa code trong lúc test.
- D-3: Hướng fix F1 đi theo pattern BUG-351 đã chứng minh (silent background refetch on picker open + merge), không phát minh cơ chế mới.

### Constraints

- additive-tests-only: khi fix, chỉ thêm tests mới — zero pre-existing test edits (các runs live 547025/548341 là evidence, không được đụng).
- oracle-rule: baseline-failing suites đã biết (TestRun144900/147126 family, TestDetectProviders*, TestTryAdvance*, TestFlowCodingPrompt*) không được "fix" bằng cách sửa test.
- TUI + runner là cùng 1 process (`flowpilot chat`) — restart TUI để refresh list đồng nghĩa giết runner; fix không được dựa vào restart.

### Open Questions

- Q-1: Workflow runs có nên được cấp `chatId` ngay lúc create (để đi chung đường transcript/chatTimeline với chat runs), hay transcript phải persist keyed by `runId` + timeline endpoint fallback cho runs không chat? (Quyết định này định hình fix F2.)
- Q-2: `chat-transcripts/chats/*/transcript.ndjson` hiện không có file nào cho runs 533004/547025/548341 — là do F2 (workflow runs không chatId) hay còn đường drop khác cho chat runs? (Cần 1 probe khi fix.)
- Q-3: Picker có nên hiển thị badge "stale — refreshing…" trong lúc background refetch (như BUG-351 giữ cache cũ hiện trước) hay block nhẹ? (Khuyến nghị: theo BUG-351.)

### Source Refs

- `apps/local-runner/internal/tui/app/history.go:537-540` — chỉ fetch khi `len(m.chatList) == 0` (F1).
- `apps/local-runner/internal/tui/app/history.go:509` — `ChatListMsg{Items: filterParentHistory(items)}` (top-level runs đi qua, không phải chỗ lọc sai).
- `apps/local-runner/internal/tui/app/app.go:924-937` — `ChatListMsg` merge vào cache, print `msg.Items`.
- `apps/local-runner/internal/tui/app/helpers.go:1512` — picker filter từ `m.chatList` cache.
- `apps/local-runner/internal/runner/chat_ssot.go:194-206` — `recordChatTranscript` early-return khi `chatID == ""` (F2).
- `apps/local-runner/internal/runner/interactive_handlers.go:1033-1102` — `projectRunHistory` (server trả đủ; đã verify live 23 items).
- `apps/local-runner/internal/tui/client/client.go:876-885` — `GetChatTimeline` cần `chatID` (workflow runs không có → TUI không có gì để fetch).

## Evidence

- Live 2026-09-05, project Gate-sandbox (`db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`), runner PID 99792 start 13:04:31 (1 process liên tục, không restart).
- `GET /client/projects/<id>/workflow-runs` → 23 items, đầu list: `run-548341 completed 06:38Z`, `run-547025 completed 06:09Z`, `run-540927 cancelled`, `run-533004 completed` (UTC; +07:00 = giờ operator).
- Cùng thời điểm, picker `/open ` trong TUI hiện 12 rows #1..#12 đứng đầu là `run-540927 Sep 4 22:58` — khớp chính xác server-minus-2-runs-mới (cache từ trước khi 2 runs tồn tại).
- `run-548341` history item: `chat: None`, `kind: workflow` (cùng cho 547025/540927/533004; chỉ chat-kind runs mới có `cht_*`).
- `~/.flowpilot/chat-transcripts/chats/*/transcript.ndjson`: `rg "run-548341|run-547025|run-533004"` → zero hits (không có record nào cho 3 workflow runs).
- `GET /client/workflow-runs/run-548341` → `status: completed`; `steps-runtime` đầy đủ từng node (preflight → audit DONE) — steps restore tốt, chỉ messages mất.
- Screenshot operator: sau `/open run-548341`, panel steps đủ 12/12 DONE, main transcript chỉ còn prompt + "Opened flow Task Harness · run-548341".
- Workaround đã verify: gõ tay `/open run-548341` (full id + Enter, bypass picker) mở được run — code cho phép manual id ngoài list (`history_chat_group_test.go:202`).

## Root Cause

1. **F1 — stale switcher cache (TUI)**: `cmdMaybePrefetchHistory` (`history.go:537`) chỉ dispatch fetch khi cache rỗng; grep toàn `tui/app` không có site nào append run mới vào `m.chatList` hay invalidate nó. Cache từ lần `/history|/open|/resume` đầu tiên của session kẹt tới khi restart. Cùng họ với BUG-351 (`/flow` picker) nhưng ở picker khác, chưa được fix.
2. **F2 — workflow runs bị loại khỏi transcript persistence (runner)**: `recordChatTranscript` (`chat_ssot.go:203-206`) resolve `chatID = chatRuns.chatIDFor(runID)` và return sớm khi rỗng. Workflow-kind runs không bao giờ có `chatId` → zero records → `/open` không có nguồn backfill messages (steps đi đường `steps-runtime` nên vẫn đủ). Hệ quả phụ: `GetChatTimeline(chatID)` cũng không gọi được cho runs không chat.

## Fix (2026-09-05, done — code + unit, live-verify pending rebuild)

**F1 — history picker background refresh (mirror BUG-351):**
- `tui/app/model.go`: thêm `chatListInflight` + `chatListFetchedAt`.
- `tui/app/history.go`: `cmdMaybePrefetchHistory` — picker mở + cache non-empty → silent `cmdFetchChats(true)` khi không in-flight và hết `chatPickerRefreshInterval = 10s`; cache cũ vẫn hiện trong lúc chờ.
- `tui/app/app.go` `ChatListMsg`: luôn clear inflight; chỉ stamp fetchedAt khi thành công (lỗi giữ cache + cho retry ngay).
- Tests mới (`bug355_history_picker_refresh_test.go`, 3 tests): stale→refresh→converge 3 runs; error giữ cache + retry; interval bound.

**F2 — run-scoped transcript (Q-1 option b):**
- `runner/chat_ssot.go` `recordChatTranscript`: chat-less runs persist dưới key = runId (run ids qua `sanitizeChatID` nguyên vẹn; ChatSeq namespaced theo key; `LegIndex` scan mọi key dir).
- `runner/run_timeline.go` (mới) + route `GET /client/workflow-runs/{runId}/timeline`: 1 leg của run (resident, fallback persisted session, else 404) + records keyed by runId + `collapseRepeatedFinals` + tái dùng `backfillLegacyChatTranscript` one-shot cho runs cũ.
- `tui/client/client.go`: `GetRunTimeline`. `tui/app/chat_switch.go`: `cmdBackfillRunTimeline` riêng (BUG-338 giữ `cmdBackfillChatTimeline` nil khi không chat — test cũ untouched) + `RunScoped` flag (render giữ own-leg records; trống → note "No saved transcript…" thay vì panel câm). `tui/app/app.go`: nối vào `ChatOpenedMsg` cho workflow không chat + error text riêng.
- Tests mới: `bug355_run_timeline_test.go` (3 tests runner) + `bug355_run_timeline_backfill_test.go` (3 tests TUI).

**CP-59 multi-leg compliance:** chat path byte-for-byte unchanged (`handleChatTimeline`, leg-join, switch divider, chat-kind gate `chat_timeline.go:93-99`); workflow runs vẫn không join chat timeline; `GetChatTimeline(chatId)` không đổi. Chỉ thêm đường song song keyed by runId.

**Verification:** 9 tests mới green; suites lân cận green (`tui/app` history/chat/open/backfill/blocked; runner chat/timeline/transcript); full `tui/...` green; `go vet` + build sạch; gofmt: lines mới sạch (noise còn lại pre-existing). Full runner package stash-diff: 19 vs 18 fails — diff đối xứng đã chứng minh flaky/pre-existing (`TestFinalizerHookSurfacesArtifacts` fail isolated cả 2 cây — assertion pluralization `file(s)` không liên quan; 2 tests kia pass 3/3 isolated trên cây fix) → zero regression. Không sửa pre-existing tests.
- Live-verify (pending rebuild TUI + restart runner): start run mới → `/open` thấy ngay (F1); `/open` run cũ sau fix thấy lại messages, run trước fix hiện note (F2).
