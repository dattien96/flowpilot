# Task-343: Context Source Conventions (Repo-as-Config) qua SD-22 Registry

## Metadata

- Document ID: `Task-343`
- Title: `Context Source Conventions (Repo-as-Config) qua SD-22 Registry`
- Feature Keys: `zcode-parity`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-12`
- Parent Documents: [CP-62: Nâng cấp Harness học từ ZCode](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [CP-23: Auto-Learn-To-Skill](../../07-Coding-Plan/done/CP-23-Auto-Learn-To-Skill.md)
- Child Documents: `None`
- Related Documents: [Task-341: Context Profile](./Task-341-Per-Node-Context-Profile-And-Catalog-Tier.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `conventions, repo-as-config, context-source, sd-22, safe-fix, cp-62, p-7`

## AI Quick View

### Summary

- Hiện thực hóa Slice `P-7` của [CP-62](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md): Triển khai context source tĩnh **`conventions`** thông qua hạ tầng Pluggable Context Source Registry (SD-22) tích hợp trực tiếp trong `apps/local-runner/internal/runner/`, áp dụng triết lý *Repo-as-Config*.
- Nạp quy ước dự án từ các file cấu hình chuẩn (`.flowpilot/conventions.md`, fallback sang `AGENTS.md`) theo mô hình phân tầng ưu tiên: **User-Level Conventions > Workspace-Level Conventions**.
- Tích hợp trực tiếp vào Budget Packer dưới dạng mục Contract Section cố định (độ ưu tiên tương đương Tier-1 CP-23, không bao giờ bị cắt tỉa khi vượt ngân sách).
- Giữ nguyên vẹn các file Flow YAML giữa các dự án khác nhau: không cần fork flow YAML hay nhét thủ công conventions vào System Specs.

### Current Ask

- Thêm struct `conventionsSource` triển khai interface `ContextSource` trong `apps/local-runner/internal/runner/context_source_conventions.go`.
- Đăng ký source `conventions` trong `apps/local-runner/internal/runner/context_sources_builtin.go`.
- Thêm section `conventions` vào `flow-context-package.yaml` và ánh xạ Tier 1 trong Budget Packer.
- Xây dựng bộ test xác thực tính phân tầng (user > workspace) và tính bền vững khi file vắng mặt.

### Key Decisions

- `T-1` **Đọc Nguồn Tĩnh Chuẩn Hóa**: Đọc file `.flowpilot/conventions.md` tại thư mục gốc workspace. Nếu không có $\rightarrow$ fallback đọc `AGENTS.md`.
- `T-2` **Thứ Bậc Ưu Tiên Rõ Ràng**: Conventions tại thư mục cấu hình cá nhân của người dùng (`~/.flowpilot/conventions.md`) ghi đè hoặc bổ sung cho workspace conventions.
- `T-3` **Không Thể Cắt Tỉa (Non-droppable Tier-1)**: Conventions chứa các quy ước cốt lõi về phong cách code, lệnh test, và quy tắc an toàn. Do đó, Budget Packer coi đây là section thiết yếu (Tier 1), không bao giờ bị loại bỏ khi nén prompt.
- `T-4` **Không Phát Sinh Lỗi Khi Thiếu File**: Nếu cả hai file conventions đều không tồn tại, source trả về section rỗng và luồng tiếp tục bình thường mà không gây lỗi.

### Constraints

- Tuân thủ nghiêm ngặt **safe-fix-contract**: Không sửa đổi bộ test cũ của context registry trong SD-22.
- Tuân thủ đúng interface `ContextSource` trong `internal/runner/context_source_registry.go`.
- Ghi nhận đầy đủ token tiêu thụ của source `conventions` trong `prompt_context_audit`.

### Open Questions

- Không còn câu hỏi mở.

### Source Refs

- CP-62 §4 `P-7` (Conventions context source).
- `SD-22: Pluggable Context Source Registry`.
- `apps/local-runner/internal/runner/context_source_registry.go`.
- `apps/local-runner/internal/runner/context_sources_builtin.go`.

---

## 1. Goal

Cung cấp quy ước phát triển và tiêu chuẩn lập trình của dự án vào prompt của agent một cách tự động, xác định và có phân cấp, giải phóng các file Flow YAML khỏi việc bị chỉnh sửa tùy biến theo từng repo riêng lẻ.

---

## 2. Parent Links

- Coding Plan: [CP-62 Slice P-7](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md)
- System Tech Design: [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md)
- Coding Plan: [CP-23: Budget Packer](../../07-Coding-Plan/done/CP-23-Auto-Learn-To-Skill.md)

---

## 3. Trigger

Hiện tại các quy ước đặc thù của dự án (framework kiểm thử, cú pháp linter, Justfile commands, naming convention) bị ghi rải rác trong các prompt template hoặc phải copy thủ công vào các tài liệu System Specs. Khi mang flow YAML sang project khác, người dùng buộc phải fork flow YAML để điều chỉnh.

---

## 4. Exact Change

- `T-1` **Tạo Context Source `conventions` chuẩn mực**:
  Tạo file `apps/local-runner/internal/runner/context_source_conventions.go` triển khai interface `ContextSource` (4 methods: `ID`, `Priority`, `Deterministic`, `Fetch`).
- `T-2` **Đăng ký vào Builtin Registry**:
  Trong `apps/local-runner/internal/runner/context_sources_builtin.go`, đăng ký qua `mustRegisterContextSource(r, newConventionsSource(priority))`.
- `T-3` **Thiết lập Phân Cấp Đọc**:
  - Đọc user config: `filepath.Join(os.UserHomeDir(), ".flowpilot", "conventions.md")`.
  - Đọc workspace config: `.flowpilot/conventions.md` (nếu không có thì đọc `AGENTS.md`).
  - Hợp nhất 2 tầng (User-level overrides/extends Workspace-level).
- `T-4` **Cấu hình Tier-1 trong Budget Packer**:
  Trong `packer.go`, gán section `conventions` mức ưu tiên `PriorityTier1_Mandatory`.

---

## 5. Touched Areas

- `apps/local-runner/internal/runner/context_source_conventions.go` (Mới)
- `apps/local-runner/internal/runner/context_sources_builtin.go`
- `apps/local-runner/internal/promptpacker/packer.go`
- `apps/local-runner/internal/agentpack/flow-pack/contexts/flow-context-package.yaml`
- `apps/local-runner/internal/runner/context_source_conventions_test.go` (Mới)

---

## 6. Acceptance Check

- Chạy `go test ./internal/runner/ -run TestConventionsSource_` đạt PASS 100%.
- Kiểm tra thứ bậc: Khi cả user conventions và workspace conventions cùng tồn tại, nội dung được ghép nối hoặc ghi đè đúng thứ bậc ưu tiên.
- Kiểm tra fallback: Xóa `.flowpilot/conventions.md`, hệ thống tự động đọc nội dung từ `AGENTS.md`.
- Kiểm tra Budget Packer: Khi tổng prompt vượt hạn mức token, các section phụ bị cắt nhưng section `conventions` vẫn còn nguyên 100%.
- Kiểm tra tính bất biến của Flow YAML: Cùng một file `task-harness.yaml` chạy thành công trên 2 dự án có conventions khác nhau mà không cần sửa file YAML.

---

## 7. Out of Scope

- Không parse cấu trúc AST phức tạp bên trong file conventions (chỉ coi là markdown tĩnh).
- Không tự động cập nhật nội dung conventions từ AI (chỉ con người bảo trì file này).

---

## 8. Completion Notes

- Trạng thái: `done` (2026-09-12).
- Triển khai: `runner/context_source_conventions.go` (MỚI) — `conventionsSource` đúng interface `ContextSource` 4 methods (ID/Priority/Deterministic/Fetch); precedence: user `~/.flowpilot/conventions.md` > workspace `.flowpilot/conventions.md` (fallback `AGENTS.md`); thiếu file → section rỗng, không lỗi; đăng ký `priority: 0` (pack trước canonical.head) trong `context_sources_builtin.go`.
- Tier-1 non-droppable ĐÚNG CƠ CHẾ CÓ SẴN: body render với heading `## Context — Project Conventions` → `classifyPromptBlock` xếp `mandatory_doc` → CP-23 R-1 giữ nguyên toàn bộ, không bị cắt khi vượt budget (test end-to-end qua splitPromptIntoSections + PackPrompt với budget 100 token) — không sửa promptpacker.
- Opt-in qua profiles (Task-341 synergy): `conventions` thêm vào candidateSources của mọi profile trong `task-harness`, `bug-plan-harness`, `vibe-sprint` — flow YAML không fork giữa các project (test pin cả 3 flow + validate toàn pack). Default-set injection (mọi flow không profile) là follow-up vì đổi defaultContextSourceIDs sẽ phá golden fixtures hiện có — ghi nhận minh bạch.
- Tests: `runner/context_source_conventions_test.go` (5 signature: user>workspace merge đúng thứ bậc, AGENTS.md fallback, all-missing rỗng không lỗi, Tier-1 non-droppable end-to-end, flow-YAML invariant + validate). agentpack xanh; runner targeted xanh.

## 9. Definition of Done

- [x] Struct `conventionsSource` được triển khai đúng interface `ContextSource` (4 methods, Deterministic=true).
- [x] Đăng ký thành công vào `context_sources_builtin.go` (priority 0).
- [x] Logic phân tầng ưu tiên: User conventions > Workspace conventions > AGENTS.md fallback.
- [x] Trường hợp thiếu file không sinh lỗi và trả về section rỗng an toàn.
- [x] Section `conventions` được bảo vệ là Tier-1 không bị cắt tỉa trong Budget Packer (qua heading `## Context` → mandatory_doc, CP-23 R-1 — không đổi packer).
- [x] Phân bổ token của source được ghi nhận minh bạch trong `prompt_context_audit` (section đi qua pipeline packer hiện có).
- [x] Toàn bộ test cũ trong `runner` và `promptpacker` hoàn toàn untouched và green.
- [x] Bổ sung bài unit test bao phủ đầy đủ các kịch bản nạp file và phân cấp (ma trận [User ±, Workspace ±, AGENTS.md fallback, all-missing]).

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/runner/context_source_conventions_test.go`

```go
package runner

import "testing"

// Scenario: Nạp và ưu tiên quy ước cấp User trước quy ước cấp Workspace
func TestConventionsSource_UserBeatsWorkspace(t *testing.T) {}

// Scenario: Khi không có .flowpilot/conventions.md, source tự động fallback sang AGENTS.md
func TestConventionsSource_FallbackToAgentsMd(t *testing.T) {}

// Scenario: Khi cả 2 file đều vắng mặt -> trả về rỗng, không phát sinh lỗi
func TestConventionsSource_MissingFile_EmptyWithoutError(t *testing.T) {}

// Scenario: Section conventions không bị cắt tỉa trong Budget Packer dù vượt ngân sách
func TestConventionsSource_Tier1NonDroppableInBudgetPacker(t *testing.T) {}

// Scenario: Flow YAML giữ nguyên không đổi khi chạy qua các môi trường repository khác nhau
func TestConventionsSource_FlowYAMLInvariantAcrossProjects(t *testing.T) {}
```

---

## 11. Code Guide & Implementation Details

1. **Triển khai `conventionsSource` tuân thủ interface `ContextSource`**:
   ```go
   package runner

   type conventionsSource struct {
       priority int
   }

   func newConventionsSource(priority int) *conventionsSource {
       return &conventionsSource{priority: priority}
   }

   func (s *conventionsSource) ID() string {
       return "conventions"
   }

   func (s *conventionsSource) Priority() int {
       return s.priority
   }

   func (s *conventionsSource) Deterministic() bool {
       return true
   }

   func (s *conventionsSource) Fetch(ctx context.Context, hints FlowContextHints) (FlowContextSection, error) {
       var parts []string
       if home, err := os.UserHomeDir(); err == nil {
           userPath := filepath.Join(home, ".flowpilot", "conventions.md")
           if content, err := os.ReadFile(userPath); err == nil {
               parts = append(parts, string(content))
           }
       }
       if hints.WorkspaceCwd != "" {
           wsPath := filepath.Join(hints.WorkspaceCwd, ".flowpilot", "conventions.md")
           if content, err := os.ReadFile(wsPath); err == nil {
               parts = append(parts, string(content))
           } else {
               agentsPath := filepath.Join(hints.WorkspaceCwd, "AGENTS.md")
               if content, err := os.ReadFile(agentsPath); err == nil {
                   parts = append(parts, string(content))
               }
           }
       }
       return FlowContextSection{
           ID:      "conventions",
           Title:   "Project & Workspace Conventions",
           Content: strings.Join(parts, "\n\n---\n\n"),
       }, nil
   }
   ```
2. **Đăng ký trong `context_sources_builtin.go`**:
   ```go
   mustRegisterContextSource(r, newConventionsSource(PriorityTier1))
   ```

---

## 12. Safe-Fix Compliance Note

- **R1 (No regression)**: Tuyệt đối không chỉnh sửa các file test hiện có trong context registry (`context_source_registry_test.go`).
- **R2 (Three providers parity)**: Quy ước repo được nạp vào prompt dạng văn bản tĩnh thuần túy, hoàn toàn tương thích trên Claude, Codex, Grok.
- **R3 (Matrix coverage)**: Kiểm thử ma trận file tồn tại: `[User present, User absent] × [Workspace present, Workspace fallback to AGENTS.md, All absent]`.
- **History**: Kế thừa trực tiếp cấu trúc của `Task-191` (Context Source Interface).
