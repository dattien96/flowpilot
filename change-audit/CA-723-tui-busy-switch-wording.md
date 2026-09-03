# CA-723 — TUI busy-switch wording: turn-in-progress ≠ question/approval (C1 hit)

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: BUG-347
change_type: bugfix
summary: split the busy-switch notice in routeProviderSwitch/routePostureSwitch — a live turn now says "A turn is in progress — wait for it to finish, then switch provider/model" instead of the misleading question/approval message; footer provider/model never changes on a blocked switch
# --->8---

## Problem

- Operator C1 test (`run-505761`, `cht_0f6464859ab2`): `/provider`/`/model`
  trong lúc turn đang stream. Transcript sạch (0 switch record, 0 leg mới) —
  runner không mint leg. Nhưng message busy hiện "Cannot switch
  provider/model while a question or approval is pending" dù không có
  question/approval nào — cùng class BUG-347 đã fix ở `chat_posture.go` nhưng
  chưa fix 2 site trong `chat_switch.go`.
- Lưu ý operator: `/model <model cùng provider>` trong lúc stream là **in-place
  theo thiết kế** (footer đổi, không lỗi, áp turn sau) — không phải bug. C1
  phải test bằng `/provider <foreign>`.

## What changed

- `chat_switch.go`:
  - Helper `busySwitchNotice()`: nếu có question/approval → message cũ; chỉ có
    turn đang chạy (`turnLive`/`turnStream`/`turnSendPending`) → "A turn is in
    progress — wait for it to finish, then switch provider/model".
  - `routeProviderSwitch` + `routePostureSwitch` dùng helper.
- `chat_posture.go`: nhánh non-in-flight của busy-check chuyển sang dùng
  helper (thống nhất wording, không dupl code).
- Footer provider/model không bao giờ đổi trên switch bị chặn (guard trả
  notice cmd trước mọi mutation).

## R1 — old-suite regression evidence

- Operator-approved update: `TestBug342_TabBusyNotice` (turnLive=true, không
  question) assert message mới "A turn is in progress".
- `TestTabPendingWhileInFlightDoesNotApplyInPlace`,
  `TestDoubleTabWhileInFlightOnlyOneLeg`, `TestTabCrossProviderRoutesToSwitch`,
  `TestProviderAndModelCommandsOnLiveChat` PASS nguyên vẹn.
- Full `go test ./internal/tui/... -count=1` → ok (app 7.4s).

## R2 — provider parity

- Message + guard là UI layer thuần, không branch theo provider.

## R3 — new coverage (thêm vào `bug347_seed_turn_suppress_test.go`)

- `TestBusySwitchDuringLiveTurn_MessageIsTurnNotQuestionAndFooterStays` —
  `routeProviderSwitch("grok", …)` với `turnLive=true`: notice detached,
  message "A turn is in progress" (không chứa "question or approval"),
  footer provider/model giữ nguyên.

## Honest gaps

- `run-505761` là test `/model` (hoặc `/provider`) không xác định được lệnh
  chính xác operator gõ; transcript 0 switch → mọi path đều không mint leg.
  C1 cần re-run đúng `/provider grok` để xác nhận message mới hiện.

## Prior CA not undone

- CA-722 (seed suppress + sync divider) intact.
- BUG-347 busy split ở `chat_posture.go` (in-flight switch) intact — giờ 3 site
  thống nhất qua `busySwitchNotice`.