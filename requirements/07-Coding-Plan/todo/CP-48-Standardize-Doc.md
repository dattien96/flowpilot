# CP-48: Bộ máy kiểm định và chuẩn hóa tài liệu theo hợp đồng giai đoạn (SS-13)

## Metadata

- Document ID: `CP-48`
- Title: `Bộ máy kiểm định và chuẩn hóa tài liệu theo hợp đồng giai đoạn (SS-13)`
- Feature Keys: `context-regression-engine, doc-standardization`
- Phase: `coding_plan`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-07-13`
- Last Updated: `2026-09-11`
- Parent Documents: [SS-13: Hợp đồng tài liệu cho AI](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Child Documents: [Task-332: Bộ máy quét và tự động sửa định dạng tài liệu](../../08-Task/todo/Task-332-Doc-Conformance-Scanner-And-AutoFixer.md)
- Related Documents: [CP-49: Trích xuất tài liệu từ code và nạp tài liệu tự do](./CP-49-Reverse-Documentation-And-Doc-Ingestion.md), [BUG-280](../../09-BugFix/done/BUG-280-Features-Never-Link-To-Governing-Docs-So-Canonical-Head-Stays-Spec-Less.md), `reqscaffold` package, skills `phase-document-authoring`, `phase-document-compliance`
- Replaces: `None`
- Tags: `context-regression-engine, doc-standardization, reqscaffold, phase-contract, conformance`

## AI Quick View

### Summary

- Cung cấp cho FlowPilot một **bộ máy kiểm định tính tuân thủ (Conformance Engine)** có thể lặp lại cho toàn bộ tài liệu yêu cầu: quét mọi file SS/SD/CP/Task/BugFix đối chiếu với hợp đồng [SS-13](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), báo cáo các điểm vi phạm chuẩn và sửa chữa chúng.
- Tích hợp trực tiếp vào lệnh chung **`/standardize [scope]`** (hoặc nút bấm trên giao diện Desktop/TUI):
  - Khi người dùng muốn chuẩn hóa một feature hoặc toàn bộ project, nếu tài liệu liên quan đã tồn tại, runner sẽ tự động kích hoạt bộ máy kiểm định CP-48.
- Cơ chế sửa chữa 3 tầng rõ rệt:
  - **Auto-Fix (Tự động)**: Codemod Go xác định (deterministic), sửa các trường cấu trúc, header, bảng metadata skeleton mà không cần gọi LLM (0 token cost).
  - **Assisted-Fix (AI hỗ trợ)**: Dùng AI subagent điền các nội dung ngữ nghĩa (Summary, Traces, Intent) nhưng bắt buộc xuất ra Diff để con người duyệt trước khi ghi đè.
  - **Manual-Flag (Cảnh báo thủ công)**: Gắn cờ các trường mâu thuẫn lớn để con người tự quyết.
- Đảm bảo khi tiêu chuẩn hợp đồng tài liệu phát triển thêm quy tắc mới (ví dụ như trường `Feature Keys`), việc cập nhật toàn bộ kho tài liệu sẽ được tự động hóa thay vì chỉnh sửa thủ công bằng tay.

### Current Ask

- Xây dựng **Bộ quét và báo cáo tuân thủ (Scanner & Conformance Report)** (P-1) và **Bộ tự động sửa cấu trúc (Auto-Fixer)** (P-2) bằng Go, kết nối vào luồng xử lý của lệnh `/standardize` qua `Task-332`.

### Key Decisions

- `P-1` **Bộ quy tắc mã hóa trực tiếp trong Go**: Nhanh, chạy offline, hoàn toàn xác định; kiểm thử tự động đảm bảo đồng bộ với các file mẫu `FORMAT-REFERENCE-*.md`. `SS-13` đóng vai trò là đặc tả tài liệu nguồn.
- `P-2` **Phân lớp sửa đổi rõ ràng {auto, assisted, manual}**: Chỉ tầng `auto` mới được phép ghi trực tiếp; tầng `assisted` luôn bắt buộc hiển thị Diff cho con người duyệt (tuân thủ nguyên tắc an toàn human-in-the-loop).
- `P-3` **Tái sử dụng tài nguyên hiện có**: Sử dụng `SS-13` và `FORMAT-REFERENCE-*` làm hợp đồng chuẩn, package `reqscaffold` cho cấu trúc khung, và skill `phase-document-compliance` cho tầng đánh giá ngữ nghĩa.

### Constraints

- Không phá hủy dữ liệu: Tuyệt đối không xóa hay ghi đè mất nội dung tài liệu hiện có; chỉ chuẩn hóa cấu trúc và bổ sung metadata còn thiếu.
- Bộ quét phải chạy offline với chi phí 0 LLM token.
- Không làm suy yếu khả năng tạo khung tài liệu mới của `reqscaffold`.

### Source Refs

- `requirements/05-System-Specs/SS-13-AI-Followable-Document-Contract.md` (Đặc tả hợp đồng cấu trúc chuẩn).
- `requirements/*/FORMAT-REFERENCE-*.md` (Thứ tự các section theo từng giai đoạn).
- `.agents/skills/phase-document-compliance/SKILL.md` (Các cấp độ vi phạm: Critical/Important/Minor).

### Open Questions

- Đã giải quyết: Bộ quét sử dụng quy tắc Go cố định, không có ambiguity.
- Mở: `P-3` (Assisted-Fixer tầng AI) chưa được gán vào Task cụ thể, sẽ triển khai ở CP riêng sau.

---

## 1. Mục tiêu

Chuyển đổi bản hợp đồng tài liệu dạng văn bản (`SS-13` + `FORMAT-REFERENCE-*`) thành một **bộ máy kiểm định và chuẩn hóa tự động có thể thực thi lặp lại**, giúp quét và sửa chữa các tài liệu yêu cầu trong dự án đích — đảm bảo khi tiêu chuẩn mở rộng, việc đưa toàn bộ tài liệu về đúng chuẩn chỉ là một lệnh đơn giản, không tốn công sức thủ công.

---

## 2. Tài liệu đầu vào

- Hợp đồng tài liệu: [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md).
- Định dạng tham chiếu: `FORMAT-REFERENCE-{SS,SD,CP,TASK,BUGFIX}.md`.
- Trường hợp thực tế truyền cảm hứng: `BUG-280` (Quy tắc bổ sung `Feature Keys` phải điền thủ công bằng tay từng file).

---

## 3. Chiến lược triển khai

- **Cách tiếp cận tổng thể**:
  1. Xây dựng package `docscan` trong runner: Phân tích cú pháp markdown thành AST cấu trúc (tiêu đề H1, Metadata block, AI Quick View, danh sách section được đánh số).
  2. Xây dựng bảng quy tắc kiểm tra đối soát từng loại tài liệu theo giai đoạn (`phase`).
  3. Xây dựng bộ tự động sửa (`autofix`): Sửa thứ tự section, bổ sung các trường metadata bị thiếu với giá trị mặc định/placeholder hợp lệ, chuẩn hóa ký tự phân cách.
  4. Tích hợp lệnh giao diện `/standardize`: Khi gọi lệnh cho scope đã có sẵn doc, kích hoạt bộ quét và hiển thị báo cáo kèm tùy chọn tự động sửa.

---

## 4. Phân chia công việc (Work Breakdown & Task Mapping)

- `P-1` **Bộ quét cú pháp & Báo cáo đối soát (Scanner)** $\rightarrow$ Nằm trong `Task-332`:
  - Đọc toàn bộ thư mục `requirements/**` (hoặc theo scope đường dẫn người dùng chỉ định).
  - Kiểm tra tính hợp lệ: Bảng Metadata (đầy đủ các trường bắt buộc theo `SS-13`), khối `AI Quick View` (đủ 6 mục con), thứ tự section từ 1 đến N.
  - Xuất báo cáo vi phạm phân cấp: Lỗi cấu trúc (`Critical`), Thiếu trường dữ liệu (`Important`), Lỗi format nhỏ (`Minor`).
- `P-2` **Bộ tự động sửa đổi định dạng (Auto-Fixer)** $\rightarrow$ Nằm trong `Task-332`:
  - Thực thi các codemod Go xác định: Bổ sung trường `Feature Keys: None` nếu chưa có, sắp xếp lại vị trí section cho đúng thứ tự chuẩn của phase, bổ sung section còn thiếu dưới dạng khung rỗng kèm `TODO`.
- `P-3` **Tầng sửa đổi có trợ lý AI (Assisted-Fixer)**:
  - Tận dụng subagent đọc nội dung code/lịch sử để điền bản tóm tắt Summary, Traces và tạo bản Diff trình lên UI để người dùng duyệt.

---

## 5. Các vùng bị ảnh hưởng (Touched Areas)

- `apps/local-runner/internal/docscan/` (Mới: Package kiểm tra tính tuân thủ tài liệu).
- `apps/local-runner/internal/docscan/scanner.go` (Bộ phân tích và đối soát quy tắc).
- `apps/local-runner/internal/docscan/autofix.go` (Bộ codemod sửa định dạng).
- `apps/local-runner/internal/docscan/docscan_test.go` (Bộ test đối soát trên các file doc mẫu).
- `apps/local-runner/internal/runner/standardize_cmd.go` (Kết nối lệnh `/standardize` vào runner).

---

## 6. Kế hoạch kiểm thử & nghiệm thu

- **Unit tests**:
  - Test quét trên doc chuẩn `SS-13`: Trả về 0 lỗi vi phạm.
  - Test quét trên doc thiếu metadata / sai thứ tự section: Báo đúng mã lỗi và dòng vi phạm.
  - Test Auto-Fixer: Thực hiện sửa một doc bị xáo trộn section, xác nhận kết quả sau khi sửa khớp hoàn toàn với mẫu chuẩn mà không làm mất nội dung gốc.
- **Tích hợp thực tế**:
  - Chạy quét trên chính thư mục `requirements/` của FlowPilot, đối soát và tự động chuẩn hóa các doc cũ.

---

## 7. Dữ liệu và Di chuyển

- Không có thay đổi schema database.
- Không có data migration.

---

## 8. Triển khai và Dự phòng

- **Thứ tự triển khai**: Scanner (P-1) trước, Auto-Fixer (P-2) sau.
- **Dự phòng**: Auto-Fixer luôn xuất kết quả sang file `.fixed.md` tạm thời. Nếu sai, file gốc không bị ảnh hưởng.

---

## 9. Rủi ro

- `R-1` **Auto-fix bị mất nội dung**: Codemod sắp xếp section sai nếu doc có heading không chuẩn. Giảm thiểu: Test round-trip đảm bảo content byte-for-byte bảo toàn.
- `R-2` **Feature Keys false positive**: Gắn cờ thiếu `Feature Keys` trên file Task/BUG không phải doc quản trị. Giảm thiểu: Chỉ kiểm tra `Feature Keys` trên doc SS/SD/CP.

---

## 10. Tiêu chí hoàn thành tổng thể

- [ ] Bộ quét trả về 0 lỗi khi chạy trên doc chuẩn FORMAT-REFERENCE.
- [ ] Auto-Fixer sửa section sai thứ tự mà không mất nội dung.
- [ ] Quét toàn bộ `requirements/` của FlowPilot chạy xong trong < 500ms.
- [ ] P-3 Assisted-Fixer được ghi chú rõ là deferred.
