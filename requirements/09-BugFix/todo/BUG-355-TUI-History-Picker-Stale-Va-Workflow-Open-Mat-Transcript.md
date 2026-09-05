# BUG-355: TUI history picker kẹt cache cũ + workflow run mở lại mất transcript

## Metadata

- Document ID: `BUG-355`
- Title: `TUI history picker kẹt cache cũ + workflow run mở lại mất transcript`
- Phase: `bugfix`
- Status: `open`
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

- F1: picker `/history|/open|/resume` lúc nào mở cũng tự tươi (không cần restart TUI), vẫn hiện cache cũ trong lúc chờ fetch.
- F2: `/open <workflow-run>` khôi phục được main transcript (hoặc báo rõ "run này không có transcript persisted" thay vì panel trống gây hiểu nhầm mất data).
- Capture-only ở bước này: chưa sửa code, chưa thêm test.

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

## Fix direction (proposed, NOT implemented)

- F1 — copy pattern BUG-351 đã làm cho `/flow` picker: mỗi lần mở picker `/history|/open|/resume` dispatch silent background refetch (`cmdFetchChats(true)`), hiện cache cũ trong lúc chờ, merge khi `ChatListMsg` về; dedup in-flight + TTL nhẹ (BUG-351 dùng `flowPickerRefreshInterval = 10s`) để khỏi spam; refresh lỗi giữ cache cũ. Tests mới kiểu `bug351_flow_picker_refresh_test.go` cho history picker.
- F2 — cần quyết Q-1 trước. Hai options: (a) cấp `chatId` cho workflow runs lúc create để tái dùng toàn bộ đường chat transcript/timeline (rủi ro: chạm run lifecycle + sync); (b) persist transcript keyed by `runId` + cho timeline endpoint fallback khi không chat (rủi ro thấp hơn, nhưng TUI phải có đường fetch cho chat-less runs). Dù chọn gì, `/open` run không transcript phải báo rõ thay vì panel trống.
- Validation khi fix: unit mới (stale picker refetch + merge; workflow-run transcript record persisted + timeline trả về) + live-verify trên TUI thật (start run mới → `/open` thấy ngay; `/open` run cũ thấy lại messages).
