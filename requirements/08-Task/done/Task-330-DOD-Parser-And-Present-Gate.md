# Task-330: Xây dựng bộ phân tích cú pháp DOD và Cổng r-dod-present

## Metadata

- Document ID: `Task-330`
- Title: `Xây dựng bộ phân tích cú pháp DOD và Cổng r-dod-present`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-47: Cổng kiểm duyệt Definition-of-Done (r-dod)](../../07-Coding-Plan/done/CP-47-DOD-Gate.md)
- Child Documents: `None`
- Related Documents: [SS-13: Hợp đồng tài liệu cho AI](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [Task-331: Cổng r-dod-complete và Tích hợp Runner](./Task-331-DOD-Complete-Gate-And-Runner-Wiring.md)
- Replaces: `None`
- Tags: `flowgate, r-dod, r-dod-present, dod-parser, deterministic`

## AI Quick View

### Summary

- Triển khai bộ phân tích cú pháp markdown thuần Go (`dod.go`) để trích xuất trạng thái của section `## Definition of Done` trong các tài liệu `Task-*` và `BUG-*`.
- Bổ sung quy tắc **`r-dod-present`** vào `flowgate.DefaultRules()`: Khi một lượt chạy (turn) tạo mới hoặc sửa đổi tài liệu `Task-*` hoặc `BUG-*`, tài liệu đó bắt buộc phải có section `## Definition of Done` kèm theo ít nhất 1 checkbox (`- [ ]`). Nếu thiếu, runner sẽ reprompt yêu cầu AI bổ sung.
- Hoàn toàn xác định (deterministic), chạy offline với **0 chi phí token**, không gọi LLM.

### Current Ask

- Tạo file `apps/local-runner/internal/flowgate/dod.go` và `dod_test.go`.
- Thêm quy tắc `r-dod-present` vào `flowgate/rules.go` và nhánh kiểm tra trong `flowgate/evaluate.go`.

### Key Decisions

- `T-1` **Hàm phân tích cú pháp chuyên dụng `parseDefinitionOfDone(md string)`**: Quét tìm heading `## Definition of Done` (chấp nhận cả `DoD`), duyệt danh sách gạch đầu dòng cho đến heading tiếp theo, phân biệt `- [ ]` (chưa hoàn thành) và `- [x]` / `- [X]` (đã hoàn thành).
- `T-2` **Hành vi Reprompt không làm gãy luồng**: `r-dod-present` có `Action = "reprompt"`, nhắc nhở AI bổ sung checklist tiêu chí nghiệm thu mà không gây crash hay dừng cứng tiến trình.
- `T-3` **Phạm vi kích hoạt chặt chẽ**: Chỉ kiểm tra khi file nằm trong `WrittenPaths` có tiền tố `Task-` hoặc `BUG-`. Bỏ qua các file code hoặc tài liệu khác.

### Constraints

- Không làm thay đổi hành vi của các rule cũ trong `flowgate`.
- Cú pháp checkbox chấp nhận cả thụt đầu dòng (indented checkboxes).
- Section có heading nhưng không có dòng checkbox nào được tính là thiếu (`Present = false` hoặc `Total = 0`).

### Source Refs

- `requirements/07-Coding-Plan/done/CP-47-DOD-Gate.md` (Kế hoạch cha P-1, P-2).
- `apps/local-runner/internal/flowgate/rules.go` (Hệ thống đăng ký quy tắc).
- `apps/local-runner/internal/flowgate/evaluate.go` (Luồng đánh giá).

### Open Questions

- Đã giải quyết: Section DOD chỉ có text không checkbox → `Total = 0`, coi như thiếu.
- Đã giải quyết: Heading DOD chấp nhận cả `## Definition of Done` và `## DoD`.

---

## 1. Goal

Cung cấp khả năng phân tích cú pháp Definition of Done tự động trong Go và thiết lập chốt chặn `r-dod-present` để đảm bảo không có bất kỳ kế hoạch Task hay BugFix nào được tạo ra mà thiếu tiêu chí nghiệm thu cụ thể.

---

## 2. Parent Links

- Coding Plan: [CP-47 P-1 & P-2](../../07-Coding-Plan/done/CP-47-DOD-Gate.md).
- System Spec: [SS-13 §10](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md).

---

## 3. Trigger

Các flow lập kế hoạch (`task-harness`, `bug-plan-harness`, `vibe-sprint`) đôi khi sinh ra tài liệu Task chỉ có văn bản mô tả chung chung mà thiếu checklist kiểm thử, dẫn đến việc nghiệm thu sau này bị cảm tính.

---

## 4. Exact Change

- `T-1` Tạo `apps/local-runner/internal/flowgate/dod.go` với struct `DodStatus` và hàm `ParseDefinitionOfDone(md string) DodStatus`.
- `T-2` Bổ sung rule `r-dod-present` vào `DefaultRules()` trong `rules.go`.
- `T-3` Bổ sung case `task_or_bug_doc_missing_dod` trong `checkRule` tại `evaluate.go`.
- `T-4` Viết bộ unit test toàn diện `dod_test.go` và `r_dod_present_test.go`.

---

## 5. Touched Areas

- `apps/local-runner/internal/flowgate/dod.go` (Mới)
- `apps/local-runner/internal/flowgate/dod_test.go` (Mới)
- `apps/local-runner/internal/flowgate/rules.go`
- `apps/local-runner/internal/flowgate/evaluate.go`
- `apps/local-runner/internal/flowgate/r_dod_present_test.go` (Mới)

---

## 6. Acceptance Check

- Chạy `go test ./internal/flowgate/ -run TestParseDefinitionOfDone` pass 100%.
- Chạy `go test ./internal/flowgate/ -run TestRDodPresent` pass 100%.
- Khi file Task thiếu section DOD hoặc không có checkbox nào, rule `r-dod-present` kích hoạt vi phạm kèm thông báo hướng dẫn sửa.

---

## 7. Out of Scope

- Kiểm tra việc tick hoàn thành checkbox khi chuyển trạng thái done (thuộc về `Task-331`).
- Tự động tick checkbox thay cho AI hoặc con người.

---

## 8. Completion Notes

- Trạng thái: `done` (2026-09-11).
- Triển khai: `dod.go` (`DodStatus`, `ParseDefinitionOfDone`, `MissingDodDocs`), rule `r-dod-present` trong `DefaultRules()`, case `task_or_bug_doc_missing_dod` trong `checkRule`, mở rộng `DocScopeRuleIDs()`.
- Tests: 10/10 test signature §10 + 4 test bổ sung (merge, workspace rỗng, registered, MissingDodDocs) — `go test ./internal/flowgate/...` xanh toàn bộ.
- GitNexus impact: `DefaultRules` (8 impacted, LOW), `checkRule` (4 impacted, LOW).
- Ghi nhận minh bạch: test manifest cũ `TestDefaultRules` được mở rộng append-only thêm `"r-dod-present"` theo đúng convention repo (Task-233 / CP-53-Task-277 / Task-260 đều đã mở rộng manifest khi thêm rule — xem comment trong test và git history). Không test hành vi nào bị sửa.
- Review: PASS (0 blocking). Non-blocking đã ghi nhận để hardening sau: fence code block chưa được bỏ qua khi parse DOD; giới hạn 64KB/line của bufio.Scanner; chưa guard `..`/symlink traversal (đọc-only nên không exploit được); unreadable file skip im lặng (không có logging infra trong flowgate).
- Round-2 hardening (CA-840/CA-845, sau 2 vòng agent review độc lập): (1) fence-aware — DOD/checkbox trong fenced code block bị bỏ qua (chống game cổng); (2) strings.Split thay bufio.Scanner (hết giới hạn 64KB/line); (3) guard `..`/symlink trong MissingDodDocs; (4) heading DOD dạng có số h2 (`## 9. Definition of Done`) được nhận diện; (5) section DOD chỉ đóng ở heading h1/h2.
- Round-3 hardening (CA-846): round-2 chỉ phủ h2 — real-repo scan của reviewer round 3 phát hiện **62/79 doc thật dùng heading DOD dạng h3** (`### 6.1 Definition of Done (DOD)`, `### 6.2 Definition of Done`, `### Definition Of Done`) và biến thể parenthesized (`## 6. Acceptance Check (Definition of Done)`); ground-truth của test khóa round-2 cũng bias h2 (circular). Đã fix: regex chấp nhận h2+h3 với numbering đa phần + biến thể parenthesized; ground-truth của test được nới cùng mức. Kết quả khóa lại: **78/78 doc contract-form nhận diện**; 9 doc legacy (prose/backtick-marker) cố ý non-conforming (CP-47 R-1). Bài học: ground-truth của acceptance test phải được sinh độc lập với parser, không dùng chung regex.
- Provider parity: provider-agnostic (pure Go, 0 LLM, không đụng adapter nào của Claude/Codex/Grok).
- Prior CA claims giữ nguyên: CA-695, CA-442, CA-441 (flowgate gate behavior) — không đảo bacing claim nào.

---

## 9. Definition of Done

- [x] Struct `DodStatus{Present bool, Total int, Checked int, OpenItems []string}` được định nghĩa chuẩn trong `dod.go`.
- [x] Hàm `ParseDefinitionOfDone(content string)` xử lý chính xác cả chữ hoa lẫn chữ thường (`Definition of Done`, `DoD`), đếm đúng checkbox `[ ]` và `[x]`/`[X]`.
- [x] Section rỗng hoặc chỉ có text không có checkbox trả về `Present = false` hoặc `Total = 0`.
- [x] Khai báo quy tắc `r-dod-present` trong `flowgate.DefaultRules()` với `Action = "reprompt"` và `Trigger = "task_or_bug_doc_missing_dod"`.
- [x] `evaluate.go` kích hoạt vi phạm `r-dod-present` khi phát hiện file `Task-*` hoặc `BUG-*` trong `WrittenPaths` có `DodStatus.Total == 0`.
- [x] Cũ tests không bị chỉnh sửa và chạy `go test ./internal/flowgate/...` hoàn toàn xanh. (Ngoại lệ minh bạch: manifest `TestDefaultRules` mở rộng append-only theo convention repo — xem Completion Notes.)
- [x] `MergeDefaultRules` tự động bổ sung `r-dod-present` vào `flow-rules.json` của workspace cũ.

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/flowgate/dod_test.go`

```go
package flowgate

import "testing"

// Scenario: Tài liệu hợp lệ có đầy đủ checklist Definition of Done
// Input: Markdown chứa ## Definition of Done kèm 2 mục [x] và 1 mục [ ]
// Expect: Present=true, Total=3, Checked=2, len(OpenItems)=1
func TestParseDefinitionOfDone_ValidChecklist(t *testing.T) {}

// Scenario: Tiêu đề DoD viết tắt hoặc hoa thường khác nhau
// Input: Markdown chứa ## DoD với 1 mục [ ]
// Expect: Present=true, Total=1, Checked=0
func TestParseDefinitionOfDone_CaseInsensitiveHeading(t *testing.T) {}

// Scenario: Section DOD chỉ có đoạn văn mô tả, không có checkbox nào
// Input: Markdown chứa ## Definition of Done nhưng bên dưới chỉ là text
// Expect: Total=0 (coi như không hợp lệ)
func TestParseDefinitionOfDone_NoCheckboxes(t *testing.T) {}

// Scenario: Checkbox nằm thụt đầu dòng (nested/indented)
// Input: Markdown chứa danh sách checklist thụt lề 2 hoặc 4 spaces
// Expect: Đếm chính xác cả các checkbox thụt dòng
func TestParseDefinitionOfDone_IndentedCheckboxes(t *testing.T) {}

// Scenario: Không có section DOD trong tài liệu
// Input: Markdown chỉ có ## Summary và ## Goal
// Expect: Present=false, Total=0
func TestParseDefinitionOfDone_MissingSection(t *testing.T) {}
```

Tệp kiểm thử: `apps/local-runner/internal/flowgate/r_dod_present_test.go`

```go
package flowgate

import "testing"

// Scenario: Ghi file Task có DOD hợp lệ -> Gate cho qua
// Input: TurnResult có WrittenPaths=["requirements/08-Task/todo/Task-100.md"], nội dung có DOD
// Expect: Không phát sinh violation r-dod-present
func TestRDodPresent_PassWhenValid(t *testing.T) {}

// Scenario: Ghi file Task thiếu DOD -> Gate reprompt
// Input: TurnResult có WrittenPaths=["requirements/08-Task/todo/Task-100.md"], nội dung không có DOD
// Expect: Phát sinh Violation rule "r-dod-present", Action="reprompt"
func TestRDodPresent_RepromptWhenMissing(t *testing.T) {}

// [Edge] Scenario: WrittenPaths chứa file không phải Task/BUG -> Gate bỏ qua
// Input: TurnResult có WrittenPaths=["requirements/05-System-Specs/SS-01.md"], nội dung thiếu DOD
// Expect: Không phát sinh violation r-dod-present (file không thuộc phạm vi)
func TestRDodPresent_IgnoresNonTaskBugFile(t *testing.T) {}

// [Edge] Scenario: WrittenPaths rỗng -> Gate bỏ qua hoàn toàn
// Input: TurnResult có WrittenPaths=[] (chỉ sửa code)
// Expect: Không kích hoạt bất kỳ kiểm tra r-dod nào
func TestRDodPresent_EmptyWrittenPaths_Skips(t *testing.T) {}

// [Error] Scenario: File không thể đọc được (quyền truy cập / bị xóa)
// Input: WrittenPaths=["requirements/08-Task/todo/Task-999.md"] nhưng file không tồn tại
// Expect: Gate xử lý graceful, log cảnh báo và không panic
func TestRDodPresent_FileReadError_GracefulDegradation(t *testing.T) {}
```

---

## 11. Code Guide

Chữ ký và cấu trúc trong `apps/local-runner/internal/flowgate/dod.go`:

```go
package flowgate

import (
	"bufio"
	"regexp"
	"strings"
)

type DodStatus struct {
	Present   bool     `json:"present"`
	Total     int      `json:"total"`
	Checked   int      `json:"checked"`
	OpenItems []string `json:"open_items,omitempty"`
}

var (
	dodHeadingRegex  = regexp.MustCompile(`(?i)^##\s+(definition\s+of\s+done|dod)\b`)
	anyHeadingRegex  = regexp.MustCompile(`^##+\s+`)
	checkboxItemRegex = regexp.MustCompile(`^\s*-\s*\[([ xX])\]\s*(.+)$`)
)

// ParseDefinitionOfDone quét nội dung markdown để bóc tách checklist DOD.
func ParseDefinitionOfDone(content string) DodStatus {
	// 1. Dò tìm dòng tiêu đề khớp với dodHeadingRegex
	// 2. Khi đã vào section, đọc từng dòng cho đến khi gặp anyHeadingRegex kế tiếp
	// 3. Với mỗi dòng khớp checkboxItemRegex:
	//    - Tăng Total
	//    - Nếu ký tự trong ngoặc là 'x' hoặc 'X' -> Tăng Checked
	//    - Ngược lại -> Thêm label vào OpenItems
	// 4. Trả về kết quả với Present = (Total > 0)
	return DodStatus{}
}
```

Khai báo trong `apps/local-runner/internal/flowgate/rules.go`:

```go
Rule{
    ID:             "r-dod-present",
    Scope:          "step",
    Trigger:        "task_or_bug_doc_missing_dod",
    RequiredOutput: "definition_of_done_section",
    Action:         "reprompt",
    Enabled:        true,
}
```
