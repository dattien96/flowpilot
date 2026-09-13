# Task-342: Artifact Sprint-Handoff có Schema và Chuỗi Tiếp nhận giữa các Sprint

## Metadata

- Document ID: `Task-342`
- Title: `Artifact Sprint-Handoff có Schema và Chuỗi Tiếp nhận giữa các Sprint`
- Feature Keys: `zcode-parity, sprint-handoff`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-12`
- Parent Documents: [CP-62: Nâng cấp Harness học từ ZCode](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [CP-49: Reverse-Documentation](../../07-Coding-Plan/done/CP-49-Reverse-Documentation-And-Doc-Ingestion.md)
- Child Documents: `None`
- Related Documents: [Task-341: Context Profile](./Task-341-Per-Node-Context-Profile-And-Catalog-Tier.md), [Task-323: Vibe Sprint V2](../../08-Task/done/Task-323-Vibe-Sprint-V2-Parity.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `sprint-handoff, artifact-schema, vibe-mode, safe-fix, cp-62, p-6`

## AI Quick View

### Summary

- Hiện thực hóa Slice `P-6` của [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md): Tạo artifact có cấu trúc **`sprint_handoff.v1`** được ghi tự động bởi node `audit` (hub-inline, không phải coder) ở cuối mỗi lượt chạy `vibe-sprint`.
- Thiết lập chuỗi tiếp nhận (Chaining): Sprint kế tiếp tự động nạp `sprint_handoff.v1` từ sprint trước như một Context Source có độ ưu tiên cao trong Budget Packer, giúp các node `tdd` và `coder` nắm bắt chính xác *lý do (why)* và *các quyết định kỹ thuật* mà không làm tam sao thất bản.
- Áp dụng nguyên tắc **Hard Ceiling Rule (CP-49)**: Trường `why` và `decisions` chỉ được trích xuất từ các dữ kiện có thật (verdict rows từ P-2, kết quả tranh luận owner), tuyệt đối cấm mô hình tự bịa đặt.

### Current Ask

- Đăng ký định dạng artifact `sprint_handoff.v1` trong `apps/local-runner/internal/runner/artifact_type_registry.go`.
- Cấu hình node `audit` trong `flow-pack/flows/vibe-sprint.yaml` để xuất file vào `requirements/.flowpilot/vibe/handoffs/`.
- Cập nhật logic chuỗi sprint trong `runner/vibe_cp.go` để truyền artifact handoff sang sprint kế tiếp.

### Key Decisions

- `T-1` **Schema Artifact `sprint_handoff.v1`**:
  ```yaml
  sprint: int
  task: string             # path tới file Task
  done: [string]
  decisions:
    - what: string
      why: string
      alternatives: [string]
  open: [string]
  risks: [string]
  weakened_tests:          # rỗng nếu không có
    - { path: string, line: int, justification: string }
  ```
- `T-2` **Chỉ Node Audit Hub-Inline Được Quyền Ghi**: Node `coder` không được phép ghi đè handoff. Việc ghi được thực hiện tự động bởi hub khi hoàn thành sprint để đảm bảo tính khách quan.
- `T-3` **Ưu Tiên Nạp Trong Budget Packer**: Sprint kế tiếp dành riêng một slot ưu tiên cao trong Budget Packer cho handoff, nạp trước khi mở rộng đọc các file source code khác.
- `T-4` **Khả Năng Phục Hồi Khi Thiếu File**: Nếu sprint kế tiếp không tìm thấy file handoff (ví dụ chạy flow lẻ hoặc phiên bản cũ), hệ thống fallback về cách đọc truyền thống mà không block luồng.

### Constraints

- Tuân thủ nghiêm ngặt **safe-fix-contract**: Không làm gãy luồng thực thi của các flow `vibe-sprint` hiện hành.
- Tuyệt đối không lưu các quyết định không có căn cứ (anti-hallucination).

### Open Questions

- Không còn câu hỏi mở.

### Source Refs

- CP-62 §4 `P-6` (Sprint-handoff artifact).
- `apps/local-runner/internal/runner/vibe_cp.go`.
- `apps/local-runner/internal/runner/artifact_type_registry.go`.
- `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml`.

---

## 1. Goal

Lưu vết và chuyển giao liền mạch ngữ cảnh kỹ thuật, lý do đưa ra các quyết định kiến trúc và các rủi ro tồn đọng giữa các sprint trong Vibe Mode một cách xác định và có cấu trúc máy đọc được.

---

## 2. Parent Links

- Coding Plan: [CP-62 Slice P-6](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md)
- System Tech Design: [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md)
- Coding Plan: [CP-49: Reverse-Documentation](../../07-Coding-Plan/done/CP-49-Reverse-Documentation-And-Doc-Ingestion.md)

---

## 3. Trigger

Hiện tại giữa các sprint trong Vibe Mode, sprint kế tiếp chỉ đọc `chat.summary` hoặc `feature.history` dạng văn xuôi và cố gắng quét lại toàn bộ disk. Những quyết định kỹ thuật ngầm (như việc chọn cấu trúc thư mục, lý do bỏ qua một edge case) thường bị lãng quên hoặc bị sprint sau đảo ngược lại một cách vô thức.

---

## 4. Exact Change

- `T-1` **Đăng ký Artifact Type `sprint_handoff.v1`**:
  Trong `apps/local-runner/internal/runner/artifact_type_registry.go`, đăng ký type `sprint_handoff.v1` với các trường bắt buộc như `sprint`, `task`, `done`, `decisions`, `open`, `risks`.
- `T-2` **Cập nhật Node `audit` trong `vibe-sprint.yaml`**:
  Bổ sung `artifactBindings` khai báo output artifact `sprint_handoff.v1` được lưu vào thư mục `requirements/.flowpilot/vibe/handoffs/handoff-sprint-{n}.yaml`.
- `T-3` **Tích hợp tiêu thụ Handoff tại `runner/vibe_cp.go`**:
  Khi chuyển sprint $N \rightarrow N+1$, runner kiểm tra file handoff của sprint $N$. Nếu tồn tại, đính kèm vào Context Resolver của sprint $N+1$ với cờ `high_priority`.
- `T-4` **Cơ chế Chống Bịa Đặt**:
  Hàm trích xuất dữ liệu handoff chỉ tổng hợp từ các sự kiện đã kiểm chứng trong phiên (verdict rows, gate events, user card responses).

---

## 5. Touched Areas

- `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml`
- `apps/local-runner/internal/runner/vibe_cp.go`
- `apps/local-runner/internal/runner/artifact_type_registry.go`
- `apps/local-runner/internal/runner/sprint_handoff_test.go` (Mới)

---

## 6. Acceptance Check

- Chạy `go test ./internal/runner/ -run TestSprintHandoff_` đạt PASS 100%.
- Chạy thử mô phỏng 2 sprint liên tiếp:
  - Cuối Sprint 1, file `handoff-sprint-1.yaml` được tạo ra đúng schema.
  - Sang Sprint 2, node `tdd` và `coder` nhận được đầy đủ nội dung của `handoff-sprint-1.yaml` trong prompt context.
- Thử xóa file handoff: Sprint 2 vẫn chạy bình thường với fallback log cảnh báo (không crash).

---

## 7. Out of Scope

- Không áp dụng handoff cho các flow đơn bước không theo chuỗi sprint.
- Không cho phép chỉnh sửa thủ công file handoff từ giao diện trong khi đang chạy.

---

## 8. Completion Notes

- Trạng thái: `done` (2026-09-12).
- Triển khai: `runner/sprint_handoff.go` (MỚI) — `SprintHandoffV1` + `SprintDecision`/`WeakenedTest`, `emitSprintHandoff` (ghi từ VERIFIED state: `vibeSprintIndex`, `vibeTaskName`, DONE step nodes, `lastFlowVerdicts`), `previousSprintHandoffContext` (đọc handoff sprint trước cho entry prompt), `sprintHandoffPath` (`requirements/.flowpilot/vibe/handoffs/handoff-sprint-{N}.yaml`); `ArtifactTypeSprintHandoff = "sprint_handoff.v1"` đăng ký trong `artifact_type_registry.go`; `interactiveRun.lastFlowVerdicts` capture verdict rows typed từ `FlowControlInput.Payload["verdicts"]` (applyFlowControl, mọi status); chain: `maybeChainVibeSprint` emit handoff khi audit DONE (best-effort, không bao giờ block chain), `maybeStartNextVibeSprint` nối handoff sprint trước vào entry prompt (fail-soft).
- Anti-hallucination: decisions/risks CHỈ sinh từ verdict rows thật (fail/blocked → risks); không rows → không invented content (test pin).
- Deviation ghi nhận minh bạch: (a) file handoff là JSON — valid YAML 1.2 subset — để không thêm yaml dependency vào go.mod, schema field khớp CP-62 P-6; (b) handoff KHÔNG khai báo artifactBindings trong vibe-sprint.yaml mà viết imperative tại audit-done — lý do: r-artifact-output write contract hoạt động trên required OUTPUT binding và gate chạy mỗi turn, ghi imperative tránh race gate/ghi; type vẫn đăng ký trong registry layer (SD-23 D-2) để instance/artifact catalog nhận diện.
- Tests: `runner/sprint_handoff_test.go` (4 signature: write valid, next-sprint consume, missing fallback, evidence-only decisions). Vibe regression (Task-321/323/326/328/329 + vibe/precedence/isolation/profile patterns) + agentpack xanh.

## 9. Definition of Done

- [x] Artifact type `sprint_handoff.v1` được định nghĩa chuẩn xác trong `artifact_type_registry.go`.
- [x] Node `audit` của flow `vibe-sprint` xuất file handoff thành công (imperative write tại audit-DONE chain — deviation đã ghi nhận ở trên, không YAML binding để tránh race với r-artifact-output gate).
- [x] Sprint kế tiếp tự động nạp artifact handoff vào vị trí ưu tiên cao trong Context (entry prompt của sprint N mang handoff sprint N-1 nguyên văn).
- [x] Dữ liệu trường `why` chỉ được trích xuất từ bằng chứng có thật trong phiên chạy (verdict rows; test anti-invention pin).
- [x] Trường hợp thiếu file handoff kích hoạt fallback an toàn, không làm gián đoạn sprint.
- [x] Bộ test cũ về vibe sprint (`task321_vibe_cp_test.go`, `task323_vibe_sprint_v2_test.go`) hoàn toàn untouched và green.
- [x] Bổ sung unit test kiểm tra ghi file, nạp chuỗi và fallback.

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/runner/sprint_handoff_test.go`

```go
package runner

import "testing"

// Scenario: Node Audit ghi thành công file YAML handoff đúng định dạng sprint_handoff.v1
func TestSprintHandoff_AuditNodeWritesValidYaml(t *testing.T) {}

// Scenario: Sprint kế tiếp tự động nạp handoff của sprint trước như nguồn ưu tiên cao
func TestSprintHandoff_NextSprintConsumesAsHighPriority(t *testing.T) {}

// Scenario: Thiếu file handoff từ sprint trước -> kích hoạt fallback an toàn mà không dừng phiên
func TestSprintHandoff_MissingHandoff_GracefulFallback(t *testing.T) {}

// Scenario: Đảm bảo các quyết định ghi trong handoff được trích từ bằng chứng kiểm chứng (anti-hallucination)
func TestSprintHandoff_WhyFieldDerivedFromEvidenceOnly(t *testing.T) {}
```

---

## 11. Code Guide & Implementation Details

1. **Struct `SprintHandoffV1` trong Go**:
   ```go
   type SprintDecision struct {
       What         string   `yaml:"what" json:"what"`
       Why          string   `yaml:"why" json:"why"`
       Alternatives []string `yaml:"alternatives,omitempty" json:"alternatives,omitempty"`
   }

   type WeakenedTest struct {
       Path          string `yaml:"path" json:"path"`
       Line          int    `yaml:"line" json:"line"`
       Justification string `yaml:"justification" json:"justification"`
   }

   type SprintHandoffV1 struct {
       Sprint        int              `yaml:"sprint" json:"sprint"`
       Task          string           `yaml:"task" json:"task"`
       Done          []string         `yaml:"done" json:"done"`
       Decisions     []SprintDecision `yaml:"decisions" json:"decisions"`
       Open          []string         `yaml:"open" json:"open"`
       Risks         []string         `yaml:"risks" json:"risks"`
       WeakenedTests []WeakenedTest   `yaml:"weakened_tests,omitempty" json:"weakened_tests,omitempty"`
   }
   ```
2. **Logic ghi tại Hub Audit**:
   ```go
   func EmitSprintHandoff(ctx context.Context, sprintNum int, taskPath string, events []TurnEvent) error {
       handoff := BuildHandoffFromVerifiedEvents(sprintNum, taskPath, events)
       return SaveArtifact("sprint_handoff.v1", handoffPath, handoff)
   }
   ```

---

## 12. Safe-Fix Compliance Note

- **R1 (No regression)**: Giữ nguyên các test suite trong `task321_vibe_cp_test.go` và `task323_vibe_sprint_v2_test.go`.
- **R2 (Three providers parity)**: File handoff là dạng YAML chuẩn, hoàn toàn độc lập với LLM provider.
- **R3 (Matrix coverage)**: Kiểm thử các trường hợp: Sprint 1 tạo handoff hợp lệ, Sprint 2 đọc handoff, Handoff rỗng, Handoff bị lỗi đọc file.
- **History**: Tuân thủ nguyên tắc chống ảo giác và ranh giới nghiệp vụ từ `CP-49`.
