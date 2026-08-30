# CP-59 Test Steps — Manual Validation Guide (Task-313/314/315 slices)

## Metadata

- Document ID: `CP-59-Test-Steps`
- Phase: `coding_plan` (manual validation companion)
- Status: `draft`
- Scope: những gì ĐÃ implement — Task-313 (chatId/timeline/backfill), Task-314 (switch endpoint), Task-315 slice 1+2 (TUI `/provider`, `/model`, posture Tab). **Chưa có**: Desktop (Task-316), Drive sync/restore (Task-317), reattach predicate + `/open` restore-by-chat (Task-315 slice 3).
- Created: `2026-08-30`

## 0. Chuẩn bị (bắt buộc)

| # | Việc | Cách kiểm |
|---|------|-----------|
| P1 | Chạy trên worktree `flowpilot-cp59`, branch `cp59-chat-ssot` | `git -C ../flowpilot-cp59 log --oneline -1` → commit chat-ssot |
| P2 | Bật flag trong `.env.dev` của worktree | `FLOWPILOT_CHAT_SSOT=1` (mặc định off — không set thì toàn bộ route 404 `chat_ssot_disabled`) |
| P3 | (Supabase-backed runner) Apply migration trước | `supabase/migrations/20260830080000_chat_ssot_chat_columns_and_events.sql` — chưa apply mà bật flag → lỗi ghi cột mới |
| P4 | Restart runner sau khi set env | `just chat-dev <project>` trong worktree; banner Ready |
| P5 | Posture pins chuẩn bị (repro BUG-330) | `/mode-setup plan provider grok` + `/mode-setup plan model grok-4.5`; scan/code giữ opencode |

## S. Smoke 10 phút

1. `/provider` → catalog có `grok` (ready).
2. Chat opencode: gửi "hello ban la model gi" → trả lời; status line có token usage.
3. `/provider grok` → **không còn** "Cannot change provider..." — thay bằng hệ thống chuyển leg; gửi "ban la model gi" → trả lời **như Grok** (không phải Muse Spark).
4. Gửi tiếp 1 prompt → hội thoại vẫn liền, **không mất chữ cũ** trên màn hình.
5. `/mode plan` (pin grok-4.5) → Tab hoạt động trên leg grok, không 404 `model not found` ở bất kỳ log nào.

## A. Switch qua `/model` (cross-provider)

| # | Bước | Kết quả mong đợi |
|---|---|---|
| A1 | Trên chat codex (≥3 turn), `/model opencode-go/muse-spark-1.2-contributor` | Chuyển leg sang opencode; statusline provider đổi; 1 seed envelope vào leg mới (nội dung chèn bối cảnh, KHÔNG hiện raw blob — collapse thành divider) |
| A2 | Hỏi "2 câu trước tôi hỏi gì?" | Model mới trả lời được nội dung các turn cũ (envelope raw) |
| A3 | Trong `/tmp` runner log: `handoffMode` + `includedTurnCount` | Xuất hiện đúng số turn; `chat_provider_switch` ghi đúng 1 lần (xem timeline) |
| A4 | Footer/session panel sau switch | Provider/model hiển thị = provider/model ĐÚNG của leg mới (không footer dối) |

## B. Posture Tab (BUG-330 repro chính thức)

| # | Bước | Kết quả mong đợi |
|---|---|---|
| B1 | Chat opencode, Tab/mode sang `plan` (pin grok-4.5) | Switch sang **leg grok thật**; divider system hiện; reply identity = Grok |
| B2 | Prompt tiếp theo → trả lời bằng Grok; log runner | 0 occurrences `model not found`; opencode adapter KHÔNG nhận turn model=grok-4.5 |
| B3 | Lần đầu derive pin bare-model (nếu pin chưa có provider) | 1 cảnh báo system "derived provider ... persisting"; `/mode` hiển thị pin đã có provider (persisted — không derive lại) |
| B4 | Tab về mode cùng provider (opencode→opencode) | **In-place**: không leg mới, tiếp tục cùng session như BUG-329 (CA-680) |

## C. Guards + trạng thái đặc biệt

| # | Bước | Kết quả mong đợi |
|---|---|---|
| C1 | `/provider codex` ngay khi turn đang stream | `handoff_run_busy` — dòng lỗi, chat vẫn dùng leg cũ, không leg mồ côi |
| C2 | Switch sang provider CHƯA cài (ví dụ gỡ gemini) | `provider_unavailable` + install hint; **zero mutation** (leg cũ nguyên) |
| C3 | Chat mới tạo, chưa gửi turn nào → switch ngay | `fresh_start` — leg mới không envelope, không lỗi |
| C4 | Double-action nhanh (Tab liên tiếp trong lúc switch) | Chỉ **1** leg mới (guard in-flight); message không nhân bản |

## D. Timeline / transcript (FlowPilot SSOT)

| # | Bước | Kết quả mong đợi |
|---|---|---|
| D1 | Sau switch, mở file transcript | `~/.flowpilot/chat-transcripts/chats/<chatId>/transcript.ndjson` (local) — có `turn_started`/`message_completed`/`tool_*`/`approval_requested`/`token_usage` + **1** `chat_provider_switch` |
| D2 | Kill runner giữa chat → mở lại `just chat-dev` + tạm disable capture regression | Chat cũ legacy (pre-flag) đọc timeline → backfill raw đúng 1 lần (thử đọc 2 lần, số dòng không tăng) |
| D3 | `GET /client/chats/{chatId}/timeline?afterSeq=N&limit=M` (curl) | Phân trang đúng, `truncated`/`nextSeq` đúng |
| D4 | Workflow run | `GET /client/chats/<runId-workflow>/timeline` → 404 `chat_not_found` (chat-kind gate) |

## E. Chưa test được ở giai đoạn này (đừng đánh fail)

- Desktop chip switch (Task-316), Drive sync/restore (Task-317), `/open` restore-by-chat, reattach detached chat, hybrid summary (hiện ladder `target_summary` instruction), 12-pair matrix live — thuộc manual DOD của task tương ứng khi land.

## Kết luận phiên

Ghi Pass/Fail từng mục kèm run ID. Fail B1/B2/A4 → mở bug theo quy trình (`$add-new-bug`, feature_key `chat-history`, prior CA-693..697). Mọi fail KHÔNG được sửa test cũ — sửa production code (oracle-rule).
