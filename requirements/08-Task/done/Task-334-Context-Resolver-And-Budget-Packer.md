# Task-334: Xây dựng Context Resolver và Bộ đóng gói Budget Packer

## Metadata

- Document ID: `Task-334`
- Title: `Xây dựng Context Resolver và Bộ đóng gói Budget Packer`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-23: Bộ trí tuệ vận hành tích hợp](../../07-Coding-Plan/todo/CP-23-Auto-Learn-To-Skill.md)
- Child Documents: `None`
- Related Documents: [SD-10: Memory and Prompt Architecture](../../06-System-Tech-Design/SD-10-Memory-And-Prompt-Architecture.md), [Task-335: Bộ phát hiện lệch hướng Drift Detector](./Task-335-Drift-Wrong-Way-Detector-And-Correction-Ladder.md)
- Replaces: `None`
- Tags: `runtime-intelligence, budget-packer, context-resolver, deduplication, audit-prompt`

## AI Quick View

### Summary

- Hiện thực hóa **Phase 1 của CP-23**: Thay thế cơ chế cộng dồn prompt thô thiển bằng bộ máy **`Context Resolver + Budget Packer`** thông minh.
- Phân bổ ngân sách token cố định cho từng section trong prompt:
  1. Hợp đồng hệ thống & thực thi (System Contract).
  2. Yêu cầu của lượt chạy / Task hiện tại.
  3. Context bắt buộc (Canonical Head, Change Contract).
  4. Tóm tắt bộ nhớ làm việc (`artifact_memories` - thay vì chèn raw file).
  5. Đoạn trích mã nguồn thô (Raw excerpts - chỉ chèn phần cần thiết khi tóm tắt không đủ).
  6. Compact Skill Cards (thay vì chèn cả file markdown dài).
- **Lọc trùng lặp (Deduplication)**: Loại bỏ các đoạn văn bản trùng giữa lịch sử chat và context bộ nhớ.
- **Lưu vết kiểm toán (Prompt Context Audit)**: Ghi log chi tiết mỗi lượt chạy: section nào được chọn, context nào bị drop, lý do drop và số token sử dụng.

### Current Ask

- Xây dựng package `apps/local-runner/internal/promptpacker/` gồm `resolver.go`, `packer.go`, `dedup.go`, `audit.go` và bộ kiểm thử `packer_test.go`.

### Key Decisions

- `T-1` **Hạn mức token theo từng section (Token Budget Slicing)**: Đặt giới hạn token tối đa cho từng thành phần (ví dụ: System 15%, Task 25%, Memory Summary 30%, Excerpt 20%, Compact Skills 10%). Nếu một section vượt quá ngân sách, thực hiện cắt tỉa theo mức độ ưu tiên giảm dần.
- `T-2` **Ưu tiên Compact Skill Card thay cho Full Skill**: Thay vì nhúng toàn bộ file `.agents/skills/.../SKILL.md` (thường tốn 2000-4000 tokens), chỉ trích xuất phần Core Rule 3-5 dòng vào prompt, chỉ nạp full file khi có yêu cầu đặc biệt.
- `T-3` **Lưu trữ Assembled Prompt sau cùng**: Lưu toàn bộ nội dung prompt đã ghép hoàn chỉnh (sau khi đã nhúng context và skill) để phục vụ kiểm toán và chẩn đoán lỗi tại Phase 2.

### Constraints

- Không làm thay đổi kết quả đầu ra của các prompt template hiện tại.
- Thuật toán ước lượng token (token estimator) phải nhanh, sử dụng heuristic xấp xỉ (~4 ký tự = 1 token) hoặc bộ mã hóa nhẹ.

### Source Refs

- `requirements/07-Coding-Plan/todo/CP-23-Auto-Learn-To-Skill.md` (Phase 1).
- `requirements/06-System-Tech-Design/SD-10-Memory-And-Prompt-Architecture.md`.

### Open Questions

- Đã giải quyết: Token estimator sử dụng heuristic ~4 ký tự = 1 token.
- Đã giải quyết: Compact Skill Card trích xuất phần Core Rule 3-5 dòng từ SKILL.md.

---

## 1. Goal

Tối ưu hóa triệt để kích thước prompt gửi tới AI, loại bỏ tình trạng phình to context gây lãng phí token và làm loãng sự chú ý của model trong các phiên làm việc kéo dài hoặc trong Vibe Mode.

---

## 2. Parent Links

- Coding Plan: [CP-23 Phase 1](../../07-Coding-Plan/todo/CP-23-Auto-Learn-To-Skill.md).

---

## 3. Trigger

Khi chạy Vibe Mode hoặc chuỗi task liên tục, kích thước prompt tăng dần theo cấp số cộng do nhồi nhét toàn bộ file markdown và lịch sử chat cũ, dẫn đến chi phí token tăng vọt và AI dễ bị nhầm lẫn thông tin.

---

## 4. Exact Change

- `T-1` Tạo package `internal/promptpacker/` với cấu trúc `SectionBudget` và `PromptSection`.
- `T-2` Triển khai `BudgetPacker`: Gom các section theo thứ tự ưu tiên, kiểm soát ngân sách token và cắt tỉa (prune) khi chạm ngưỡng.
- `T-3` Triển khai `ContextDeduplicator`: So khớp các khối văn bản để loại bỏ các đoạn lặp lại giữa Chat history và Artifact memories.
- `T-4` Triển khai ghi vết `PromptContextAudit` lưu vào log của workflow run.

---

## 5. Touched Areas

- `apps/local-runner/internal/promptpacker/packer.go` (Mới)
- `apps/local-runner/internal/promptpacker/dedup.go` (Mới)
- `apps/local-runner/internal/promptpacker/audit.go` (Mới)
- `apps/local-runner/internal/promptpacker/packer_test.go` (Mới)
- `apps/local-runner/internal/runner/interactive_service.go` (Tích hợp bộ đóng gói trước khi gọi model)

---

## 6. Acceptance Check

- Chạy `go test ./internal/promptpacker/ -run TestBudgetPacker` pass 100%.
- Khi đưa vào tập context vượt quá ngân sách quy định (ví dụ 8,000 tokens) $\rightarrow$ Bộ đóng gói tự động cắt giảm phần raw excerpts và giữ nguyên phần core task, tổng token đầu ra không vượt quá ngân sách.
- Log `prompt_context_audit` thể hiện rõ danh sách item được giữ lại và item bị lược bỏ.

---

## 7. Out of Scope

- Logic phát hiện AI đi lạc đề (thuộc `Task-335`).
- Sinh skill tự động từ bài học (thuộc `Task-336`).

---

## 8. Completion Notes

- Trạng thái: `done` (2026-09-11).
- Triển khai: package `internal/promptpacker/` (packer.go: SectionKind/PromptSection/SectionBudget/PackerOptions/PromptAuditReport/PackPrompt/EstimateTokens; dedup.go: sliding-window fingerprint fence-aware, giữ bản Priority cao hơn; audit.go: WriteAuditLog JSONL + CompactSkillCard nhận cả `## Always Do`/`## Core Rules` với fallback 3-5 bullet, <200 tokens).
- Tích hợp: seam `applyBudgetPackerIfEnabled` trong `runTurn` (sau `injectFlowContextIfCoding` + `injectFeatureHistoryPromptCtx`, trước `logComposedPrompt`), opt-in qua env flag `FLOWPILOT_ENABLE_BUDGET_PACKER` (pattern `FLOWPILOT_DISPATCH_V2` của repo), mặc định OFF → byte-identical. Flag ON: budget mặc định 8000/1600/2400/800 qua `promptpacker.DefaultPackerOptions()`; audit JSONL ghi vào `.flowpilot/runs/<project>/<run>/prompt-context-audit-<turn>.jsonl`; lỗi pack → fallback prompt gốc.
- Demo thực tế (test log): 369,398 → 6,872 bytes, dropped_tokens=90663, giữ nguyên current task + canonical head (CP-23 R-1).
- Tests: 8/8 test signature §10 + TestDefaultPackerOptions + 4 runner integration tests (flag-off byte-identity, flag-on prune+audit, phân loại section, fence atomicity) — all pass.
- GitNexus impact: `runTurn` LOW (0 impacted).
- Non-blocking từ review (hardening sau): classifier mặc định block không nhận diện → memory_summary (cần gate theo flow-context prefix); per-kind cap áp theo section không phải aggregate; EstimateTokens đếm byte (Vietnamese/CJK bị x3 — conservative direction); nil-rs defensive nit.
- Provider parity: provider-agnostic (stdlib-only, heuristic 4 chars/token, 0 LLM; flag-OFF passthrough byte-identical cho mọi provider).
- Prior CA claims giữ nguyên: CA-833..CA-836, CA-695, CA-442, CA-441.

---

## 9. Definition of Done

- [x] Struct `SectionBudget` và `PackerOptions` được định nghĩa chuẩn, cho phép cấu hình giới hạn token theo từng section.
- [x] Hàm `PackPrompt(sections []PromptSection, budget SectionBudget)` thực hiện gom và cắt tỉa đúng theo thứ tự ưu tiên quy định trong CP-23.
- [x] Cơ chế `DeduplicateContext` loại bỏ thành công các đoạn văn bản trùng lặp.
- [x] Hỗ trợ chuyển đổi Skill dạng Markdown đầy đủ thành Compact Skill Card gọn nhẹ.
- [x] Xuất bản bản ghi `PromptContextAudit` chi tiết cho mỗi lượt đóng gói.
- [x] Chạy `go test ./internal/promptpacker/...` pass 100%.
- [x] Tích hợp `PackPrompt` vào `interactive_service.go` trước khi gọi model. (Opt-in qua `FLOWPILOT_ENABLE_BUDGET_PACKER`, mặc định OFF byte-identical — đúng constraint "không đổi kết quả prompt hiện tại".)
- [x] Compact Skill Card trích xuất chính xác phần Core Rule từ SKILL.md đầy đủ.

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/promptpacker/packer_test.go`

```go
package promptpacker

import "testing"

// Scenario: Đóng gói các section nằm trong ngân sách cho phép
// Input: 3 section với tổng 2,000 tokens, ngân sách 4,000 tokens
// Expect: Toàn bộ 3 section được giữ nguyên vẹn, 0 item bị drop
func TestBudgetPacker_WithinBudget_RetainsAll(t *testing.T) {}

// Scenario: Ngữ cảnh vượt quá ngân sách -> Cắt tỉa raw excerpts trước, bảo vệ core task
// Input: Raw excerpt chiếm 5,000 tokens, ngân sách cho phép 3,000 tokens
// Expect: Core task được bảo toàn, raw excerpt bị cắt gọn hoặc drop, tổng token <= 3,000
func TestBudgetPacker_OverBudget_PrunesLowestPriority(t *testing.T) {}

// Scenario: Lọc trùng lặp giữa chat history và memory summary
// Input: Đoạn văn bản X xuất hiện ở cả Chat History và Working Memory
// Expect: Đoạn văn bản X chỉ xuất hiện một lần trong prompt ghép cuối cùng
func TestContextDeduplicator_RemovesDuplicateText(t *testing.T) {}

// Scenario: Sinh audit record đầy đủ sau khi đóng gói
// Input: Quá trình đóng gói có 1 item bị drop do hết ngân sách
// Expect: Audit log ghi nhận DroppedCount=1 kèm lý do "exceeded_section_budget"
func TestPromptContextAudit_RecordsDroppedItems(t *testing.T) {}

// [Edge] Scenario: Danh sách section rỗng
// Input: sections=[], budget mặc định
// Expect: Prompt rỗng, audit log ghi 0 selected 0 dropped
func TestBudgetPacker_EmptySections_ReturnsEmpty(t *testing.T) {}

// [Edge] Scenario: Một section bắt buộc đơn lẻ vượt quá toàn bộ ngân sách
// Input: 1 section SectionSystemContract chiếm 10,000 tokens, ngân sách 4,000 tokens
// Expect: Section bắt buộc được giữ nguyên (không cắt), audit ghi cảnh báo vượt ngân sách
func TestBudgetPacker_MandatorySectionExceedsBudget_Retained(t *testing.T) {}

// [Error] Scenario: Ngân sách âm hoặc bằng 0
// Input: budget.TotalMaxTokens = 0
// Expect: Trả về error rõ ràng "invalid budget: total must be positive"
func TestBudgetPacker_ZeroBudget_ReturnsError(t *testing.T) {}

// [Edge] Scenario: Chuyển đổi Full Skill thành Compact Skill Card
// Input: SKILL.md đầy đủ ~3000 tokens
// Expect: Compact card chỉ còn 3-5 dòng Core Rule, token < 200
func TestCompactSkillCard_ConvertsFromFullSkill(t *testing.T) {}
```

---

## 11. Code Guide

Chữ ký và cấu trúc trong `apps/local-runner/internal/promptpacker/packer.go`:

```go
package promptpacker

type SectionKind string

const (
	SectionSystemContract SectionKind = "system_contract"
	SectionCurrentTask    SectionKind = "current_task"
	SectionMandatoryDoc   SectionKind = "mandatory_doc"
	SectionMemorySummary  SectionKind = "memory_summary"
	SectionRawExcerpt     SectionKind = "raw_excerpt"
	SectionCompactSkills  SectionKind = "compact_skills"
)

type PromptSection struct {
	Kind     SectionKind `json:"kind"`
	Title    string      `json:"title"`
	Content  string      `json:"content"`
	Priority int         `json:"priority"` // 1 (Cao nhất) đến 6 (Thấp nhất)
}

type SectionBudget struct {
	TotalMaxTokens    int `json:"total_max_tokens"`
	MaxExcerptTokens  int `json:"max_excerpt_tokens"`
	MaxMemoryTokens   int `json:"max_memory_tokens"`
	MaxSkillTokens    int `json:"max_skill_tokens"`
}

type PromptAuditReport struct {
	SelectedTokens int               `json:"selected_tokens"`
	DroppedTokens  int               `json:"dropped_tokens"`
	DroppedItems   []string          `json:"dropped_items"`
	SectionUsage   map[string]int    `json:"section_usage"`
}

// PackPrompt điều phối đóng gói các section theo hạn mức token quy định.
func PackPrompt(sections []PromptSection, budget SectionBudget) (string, PromptAuditReport, error) {
	// 1. Khử trùng lặp nội dung giữa các section (Deduplicate)
	// 2. Sắp xếp các section theo độ ưu tiên Priority
	// 3. Phân bổ và kiểm tra token cho từng section theo hạn mức SectionBudget
	// 4. Cắt tỉa (prune) các phần phụ nếu vượt quá ngân sách
	// 5. Ghi vết kiểm toán PromptAuditReport
	// 6. Ghép thành chuỗi prompt hoàn chỉnh
	return "", PromptAuditReport{}, nil
}
```

Chữ ký trong `apps/local-runner/internal/promptpacker/dedup.go`:

```go
package promptpacker

// DeduplicateContext loại bỏ các đoạn văn bản trùng lặp giữa các section.
func DeduplicateContext(sections []PromptSection) []PromptSection {
	// 1. Xây dựng bảng hash fingerprint cho mỗi đoạn văn bản (sliding window)
	// 2. Khi phát hiện đoạn trùng lặp, giữ lại bản có Priority cao hơn
	// 3. Trả về danh sách sections đã loại bỏ nội dung trùng
	return sections
}
```

Chữ ký trong `apps/local-runner/internal/promptpacker/audit.go`:

```go
package promptpacker

import "io"

// WriteAuditLog ghi bản ghi kiểm toán prompt context ra writer.
func WriteAuditLog(w io.Writer, report PromptAuditReport) error {
	// 1. Format report thành JSON hoặc human-readable text
	// 2. Ghi vào writer (thường là file log của workflow run)
	return nil
}

// CompactSkillCard trích xuất phần Core Rule từ SKILL.md đầy đủ.
func CompactSkillCard(fullSkillContent string) string {
	// 1. Phân tích SKILL.md: Tìm section ## Always Do hoặc ## Core Rules
	// 2. Trích xuất 3-5 dòng quan trọng nhất
	// 3. Trả về dạng text gọn
	return ""
}
```
