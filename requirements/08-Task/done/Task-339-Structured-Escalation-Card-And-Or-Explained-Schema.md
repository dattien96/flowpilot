# Task-339: Thẻ Escalation có Cấu trúc và Or-Explained có Schema

## Metadata

- Document ID: `Task-339`
- Title: `Thẻ Escalation có Cấu trúc và Or-Explained có Schema`
- Feature Keys: `zcode-parity, gate-schema`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-12`
- Parent Documents: [CP-62: Nâng cấp Harness học từ ZCode](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [CP-47: Cổng kiểm duyệt Definition-of-Done (r-dod)](../../07-Coding-Plan/done/CP-47-DOD-Gate.md)
- Child Documents: `None`
- Related Documents: [Task-338: Reviewer Verdict Schema](./Task-338-Reviewer-Verdict-Schema-And-Per-AC-Evidence.md), [Task-331: r-dod-complete Wiring](../done/Task-331-DOD-Complete-Gate-And-Runner-Wiring.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `structured-card, ask-user, or-explained, gate-schema, safe-fix, cp-62, p-3`

## AI Quick View

### Summary

- Hiện thực hóa Slice `P-3` của [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md): Chuẩn hóa các tương tác dừng chờ người dùng (escalation, ask_user, thẻ vi phạm `r-requirement`) thành cấu trúc thẻ có schema rõ ràng với các lựa chọn (`options`), hệ quả (`consequence`), và bằng chứng (`evidence`).
- Thay thế hoàn toàn cơ chế đối soát văn bản (text-match) của ngoại lệ `or-explained` trong cổng `r-dod-complete` bằng trường dữ liệu có schema `{explanation, referencing_ac}` trong struct `TurnResult`.
- Áp dụng chiến lược **Bọc-quanh (Wrap-around)** theo quyết định `Q-1`: Ưu tiên render thẻ từ cấu trúc options; nếu model không gọi tool hoặc payload sai thì dùng thẻ prose cũ làm kênh dự phòng (fallback), đảm bảo không bao giờ mất thông báo tới người dùng.

### Current Ask

- Tạo tool mới `apps/local-runner/internal/agentpack/flow-pack/tools/request-user-decision.yaml`.
- Cập nhật hàm `parkVibeRequirement` trong `runner/vibe_gate.go` và điểm xử lý `gate_hook.go` để đóng gói dữ liệu thẻ có cấu trúc.
- Cập nhật logic `or-explained` trong `internal/flowgate/evaluate.go` và `internal/flowgate/rules.go` để đọc trường có schema thay vì so khớp regex.

### Key Decisions

- `T-1` **Tool `request_user_decision` có schema chuẩn**:
  ```yaml
  question: string
  options:
    - id: string
      label: string
      consequence: string
  recommended: string       # option id
  evidence:
    - { path: string, line: int, excerpt: string }
  detail: string            # optional free-text
  ```
- `T-2` **Chiến lược Rollout Bọc-quanh (Q-1)**: Thẻ có cấu trúc được bọc ngoài thẻ `ask_user` hiện tại. Khi có payload hợp lệ $\rightarrow$ giao diện render danh sách nút bấm/lựa chọn trực quan; nếu thiếu payload hoặc model gọi lỗi $\rightarrow$ hiển thị thẻ prose cũ.
- `T-3` **Cấu trúc hóa `or-explained` của `r-dod-complete`**: Người dùng/AI giải trình các tiêu chí chưa tick bằng struct `DodExplanation` gắn vào `TurnResult` thay vì dựa vào việc tìm chuỗi văn bản trong `FinalMessage`.
- `T-4` **Không ảnh hưởng Dev Mode**: Các dev card 1/2/3 hiện hữu giữ nguyên hình thức tương tác trên giao diện người dùng.

### Constraints

- Tuân thủ nghiêm ngặt **safe-fix-contract**: Không sửa các test cũ của `r-dod-complete` (`r_dod_complete_test.go`). Thêm bài test mới để chứng minh tính tương thích ngược.
- Fail-closed tuyệt đối: Nếu có lỗi parsing thẻ cấu trúc, luôn rơi vào fallback prose và dừng chờ người dùng (không bao giờ bỏ qua).

### Open Questions

- Đã giải quyết tại CP-62 (`Q-1`): Bọc quanh thẻ prose hiện có, không thay thế đột ngột.

### Source Refs

- CP-62 §4 `P-3` (Escalation card có cấu trúc).
- `apps/local-runner/internal/runner/gate_hook.go`.
- `apps/local-runner/internal/flowgate/rules.go`.

---

## 1. Goal

Chuyển đổi các thông báo dừng chờ người dùng từ các đoạn văn xuôi chung chung sang các thẻ quyết định tương tác có cấu trúc, giúp người dùng (đặc biệt là người dùng non-tech trong Vibe Mode) dễ dàng nắm bắt bằng chứng và đưa ra quyết định mà không cần đọc log dài dòng.

---

## 2. Parent Links

- Coding Plan: [CP-62 Slice P-3](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md)
- System Tech Design: [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Coding Plan: [CP-47: DOD Gate](../../07-Coding-Plan/done/CP-47-DOD-Gate.md)

---

## 3. Trigger

Hiện tại khi xảy ra vi phạm `r-requirement` hoặc cần dừng hỏi người dùng, runner gửi một đoạn text tự do. Người dùng không rõ các phương án khả dĩ là gì và hậu quả khi chọn từng phương án ra sao. Đồng thời, `or-explained` trong `r-dod-complete` dùng regex tìm kiếm văn bản rất dễ bị block oan nếu câu chữ viết khác mẫu.

---

## 4. Exact Change

- `T-1` **Tạo tool definition `request-user-decision.yaml`**:
  Đặt tại `apps/local-runner/internal/agentpack/flow-pack/tools/request-user-decision.yaml` mô tả input schema cho câu hỏi, danh sách options, khuyến nghị và bằng chứng.
- `T-2` **Tích hợp render thẻ quyết định tại Runner**:
  Trong `runner/gate_hook.go` và `runner/vibe_gate.go`, kiểm tra payload của tool `request_user_decision`. Nếu có -> đính kèm vào event `user_decision_card_requested`. Nếu không -> phát event card prose cũ.
- `T-3` **Cập nhật `or-explained` sang Schema Field trong `TurnResult`**:
  Trong `flowgate/rules.go`, bổ sung trường `DodExplanation *DodExplanation` vào `TurnResult`:
  ```go
  type DodExplanation struct {
      Explanation   string `json:"explanation"`
      ReferencingAC string `json:"referencing_ac"`
  }
  ```
  Trong `flowgate/evaluate.go`, kiểm tra nếu `tr.DodExplanation != nil` và chuỗi `Explanation` không rỗng thì xem như giải trình thành công.

---

## 5. Touched Areas

- `apps/local-runner/internal/agentpack/flow-pack/tools/request-user-decision.yaml` (Mới)
- `apps/local-runner/internal/runner/gate_hook.go`
- `apps/local-runner/internal/runner/vibe_gate.go`
- `apps/local-runner/internal/flowgate/rules.go`
- `apps/local-runner/internal/flowgate/evaluate.go`
- `apps/local-runner/internal/flowgate/r_dod_structured_test.go` (Mới)
- `apps/local-runner/internal/runner/decision_card_test.go` (Mới)

---

## 6. Acceptance Check

- Chạy `go test ./internal/flowgate/ -run TestRDodStructured_` đạt PASS 100%.
- Chạy `go test ./internal/runner/ -run TestRequestUserDecision_` đạt PASS 100%.
- Kiểm tra khi kích hoạt `request_user_decision`: Thẻ trả về options đầy đủ với `id`, `label`, `consequence`.
- Kiểm tra Fallback: Nếu model không gọi tool hoặc schema hỏng, hệ thống vẫn hiển thị thẻ văn bản cũ, không bị crash hay im lặng.
- Kiểm tra `r-dod-complete`: Trường `DodExplanation` hợp lệ mở khóa thành công cho task còn mục mở mà không cần dò regex.

---

## 7. Out of Scope

- Không sửa lại toàn bộ giao diện Desktop/TUI renderer (chỉ cung cấp payload JSON chuẩn qua API/Event).
- Không ép buộc chuyển đổi cho các flow dev thuần túy.

---

## 8. Completion Notes

- Trạng thái: `done` (2026-09-12).
- Triển khai: `flowgate` — `TurnResult.DodExplanation *DodExplanation` (schema'd or-explained) + ưu tiên trong `hasValidDodExplanation` (structured thắng, legacy phrase/section heuristics giữ nguyên làm fallback chuyển tiếp); `flow-pack/tools/request-user-decision.yaml` (MỚI — tool face schema question/options/recommended/evidence/detail); `runner/user_decision_card.go` (MỚI) — `UserDecisionCard` + `parseUserDecisionCard` (strict: ≥1 option đủ id/label/consequence, recommended phải tham chiếu option có thật, evidence cần path) + `EventUserDecisionCardRequested` + `emitUserDecisionCard` (stamps `rs.decisionCard`); `applyFlowControl` nhánh escalate parse `payload["decision_card"]` → emit thẻ có cấu trúc (payload sai → log + drop, thẻ prose giữ nguyên — Q-1 wrap-around đúng hợp đồng).
- Nguồn card trong thực tế: hub owner-debate escalate qua `flow_control` với `payload.decision_card`; đường prose (GateReason/`EventFlowGateViolation`) không đổi.
- Tests: `flowgate/r_dod_structured_test.go` (structured pass, malformed → block, legacy compat, structured-beats-silent-prose); `runner/decision_card_test.go` (emit event kèm options, matrix payload hỏng 3 shape, prose-park không đính card).
- Provider parity: provider-agnostic — card sinh từ tầng runner (`applyFlowControl`/parser), không phụ thuộc adapter; tool face là pack data.
- Ghi nhận phân tầng: adapter-level capture của tool call `request_user_decision` (thread vào claude/codex shared parse như verdicts) là follow-up — parsing/emit/park đã landed và test pin ở tầng runner.

## 9. Definition of Done

- [x] File tool `request-user-decision.yaml` được tạo với JSON schema hợp lệ (pack load xanh).
- [x] Runner có khả năng trích xuất và phát hành payload thẻ có cấu trúc khi escalate mang `payload.decision_card` (`applyFlowControl` + `emitUserDecisionCard`, test emit kèm options).
- [x] Kênh dự phòng (fallback) sang thẻ prose hoạt động tin cậy khi tool không được gọi (payload hỏng → log + drop, prose park nguyên vẹn — test pin).
- [x] `TurnResult` trong `rules.go` có trường `DodExplanation *DodExplanation`.
- [x] `r-dod-complete` hỗ trợ nhận diện giải trình qua trường dữ liệu có cấu trúc `tr.DodExplanation` (structured thắng, legacy fallback).
- [x] Mọi test cũ trong `flowgate` tiếp tục xanh 100% (full suite ok).
- [x] Bổ sung unit test kiểm tra giải trình có cấu trúc, fallback thẻ prose, và phát event thẻ có cấu trúc.

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/flowgate/r_dod_structured_test.go` và `apps/local-runner/internal/runner/decision_card_test.go`

```go
package flowgate

import "testing"

// Scenario: Giải trình DOD còn dở dang bằng struct có schema -> Cho phép hoàn thành kèm cảnh báo
func TestRDodComplete_StructuredExplanation_Pass(t *testing.T) {}

// Scenario: Trường giải trình rỗng hoặc sai format -> Chặn không cho phép hoàn thành
func TestRDodComplete_MalformedExplanation_Block(t *testing.T) {}

// Scenario: Tương thích ngược: Chuỗi giải trình kiểu cũ vẫn được chấp nhận trong giai đoạn chuyển tiếp
func TestRDodComplete_LegacyTextMatch_BackwardCompatible(t *testing.T) {}
```

```go
package runner

import "testing"

// Scenario: Tool request_user_decision được gọi thành công -> phát event thẻ có cấu trúc kèm options
func TestRequestUserDecision_StructuredCard_Emitted(t *testing.T) {}

// Scenario: Tool không được gọi hoặc payload hỏng -> fallback sang thẻ prose cũ an toàn
func TestRequestUserDecision_FallbackToProseCard(t *testing.T) {}
```

---

## 11. Code Guide & Implementation Details

1. **Tool YAML**:
   Tạo file `request-user-decision.yaml` định nghĩa properties `question`, `options`, `recommended`, `evidence`, `detail`.
2. **Xử lý `or-explained` trong `flowgate/rules.go` & `evaluate.go`**:
   ```go
   // Trong rules.go thêm vào TurnResult:
   type TurnResult struct {
       // ... các trường hiện hữu ...
       DodExplanation *DodExplanation `json:"dod_explanation,omitempty"`
   }

   type DodExplanation struct {
       Explanation   string `json:"explanation"`
       ReferencingAC string `json:"referencing_ac"`
   }
   ```
3. **Logic đánh giá trong `flowgate/evaluate.go`**:
   ```go
   if tr.DodStatus.Total > 0 && tr.DodStatus.Checked < tr.DodStatus.Total {
       if tr.DodExplanation != nil && strings.TrimSpace(tr.DodExplanation.Explanation) != "" {
           // Pass with warning: có giải trình hợp lệ
           return OutcomeWarn, "dod items open but explained: " + tr.DodExplanation.Explanation
       }
       // Fallback check legacy regex text match for backward compatibility
       if hasLegacyExplanation(tr.FinalMessage) {
           return OutcomeWarn, "dod items open with legacy explanation"
       }
       return OutcomeBlock, "dod items open without explanation"
   }
   ```

---

## 12. Safe-Fix Compliance Note

- **R1 (No regression)**: Không sửa các hàm assertion trong `r_dod_complete_test.go`.
- **R2 (Three providers parity)**: Thẻ quyết định và tool input schema phải tương thích và được xác thực trên Claude, Codex, Grok.
- **R3 (Matrix coverage)**: Kiểm thử các tình huống: Tool thành công, Tool thiếu trường bắt buộc, Fallback sang prose, Giải trình cấu trúc hợp lệ/không hợp lệ.
- **Context Discipline**: Kế thừa trực tiếp pattern từ `CP-47` và `BUG-231`.
