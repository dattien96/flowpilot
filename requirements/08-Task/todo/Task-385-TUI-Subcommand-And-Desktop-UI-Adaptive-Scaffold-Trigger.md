# Task-385: TUI Subcommand & Desktop UI Adaptive Scaffold Trigger

## Metadata

- Document ID: `Task-385`
- Title: `TUI Subcommand & Desktop UI Adaptive Scaffold Trigger`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot Architecture`
- Reviewers: `Operator, Claude Sonnet MAX`
- Created: `2026-09-18`
- Last Updated: `2026-09-18`
- Parent Documents: [CP-68 P-3](../../07-Coding-Plan/todo/CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md)
- Child Documents: `None`
- Related Documents: [Task-383](../todo/Task-383-Platform-Scaffold-Recipe-Discovery-And-Skill-Integrity-Validator.md), [Task-384](../todo/Task-384-Runner-Scaffold-Dispatcher-And-AI-Turn-Orchestration.md), [Task-386](../todo/Task-386-Compiler-Verification-Gate-And-Self-Healing-Loop.md)
- Replaces: `None`
- Tags: `tui, desktop, subcommand, slash-command, init, adaptive-ui, local-runner`
- Feature Keys: `skill-anchored-init, scaffold-engine`

---

## AI Quick View

### Summary

- Hiện thực hóa Lát cắt P-3 của CP-68: Nâng cấp trải nghiệm người dùng trên cả Terminal TUI và Desktop UI để kích hoạt luồng Scaffold một cách thích ứng (Adaptive).
- **Trong TUI:** Mở rộng subcommand của `/init` thành: `skill`, `scaffold`, `all`. Khi người dùng gõ `/init ` và bấm Tab, hệ thống kiểm tra năng lực scaffold của project platform hiện tại:
  - Nếu platform có hỗ trợ (`react-native`): Hiển thị gợi ý `/init scaffold`.
  - Nếu platform chưa có hỗ trợ (`vuejs`): **ẨN HOÀN TOÀN** gợi ý `/init scaffold` (chỉ hiển thị `skill` và `all`).
- **Trên HTTP API / Desktop UI:** Cung cấp endpoint `GET /client/projects/{projectId}/scaffold/status` để Desktop kiểm tra tính khả dụng của platform. Nếu không hỗ trợ, Desktop UI **ẩn hoàn toàn** tùy chọn "AI Scaffold" khi tạo project mới.

### Current Ask

- Cập nhật file `apps/local-runner/internal/tui/app/init_suggestions.go` và `init_engine.go` để tích hợp subcommand `scaffold` và cơ chế ẩn/hiện thích ứng.
- Bổ sung HTTP handler `GET/POST /client/projects/{projectId}/scaffold` trong `internal/runner/scaffold_handler.go`.
- Viết unit tests kiểm tra gợi ý Tab trên TUI và endpoint API.

### Key Decisions

- `T-1` Tab Suggestion Filter: Hàm `filterInitSuggestions` nhận thêm tham số `platform string` để kiểm tra `skillpack.HasScaffoldCapability(platform)`. Nếu false, loại bỏ mục `scaffold` khỏi danh sách gợi ý.
- `T-2` Backward Compatibility: Lệnh `/init skill` và `/init all` giữ nguyên hành vi 100% như CP-34 để không ảnh hưởng đến người dùng hiện tại.
- `T-3` Desktop Hide Contract: API trả về `{ "capable": false }` đối với các platform chưa có `scaffold.yaml`, chỉ dẫn Desktop UI ẩn hoàn toàn checkbox AI Scaffold, tránh gây hiểu nhầm hoặc lỗi click.

### Constraints

- Không làm gián đoạn luồng gõ phím mượt mà trong TUI client.
- Lệnh `/init scaffold` phải hiển thị live spinner / message trong khung chat TUI khi AI đang sinh mã nguồn.

### Open Questions

- None.

### Source Refs

- [CP-68 P-3, Key Decisions P-4 & Open Question Q-2](../../07-Coding-Plan/todo/CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md)
- `apps/local-runner/internal/tui/app/init_suggestions.go`
- `apps/local-runner/internal/tui/app/init_engine.go`
- `apps/local-runner/internal/tui/app/init_suggestions_test.go`

---

## 1. Goal

Hoàn thiện giao diện điều khiển của lệnh `/init` trên TUI và Desktop UI, hỗ trợ người dùng kích hoạt luồng AI Scaffold chủ động hoặc bị động một cách trực quan, đồng thời tự động ẩn các tùy chọn chưa được hỗ trợ theo quyết định kiến trúc Q-2.

---

## 2. Parent Links

- coding plan: `CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md` P-3
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SS-20-Definition-Of-Done-Gate-Contract.md`

---

## 3. Trigger

User trong TUI cần gõ `/init scaffold` để kích hoạt AI tạo code, và Desktop cần biết khi nào nên hiển thị checkbox dựng khung dự án.

---

## 4. Exact Change

- `T-1` **`apps/local-runner/internal/tui/app/init_suggestions.go`** (modified):
  - Bổ sung subcommand `scaffold`:
    ```go
    {name: "scaffold", detail: "Install skills & bootstrap Monorepo codebase via AI"}
    ```
  - Cập nhật hàm `filterInitSuggestions(input string, platform string) []suggestItem`:
    - Nếu `!skillpack.HasScaffoldCapability(platform)`: Lọc bỏ mục `scaffold` khỏi kết quả trả về.
- `T-2` **`apps/local-runner/internal/tui/app/init_engine.go`** (modified):
  - Xử lý kind `"scaffold"` trong `cmdInitEngine(kind)`:
    - Hiển thị tin nhắn: `m.addMessage("system", "Starting AI Scaffold turn for <project>...", "")`
    - Gọi API client hoặc trực tiếp gọi `ScaffoldDispatcher.Dispatch`.
- `T-3` **`apps/local-runner/internal/runner/scaffold_handler.go`** (new):
  - `GET /client/projects/{projectId}/scaffold/status`:
    - Trả về JSON: `{ "platform": "...", "capable": bool, "recipe": ... }`
  - `POST /client/projects/{projectId}/scaffold`:
    - Nhận request và chuyển giao cho `ScaffoldDispatcher`.
- `T-4` **Unit Tests**:
  - `TestInitSuggestions_HidesScaffoldForUnsupportedPlatform`: Xác nhận gõ `/init ` trên project `vuejs` không xuất hiện gợi ý `scaffold`.
  - `TestInitSuggestions_ShowsScaffoldForReactNative`: Xác nhận gõ `/init ` trên project `react-native` xuất hiện đủ 3 gợi ý: `skill`, `scaffold`, `all`.

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/tui/app/init_suggestions.go` (modified)
  - `apps/local-runner/internal/tui/app/init_engine.go` (modified)
  - `apps/local-runner/internal/tui/app/init_suggestions_test.go` (modified)
  - `apps/local-runner/internal/runner/scaffold_handler.go` (new)
  - `apps/local-runner/internal/runner/scaffold_handler_test.go` (new)
- modules: `tui/app`, `runner`
- routes:
  - `GET /client/projects/{projectId}/scaffold/status`
  - `POST /client/projects/{projectId}/scaffold`
- tables: none

---

## 6. Acceptance Check

- Chạy `go test ./internal/tui/app/...` đạt kết quả PASS.
- Trên TUI: Khi bind project `react-native`, gõ `/init ` và ấn Tab thấy xuất hiện dòng `scaffold`.
- Trên TUI: Khi bind project `vuejs`, gõ `/init ` và ấn Tab KHÔNG thấy dòng `scaffold`.
- Lệnh `/init scaffold` kích hoạt thành công Dispatcher.

---

## 7. Out of Scope

- Chưa kiểm tra compiler gate (thuộc Task-386).

---

## 8. Completion Notes

- result: pending
- follow-ups: Task-386
- upstream docs updated: None
