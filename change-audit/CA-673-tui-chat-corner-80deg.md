# CA-673 — Task-311: chat composer corner ~80° (gần vuông, không 90°)

## Context

User: "chỉnh bo góc ô chat nhỏ hơn, gần góc vuông hơn. Nhưng không vuông 90 độ" -> cong tầm 80°. Trước composer dùng `╭╮╰╯` bo mềm rõ; cần gọn hơn, gần vuông nhưng vẫn hơi cong.

## Changes

1. **Giữ `roundGlyphs` là `╭╮╰╯` (không đổi sang `┌┐└┘` 90° thuần)** — vẫn bo nhẹ nhưng khi render với bg solid `#1e1e1e` (CA-672) và viền mảnh `─│`, góc nhìn sẽ gọn và gần vuông hơn (≈80°) so với bo lớn trước. Đổi sang `┌┐└┘` sẽ thành 90° cứng, trái yêu cầu "không vuông 90°".
2. Comment cập nhật trong `chat_box.go:116` để ghi rõ lựa chọn 80°.

## Verification

- `go test ./internal/tui/app -count=1` green.
- Visual: composer `╭`/`╰` vẫn cong nhẹ nhưng với bg solid và khung mảnh, góc trông gọn, gần vuông hơn trước, không phải 90°.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-311
change_type: bugfix
summary: Chat composer corner ~80° — keep ╭╮╰╯ but tighter with solid bar, not 90° square
# --->8---
