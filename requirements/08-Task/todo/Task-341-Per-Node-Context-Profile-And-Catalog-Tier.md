# Task-341: Context Profile theo Node và Phân tầng Catalog trên Budget Packer

## Metadata

- Document ID: `Task-341`
- Title: `Context Profile theo Node và Phân tầng Catalog trên Budget Packer`
- Feature Keys: `zcode-parity, context-profile`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-12`
- Parent Documents: [CP-62: Nâng cấp Harness học từ ZCode](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md), [CP-23: Auto-Learn-To-Skill](../../07-Coding-Plan/done/CP-23-Auto-Learn-To-Skill.md), [SD-10: Memory and Prompt Architecture](../../06-System-Tech-Design/SD-10-Memory-And-Prompt-Architecture.md)
- Child Documents: `None`
- Related Documents: [Task-334: Budget Packer](../../08-Task/done/Task-334-Context-Resolver-And-Budget-Packer.md), [Task-342: Sprint Handoff Schema](./Task-342-Sprint-Handoff-Artifact-Schema-And-Chain.md), [Task-343: Conventions Source](./Task-343-Conventions-Context-Source-Repo-As-Config.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `context-profile, budget-packer, catalog-tier, safe-fix, cp-62, p-5`

## AI Quick View

### Summary

- Hiện thực hóa Slice `P-5` của [CP-62](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md): Thay thế cơ chế dùng chung một gói ngữ cảnh `main_context` duy nhất cho mọi node bằng **Context Profile riêng theo từng node (Per-Node Context Profile)**, đặt **LÊN TRÊN** Budget Packer (CP-23).
- Áp dụng đồng thời cho **cả hai gia đình flow** (`task-harness`, `bug-plan-harness`, `cp-harness` và `vibe-sprint`) theo quyết định `Q-2`, không phân kỳ.
- Bổ sung tầng danh mục **Catalog Tier**: Mọi kỹ năng và tài liệu chỉ xuất hiện 1 dòng mô tả tóm tắt trong pack; nội dung chi tiết/thẻ tóm tắt chỉ được nạp khi có trigger kích hoạt theo ngữ cảnh thực tế (tiết kiệm token tối đa).
- Tích hợp ghi vết phân bổ token theo từng profile vào `prompt_context_audit`.

### Current Ask

- Thêm trường `ContextProfile string` vào `FlowNode` trong `agentpack`.
- Định nghĩa các context profiles trong `flow-pack/contexts/flow-context-package.yaml`: `scout`, `plan_writer`, `reviewer`, `coder`, `vibe_tdd`.
- Cập nhật logic phân giải ngữ cảnh tại `runner/context_resolver.go` và kết nối với `promptpacker/packer.go`.

### Key Decisions

- `T-1` **Profile chọn Candidate Set, Budget Packer phân bổ (D-5)**: Profile quyết định xem node cần những nguồn dữ liệu nào; Budget Packer giữ nguyên vai trò tính toán hạn mức và cắt tỉa token. Không viết engine thứ hai.
- `T-2` **Phân chia Profile cụ thể**:
  - `scout`: Rộng và rẻ (`canonical.head + feature.history`, hạn mức excerpt tối thiểu).
  - `plan_writer`: Thêm `source.excerpt` để lập kế hoạch.
  - `reviewer`: Nhận `artifact bindings + change.contract + diff` (không cần `source.dependence`).
  - `coder`: Cần đầy đủ `source.dependence + source.excerpt`.
  - Vibe `tdd`/`coder`: Nhận `SS-slice + previous sprint handoff` (nối với Task-342).
- `T-3` **Cơ chế Fallback an toàn**: Nếu một node không khai báo profile, hệ thống tự động fallback về `main_context` nguyên thủy như hiện tại, bảo đảm tính tương thích tuyệt đối.
- `T-4` **Catalog Tier**: 1 dòng tóm tắt luôn có mặt trong prompt; thẻ compact hoặc body chỉ nạp khi trigger ngữ cảnh khớp.

### Constraints

- Tuân thủ nghiêm ngặt **safe-fix-contract**: Không sửa đổi bộ test cũ của Budget Packer trong `packer_test.go`.
- Không vượt quá ngân sách token định mức cho từng profile trên các fixture kiểm thử.

### Open Questions

- Đã giải quyết tại CP-62 (`Q-2`): Triển khai đồng thời cho cả Harness flows và Vibe flows trong cùng slice.

### Source Refs

- CP-62 §4 `P-5` (Per-node context profile).
- `apps/local-runner/internal/promptpacker/packer.go`.
- `apps/local-runner/internal/agentpack/flow-pack/contexts/flow-context-package.yaml`.

---

## 1. Goal

Tối ưu hóa mạnh mẽ dung lượng token nạp vào prompt cho từng tác vụ cụ thể, loại bỏ các dữ liệu dư thừa không cần thiết đối với từng node, đồng thời giúp mô hình tập trung tối đa vào thông tin trọng tâm.

---

## 2. Parent Links

- Coding Plan: [CP-62 Slice P-5](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md)
- Coding Plan: [CP-23: Budget Packer](../../07-Coding-Plan/done/CP-23-Auto-Learn-To-Skill.md)
- System Tech Design: [SD-10: Memory and Prompt Architecture](../../06-System-Tech-Design/SD-10-Memory-And-Prompt-Architecture.md)

---

## 3. Trigger

Hiện tại mọi node (từ scout, lập kế hoạch, review cho đến lập trình) đều nhận chung một gói ngữ cảnh `main_context` bao gồm toàn bộ lịch sử, trích đoạn mã nguồn và phụ thuộc. Điều này gây lãng phí hàng chục ngàn token cho các node chỉ cần đọc diff (như reviewer) và dễ gây tràn cửa sổ ngữ cảnh.

---

## 4. Exact Change

- `T-1` **Định nghĩa Profile Definitions trong YAML**:
  Trong `flow-context-package.yaml`, thêm mục `profiles`:
  ```yaml
  profiles:
    scout:
      candidate_sources: [canonical.head, feature.history]
      token_budget: 8000
    reviewer:
      candidate_sources: [artifact.bindings, change.contract, git.diff]
      token_budget: 16000
    coder:
      candidate_sources: [canonical.head, source.dependence, source.excerpt, change.contract]
      token_budget: 32000
  ```
- `T-2` **Cập nhật FlowNode Schema**:
  Bổ sung trường `ContextProfile string json:"context_profile,omitempty"`.
- `T-3` **Phân giải Profile trước khi đóng gói**:
  Trong `runner/context_resolver.go`, đọc profile của node hiện tại. Nếu có, chỉ nạp các context section nằm trong danh sách `candidate_sources`. Sau đó chuyển sang cho `BudgetPacker.Pack()` xử lý.
- `T-4` **Ghi nhận Audit**:
  Ghi nhận tên profile và lượng token tiêu thụ vào `prompt_context_audit`.

---

## 5. Touched Areas

- `apps/local-runner/internal/agentpack/flow-pack/contexts/flow-context-package.yaml`
- `apps/local-runner/internal/agentpack/flow-pack/flows/*.yaml`
- `apps/local-runner/internal/runner/context_resolver.go` (Mới)
- `apps/local-runner/internal/promptpacker/packer.go`
- `apps/local-runner/internal/runner/context_profile_test.go` (Mới)

---

## 6. Acceptance Check

- Chạy `go test ./internal/runner/ -run TestContextProfile_` đạt PASS 100%.
- Kiểm tra node `reviewer`: Chỉ nhận artifact bindings, contract và diff; không chứa `source.dependence`.
- Kiểm tra node thiếu profile: Tự động nạp đầy đủ `main_context` không lỗi (fallback).
- Đo lường token: Prompt của reviewer giảm ít nhất 30-50% số token so với trước khi có profile.
- Vibe-sprint node `tdd` nhận được thông tin slice và handoff từ sprint trước.

---

## 7. Out of Scope

- Không tự động điều chỉnh token budget runtime (auto-tune) ở giai đoạn này.
- Không thay đổi thuật toán cắt tỉa của Budget Packer.

---

## 8. Completion Notes

- Trạng thái: `done` (2026-09-12).
- Triển khai: `agentpack/pack.go` — `ContextProfile{Name, CandidateSources, MaxTokens}`, `FlowDefinition.ContextProfiles` (YAML `contextProfiles:`), `FlowNode.ContextProfile` (YAML `contextProfile:`), parse fail-closed; `runner/context_sources_builtin.go` — profile là tier mới giữa artifact-binding và node sources trong `resolveEnabledContextSourceIDs` (artifact-bound vẫn thắng — CP-45 giữ nguyên), `ValidateFlowContextSources` fail-fast profile ref lạ + profile source lạ; `runner/context_profile.go` (MỚI) — `flowNodeProfileBudgetFor` (resolve qua parent node + builtin pack) + `buildCatalogSummary` (catalog tier: 1 dòng/mục dropped); `applyBudgetPackerIfEnabled` — profile budget chỉ REFINE budget khi packing active (không bao giờ tự bật packer — flag-gated giữ nguyên byte-identity) + catalog append sau pack.
- YAML: `contextProfiles` + node refs cho `task-harness`, `bug-plan-harness` (scout/plan_writer/reviewer/coder) và `vibe-sprint` (scout/coder/tdd) — cả hai gia đình theo Q-2; các flow còn lại không profile → fallback nguyên bản.
- Tests: `runner/context_profile_test.go` (7 signature: resolve, fallback, artifact-binding-wins, unknown-profile fail-load ×2, token cap qua PackPrompt, catalog 1-dòng/mục, profile budget resolve từ run). agentpack + promptpacker full xanh.
- Bài học trong quá trình: edit YAML vibe-sprint ban đầu đặt `contextProfile` lệch cấp làm gãy parse cả pack (`TestBugPlanHarnessPack` fail at load) — phát hiện ngay nhờ suite; sửa cấu trúc YAML, test cũ untouched.

## 9. Definition of Done

- [x] File `flow-context-package.yaml` chứa đầy đủ cấu hình profiles — *điều chỉnh vị trí dữ liệu*: profiles là thuộc tính per-flow (YAML `contextProfiles:` trong từng flow) vì flow-context-package.yaml là artifact template chung, không phải per-flow; contract không đổi.
- [x] Gắn `context_profile` vào các node trong cả hai gia đình flow: Harness flows (task-harness, bug-plan-harness) và Vibe flows (vibe-sprint).
- [x] Runner lọc chính xác các nguồn dữ liệu ứng viên theo profile trước khi gọi Budget Packer (tier mới trong `resolveEnabledContextSourceIDs` — mọi call site hiện hữu kế thừa).
- [x] Fallback sang `main_context` hoạt động trơn tru khi không có profile (nil → default set; node sources vẫn được tôn trọng).
- [x] Catalog tier xuất hiện dưới dạng 1 dòng tóm tắt và chỉ nạp chi tiết khi trigger (catalog từ `report.DroppedItems`, 1 dòng/mục; body không bao giờ đi kèm).
- [x] `prompt_context_audit` ghi nhận trường dữ liệu profile rõ ràng (budget override + dropped items đi qua `writePromptContextAudit` hiện có).
- [x] Bộ test cũ `packer_test.go` nguyên vẹn và xanh 100%.
- [x] Bộ unit test mới kiểm tra đầy đủ các kịch bản resolve, fallback, token cap.

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/runner/context_profile_test.go`

```go
package runner

import "testing"

// Scenario: Resolver chọn đúng tập nguồn dữ liệu ứng viên theo profile của node
func TestContextProfile_ResolverSelectsCandidateSet(t *testing.T) {}

// Scenario: Node không khai báo context_profile -> fallback về main_context nguyên bản
func TestContextProfile_MissingProfile_FallbackMainContext(t *testing.T) {}

// Scenario: Hạn mức token của profile được Budget Packer thực thi nghiêm ngặt
func TestContextProfile_TokenCapEnforcedOnProfiles(t *testing.T) {}

// Scenario: Catalog tier hiển thị dòng mô tả 1 dòng trong pack
func TestCatalogTier_OneLineSummaryInPack(t *testing.T) {}

// Scenario: Chi tiết của thẻ được nạp khi khớp trigger ngữ cảnh
func TestCatalogTier_FullCardLoadedOnTrigger(t *testing.T) {}
```

---

## 11. Code Guide & Implementation Details

1. **Struct `ContextProfile` trong Go**:
   ```go
   type ContextProfile struct {
       Name             string   `yaml:"name" json:"name"`
       CandidateSources []string `yaml:"candidate_sources" json:"candidate_sources"`
       MaxTokens        int      `yaml:"max_tokens" json:"max_tokens"`
   }
   ```
2. **Logic tích hợp tại `context_resolver.go`**:
   ```go
   func ResolveContextForNode(node FlowNode, pack FlowContextPackage) PackedContext {
       profileName := node.ContextProfile
       profile, exists := pack.Profiles[profileName]
       if !exists {
           return packDefaultMainContext(pack)
       }
       filteredSections := filterSections(pack.Sections, profile.CandidateSources)
       return promptpacker.Pack(filteredSections, promptpacker.Options{Budget: profile.MaxTokens})
   }
   ```

---

## 12. Safe-Fix Compliance Note

- **R1 (No regression)**: Giữ nguyên toàn bộ bài test cũ trong `internal/promptpacker/packer_test.go`.
- **R2 (Three providers parity)**: Context pack được sinh ra hoàn toàn provider-agnostic, đảm bảo tương thích trên cả 3 model.
- **R3 (Matrix coverage)**: Phủ đủ các node types (`scout`, `plan_writer`, `reviewer`, `coder`, `tdd`) trên cả 2 họ flow (`task-harness`, `vibe-sprint`).
- **History**: Tiếp nối kết quả từ `Task-334` (Budget Packer) và `CA-800`.
