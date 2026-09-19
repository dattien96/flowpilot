# Task-385: TUI Single-Command Init & Desktop UI Adaptive Scaffold Trigger

## Metadata

- Document ID: `Task-385`
- Title: `TUI Single-Command Init & Desktop UI Adaptive Scaffold Trigger`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot Architecture`
- Reviewers: `Operator, Claude Sonnet MAX`
- Created: `2026-09-18`
- Last Updated: `2026-09-19`
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
- **Trong TUI (single command):** KHÔNG thêm subcommand `scaffold`. Lệnh `/init` (bare, mặc định `all`) chạy luồng engine init hiện có (skillpack + ledger + catalog) rồi tự kiểm tra năng lực scaffold của platform: capable (`scaffold.yaml` enabled + đủ `scaffold_skills`) → tự trigger AI Scaffold Turn; không capable (`vuejs`) → bỏ qua an toàn nhánh scaffold, log `scaffold: skipped (no verified recipe)`. Picker Tab giữ nguyên `skill` và `all`.
- **Trên HTTP API / Desktop UI:** Cung cấp endpoint `GET /client/projects/{projectId}/scaffold/status` để Desktop kiểm tra tính khả dụng của platform. Nếu không hỗ trợ, Desktop UI **ẩn hoàn toàn** tùy chọn "AI Scaffold" khi tạo project mới.
- **Auto-run (CP-68):** Khi platform capable, scaffold chạy tự động trên cả TUI và Desktop: TUI tự kích hoạt Scaffold Turn ngay trong luồng `/init`; Desktop tự gọi `POST /client/projects/{projectId}/scaffold` sau khi tạo project. Platform không capable thì nhánh scaffold bị bỏ qua an toàn.

### Current Ask

- Cập nhật `apps/local-runner/internal/tui/app/init_engine.go` để luồng `/init` (kind `all` / bare) tự trigger Scaffold Turn khi capable; giữ nguyên `init_suggestions.go` (không thêm dòng `scaffold`).
- Bổ sung HTTP handler `GET/POST /client/projects/{projectId}/scaffold` trong `internal/runner/scaffold_handler.go`.
- Viết unit tests kiểm tra auto-trigger trong luồng `/init` và endpoint API.

### Key Decisions

- `T-1` Single Command Contract: KHÔNG thêm subcommand `scaffold` vào `initSubcommands` — picker giữ nguyên `skill` và `all`. Luồng `/init` (bare, mặc định `all`) là entry point duy nhất cho cả init tĩnh lẫn AI Scaffold.
- `T-2` Backward Compatibility: `/init skill` giữ nguyên 100% hành vi CP-34 (chỉ cài skill, không gọi AI); `/init all` vẫn chạy đủ engine init hiện có trước khi đánh giá scaffold.
- `T-3` Desktop Hide Contract: API trả về `{ "capable": false }` đối với các platform chưa có `scaffold.yaml`, chỉ dẫn Desktop UI ẩn hoàn toàn checkbox AI Scaffold, tránh gây hiểu nhầm hoặc lỗi click.
- `T-4` Auto-run Mode: Sau khi engine init hoàn tất, luồng `/init` (kind `all` / bare) gọi `skillpack.HasScaffoldCapability(platform)`: capable → tự dispatch `ScaffoldDispatcher.Dispatch`; không capable → log `scaffold: skipped (no verified recipe)` và kết thúc bình thường. Desktop luôn auto-trigger POST scaffold khi capable (checkbox mặc định bật).

### Constraints

- Không làm gián đoạn luồng gõ phím mượt mà trong TUI client.
- Luồng Scaffold Turn trong `/init` phải hiển thị live spinner / message trong khung chat TUI khi AI đang sinh mã nguồn.

### Open Questions

- None.

### Source Refs

- [CP-68 P-3, Key Decisions P-4 & Open Question Q-2](../../07-Coding-Plan/todo/CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md)
- `apps/local-runner/internal/tui/app/init_suggestions.go`
- `apps/local-runner/internal/tui/app/init_engine.go`
- `apps/local-runner/internal/tui/app/init_suggestions_test.go`

---

## 1. Goal

Hoàn thiện giao diện điều khiển của lệnh `/init` trên TUI và Desktop UI theo thiết kế single-command của CP-68: một lệnh `/init` duy nhất chạy init hiện có rồi tự trigger AI Scaffold khi platform capable, bỏ qua an toàn khi không capable; Desktop tự ẩn tùy chọn chưa được hỗ trợ theo quyết định kiến trúc Q-2.

---

## 2. Parent Links

- coding plan: `CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md` P-3
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SS-20-Definition-Of-Done-Gate-Contract.md`

---

## 3. Trigger

User chỉ cần gõ `/init` trong TUI để chạy init (tự kèm AI Scaffold khi platform capable), và Desktop cần biết khi nào nên hiển thị checkbox dựng khung dự án.

---

## 4. Exact Change

- `T-1` **`apps/local-runner/internal/tui/app/init_suggestions.go`** (unchanged):
  - Giữ nguyên `initSubcommands` (`skill`, `all`) — không thêm dòng `scaffold` vào picker.
- `T-2` **`apps/local-runner/internal/tui/app/init_engine.go`** (modified):
  - Trong `cmdInitEngine(kind)` với kind `all` (mặc định của bare `/init`): sau khi engine init trả về thành công, kiểm tra `skillpack.HasScaffoldCapability(platform)`.
    - Capable: hiển thị `m.addMessage("system", "Starting AI Scaffold turn for <project>...", "")` rồi dispatch `ScaffoldDispatcher.Dispatch`.
    - Không capable: hiển thị `scaffold: skipped (no verified recipe)` và kết thúc bình thường.
  - Kind `skill` không thay đổi (không bao giờ gọi AI).
- `T-3` **`apps/local-runner/internal/runner/scaffold_handler.go`** (new):
  - `GET /client/projects/{projectId}/scaffold/status`:
    - Trả về JSON: `{ "platform": "...", "capable": bool, "recipe": ... }`
  - `POST /client/projects/{projectId}/scaffold`:
    - Nhận request và chuyển giao cho `ScaffoldDispatcher`.
- `T-4` **Unit Tests**:
  - `TestInitSuggestions_NoScaffoldRow`: Xác nhận picker `/init ` chỉ có `skill` và `all` cho mọi platform (không có dòng `scaffold`).
  - `TestInitEngine_AutoTriggersScaffoldForCapablePlatform`: Xác nhận `/init` trên project `react-native` chạy engine init rồi tự dispatch Scaffold Turn.
  - `TestInitEngine_SkipsScaffoldForUnsupportedPlatform`: Xác nhận `/init` trên project `vuejs` kết thúc sau engine init, không gọi AI.

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/tui/app/init_engine.go` (modified)
  - `apps/local-runner/internal/tui/app/init_engine_test.go` (new — auto-trigger tests)
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
- Trên TUI: Gõ `/init ` và ấn Tab chỉ thấy `skill` và `all` (KHÔNG có dòng `scaffold`) cho mọi platform.
- Trên TUI: project `react-native`, gõ `/init` → chạy engine init rồi tự động trigger AI Scaffold Turn, không cần thêm thao tác.
- Trên TUI: project `vuejs`, gõ `/init` → chỉ chạy engine init tĩnh, log `scaffold: skipped (no verified recipe)`, không gọi AI.
- Luồng `/init` (kind `all`) kích hoạt thành công Dispatcher khi platform capable.

---

## 7. Out of Scope

- Chưa kiểm tra compiler gate (thuộc Task-386).

---

## 8. Completion Notes

- result: pending
- follow-ups: Task-386
- upstream docs updated: None
