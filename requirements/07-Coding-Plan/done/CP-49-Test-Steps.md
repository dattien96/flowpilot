# CP-49 Test Steps — Reverse-Documentation & Doc Ingestion with SS-Lock

## Metadata

- Document ID: `CP-49-TEST-STEPS`
- Title: `CP-49 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `ready`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-13` (bổ sung hard-ceiling kiểm chứng trên sprint handoff — Task-346)
- Parent Documents: [CP-49: Reverse-Documentation](./CP-49-Reverse-Documentation-And-Doc-Ingestion.md)
- Child Documents: `None`
- Related Documents: [Task-333: Standardize Command & SS-Lock](../../08-Task/done/Task-333-Standardize-Command-And-Reverse-Doc-With-SS-Lock.md), [CP-48: Standardize Doc](./CP-48-Standardize-Doc.md), [CP-60: Vibe Mode](./CP-60-Vibe-Working-Mode.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `reverse-doc, doc-ingestion, ss-lock, standardize, test-steps, verification, cp-49`
- Feature Keys: `context-regression-engine, reverse-documentation`

## AI Quick View

### Summary

- Danh mục kiểm thử nghiệm thu toàn diện cho [CP-49](./CP-49-Reverse-Documentation-And-Doc-Ingestion.md) — Khả năng trích xuất ngược tài liệu từ mã nguồn thực tế (Reverse-Documentation) và nạp tài liệu tự do, tích hợp vào lệnh `/standardize [scope]`.
- Kiểm chứng nguyên tắc **Hard Ceiling Rule (Ranh giới bất di bất dịch)**:
  1. Trích xuất thiết kế kỹ thuật kiến trúc (**SD**) từ code dựa trên bằng chứng có thật (GitNexus graph, symbols, call trace). Mọi nhận định đều có dẫn chứng cụ thể (anti-hallucination).
  2. Đối với đặc tả nghiệp vụ (**SS**): AI tuyệt đối không tự bịa đặt *Ý đồ nghiệp vụ (Why)*, chỉ dựng khung sườn kèm đánh dấu `TODO: human intent needed`.
  3. **Cổng chặn bắt buộc SS-Lock (`user.confirm`)**: Hệ thống tạm dừng bắt buộc con người xem xét, bổ sung Acceptance Criteria và xác nhận khóa SS trước khi cho phép sinh tiếp CP/Task.
  4. Bàn giao tự động cho bộ máy CP-48 để chuẩn hóa định dạng SS-13.

### Current Ask

- Chạy kiểm thử tự động xác nhận các thành phần parser và cổng `ss_lock`.
- Thực hiện kiểm thử thủ công lệnh `/standardize` trên module chưa có tài liệu trong `gate-sandbox`.

### Key Decisions

- `V-1` **Dẫn chứng bằng chứng thật**: Không trích xuất mơ hồ; các hàm/struct/flow trong bản thảo SD phải trỏ tới file:line có thật trong mã nguồn.
- `V-2` **Cổng SS-Lock không thể vượt qua (Non-bypassable Gate)**: Không có cờ nào cho phép AI tự động bypass cổng SS-Lock; bắt buộc phải có sự xác nhận của người dùng.
- `V-3` **Liên hoàn CP-49 $\rightarrow$ CP-48**: Tài liệu sau khi được con người phê duyệt qua SS-Lock sẽ được tự động chuyển cho [CP-48](./CP-48-Standardize-Doc.md) để format chuẩn xác.

### Constraints

- Không sửa test cũ của codebase.
- Bed kiểm thử: `gate-sandbox` (`/Users/tiendat/Desktop/BE/gate-sandbox`).

---

## 1. Goal

Xác minh quy trình trích xuất tài liệu từ mã nguồn legacy/brownfield diễn ra khách quan, chính xác theo bằng chứng thực tế, và ý đồ nghiệp vụ (SS) luôn nằm dưới sự kiểm soát và phê duyệt tối cao của con người thông qua cổng SS-Lock.

---

## 2. Automated — run first

Thư mục làm việc: `apps/local-runner`.

```bash
# Kiểm tra bộ máy quét tài liệu kết hợp và cổng khóa ss-lock
go test ./internal/docscan/ ./internal/runner/ -count=1 -run 'TestScanDocument_|TestAutoFixDocument_|TestVibeSession_ReconstructAwaitingLock' -v

# Task-346 (CA-860): hard-ceiling áp xuống runtime — sprint handoff chỉ ghi verified state
go test ./internal/runner/ -count=1 -race -run 'TestHandoffEnrichment_' -v
```

| Step | Nhóm kiểm thử | Kịch bản kiểm tra | Pass khi | Tick |
|---|---|---|---|---|
| 2.1 | Conformance | `TestScanDocument_ValidConformingDoc` | Tài liệu chuẩn SS-13 pass 100% | [x] PASS 0.00s |
| 2.2 | Auto-Fixer | `TestAutoFixDocument_FixesStructureAndMetadata` | Tự động bổ sung các section thiếu khi trích xuất | [x] PASS 0.00s |
| 2.3 | SS-Lock | `TestVibeSession_ReconstructAwaitingLock` | Phiên dừng chờ tại chốt khóa SS, bảo toàn trạng thái | [x] PASS 0.00s |
| 2.4 | Commit Gate | `TestCA793_CommitRequiresExistingFile` | Không commit khi file tài liệu chưa tồn tại thực trên đĩa | [x] PASS 0.00s |
| 2.5 | Demotion | `TestCA793_DeleteCPDemotesToSS` | Xóa CP tự động lùi về trạng thái khóa SS | [x] PASS 0.00s |
| 2.6 | Handoff (Task-346) | `TestHandoffEnrichment_CardWithChoice` | Choice chỉ được ghi khi khớp option id/label — không đoán từ prose | [ ] |
| 2.7 | Handoff (Task-346) | `TestHandoffEnrichment_CardWithoutChoiceFallsBackToRecommended` | Trả lời prose → không bịa choice, chỉ fallback "recommended:" | [ ] |
| 2.8 | Handoff (Task-346) | `TestHandoffEnrichment_TamperedTestsRecorded` + `TestHandoffEnrichment_NoSourcesOmitsFields` | weakened_tests chỉ từ oracle guard; không có nguồn → field omitted | [ ] |

---

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm | Tick |
|---|---|---|---|
| P1 | Sandbox sẵn sàng | Thư mục `/Users/tiendat/Desktop/BE/gate-sandbox` | [ ] |
| P2 | Mã nguồn chưa có doc | Thư mục `snake/` hoặc module `calc-core` chưa có tài liệu SS tương ứng | [ ] |
| P3 | Engine khởi chạy | `cd apps/local-runner && just chat-dev /Users/tiendat/Desktop/BE/gate-sandbox` | [ ] |

---

## 4. Manual Verification Steps on gate-sandbox

### Kịch bản: Thực thi `/standardize` trích xuất tài liệu kèm Cổng SS-Lock

1. Trên giao diện chat TUI/Desktop, nhập lệnh:
   ```text
   /standardize snake
   ```
2. **Giai đoạn 1: Thu thập bằng chứng & Dựng bản thảo**:
   - AI phân tích mã nguồn Go trong thư mục `snake/` (đọc symbols, cấu trúc struct `Snake`, `Point`, `Game`).
   - AI tự động sinh bản thảo thiết kế kỹ thuật `SD-xx-Snake-Game-Engine.md` với các đường dẫn dẫn chứng cụ thể.
   - AI sinh khung sườn đặc tả nghiệp vụ `SS-xx-Snake-Game.md` với các ghi chú `TODO: human intent needed`.
3. **Giai đoạn 2: Kích hoạt Cổng SS-Lock**:
   - Quá trình chạy **tự động tạm dừng**.
   - Thẻ `ss_lock` xuất hiện trên giao diện người dùng yêu cầu xác nhận.
   - Thử nghiệm: AI không được tự động tiếp tục sinh file `CP` hay `Task` chừng nào người dùng chưa nhấn chấp thuận.
4. **Giai đoạn 3: Người dùng phê duyệt**:
   - Người dùng điền thêm 2 tiêu chí nghiệm thu vào SS và bấm **Confirm / Lock**.
   - Phiên chạy tiếp tục, chuyển giao tài liệu cho bộ máy CP-48 để căn chỉnh định dạng hoàn hảo.

### Kịch bản 2: Hard-ceiling kiểm chứng trên sprint handoff (Task-346)

1. Sau Kịch bản `/standardize`, chạy thêm một sprint Vibe có diễn ra decision card (agent hỏi user) và/hoặc một test cũ bị oracle guard bắt đụng.
2. Mở `requirements/.flowpilot/vibe/handoffs/handoff-sprint-1.yaml` và kiểm chứng hard-ceiling (CP-49 áp xuống runtime):
   - Entry `decisions` từ card chỉ chứa question thật + lựa chọn thật (khớp option id/label qua kênh continue). Trả lời bằng văn xuôi → **không** bịa choice, chỉ fallback "recommended: <id>".
   - `weakened_tests` chỉ liệt kê path mà oracle guard thực sự bắt (kèm justification) — không tự phát minh.
   - Không có nguồn nào → field omitted; runner (không phải AI) là người lắp ráp file — đúng nguyên tắc "AI/User sinh quyết định qua tool call, runner chỉ tổng hợp".
3. Sprint kế tiếp đọc handoff → hiểu why và không sửa ngược code/test của sprint trước.

---

## 5. Log Grep (Bằng chứng Kiểm toán)

```text
reverse_doc_evidence_extracted symbols_found=...
ss_lock_gate_reached awaiting_user_confirm=true
ss_lock_confirmed_by_user
docscan_autofix_applied
```

---

## 6. CP-49 Verification Complete When

- [x] §2 Automated tests chạy xanh 100%.
- [ ] Bản thảo SD được sinh ra có dẫn chứng chính xác vào các hàm và file mã nguồn thật.
- [ ] Bản thảo SS không bịa đặt nghiệp vụ và dừng lại chuẩn xác tại cổng `ss_lock`.
- [ ] Không có CP/Task nào được sinh ra trước khi người dùng xác nhận khóa SS.
- [ ] Bộ tài liệu kết quả vượt qua kiểm định định dạng của CP-48.
- [ ] Kịch bản 2: Sprint handoff chỉ ghi verified state (hard-ceiling) — choice/tamper đều có nguồn thật, không bịa.
