# Task-332: Bộ máy quét và tự động sửa định dạng tài liệu (Doc Conformance)

## Metadata

- Document ID: `Task-332`
- Title: `Bộ máy quét và tự động sửa định dạng tài liệu (Doc Conformance)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-48: Bộ máy kiểm định và chuẩn hóa tài liệu (SS-13)](../../07-Coding-Plan/todo/CP-48-Standardize-Doc.md)
- Child Documents: `None`
- Related Documents: [SS-13: Hợp đồng tài liệu cho AI](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [CP-49: Trích xuất tài liệu từ code](../../07-Coding-Plan/done/CP-49-Reverse-Documentation-And-Doc-Ingestion.md)
- Replaces: `None`
- Tags: `docscan, conformance, autofix, codemod, ss-13`

## AI Quick View

### Summary

- Xây dựng package Go `docscan` trong runner để phân tích và kiểm tra tính tuân thủ của các file tài liệu trong `requirements/` (`SS`, `SD`, `CP`, `Task`, `BUG`) đối chiếu với hợp đồng [SS-13](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md) và các mẫu `FORMAT-REFERENCE-*.md`.
- Cung cấp 2 khả năng cốt lõi:
  - **Scanner**: Quét cú pháp markdown, phát hiện metadata thiếu, thiếu khối `AI Quick View`, sai thứ tự section, hoặc liên kết gãy. Xuất báo cáo chi tiết.
  - **Auto-Fixer**: Bộ codemod tự động sửa cấu trúc: tự động chèn trường metadata mặc định còn thiếu (ví dụ `Feature Keys: None`), tự động sắp xếp lại vị trí các section cho đúng thứ tự chuẩn, tự động tạo khung section còn thiếu kèm cờ `TODO`.
- Tích hợp để phục vụ trực tiếp cho lệnh `/standardize [scope]`.

### Current Ask

- Tạo package `apps/local-runner/internal/docscan/` gồm `scanner.go`, `rules.go`, `autofix.go` và bộ kiểm thử `docscan_test.go`.

### Key Decisions

- `T-1` **Phân tích cú pháp AST Markdown ngoại tuyến (Offline Parser)**: Sử dụng Go thuần để bóc tách tiêu đề `#`, metadata block YAML/Markdown, khối `## AI Quick View`, và danh sách `## N. Section Name`. Không tiêu tốn token LLM.
- `T-2` **Phân loại lỗi 3 cấp độ**:
  - `Critical`: Thiếu Metadata block, tiêu đề chính sai format, sai cấu trúc nghiêm trọng.
  - `Important`: Thiếu trường bắt buộc trong Metadata (`Status`, `Parent Documents`), thiếu `AI Quick View`.
  - `Minor`: Sai thứ tự section, thiếu dấu gạch ngang phân cách, link nội bộ chưa chuẩn.
- `T-3` **Nguyên tắc Auto-Fix an toàn (Non-destructive)**: Auto-fix chỉ sắp xếp lại vị trí hoặc bổ sung trường thiếu; tuyệt đối không xóa nội dung văn bản bên trong các section.

### Constraints

- Tốc độ quét cực nhanh (< 500ms cho toàn bộ 100+ file trong repo).
- Bộ codemod phải bảo toàn nguyên vẹn encoding UTF-8 và ký tự xuống dòng.

### Source Refs

- `requirements/07-Coding-Plan/todo/CP-48-Standardize-Doc.md` (P-1, P-2).
- `requirements/05-System-Specs/SS-13-AI-Followable-Document-Contract.md`.
- `requirements/*/FORMAT-REFERENCE-*.md`.

### Open Questions

- Đã giải quyết: `Feature Keys` chỉ bắt buộc trên doc quản trị (SS/SD/CP), không bắt buộc trên Task/BUG.
- Đã giải quyết: Bộ quét hoàn toàn xác định, không có ambiguity.

---

## 1. Goal

Xây dựng công cụ kiểm định và tự động sửa lỗi định dạng tài liệu tự động bằng Go, giúp duy trì kho tài liệu kỹ thuật luôn chuẩn chỉ theo hợp đồng `SS-13` với chi phí vận hành bằng 0.

---

## 2. Parent Links

- Coding Plan: [CP-48 P-1 & P-2](../../07-Coding-Plan/todo/CP-48-Standardize-Doc.md).
- System Spec: [SS-13](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md).

---

## 3. Trigger

Mỗi khi chuẩn tài liệu được mở rộng thêm trường mới (như `Feature Keys` trong BUG-280), hàng loạt tài liệu cũ bị lỗi thời. Cần một công cụ quét và tự động sửa hàng loạt thay vì làm thủ công bằng tay.

---

## 4. Exact Change

- `T-1` Tạo package `internal/docscan/` và định nghĩa model dữ liệu vi phạm `ScanIssue{FilePath, Line, Severity, RuleID, Message, CanAutoFix}`.
- `T-2` Triển khai `ScanDocument(path string, content string) ([]ScanIssue, error)` kiểm tra đối soát với các bảng mẫu `FORMAT-REFERENCE-*.md`.
- `T-3` Triển khai `AutoFixDocument(content string, phase string) (string, error)` thực hiện căn chỉnh vị trí section và bổ sung metadata thiếu.
- `T-4` Viết bộ unit test `docscan_test.go` bao phủ các trường hợp tài liệu chuẩn và tài liệu lỗi.

---

## 5. Touched Areas

- `apps/local-runner/internal/docscan/scanner.go` (Mới)
- `apps/local-runner/internal/docscan/autofix.go` (Mới)
- `apps/local-runner/internal/docscan/rules.go` (Mới)
- `apps/local-runner/internal/docscan/docscan_test.go` (Mới)

---

## 6. Acceptance Check

- Chạy `go test ./internal/docscan/ -run TestScanDocument` pass 100%.
- Chạy `go test ./internal/docscan/ -run TestAutoFixDocument` pass 100%.
- Quét thử nghiệm một file lỗi: Trả về danh sách lỗi chính xác; sau khi chạy AutoFix, file trở nên hợp lệ và scan lại đạt 0 lỗi.

---

## 7. Out of Scope

- Phần AI đọc nội dung để điền tóm tắt ngữ nghĩa (thuộc Assisted-Fixer).
- Trích xuất tài liệu từ mã nguồn (thuộc về `Task-333`).

---

## 8. Completion Notes

- Trạng thái: `done` (2026-09-11).
- Triển khai: package `internal/docscan/` — `scanner.go` (DetectPhase, ScanDocument, ScanDirectory, parser nhận biết fenced code block), `rules.go` (9 conformance rules + bảng canonical section/metadata/AIQ theo 5 file FORMAT-REFERENCE), `autofix.go` (codemod non-destructive: chèn metadata, sắp xếp section, skeleton TODO).
- Tests: 8/8 test signature §10 + 15 test bổ sung (sync FORMAT-REFERENCE, perf 120 files ~12ms, CRLF, round-trip) — 23/23 pass, gofmt/vet sạch. Package chỉ import stdlib (offline, 0 LLM).
- Hiệu năng thực tế: quét 768 file governed của `requirements/` trong ~0.14s (< 500ms budget).
- Deviation đã duyệt trong review: (1) `missing_feature_keys` bỏ qua các file `FORMAT-REFERENCE-*` theo SS-13 §11 (writing guides, không phải business truth); (2) thêm 2 rule phụ `missing_ai_quick_view_subsection` + `missing_required_section`; (3) doc legacy tiêu đề tiếng Việt cố ý không match tên section tiếng Anh — ghi chú deferral trong code (scanner.go + rules.go), chính sách chuyển đổi thuộc Task-333; (4) `ScanDocument` trả error cho phase không nhận diện được / non-UTF-8.
- Review: PASS sau khi bổ sung comment deferral (blocking duy nhất đã fix). Non-blocking đã ghi nhận: chưa có test fence trong suite (đã verify bằng probe), shared slice trong `DefaultConformanceRules`, unnumbered heading được chuyển xuống cuối khi rebuild, off-by-one numbering được chấp nhận.
- Provider parity: provider-agnostic (pure Go offline scanner, không LLM, không adapter).
- Prior CA claims giữ nguyên: CA-695, CA-442, CA-441 — package lá mới, không có caller, không đụng shared symbol.

---

## 9. Definition of Done

- [x] Struct `ScanReport` và `ScanIssue` được định nghĩa rõ ràng với mức độ nghiêm trọng (`Critical`, `Important`, `Minor`).
- [x] `ScanDocument` kiểm tra đầy đủ các thành phần bắt buộc của `SS-13`: Metadata block, `AI Quick View` (đủ 6 mục), thứ tự section được đánh số theo từng phase.
- [x] `AutoFixDocument` có thể tự động chèn trường `Feature Keys: None` khi bị thiếu mà không làm thay đổi các trường khác.
- [x] `AutoFixDocument` sắp xếp lại đúng thứ tự các section số (`## 1. Goal`, `## 2. Parent Links`,...).
- [x] Quá trình quét và sửa không làm mất bất kỳ ký tự nội dung văn bản nào của tài liệu gốc.
- [x] Chạy `go test ./internal/docscan/...` pass 100% không cảnh báo.
- [x] `ScanDirectory` quét toàn bộ thư mục `requirements/` trong < 500ms cho 100+ file.
- [x] Tests kiểm tra đồng bộ với `FORMAT-REFERENCE-*.md` đảm bảo rules luôn khớp mẫu chuẩn.
- [x] Trường `Feature Keys` chỉ bị gắn cờ thiếu trên doc quản trị (SS/SD/CP), không trên Task/BUG.

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/docscan/docscan_test.go`

```go
package docscan

import "testing"

// Scenario: Quét tài liệu chuẩn 100%
// Input: Markdown Task hợp lệ có đầy đủ Metadata, AI Quick View, và section 1-9
// Expect: len(issues) == 0
func TestScanDocument_ValidConformingDoc(t *testing.T) {}

// Scenario: Quét tài liệu thiếu trường Feature Keys trong Metadata
// Input: Markdown hợp lệ nhưng thiếu dòng Feature Keys
// Expect: len(issues) == 1, RuleID="missing_feature_keys", CanAutoFix=true
func TestScanDocument_MissingFeatureKeys(t *testing.T) {}

// Scenario: Quét tài liệu bị đảo lộn thứ tự các section
// Input: Markdown có ## 3. Trigger đứng trước ## 1. Goal
// Expect: len(issues) >= 1, RuleID="section_out_of_order", CanAutoFix=true
func TestScanDocument_SectionOutOfOrder(t *testing.T) {}

// Scenario: AutoFix tự động bổ sung Feature Keys và sắp xếp section
// Input: Markdown lỗi từ testcase trên
// Expect: Kết quả sau AutoFix trả về markdown chuẩn, ScanDocument lại đạt 0 lỗi
func TestAutoFixDocument_FixesStructureAndMetadata(t *testing.T) {}

// [Edge] Scenario: Quét tài liệu đã hoàn toàn chuẩn -> AutoFix không thay đổi gì
// Input: Markdown chuẩn 100% chạy qua AutoFix
// Expect: Output trả về giống byte-for-byte với input
func TestAutoFixDocument_AlreadyCompliant_NoChange(t *testing.T) {}

// [Error] Scenario: Nội dung file rỗng hoàn toàn
// Input: content = ""
// Expect: Trả về ScanIssue Severity=Critical, RuleID="empty_document"
func TestScanDocument_EmptyContent_ReturnsCritical(t *testing.T) {}

// [Error] Scenario: File chứa binary / non-UTF8 content
// Input: Nội dung binary không phải markdown hợp lệ
// Expect: Trả về error rõ ràng, không corrupt dữ liệu
func TestAutoFixDocument_BinaryContent_ReturnsError(t *testing.T) {}

// [Error] Scenario: Phase không được hỗ trợ
// Input: phase = "unknown_phase"
// Expect: Trả về error rõ ràng "unsupported phase"
func TestAutoFixDocument_UnsupportedPhase_ReturnsError(t *testing.T) {}
```

---

## 11. Code Guide

Chữ ký và cấu trúc trong `apps/local-runner/internal/docscan/scanner.go`:

```go
package docscan

type Severity string

const (
	SeverityCritical  Severity = "critical"
	SeverityImportant Severity = "important"
	SeverityMinor     Severity = "minor"
)

type ScanIssue struct {
	FilePath   string   `json:"file_path"`
	Line       int      `json:"line"`
	Severity   Severity `json:"severity"`
	RuleID     string   `json:"rule_id"`
	Message    string   `json:"message"`
	CanAutoFix bool     `json:"can_auto_fix"`
}

type ScanReport struct {
	TotalFilesScanned int         `json:"total_files_scanned"`
	ConformingFiles   int         `json:"conforming_files"`
	Issues            []ScanIssue `json:"issues"`
}

// ScanDocument phân tích cấu trúc markdown đối chiếu với format chuẩn của phase.
func ScanDocument(filePath string, content string) ([]ScanIssue, error) {
	// 1. Nhận diện phase từ tên file hoặc đường dẫn (SS, SD, CP, Task, BUG)
	// 2. Phân tích AST markdown: Metadata block, AI Quick View, Danh sách section
	// 3. Đối chiếu với cấu trúc chuẩn tương ứng trong format reference
	// 4. Gom danh sách vi phạm và trả về
	return nil, nil
}

// AutoFixDocument thực hiện codemod xác định để sửa các lỗi cấu trúc.
func AutoFixDocument(content string, phase string) (string, error) {
	// 1. Phân tích các khối nội dung của tài liệu
	// 2. Bổ sung các trường metadata bị thiếu với giá trị hợp lệ
	// 3. Đưa các section về đúng thứ tự số học chuẩn
	// 4. Ghép lại tài liệu và trả về
	return content, nil
}
```

Quét toàn bộ thư mục trong `apps/local-runner/internal/docscan/scanner.go`:

```go
// ScanDirectory quét đệ quy toàn bộ thư mục requirements/ và trả về báo cáo tổng hợp.
func ScanDirectory(dirPath string) (*ScanReport, error) {
	// 1. Duyệt đệ quy tìm tất cả file .md trong dirPath
	// 2. Nhận diện phase từ đường dẫn (05-System-Specs -> ss, 06-System-Tech-Design -> sd, ...)
	// 3. Gọi ScanDocument cho từng file
	// 4. Gom kết quả vào ScanReport
	return nil, nil
}
```

Định nghĩa quy tắc trong `apps/local-runner/internal/docscan/rules.go`:

```go
package docscan

type ConformanceRule struct {
	ID          string   `json:"id"`
	Phases      []string `json:"phases"` // ["ss", "sd", "cp", "task", "bugfix"]
	Severity    Severity `json:"severity"`
	Description string   `json:"description"`
	CanAutoFix  bool     `json:"can_auto_fix"`
}

// DefaultConformanceRules trả về bảng quy tắc kiểm tra đối soát.
func DefaultConformanceRules() []ConformanceRule {
	// Trả về danh sách rules:
	// - "missing_metadata_block" (Critical, all phases)
	// - "missing_ai_quick_view" (Important, all phases)
	// - "missing_open_questions" (Minor, all phases)
	// - "section_out_of_order" (Minor, all phases, auto-fixable)
	// - "missing_feature_keys" (Important, ["ss","sd","cp"] only, auto-fixable)
	// - "missing_required_field" (Important, field-specific)
	return nil
}
```
