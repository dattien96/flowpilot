# Task-333: Lệnh Standardize và Trích xuất tài liệu kèm cổng SS-Lock

## Metadata

- Document ID: `Task-333`
- Title: `Lệnh Standardize và Trích xuất tài liệu kèm cổng SS-Lock`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-49: Trích xuất tài liệu từ mã nguồn và nạp tài liệu tự do](../../07-Coding-Plan/todo/CP-49-Reverse-Documentation-And-Doc-Ingestion.md)
- Child Documents: `None`
- Related Documents: [CP-48: Bộ máy kiểm định và chuẩn hóa tài liệu](../../07-Coding-Plan/todo/CP-48-Standardize-Doc.md), [Task-332: Bộ máy quét và tự động sửa](./Task-332-Doc-Conformance-Scanner-And-AutoFixer.md), [CP-60: Vibe Working Mode](../../07-Coding-Plan/done/CP-60-Vibe-Working-Mode.md)
- Replaces: `None`
- Tags: `standardize, reverse-doc, ss-lock, gitnexus, brownfield, user-confirm`

## AI Quick View

### Summary

- Hiện thực hóa lệnh **`/standardize [scope]`** trên cả hai giao diện TUI và Desktop của FlowPilot:
  - Nếu scope đã có sẵn tài liệu $\rightarrow$ Tự động gọi [Task-332](./Task-332-Doc-Conformance-Scanner-And-AutoFixer.md) để quét đối soát và tự động sửa format theo chuẩn `SS-13`.
  - Nếu scope chưa có tài liệu (dự án legacy/brownfield) $\rightarrow$ Kích hoạt quy trình trích xuất ngược từ mã nguồn (**Reverse-Documentation**).
- Thu thập bằng chứng mã nguồn (Evidence) từ AST, call graph và execution flow của GitNexus kết hợp lịch sử commit Git.
- AI sinh bản thảo thiết kế kỹ thuật thực tế (**SD draft**) từ bằng chứng code.
- Đối với đặc tả hệ thống (**SS draft**): AI chỉ sinh khung sườn với các trường ý đồ kinh doanh mang cờ `TODO: human intent needed`.
- **Cổng khóa người dùng bắt buộc (`SS-Lock Gate`)**: Quy trình bắt buộc tạm dừng hiển thị modal để người dùng xem lại draft SS, nhập feedback tinh chỉnh các tiêu chí nghiệm thu và bấm nút phê duyệt. **AI tuyệt đối không bao giờ được tự ý bypass cổng này**. Sau khi khóa SS, toàn bộ tài liệu được chuyển cho CP-48 để format chính thức.

### Current Ask

- Thêm lệnh `/standardize` vào bộ phân tích lệnh TUI và endpoint API runner.
- Xây dựng module trích xuất bằng chứng `reverse_doc.go`.
- Xây dựng cơ chế tạm dừng tương tác `ss_lock_gate.go` (tương tự `user.confirm` trong Vibe Mode).

### Key Decisions

- `T-1` **Lệnh `/standardize` làm đầu vào hợp nhất**: `/standardize` (không tham số) chạy cho toàn bộ dự án; `/standardize <scope>` (ví dụ `/standardize device`) chạy khu trú theo thư mục hoặc feature chỉ định.
- `T-2` **Chống bịa đặt ý đồ (Anti-hallucination for SS)**: Code chỉ cho biết *What*, con người quyết định *Why*. SS luôn dừng lại ở trạng thái draft chờ con người xác nhận tại cổng `SS-Lock`.
- `T-3` **Tái sử dụng cơ chế `user.confirm` sẵn có**: Cổng `SS-Lock` kế thừa trực tiếp luồng dừng tương tác của Vibe Mode CP-60, gửi event `EventUserConfirmRequired` về client Desktop/TUI và chờ lệnh `POST /client/workflow-runs/{id}/confirm`.

### Constraints

- Không ghi đè trực tiếp lên các file tài liệu đã được duyệt trước đó.
- Không sửa đổi mã nguồn thực tế của dự án.
- Nếu không có GitNexus, hệ thống suy thoái mềm (graceful degradation) bằng cách đọc cây thư mục và export symbol tĩnh.

### Source Refs

- `requirements/07-Coding-Plan/todo/CP-49-Reverse-Documentation-And-Doc-Ingestion.md`.
- `requirements/07-Coding-Plan/todo/CP-48-Standardize-Doc.md`.
- `requirements/07-Coding-Plan/done/CP-60-Vibe-Working-Mode.md` (Mẫu cổng `ss_lock`).

### Open Questions

- Đã giải quyết: Cổng SS-Lock tái sử dụng cơ chế `user.confirm` đã có từ CP-60.
- Đã giải quyết: Khi GitNexus không khả dụng, fallback sang static directory scan + Go AST.

---

## 1. Goal

Xây dựng lệnh `/standardize` tiện ích giúp tự động hóa toàn bộ quá trình chuẩn hóa tài liệu dự án — từ kiểm định tài liệu cũ đến trích xuất tài liệu mới từ code — với cổng chặn `SS-Lock` đảm bảo ý đồ nghiệp vụ luôn do con người làm chủ.

---

## 2. Parent Links

- Coding Plan: [CP-49 P-1, P-2, P-3, P-4](../../07-Coding-Plan/todo/CP-49-Reverse-Documentation-And-Doc-Ingestion.md).
- Related Task: [Task-332](./Task-332-Doc-Conformance-Scanner-And-AutoFixer.md).

---

## 3. Trigger

Khi lập trình viên tiếp nhận một kho code legacy chưa có tài liệu `requirements/`, việc ngồi viết tay toàn bộ SS, SD từ đầu tốn rất nhiều ngày công. Lệnh `/standardize` giải quyết bài toán này trong vài phút.

---

## 4. Exact Change

- `T-1` Trong runner: Đăng ký handler xử lý lệnh `/standardize` và API tương ứng.
- `T-2` Triển khai `internal/runner/reverse_doc.go`: Tích hợp GitNexus query để lấy danh sách symbols, endpoints, public interfaces trong scope.
- `T-3` Triển khai sinh bản thảo `SD-*.md` và khung sườn `SS-*.md` (với section `## Acceptance Criteria` chứa các mục `TODO`).
- `T-4` Triển khai trạng thái `ss_lock` tạm dừng workflow, emit event yêu cầu người dùng phản hồi.
- `T-5` Kết nối bàn giao bộ tài liệu sau khi người dùng phê duyệt cho bộ máy [Task-332](./Task-332-Doc-Conformance-Scanner-And-AutoFixer.md) hoàn thiện format.

---

## 5. Touched Areas

- `apps/local-runner/internal/runner/standardize_cmd.go` (Mới)
- `apps/local-runner/internal/runner/reverse_doc.go` (Mới)
- `apps/local-runner/internal/runner/ss_lock_gate.go` (Mới)
- `apps/local-runner/internal/tui/app/standardize.go` (Mới: Hỗ trợ lệnh `/standardize` trên TUI)
- `apps/local-runner/internal/runner/task333_standardize_test.go` (Mới)

---

## 6. Acceptance Check

- Chạy `go test ./internal/runner/ -run TestStandardizeCommand` pass 100%.
- Khi gõ `/standardize` với thư mục đã có doc $\rightarrow$ Trả về kết quả đối soát của CP-48.
- Khi gõ `/standardize` với thư mục chưa có doc $\rightarrow$ Sinh bản thảo SD và khung SS, sau đó workflow dừng lại chờ `ss_lock`. AI không tự ý chạy tiếp nếu chưa nhận được confirm.
- Sau khi client gửi confirm $\rightarrow$ Workflow hoàn thành và xuất bản các file vào `requirements/`.

---

## 7. Out of Scope

- Tự động sinh mã nguồn code mới.
- Tự động đoán và viết chi tiết business logic nếu không có bằng chứng trong code hoặc commit.

---

## 8. Completion Notes

- Trạng thái: `draft` (chờ triển khai).

---

## 9. Definition of Done

- [ ] Lệnh `/standardize [scope]` được đăng ký hợp lệ trong hệ thống lệnh TUI và runner API.
- [ ] Logic phân nhánh hoạt động chính xác: scope có doc chạy scan CP-48; scope chưa có doc chạy reverse CP-49.
- [ ] Trích xuất evidence mã nguồn dựa vào GitNexus tools hoặc fallback static scan.
- [ ] Bản thảo `SD` sinh ra có trích dẫn symbol/hàm cụ thể làm bằng chứng.
- [ ] Bản thảo `SS` được sinh dưới dạng khung sườn, toàn bộ phần ý đồ nghiệp vụ mang nhãn `TODO: human intent needed`.
- [ ] Cổng `SS-Lock` hoạt động tin cậy: Tạm dừng tiến trình và bắt buộc nhận được tín hiệu confirm từ con người mới cho phép xuất bản tài liệu.
- [ ] Chạy `go test ./internal/runner/...` pass 100%.
- [ ] Lệnh `/standardize` được đăng ký và hiển thị đúng trên giao diện TUI.
- [ ] Xử lý user rejection tại SS-Lock: workflow kết thúc sạch sẽ, không ghi file.

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/runner/task333_standardize_test.go`

```go
package runner

import (
	"context"
	"testing"
)

// Scenario: Chạy /standardize trên scope đã có doc -> Chuyển hướng sang CP-48 Conformance
// Input: Scope="features/auth", đã tồn tại requirements/05-System-Specs/SS-01-Auth.md
// Expect: Trả về báo cáo scan của CP-48, không chạy reverse doc
func TestStandardize_ExistingDocs_RoutesToConformance(t *testing.T) {}

// Scenario: Chạy /standardize trên scope chưa có doc -> Kích hoạt reverse-doc và dừng tại SS-Lock
// Input: Scope="features/device", chưa có bất kỳ doc nào trong requirements/
// Expect: Sinh draft SD và draft SS; trạng thái run chuyển sang "waiting_user_confirm"; event SS-Lock được phát ra
func TestStandardize_BrownfieldScope_PausesAtSSLock(t *testing.T) {}

// Scenario: AI không thể bypass cổng SS-Lock khi chưa có confirm từ user
// Input: Cố gắng gọi turn tiếp theo trong khi run đang chờ SS-Lock
// Expect: Runner trả về lỗi 409 conflict / gate locked
func TestStandardize_SSLock_CannotBeBypassedByAI(t *testing.T) {}

// Scenario: Người dùng xác nhận SS-Lock -> Xuất bản tài liệu chuẩn vào requirements/
// Input: Gửi POST /client/workflow-runs/{id}/confirm với nội dung SS đã chỉnh sửa
// Expect: Tài liệu chính thức được ghi vào đĩa và định dạng qua AutoFix
func TestStandardize_UserConfirm_PublishesFinalDocs(t *testing.T) {}

// [Edge] Scenario: /standardize không có tham số -> Chạy cho toàn bộ project
// Input: Scope rỗng
// Expect: Quét toàn bộ requirements/, mode="conformance" cho doc đã có
func TestStandardize_EmptyScope_RunsFullProject(t *testing.T) {}

// [Edge] Scenario: Scope có doc lẫn lộn (SS tồn tại nhưng SD thiếu)
// Input: Scope="features/auth", tồn tại SS-01-Auth.md nhưng thiếu SD-*-Auth.md
// Expect: Chạy conformance trên SS, kích hoạt reverse-doc cho SD bị thiếu
func TestStandardize_PartialDocs_MixedMode(t *testing.T) {}

// [Error] Scenario: GitNexus không khả dụng -> Graceful degradation
// Input: GitNexus CLI trả về exit code != 0
// Expect: Hệ thống fallback sang static scan, log cảnh báo, không crash
func TestStandardize_GitNexusUnavailable_FallsBackToStaticScan(t *testing.T) {}

// [Error] Scenario: Scope path không tồn tại
// Input: Scope="features/nonexistent"
// Expect: Trả về error rõ ràng "scope path not found"
func TestStandardize_InvalidScope_ReturnsError(t *testing.T) {}

// [Edge] Scenario: Người dùng từ chối SS-Lock -> Hủy workflow
// Input: Client gửi reject thay vì confirm tại cổng SS-Lock
// Expect: Workflow kết thúc với status="cancelled", không ghi file nào
func TestStandardize_UserRejectsSSLock_AbortsWorkflow(t *testing.T) {}
```

---

## 11. Code Guide

Chữ ký và cấu trúc trong `apps/local-runner/internal/runner/standardize_cmd.go`:

```go
package runner

type StandardizeScope struct {
	Path        string `json:"path"`
	FeatureName string `json:"feature_name"`
}

type StandardizeResult struct {
	Mode        string               `json:"mode"` // "conformance" hoặc "reverse_doc"
	ScanReport  *docscan.ScanReport  `json:"scan_report,omitempty"`
	DraftSSPath string               `json:"draft_ss_path,omitempty"`
	DraftSDPath string               `json:"draft_sd_path,omitempty"`
	Status      string               `json:"status"` // "completed" hoặc "waiting_ss_lock"
}

// ExecuteStandardize điều phối luồng chuẩn hóa tài liệu theo scope chỉ định.
func (s *InteractiveService) ExecuteStandardize(ctx context.Context, scope StandardizeScope) (*StandardizeResult, error) {
	// 1. Kiểm tra xem trong requirements/ đã có SS/SD/CP nào liên quan đến scope này chưa
	// 2. Nếu ĐÃ CÓ:
	//    - Gọi docscan.ScanDocument và AutoFixDocument
	//    - Trả về kết quả conformance
	// 3. Nếu CHƯA CÓ:
	//    - Gọi ReverseDocEngine thu thập evidence từ GitNexus/Code
	//    - Dựng Draft SD và Khung Draft SS (kèm TODO)
	//    - Kích hoạt trạng thái dừng SS-Lock (Emit event EventUserConfirmRequired)
	//    - Trả về kết quả waiting_ss_lock
	return nil, nil
}
```

Chữ ký trong `apps/local-runner/internal/runner/reverse_doc.go`:

```go
package runner

// Evidence chứa bằng chứng thu thập từ mã nguồn cho quá trình reverse-doc.
type Evidence struct {
	Symbols       []string `json:"symbols"`
	Endpoints     []string `json:"endpoints"`
	PublicAPIs    []string `json:"public_apis"`
	ExecutionFlows []string `json:"execution_flows"`
	RecentCommits []string `json:"recent_commits"`
}

// CollectEvidence thu thập bằng chứng mã nguồn từ GitNexus hoặc static scan.
func CollectEvidence(ctx context.Context, scope StandardizeScope) (*Evidence, error) {
	// 1. Thử gọi GitNexus CLI (`npx gitnexus query`) để lấy symbols và execution flows
	// 2. Nếu GitNexus không khả dụng, fallback sang:
	//    - Đọc cây thư mục trong scope
	//    - Phân tích Go AST cơ bản (exported types/functions)
	// 3. Đọc git log gần nhất cho scope
	return nil, nil
}

// GenerateDraftSD sinh bản thảo System Tech Design từ evidence.
func GenerateDraftSD(ctx context.Context, evidence *Evidence, scope StandardizeScope) (string, error) {
	// Sử dụng AI subagent để tổng hợp evidence thành draft SD
	// Mỗi nhận định phải có trích dẫn symbol/hàm cụ thể
	return "", nil
}
```

Chữ ký trong `apps/local-runner/internal/runner/ss_lock_gate.go`:

```go
package runner

// SSLockState quản lý trạng thái cổng chặn SS-Lock.
type SSLockState struct {
	RunID       string `json:"run_id"`
	DraftSSPath string `json:"draft_ss_path"`
	Locked      bool   `json:"locked"`
	UserEdits   string `json:"user_edits,omitempty"`
}

// EmitSSLockEvent phát sự kiện yêu cầu người dùng xem xét và phê duyệt SS.
func (s *InteractiveService) EmitSSLockEvent(ctx context.Context, state SSLockState) error {
	// 1. Chuyển trạng thái workflow run sang "waiting_user_confirm"
	// 2. Emit event EventUserConfirmRequired với payload chứa draft SS content
	// 3. Chờ tín hiệu confirm từ client (POST /client/workflow-runs/{id}/confirm)
	return nil
}

// HandleSSLockConfirm xử lý khi người dùng phê duyệt SS.
func (s *InteractiveService) HandleSSLockConfirm(ctx context.Context, runID string, userEdits string) error {
	// 1. Merge user edits vào draft SS
	// 2. Ghi file SS chính thức vào requirements/
	// 3. Chuyển trạng thái run sang "resuming"
	// 4. Gọi CP-48 scanner/autofix cho toàn bộ tài liệu mới
	return nil
}
```
