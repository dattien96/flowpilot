# Task-383: Platform Scaffold Recipe Discovery & Skill Integrity Validator

## Metadata

- Document ID: `Task-383`
- Title: `Platform Scaffold Recipe Discovery & Skill Integrity Validator`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot Architecture`
- Reviewers: `Operator, Claude Sonnet MAX`
- Created: `2026-09-18`
- Last Updated: `2026-09-19`
- Parent Documents: [CP-68 P-1](../../07-Coding-Plan/todo/CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md)
- Child Documents: `None`
- Related Documents: [Task-384](../todo/Task-384-Runner-Scaffold-Dispatcher-And-AI-Turn-Orchestration.md), [Task-385](../todo/Task-385-TUI-Subcommand-And-Desktop-UI-Adaptive-Scaffold-Trigger.md), [Task-386](../todo/Task-386-Compiler-Verification-Gate-And-Self-Healing-Loop.md)
- Replaces: `None`
- Tags: `scaffold, recipe, skillpack, discovery, integrity, graceful-ignore, local-runner`
- Feature Keys: `skill-anchored-init, scaffold-engine`

---

## AI Quick View

### Summary

- Hiện thực hóa Lát cắt P-1 của CP-68: Xây dựng cơ chế tải và kiểm định khế ước **Platform Scaffold Recipe (`scaffold.yaml`)** trong Go local-runner.
- Runner đọc động file manifest `flow-pack/<platform>/scaffold.yaml` thông qua `flowPackFS` mà không hardcode tên ngôn ngữ hay tên skill trong code Go.
- Cung cấp hàm `LoadScaffoldRecipe(platform)` và `HasScaffoldCapability(platform)`.
- Hiện thực hóa **Tầng 1 (Recipe Discovery)** và **Tầng 2 (Skill Integrity)** của cơ chế Verifiable Gate: nếu thiếu `scaffold.yaml` hoặc `enabled: false`, trả về `ok: false` để Runner **Graceful Ignore** (bỏ qua an toàn, không gọi AI). Nếu khai báo thiếu file skill trên đĩa, trả về danh sách lỗi cụ thể.

### Current Ask

- Tạo file `internal/skillpack/scaffold_recipe.go` định nghĩa kiểu dữ liệu `ScaffoldRecipe`, hàm load YAML từ `flowPackFS`, và hàm xác thực sự tồn tại của các skill được khai báo.
- Viết bộ unit test toàn diện trong `internal/skillpack/scaffold_recipe_test.go` xác thực cả 2 nhánh: platform đã hỗ trợ (`react-native`) và platform chưa có recipe (`vuejs`, `ruby`).

### Key Decisions

- `T-1` Struct `ScaffoldRecipe` ánh xạ chính xác với định dạng `flow-pack/<platform>/scaffold.yaml` (bao gồm `platform`, `version`, `enabled`, `description`, `scaffold_skills`, `verification_gate.command`, `timeout_seconds`, `cap`).
- `T-2` Hàm `HasScaffoldCapability(platform string) bool`: Trả về `true` khi và chỉ khi file `scaffold.yaml` tồn tại, parse hợp lệ, có `enabled: true`, và tất cả các skill trong `scaffold_skills` đều tồn tại trên đĩa.
- `T-3` Graceful Fail-Safe: Mọi lỗi đọc file hoặc thiếu manifest đều trả về `(nil, false, nil)` thay vì ném fatal error, đảm bảo luồng init cũ của CP-34 không bao giờ bị gián đoạn.

### Constraints

- Không làm ảnh hưởng đến hàm `skillsForPlatform` hiện hữu trong `install.go`.
- Không sử dụng thư viện bên ngoài chưa có trong `go.mod` (sử dụng `gopkg.in/yaml.v3` đã có sẵn).
- Additive tests only.

### Open Questions

- None.

### Source Refs

- [CP-68 P-1, Key Decisions P-1 & P-2](../../07-Coding-Plan/todo/CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md)
- `apps/local-runner/internal/skillpack/install.go`
- `apps/local-runner/internal/skillpack/flow-pack/react-native/scaffold.yaml`

---

## 1. Goal

Cung cấp module `internal/skillpack/scaffold_recipe.go` cho phép Runner tự động nhận diện và thẩm định năng lực Scaffolding của một nền tảng kỹ thuật bất kỳ một cách khách quan, đáng tin cậy, không hardcode, và hỗ trợ cơ chế Graceful Ignore khi nền tảng chưa được hỗ trợ.

---

## 2. Parent Links

- coding plan: `CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md` P-1
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SS-20-Definition-Of-Done-Gate-Contract.md`

---

## 3. Trigger

Cần một cơ chế độc lập trong tầng Skillpack để Runner và UI có thể truy vấn: "Nền tảng X có hỗ trợ AI Scaffolding không, cần nạp những skill nào, và dùng lệnh gì để kiểm tra compiler?".

---

## 4. Exact Change

- `T-1` Tạo file `apps/local-runner/internal/skillpack/scaffold_recipe.go`:
  - Khai báo struct:
    ```go
    type VerificationGateConfig struct {
        Command        string `yaml:"command" json:"command"`
        TimeoutSeconds int    `yaml:"timeout_seconds" json:"timeoutSeconds"`
        Cap            int    `yaml:"cap" json:"cap"`
    }

    type ScaffoldRecipe struct {
        Platform         string                 `yaml:"platform" json:"platform"`
        Version          int                    `yaml:"version" json:"version"`
        Enabled          bool                   `yaml:"enabled" json:"enabled"`
        Description      string                 `yaml:"description" json:"description"`
        ScaffoldSkills   []string               `yaml:"scaffold_skills" json:"scaffoldSkills"`
        VerificationGate VerificationGateConfig `yaml:"verification_gate" json:"verificationGate"`
    }
    ```
  - Viết hàm `LoadScaffoldRecipe(platform string) (*ScaffoldRecipe, bool, error)`: Đọc từ `flowPackFS` đường dẫn `flow-pack/<platform>/scaffold.yaml`.
  - Viết hàm `VerifyRecipeSkills(recipe *ScaffoldRecipe) (missing []string, ok bool)`: Kiểm tra từng thư mục skill trong `recipe.ScaffoldSkills` có chứa `SKILL.md` trong `flowPackFS` hay không.
  - Viết hàm `HasScaffoldCapability(platform string) bool`.
- `T-2` Tạo file `apps/local-runner/internal/skillpack/scaffold_recipe_test.go`:
  - `TestLoadScaffoldRecipe_ReactNative`: Kiểm tra đọc thành công recipe của `react-native`, đủ 3 skills (`scaffold-bootstrap`, `mobile-plumbing`, `core-ui-tokens`), verification command là `pnpm install && pnpm tsc --noEmit`.
  - `TestLoadScaffoldRecipe_MissingPlatform`: Kiểm tra với platform chưa có recipe (`vuejs`) -> trả về `ok: false, err: nil`.
  - `TestVerifyRecipeSkills_IntegrityPass`: Kiểm tra xác thực tính vẹn toàn của skills cho `react-native`.
  - `TestHasScaffoldCapability`: Kiểm tra boolean status cho các platform khác nhau.

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/skillpack/scaffold_recipe.go` (new)
  - `apps/local-runner/internal/skillpack/scaffold_recipe_test.go` (new)
- modules: `skillpack`
- routes: none
- tables: none

---

## 6. Acceptance Check

- Chạy lệnh `go test -v ./internal/skillpack/...` đạt kết quả PASS 100%.
- `HasScaffoldCapability("react-native")` trả về `true`.
- `HasScaffoldCapability("vuejs")` và `HasScaffoldCapability("unknown")` trả về `false`.
- Không có hồi quy (zero regression) trên các hàm `Install()` và `skillsForPlatform()`.

---

## 7. Out of Scope

- Chưa gọi AI hay dispatch turn (thuộc Task-384).
- Chưa tích hợp giao diện TUI/Desktop (thuộc Task-385).
- Chưa thực thi lệnh compiler kiểm tra (thuộc Task-386).

---

## 8. Completion Notes

- result: Implemented. `internal/skillpack/scaffold_recipe.go` ships `ScaffoldRecipe`/`VerificationGateConfig` (yaml+json tags), `LoadScaffoldRecipe` (reads `flowPackFS`, graceful `(nil,false,nil)` on missing/malformed manifest), `VerifyRecipeSkills` (platform-scoped `SKILL.md` integrity check) and `HasScaffoldCapability`. `skillsForPlatform` untouched; `scaffold.yaml` is never treated as a skill (react-native stays 23 skills).
- tests: `scaffold_recipe_test.go` — 8 additive tests (react-native recipe contract incl. 4 skills + gate `pnpm install && pnpm tsc --noEmit`/300s/cap 3, missing-platform matrix vuejs/ruby/unknown/empty/none, normalization, integrity pass, missing-skill near-miss, cross-platform guard, capability boolean matrix, install-path regression). `go test ./internal/skillpack/...` PASS (21 tests).
- deps: `gopkg.in/yaml.v3 v3.0.1` promoted from go.sum to a direct `go.mod` require (module cache only, no network).
- follow-ups: Task-384 (implemented in the same commit).
- upstream docs updated: `CA-892`.
