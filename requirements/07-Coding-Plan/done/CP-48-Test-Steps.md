# CP-48 Test Steps — Document Conformance Engine & Auto-Fixer

## Metadata

- Document ID: `CP-48-TEST-STEPS`
- Title: `CP-48 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `ready`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-13` (bổ sung scenario regression + liên kết task doc chuẩn ↔ AC coverage)
- Parent Documents: [CP-48: Standardize Doc](./CP-48-Standardize-Doc.md)
- Child Documents: `None`
- Related Documents: [Task-332: Conformance Scanner & AutoFixer](../../08-Task/done/Task-332-Doc-Conformance-Scanner-And-AutoFixer.md), [CP-49: Reverse-Documentation](./CP-49-Reverse-Documentation-And-Doc-Ingestion.md), [SS-13: Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `docscan, conformance, autofix, ss-13, test-steps, verification, cp-48`
- Feature Keys: `context-regression-engine, doc-standardization`

## AI Quick View

### Summary

- Quy trình kiểm thử nghiệm thu toàn diện cho [CP-48](./CP-48-Standardize-Doc.md) — Bộ máy quét kiểm định (Conformance Scanner) và Tự động sửa cấu trúc tài liệu (Auto-Fixer) theo chuẩn hợp đồng [SS-13](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md).
- Bao phủ:
  1. **Scanner (P-1)**: Quét và phát hiện các sai lệch về Metadata, AI Quick View (Summary, Current Ask, Key Decisions, Constraints, Open Questions, Source Refs), thứ tự section, trường `Feature Keys`.
  2. **Auto-Fixer (P-2)**: Tự động sắp xếp lại thứ tự các phần bị đảo lộn, chèn khung metadata còn thiếu, giữ nguyên vẹn 100% nội dung bài viết hiện có và không làm biến đổi ký tự xuống dòng (line endings).
  3. **Hiệu năng**: Quét hơn 120 tài liệu trong dưới 50ms (vượt xa chỉ tiêu 500ms).

### Current Ask

- Chạy kiểm thử tự động xác nhận toàn bộ 26 bài test của `internal/docscan` đều PASS.
- Thực hiện kiểm thử thủ công quét và tự động sửa một tài liệu bị sai format mẫu trong `gate-sandbox`.

### Key Decisions

- `V-1` **Deterministic & 0-Token**: Toàn bộ thuật toán scanner và autofix viết bằng Go xác định, không tốn token LLM.
- `V-2` **An toàn dữ liệu tuyệt đối (Lossless)**: Auto-fixer chỉ định hình lại cấu trúc, tuyệt đối không bao giờ xóa hay làm mất nội dung người dùng đã viết.
- `V-3` **Phân lớp sửa đổi**: Tự động sửa các lỗi cú pháp/khung xương (auto-fix); gắn cờ yêu cầu người dùng duyệt đối với các nội dung ngữ nghĩa lớn (manual-flag).

### Constraints

- Không sửa test cũ của codebase.
- Bed kiểm thử: `gate-sandbox` (`/Users/tiendat/Desktop/BE/gate-sandbox`).

---

## 1. Goal

Xác minh khả năng kiểm định và tự động chuẩn hóa kho tài liệu của FlowPilot theo đúng hợp đồng SS-13 một cách nhanh chóng, chính xác, và bảo toàn toàn vẹn dữ liệu.

---

## 2. Automated — run first

Thư mục làm việc: `apps/local-runner`.

```bash
# Chạy bộ test suite hoàn chỉnh của bộ máy docscan
go test ./internal/docscan/ -count=1 -v
```

| Step | Kịch bản kiểm tra | Pass khi | Tick |
|---|---|---|---|
| 2.1 | `TestScanDocument_ValidConformingDoc` | Tài liệu chuẩn SS-13 không có bất kỳ vi phạm nào | [x] PASS 0.00s |
| 2.2 | `TestScanDocument_MissingFeatureKeys` | Phát hiện thiếu trường Feature Keys ở các phase bắt buộc | [x] PASS 0.00s |
| 2.3 | `TestScanDocument_SectionOutOfOrder` | Phát hiện thứ tự các section bị đảo lộn | [x] PASS 0.00s |
| 2.4 | `TestAutoFixDocument_FixesStructureAndMetadata` | Tự động sửa cấu trúc và chèn metadata skeleton | [x] PASS 0.00s |
| 2.5 | `TestAutoFixDocument_AlreadyCompliant_NoChange` | Tài liệu đã chuẩn thì không làm thay đổi nội dung (idempotent) | [x] PASS 0.00s |
| 2.6 | `TestScanDocument_FormatReferenceSamples_ZeroIssues` | Các file mẫu `FORMAT-REFERENCE-*.md` có 0 lỗi vi phạm | [x] PASS 0.00s |
| 2.7 | `TestScanDirectory_Performance_120Files_Under500ms` | Quét 120 file trong ~10-15ms (< 500ms) | [x] PASS 0.06s |
| 2.8 | `TestAutoFixDocument_ReordersScrambledSections` | Sắp xếp lại đúng thứ tự các section 1..N | [x] PASS 0.00s |
| 2.9 | `TestAutoFixDocument_PreservesLineEndingsAndBody` | Giữ nguyên vẹn toàn bộ phần nội dung và xuống dòng | [x] PASS 0.00s |
| 2.10 | `TestScanDocument_MissingAIQuickViewSubsections` | Bắt lỗi thiếu 1 trong 6 mục con của AI Quick View | [x] PASS 0.00s |

---

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm | Tick |
|---|---|---|---|
| P1 | Sandbox sẵn sàng | Thư mục `/Users/tiendat/Desktop/BE/gate-sandbox` sạch trạng thái git | [ ] |
| P2 | Tạo tài liệu mẫu lỗi | Tạo file `requirements/08-Task/todo/Task-000-Malformed.md` bị thiếu metadata và đảo thứ tự section | [ ] |

---

## 4. Manual Verification Steps on gate-sandbox

### Kịch bản 1: Quét tài liệu sai chuẩn

1. Tạo file `Task-000-Malformed.md` chỉ có tiêu đề và nội dung tự do, không có metadata block và AI Quick View.
2. Thực thi kiểm định qua CLI hoặc runner:
   ```bash
   # Quét tài liệu trong thư mục requirements
   flowpilot-runner docscan scan ./requirements/08-Task/todo/Task-000-Malformed.md
   ```
3. **Quan sát Báo Cáo**:
   - Báo cáo liệt kê chi tiết các vi phạm:
     - Thiếu Metadata block (Critical)
     - Thiếu AI Quick View (Critical)
     - Thiếu Section Goal, Parent Links, Acceptance Check...

### Kịch bản 2: Tự động sửa chữa tài liệu (Auto-Fix)

1. Chạy lệnh tự động sửa:
   ```bash
   flowpilot-runner docscan fix ./requirements/08-Task/todo/Task-000-Malformed.md
   ```
2. **Quan sát Kết quả File**:
   - File được chèn thêm khung Metadata với Document ID `Task-000`.
   - Khung AI Quick View với 6 mục con tiêu chuẩn được bổ sung.
   - Các nội dung văn bản cũ của người dùng được giữ nguyên vẹn 100%, không mất chữ nào.
3. Chạy lại lệnh quét: Báo cáo kết quả 0 lỗi vi phạm cấu trúc.

### Kịch bản 3 (Task-344 liên kết): Task doc sai chuẩn → AC coverage bỏ qua êm đẹp

1. Dùng lại file `Task-000-Malformed.md` (Kịch bản 1 — không có `## 6. Acceptance Check`, không có token `AC-n`).
2. Cho một flow chạy với task doc này và để reviewer nộp `submit_review_outcome` **thiếu/bất kỳ** verdict rows nào.
3. **Quan sát**:
   - Task-344 resolve expectedACs từ task doc governing — file không có `AC-n` → tập expected rỗng → coverage **tự bỏ qua**, KHÔNG chặn reviewer oan (fault-tolerant fallback của CP-62 P-2).
   - Ngược lại, task doc ĐÚNG chuẩn (có `## 6. Acceptance Check` với `AC-n`) → checklist trở thành hợp đồng bắt buộc (chi tiết tại CP-47 Kịch bản 4).
4. **Kết luận kiểm định**: chuẩn hóa tài liệu (CP-48) giờ còn là điều kiện để AC coverage kích hoạt — càng đúng chuẩn SS-13, nghiệm thu càng chặt.

### Kịch bản 4 (Regression — Task-344..348): docscan không bị ảnh hưởng

1. Sau khi merge nhóm follow-up CP-62 (CA-856..860 — đụng `gate_hook.go`, `submit path`, `sprint_handoff.go`, UI desktop/TUI), chạy lại đúng Kịch bản 1 + 2 ở trên.
2. **Quan sát**: kết quả scan/fix và log `docscan_*` không đổi so với baseline — các thay đổi follow-up không đụng đường `internal/docscan` hay auto-fix (xác nhận trong CA notes).

---

## 5. Log Grep (Bằng chứng Kiểm toán)

```text
docscan_scan_completed files_scanned=... issues_found=...
docscan_autofix_applied file=... changes=...
```

---

## 6. CP-48 Verification Complete When

- [x] §2 Automated tests chạy xanh 100% (26/26 tests pass).
- [ ] Kịch bản 1: Scanner phát hiện chính xác mọi lỗi vi phạm cấu trúc SS-13.
- [ ] Kịch bản 2: Auto-Fixer khôi phục thành công cấu trúc chuẩn mà không làm biến đổi nội dung cũ.
- [ ] Kịch bản 3: Task doc đúng chuẩn → AC coverage kích hoạt; sai chuẩn → bỏ qua êm đẹp (không chặn oan).
- [ ] Kịch bản 4: Regression — docscan scan/fix không đổi sau khi merge Task-344..348.
