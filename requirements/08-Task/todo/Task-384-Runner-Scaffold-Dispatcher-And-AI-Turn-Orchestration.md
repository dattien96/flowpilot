# Task-384: Runner Scaffold Dispatcher & AI Turn Orchestration

## Metadata

- Document ID: `Task-384`
- Title: `Runner Scaffold Dispatcher & AI Turn Orchestration`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot Architecture`
- Reviewers: `Operator, Claude Sonnet MAX`
- Created: `2026-09-18`
- Last Updated: `2026-09-18`
- Parent Documents: [CP-68 P-2](../../07-Coding-Plan/todo/CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md)
- Child Documents: `None`
- Related Documents: [Task-383](../todo/Task-383-Platform-Scaffold-Recipe-Discovery-And-Skill-Integrity-Validator.md), [Task-385](../todo/Task-385-TUI-Subcommand-And-Desktop-UI-Adaptive-Scaffold-Trigger.md), [Task-386](../todo/Task-386-Compiler-Verification-Gate-And-Self-Healing-Loop.md)
- Replaces: `None`
- Tags: `runner, scaffold, orchestrator, ai-turn, context-profile, local-runner`
- Feature Keys: `skill-anchored-init, scaffold-engine`

---

## AI Quick View

### Summary

- Hiện thực hóa Lát cắt P-2 của CP-68: Xây dựng động cơ điều phối Scaffold Turn (`internal/runner/scaffold_dispatcher.go`) trong local-runner.
- Tiếp nhận yêu cầu scaffold dự án từ TUI hoặc Desktop, kiểm tra năng lực qua Task-383.
- **Graceful Ignore:** Nếu platform không có năng lực scaffold, tự động bỏ qua bước AI, chỉ cài đặt skill tĩnh và trả về kết quả `Skipped: true`.
- Nếu platform có hỗ trợ, tự động đọc toàn văn nội dung của các `scaffold_skills` tương ứng (như `react-native-scaffold-bootstrap`, `react-native-mobile-plumbing`, `react-native-core-ui-tokens`) và đóng gói thành Context Profile chuyên dụng.
- Khởi tạo lượt AI Turn với posture `scaffold_bootstrap`, hướng dẫn AI sinh toàn bộ cấu trúc Step 0 mà **người dùng không phải gõ bất kỳ prompt nào**.

### Current Ask

- Viết file `internal/runner/scaffold_dispatcher.go` triển khai `ScaffoldDispatcher` và phương thức `DispatchScaffoldTurn(ctx, projectID, workspaceDir, platform, provider)`.
- Tạo prompt template chuẩn cho Scaffold Turn: `apps/local-runner/internal/agentpack/flow-pack/prompts/scaffold-step0-bootstrap.md`.
- Viết unit tests kiểm tra luồng chuẩn bị context và điều phối turn trong `internal/runner/scaffold_dispatcher_test.go`.

### Key Decisions

- `T-1` Posture `scaffold_bootstrap`: Khác với lượt chat hay coder thông thường, AI trong lượt này có quyền ghi mã nguồn (write tools), được cấp context đầy đủ từ các blueprint skills, và bị giới hạn phạm vi chỉ thực thi Step 0 (root configs, core packages, app template).
- `T-2` Zero-Prompt Injection: Prompt gửi tới LLM được Runner tự động lắp ghép:
  `[Hệ thống] Bạn đang khởi tạo Step 0 cho dự án platform: <platform>. Hãy tuân thủ nghiêm ngặt các blueprint skills đính kèm: <nội dung skill 1, 2, 3>. Hãy sinh toàn bộ file cấu hình và package cần thiết.`
- `T-3` Result Model: Trả về `ScaffoldDispatchResult` gồm: `Status` (success, skipped, failed), `SkillsAttached` (danh sách tên skill), `TurnID`, `Error`.

### Constraints

- Tương thích với tất cả các LLM provider được hỗ trợ trong FlowPilot (`claude`, `codex`, `grok`, `opencode`).
- Tôn trọng giới hạn token của context window; nếu nội dung skill quá lớn, trích xuất phần quy chuẩn cốt lõi (Contract & Code Templates) thay vì chép phần giải thích phụ.

### Open Questions

- None.

### Source Refs

- [CP-68 P-2, Key Decisions P-2 & P-3](../../07-Coding-Plan/todo/CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md)
- `apps/local-runner/internal/runner/interactive_service.go`
- `apps/local-runner/internal/skillpack/scaffold_recipe.go` (từ Task-383)

---

## 1. Goal

Xây dựng bộ điều phối `ScaffoldDispatcher` trong local-runner chịu trách nhiệm nạp context từ các blueprint skills và kích hoạt lượt AI Turn sinh mã nguồn Step 0 một cách tự động, an toàn và có khả năng graceful ignore khi nền tảng chưa được hỗ trợ.

---

## 2. Parent Links

- coding plan: `CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md` P-2
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SS-20-Definition-Of-Done-Gate-Contract.md`

---

## 3. Trigger

Cần một tầng trung gian giữa lệnh kích hoạt (TUI/Desktop) và LLM Runtime để đọc các skills từ Task-383 và đóng gói thành một prompt hoàn chỉnh cho AI.

---

## 4. Exact Change

- `T-1` Tạo file `apps/local-runner/internal/agentpack/flow-pack/prompts/scaffold-step0-bootstrap.md`:
  - Template prompt chỉ dẫn AI nhiệm vụ khởi tạo Step 0 dựa trên các blueprint skills được gắn kèm.
- `T-2` Tạo file `apps/local-runner/internal/runner/scaffold_dispatcher.go`:
  - Khai báo interface `IScaffoldDispatcher`:
    ```go
    type ScaffoldDispatchResult struct {
        Status         string   `json:"status"` // "done", "skipped", "error"
        Platform       string   `json:"platform"`
        SkillsAttached []string `json:"skillsAttached"`
        Message        string   `json:"message"`
    }

    type ScaffoldDispatcher struct {
        skillPackRoot string
        // dependencies: interactiveService, toolingChecker
    }
    ```
  - Triển khai phương thức `Dispatch(ctx context.Context, req ScaffoldRequest) (*ScaffoldDispatchResult, error)`:
    1. Gọi `skillpack.LoadScaffoldRecipe(req.Platform)`.
    2. Nếu không có recipe hoặc `!recipe.Enabled`: Trả về `Status: "skipped"`, không gọi AI.
    3. Nếu có recipe: Đọc nội dung từng skill trong `recipe.ScaffoldSkills`, đóng gói vào Context Profile.
    4. Gửi request sang session turn của AI với posture `scaffold_bootstrap`.
- `T-3` Tạo file `apps/local-runner/internal/runner/scaffold_dispatcher_test.go`:
  - Test case: Dispatch với platform hỗ trợ (`react-native`) -> Tạo turn thành công, đính kèm đủ 3 skills.
  - Test case: Dispatch với platform không hỗ trợ (`vuejs`) -> Trả về `skipped`, zero AI call.

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/scaffold-step0-bootstrap.md` (new)
  - `apps/local-runner/internal/runner/scaffold_dispatcher.go` (new)
  - `apps/local-runner/internal/runner/scaffold_dispatcher_test.go` (new)
- modules: `runner`, `agentpack`
- routes: none
- tables: none

---

## 6. Acceptance Check

- `go test -v ./internal/runner -run TestScaffoldDispatcher` đạt PASS 100%.
- Khi gọi dispatch với `react-native`, log xác nhận nạp đủ 3 blueprint skills vào context.
- Khi gọi dispatch với `vuejs`, runner ghi log graceful ignore và không tạo turn thừa.

---

## 7. Out of Scope

- Chưa xử lý sự kiện UI trên TUI hay Desktop (thuộc Task-385).
- Chưa kiểm tra compiler gate sau khi AI hoàn tất code (thuộc Task-386).

---

## 8. Completion Notes

- result: pending
- follow-ups: Task-385, Task-386
- upstream docs updated: None
