# CP-47: Cổng kiểm duyệt Definition-of-Done (`r-dod`)

## Metadata

- Document ID: `CP-47`
- Title: `Cổng kiểm duyệt Definition-of-Done (r-dod)`
- Feature Keys: `context-regression-engine, flowgate`
- Phase: `coding_plan`
- Status: `approved`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-07-14`
- Last Updated: `2026-09-11`
- Parent Documents: [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- Child Documents: [Task-330: DOD Parser và Cổng r-dod-present](../../08-Task/todo/Task-330-DOD-Parser-And-Present-Gate.md), [Task-331: Cổng r-dod-complete và Tích hợp Runner](../../08-Task/todo/Task-331-DOD-Complete-Gate-And-Runner-Wiring.md)
- Related Documents: [SS-13: Hợp đồng tài liệu cho AI](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [CP-43: Change Contract và Canonical Intent Signature](./CP-43-Change-Contract-And-Canonical-Intent-Signature.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-gate, flowgate, definition-of-done, dod, task, bugfix, r-dod`

## AI Quick View

### Summary

- Bổ sung họ quy tắc cổng kiểm duyệt **`r-dod`** vào hệ thống đăng ký `flowgate` hiện có trong Go runner để bảo đảm mọi công việc Task/BugFix được neo vào Definition of Done có thể kiểm tra tự động, không dựa vào cam kết lời văn suông.
- Hai thời điểm thực thi độc lập (kế thừa pattern phân tách của `r-artifact-output` / `r-artifact-output-structure`):
  - **`r-dod-present`**: Khi một turn tạo mới hoặc chỉnh sửa tài liệu `Task-*` hoặc `BUG-*`, tài liệu đó bắt buộc phải chứa section `## Definition of Done` với ít nhất 1 checkbox (`- [ ]`). Nếu thiếu hoặc rỗng -> Nhắc nhở (`reprompt`) bắt bổ sung trước khi tiếp tục.
  - **`r-dod-complete`**: Khi tài liệu chuyển trạng thái sang `done` (`Status: done` trong metadata hoặc file được dời vào thư mục `done/`), tất cả các checkbox trong DOD phải được tích `[x]`. Nếu còn checkbox `[ ]` chưa tích, turn phải giải trình lý do rõ ràng (cơ chế "or-explained" giống `r-tests`), nếu không có giải trình -> Chặn (`block`) không cho phép hoàn thành.
- Cơ chế kích hoạt dựa trên **Tín hiệu thực tế (Trigger trên `WrittenPaths`)**:
  - Áp dụng cho mọi flow (`task-harness`, `bug-plan-harness`, `vibe-sprint`).
  - Đối với `bug-harness` (hotfix nhanh): Nếu turn có ghi file `BUG-*` thì gate sẽ kiểm tra; nếu là hotfix siêu tốc chỉ tạo `change-audit` note mà không đụng đến file `BUG-*` thì gate tự động bỏ qua (bypass), không gây nghẽn luồng làm việc.
- Trình phân tích markdown hoàn toàn xác định (deterministic), chạy offline với **0 chi phí token/LLM**, phân tích cú pháp checkbox theo chuẩn `SS-13`.

### Current Ask

- Triển khai `r-dod-present` + `r-dod-complete` trong `flowgate.DefaultRules()`, bộ phân tích cú pháp `dod.go`, kết nối đánh giá trong `evaluate.go` và tích hợp vào `runner/gate_hook.go` qua hai task `Task-330` và `Task-331`.

### Key Decisions

- `D-1` **Tái sử dụng hạ tầng `flowgate` hiện có**: `r-dod` là 2 quy tắc mới `Rule{ID, Scope, Trigger, RequiredOutput, Action, Enabled}` trong `DefaultRules()`, được đánh giá qua hàm `Evaluate`/`checkRule` cùng đường dẫn với `r-ca`, `r-tests`, `r-scope`. `MergeDefaultRules` tự động nạp vào `flow-rules.json` của các workspace cũ.
- `D-2` **Tín hiệu "Done" đọc trực tiếp từ trạng thái doc**: `r-dod-complete` kích hoạt khi tài liệu `Task-*`/`BUG-*` được ghi nhận chuyển sang `Status: done` hoặc file chuyển đường dẫn vào `.../done/`. Tuyệt đối không đoán trạng thái qua commit message.
- `D-3` **`present` nhắc nhở (`reprompt`), `complete` chặn hoặc giải trình (`block-or-explained`)**: `r-dod-present.Action = reprompt`. `r-dod-complete.Action = block` với `RequiredOutput = dod_all_checked_or_explained`: Tích hết `[x]` -> Pass; còn checkbox trống kèm giải trình hợp lệ trong turn -> Pass with warning; còn checkbox trống không giải trình -> Block.
- `D-4` **Trình phân tích DOD dùng chung và không phụ thuộc LLM**: Hàm `parseDefinitionOfDone(md)` trả về `{Present bool, Total int, Checked int, OpenItems []string}` bằng cách đọc tiêu đề `## Definition of Done` và danh sách checkbox. Cả `[x]` và `[X]` đều tính là hoàn thành.
- `D-5` **Kích hoạt theo phạm vi file được ghi (`WrittenPaths`)**: Gate chỉ chạy kiểm tra khi trong lượt này AI có ghi/sửa file `Task-*` hoặc `BUG-*`. Giúp hỗ trợ trơn tru cả flow có plan lẫn flow hotfix nhanh.

### Constraints

- Xác định và ngoại tuyến: Bộ phát hiện không bao giờ gọi LLM (chạy sau mỗi turn hoàn thành).
- Không phá vỡ quy tắc cũ: Bổ sung thuần túy vào danh sách quy tắc, không làm thay đổi thứ tự hay ngữ nghĩa của `r-ca`, `r-tests`, `r-scope`.
- Tuân thủ hợp đồng tài liệu: Đọc section `## Definition of Done` đúng chuẩn cú pháp `SS-13`.
- Tuyệt đối không tự động tích checkbox hộ người dùng: Việc đánh dấu `[x]` phải là hành động rõ ràng của AI/con người.

### Source Refs

- `apps/local-runner/internal/flowgate/rules.go` (Đăng ký quy tắc `DefaultRules()`).
- `apps/local-runner/internal/flowgate/evaluate.go` (Đánh giá vi phạm và hành động).
- `apps/local-runner/internal/runner/gate_hook.go` (Kết nối cổng sau mỗi lượt chạy của root và child flow).
- `requirements/05-System-Specs/SS-13-AI-Followable-Document-Contract.md` (Định dạng chuẩn của section `Definition of Done`).

### Open Questions

- Đã giải quyết: Phạm vi `r-dod-present` chỉ áp dụng cho file có tiền tố `Task-` hoặc `BUG-` trong `WrittenPaths`.
- Đã giải quyết: Section DOD có heading nhưng không có checkbox nào được tính là thiếu (`Total = 0`).
- Đã giải quyết: `[x]` và `[X]` đều được chấp nhận là đã hoàn thành.

---

## 1. Mục tiêu

Biến "công việc đã hoàn thành" thành **sự thật được cổng kiểm duyệt xác minh tự động**, thay vì một lời tuyên bố không được kiểm chứng: mọi task/bug đều phải có Definition of Done rõ ràng ngay từ khi lập kế hoạch, và không một flow nào có thể đánh dấu hoàn thành khi các tiêu chí DOD vẫn còn bỏ ngỏ trừ khi có giải trình hợp lệ.

---

## 2. Tài liệu đầu vào

- Thiết kế kiến trúc: [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md).
- Hợp đồng tài liệu: [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md).
- Tiền đề triển khai: `flowgate` rules `r-tests` (mẫu chặn có giải trình), `r-bug`/`r-task` (mẫu bắt buộc có doc), `r-artifact-output` (mẫu bắt buộc có artifact).

---

## 3. Chiến lược triển khai

- **Cách tiếp cận tổng thể**:
  1. Xây dựng package hỗ trợ `dod.go` trong `flowgate` để phân tích markdown section DOD.
  2. Bổ sung `r-dod-present` và `r-dod-complete` vào `DefaultRules()`.
  3. Bổ sung trường tín hiệu `DodPresent`, `DodTotal`, `DodChecked`, `DodOpenItems`, `DodTransitionedToDone` vào `TurnResult`.
  4. Cập nhật `runner/gate_hook.go` tính toán tín hiệu từ các file trong `WrittenPaths` và gọi `flowgate.Evaluate`.
- **Thứ tự thực hiện**:
  - Giai đoạn 1 (`Task-330`): Triển khai parser `dod.go` + quy tắc nhắc nhở `r-dod-present` (chỉ reprompt, an toàn tuyệt đối).
  - Giai đoạn 2 (`Task-331`): Triển khai tín hiệu chuyển trạng thái done + quy tắc chặn `r-dod-complete` (block-or-explained) và bề mặt hiển thị lỗi trên UI.

---

## 4. Phân chia công việc (Work Breakdown & Task Mapping)

- `P-1` **Bộ phân tích cú pháp DOD (`dod.go`)** $\rightarrow$ Nằm trong `Task-330`:
  - Hàm `parseDefinitionOfDone(md string) DodStatus`: Tìm tiêu đề `## Definition of Done` (không phân biệt hoa thường), duyệt các dòng checklist `- [ ]` vs `- [x]`/`- [X]`, đếm tổng số, số lượng đã tick và danh sách các mục còn mở.
- `P-2` **Quy tắc `r-dod-present`** $\rightarrow$ Nằm trong `Task-330`:
  - Khai báo quy tắc: `{ID: "r-dod-present", Scope: "step", Trigger: "task_or_bug_doc_missing_dod", RequiredOutput: "definition_of_done_section", Action: "reprompt", Enabled: true}`.
  - Kích hoạt khi turn có ghi file `Task-*` hoặc `BUG-*` nhưng `DodStatus.Present == false` hoặc `Total == 0`.
- `P-3` **Tín hiệu chuyển trạng thái Done** $\rightarrow$ Nằm trong `Task-331`:
  - Trong bộ dựng `TurnResult`: Kiểm tra file `Task-*`/`BUG-*` trong `WrittenPaths`. Nếu metadata chuyển sang `Status: done` hoặc đường dẫn file nằm trong thư mục con `done/`, thiết lập `DodTransitionedToDone = true` kèm `DodStatus`.
- `P-4` **Quy tắc `r-dod-complete`** $\rightarrow$ Nằm trong `Task-331`:
  - Khai báo quy tắc: `{ID: "r-dod-complete", Scope: "step", Trigger: "marked_done_with_open_dod", RequiredOutput: "dod_all_checked_or_explained", Action: "block", Enabled: true}`.
  - Kiểm tra nếu `DodTransitionedToDone == true` và `Checked < Total`: Nếu không tìm thấy đoạn giải trình hợp lệ (trong ghi chú doc hoặc trong `FinalMessage`) -> Block; nếu có giải trình -> Hạ cấp cảnh báo `warn`.
- `P-5` **Tích hợp Runner và UI Decision Card** $\rightarrow$ Nằm trong `Task-331`:
  - Định tuyến thông báo vi phạm qua luồng xử lý gate event chuẩn của runner.

---

## 5. Các vùng bị ảnh hưởng (Touched Areas)

- `apps/local-runner/internal/flowgate/dod.go` (Mới: Trình phân tích cú pháp markdown).
- `apps/local-runner/internal/flowgate/dod_test.go` (Mới: Bộ kiểm thử đơn vị cho parser).
- `apps/local-runner/internal/flowgate/rules.go` (Thêm 2 quy tắc vào `DefaultRules()`, mở rộng `DocScopeRuleIDs`).
- `apps/local-runner/internal/flowgate/evaluate.go` (Thêm các case xử lý `task_or_bug_doc_missing_dod` và `marked_done_with_open_dod`).
- `apps/local-runner/internal/runner/gate_hook.go` (Thu thập tín hiệu DOD từ file được ghi và nạp vào `TurnResult`).

---

## 6. Kế hoạch kiểm thử & nghiệm thu

- **Unit tests**:
  - Test bảng cho `parseDefinitionOfDone`: Section có/không tồn tại, section rỗng, tick một phần, tick toàn bộ, thụt đầu dòng (indentation), chữ hoa/thường `[x]` vs `[X]`.
  - Test kích hoạt `r-dod-present`: Chỉ kích hoạt khi ghi file Task/Bug thiếu DOD.
  - Test kích hoạt `r-dod-complete`: Chặn khi done mà còn checkbox mở; cho qua khi đã tick hết; cho qua dạng warning khi có giải trình hợp lệ.
  - Test merge: Đảm bảo `MergeDefaultRules` tự động bổ sung 2 rule mới vào `flow-rules.json`.
- **E2E & Harness Verification**:
  - Chạy `task-harness` hoặc `vibe-sprint`: Đảm bảo `plan_writer`/`task_slicer` sinh doc có DOD thì pass mượt mà; cố tình xóa DOD sẽ bị reprompt ngay lập tức.

---

## 7. Triển khai và Dự phòng (Rollout and Fallback)

- **Thứ tự triển khai**: Giai đoạn 1 (`Task-330`: Parser + `r-dod-present`) trước; giai đoạn 2 (`Task-331`: `r-dod-complete` + Runner wiring) sau.
- **Dự phòng**: Nếu phát sinh false positive, operator có thể tắt rule qua `flow-rules.json` (`"r-dod-present": {"enabled": false}`) mà không cần sửa code.

---

## 8. Rủi ro

- `R-1` **False positive trên tài liệu không chuẩn**: Các doc cũ viết DOD dưới dạng đoạn văn thay vì checkbox sẽ bị coi là thiếu. Giảm thiểu: Chạy scan và autofix (CP-48) trước khi bật gate.
- `R-2` **Giải trình bị từ chối nhầm**: Heuristic `hasValidDodExplanation` có thể bỏ sót giải trình viết theo format không chuẩn. Giảm thiểu: Hạ cấp `block` thành `warn` khi có bất kỳ đoạn giải trình nào.

---

## 9. Tiêu chí hoàn thành tổng thể (Definition of Done)

- [ ] Parser `dod.go` xử lý chính xác tất cả biến thể heading DOD.
- [ ] Hai quy tắc `r-dod-present` và `r-dod-complete` được đăng ký trong `DefaultRules()`.
- [ ] `MergeDefaultRules` tự động bổ sung 2 rule mới vào `flow-rules.json` cũ.
- [ ] Cổng `r-dod-present` nhắc nhở (reprompt) khi Task/Bug thiếu DOD.
- [ ] Cổng `r-dod-complete` chặn (block) khi done mà còn checkbox mở không giải trình.
- [ ] Tất cả unit tests mới pass, không làm gãy tests hiện có.
- [ ] Kiểm tra E2E trên `task-harness` xác nhận gate hoạt động đúng.
