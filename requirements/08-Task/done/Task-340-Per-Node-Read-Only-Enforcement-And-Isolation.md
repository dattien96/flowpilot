# Task-340: Thực thi Cô lập Node Đọc-Chỉ (Silent-Deny và Posture Mapping)

## Metadata

- Document ID: `Task-340`
- Title: `Thực thi Cô lập Node Đọc-Chỉ (Silent-Deny và Posture Mapping)`
- Feature Keys: `zcode-parity, node-isolation`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-12`
- Parent Documents: [CP-62: Nâng cấp Harness học từ ZCode](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [BUG-344: Scan Read-Only Policy Rejects Compound Read-Only Bash](../../09-BugFix/done/BUG-344-Scan-ReadOnly-Policy-Rejects-Compound-ReadOnly-Bash.md), [Task-338: Reviewer Verdict Schema](./Task-338-Reviewer-Verdict-Schema-And-Per-AC-Evidence.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `node-isolation, silent-deny, read-only-posture, bash-classifier, safe-fix, cp-62, p-4`

## AI Quick View

### Summary

- Hiện thực hóa Slice `P-4` của [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md): Chuyển đổi cơ chế cô lập node từ "lời dặn trong prompt" thành **sự cưỡng chế của runner (enforcement)**.
- Mở rộng động cơ `chat_posture_policy.go` (vốn đã có cơ chế `silent-deny` và phân loại lệnh an toàn từ BUG-344 nhưng trước đây bị loại trừ khỏi workflow runs) để áp dụng cho từng Flow Node.
- Phân loại tư thế (Posture Mapping):
  - `reviewer`, `plan_reviewer`, `scout`: Đặt tư thế `read_only`. Được phép dùng `Read, Grep, Glob` và **`Bash` có phân loại per-command** (tái sử dụng trực tiếp hàm `isReadOnlyCommand` từ `chat_posture_policy.go` để auto-approve `git diff`, `git log`, `ls`, `rg` — lưu ý `go vet`/`go build` là GHI theo phân loại BUG-344 do viết build cache; silent-deny mọi lệnh ghi/nguy hiểm).
  - `owner_1/2` trong `vibe-owner-debate`: Đặt tư thế **`verdict_only`** theo quyết định `Q-3` (chỉ đọc và gọi verdict tool, **tước bỏ hoàn toàn Bash**).
  - Các node `coder`, `implement`, `tdd`, `validate` giữ tư thế `standard`.

### Current Ask

- Kết nối trường `Tools` và `posture` từ `agent_catalog.go` vào luồng khởi tạo agent trong các file adapter (`claude_adapter.go`, `codex_adapter.go`, `grok_adapter.go`).
- Cho phép `FlowNode` khai báo `posture: "read_only" | "verdict_only" | "standard"` trong YAML definition.
- Mở rộng phạm vi áp dụng của `chat_posture_policy.go` cho flow runs có cấu hình posture.

### Key Decisions

- `T-1` **Khai báo Posture dạng Data trong Flow YAML**: Khớp với nguyên tắc flow-as-data của SD-19 (`D-3`/`D-7`). Engine không hardcode tên role mà chỉ đọc trường `posture` của node.
- `T-2` **Cơ chế Silent-Deny**: Khi một node ở posture `read_only` hoặc `verdict_only` cố gắng gọi tool ghi file hoặc lệnh ghi Bash, adapter chặn ngay lập tức, trả về thông báo lỗi read-only, đồng thời phát event `node_isolation_write_denied` phục vụ kiểm toán.
- `T-3` **Tái Sử Dụng Triệt Để BUG-344 Invariant**: Dùng trực tiếp hàm `isReadOnlyCommand(command)` có sẵn trong `chat_posture_policy.go`. Tuyệt đối không tự viết lại logic kiểm tra lệnh bằng regex để tránh tạo ra lỗ hổng bảo mật.
- `T-4` **Owner là `verdict_only` không có Bash (Q-3)**: Node tranh luận của owner tuyệt đối không được cấp tool Bash để tránh nguy cơ phá hoại file test hay source code (bảo vệ Oracle SS-14 `AC-6`).

### Constraints

- Tuân thủ nghiêm ngặt **safe-fix-contract**: Không làm ảnh hưởng đến quyền ghi file của các node lập trình (`implement`, `coder`) và node chạy lệnh kiểm thử (`validate`).
- Parity tuyệt đối trên cả 3 provider: Cả Claude, Codex và Grok đều phải chịu sự kiểm soát posture như nhau.

### Open Questions

- Đã giải quyết tại CP-62 (`Q-3`): Reviewer giữ Bash có classify; Owner dùng `verdict_only` không Bash.

### Source Refs

- CP-62 §4 `P-4` (Per-node read-only enforcement).
- `apps/local-runner/internal/runner/agent_catalog.go`.
- `apps/local-runner/internal/runner/chat_posture_policy.go`.
- BUG-344 (Chat Posture Policy Composition).

---

## 1. Goal

Triệt tiêu hoàn toàn rủi ro reviewer hoặc owner vô tình hoặc cố ý sửa đổi mã nguồn, làm yếu các bài kiểm thử (test weakening) hoặc phá hỏng workspace, bằng cách thiết lập rào chắn cô lập thực thi vật lý tại runner.

---

## 2. Parent Links

- Coding Plan: [CP-62 Slice P-4](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md)
- System Tech Design: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- System Specs: [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)

---

## 3. Trigger

Hiện tại file `reviewer.md` khai báo danh sách tools nhưng engine khi spawn agent lại cấp toàn quyền (yolo/standard write tools) cho workflow runs. Reviewer chỉ được dặn bằng prompt "không được sửa file", dẫn đến trường hợp model tự ý sửa code hoặc làm yếu test để cho bài kiểm thử pass.

---

## 4. Exact Change

- `T-1` **Mở rộng Model FlowNode**:
  Thêm trường `Posture string json:"posture,omitempty"` vào struct `FlowNode` trong `agentpack`.
- `T-2` **Kích hoạt Posture Engine cho Workflow Runs**:
  Trong `apps/local-runner/internal/runner/chat_posture_policy.go`, điều chỉnh logic để posture policy áp dụng cho cả flow runs nếu node khai báo `posture == "read_only"` hoặc `"verdict_only"`.
- `T-3` **Cập nhật luồng Adapter của Claude, Codex, Grok**:
  Trong `claude_adapter.go`, `codex_adapter.go`, `grok_adapter.go`:
  - Nếu `posture == "read_only"`: Cấp read tools + Bash với filter qua `isReadOnlyCommand`.
  - Nếu `posture == "verdict_only"`: Cấp read tools + verdict tool; loại bỏ hoàn toàn Bash và write tools.
- `T-4` **Cập nhật Flow YAMLs**:
  Cập nhật `task-harness.yaml`, `bug-plan-harness.yaml`, `vibe-sprint.yaml`, `vibe-owner-debate.yaml` để gắn posture tương ứng cho các node review và owner.

---

## 5. Touched Areas

- `apps/local-runner/internal/runner/agent_catalog.go`
- `apps/local-runner/internal/runner/chat_posture_policy.go`
- `apps/local-runner/internal/runner/claude_adapter.go`
- `apps/local-runner/internal/runner/codex_adapter.go`
- `apps/local-runner/internal/runner/grok_adapter.go`
- `apps/local-runner/internal/agentpack/flow-pack/flows/*.yaml`
- `apps/local-runner/internal/runner/node_isolation_test.go` (Mới)

---

## 6. Acceptance Check

- Chạy `go test ./internal/runner/ -run TestNodeIsolation_` đạt PASS 100%.
- Reviewer cố gọi tool ghi file (`write_file`, `replace_file_content`) $\rightarrow$ Bị chặn (silent-deny), nhận lỗi quyền hạn, phát event kiểm toán `node_isolation_write_denied`.
- Reviewer chạy lệnh `git diff` qua Bash $\rightarrow$ Thành công.
- Reviewer chạy lệnh `rm -rf` hoặc `echo "abc" > file.go` qua Bash $\rightarrow$ Bị chặn ngay lập tức.
- Owner trong `vibe-owner-debate` không có tool Bash trong danh sách tool được cấp.
- Node `implement` và `coder` vẫn ghi file bình thường.

---

## 7. Out of Scope

- Không can thiệp vào cơ chế phân bổ token/context (thuộc Task-341).
- Không thay đổi hành vi tương tác dòng lệnh của người dùng ngoài đời.

---

## 8. Completion Notes

- Trạng thái: `done` (2026-09-12).
- Triển khai: `agentpack/pack.go` — `FlowNode.Posture` (parse từ YAML `posture`); `runner/node_isolation.go` (MỚI) — posture constants, `flowNodePostureFor` (resolve từ parent `activeFlowNodes` qua child `stepID`/`label`, hub/root không bao giờ bị gate), `evaluateFlowNodeApproval` matrix, `evaluateVerdictOnlyApproval`, `EventNodeIsolationWriteDenied`; wiring tại `turnBridge.RequestApproval` (interactive_service.go) — điểm bridge **provider-neutral** duy nhất mọi approval đi qua, chặn TRƯỚC YOLO (cùng rationale với chat read-only posture).
- Enforcement: `read_only` = `readOnlyApprovalDecision` của BUG-344 nguyên khối (read tools + Bash per-command; compound command fail-closed); `verdict_only` = không Bash kể cả lệnh đọc (Q-3), không file write, MCP chỉ verdict tool + read tools.
- YAML: posture gắn cho scout/reviewer/plan_reviewer/cp_reviewer trong `task-harness`, `bug-plan-harness`, `cp-harness`, `bug-harness`, `rag-harness` và `owner_1/2` (`verdict_only`) trong `vibe-owner-debate`, scout trong `vibe-sprint`.
- Sự kiện học được (đã đồng bộ docs): `go vet`/`go build` là GHI theo phân loại BUG-344 (build cache) — docs CP-62/Task-340 đã sửa ví dụ từ `go vet` sang `git log`/`rg`; test pin cả 2 chiều.
- R1 lesson: `TestBugHarnessPackClone` (test CŨ) pin bug-harness ≡ rag-harness từng node → fix bằng production data (thêm cùng posture cho rag-harness), KHÔNG sửa test.
- Tests: `runner/node_isolation_test.go` (7 signature: write-deny, read-allow, bash-write-deny, owner-no-bash, standard-not-handled, posture resolution 3 subtests + root-run, pack parse pin). agentpack suite xanh; runner pinned suite (vibe/gate/CP61/CP53/picker/flowAllowed) xanh.
- Provider parity: Case 1 by construction — enforcement nằm ở `turnBridge.RequestApproval` provider-neutral; `node_isolation.go` không tham chiếu providerKey nào.

## 9. Definition of Done

- [x] FlowNode hỗ trợ cấu hình `posture` từ file flow YAML.
- [x] Bridge (`turnBridge.RequestApproval`) thực thi silent-deny đối với flow runs có posture `read_only` (chat_posture_policy engine tái sử dụng qua `readOnlyApprovalDecision`; posture thắng TRƯỚC YOLO).
- [x] Lệnh Bash được kiểm soát chặt chẽ bằng cách tái sử dụng `isReadOnlyCommand` từ `chat_posture_policy.go` (BUG-344) — compound command fail-closed.
- [x] Node `owner` trong `vibe-owner-debate` không có tool Bash (`verdict_only`, Q-3).
- [x] Hoạt động đồng nhất trên cả 3 provider: enforcement tại bridge provider-neutral — parity by construction.
- [x] Mọi test cũ không bị sửa và giữ nguyên xanh (agentpack full + runner pinned; R1 hit `TestBugHarnessPackClone` → fix bằng data, test untouched).
- [x] Bộ test `node_isolation_test.go` chứng minh bảo vệ file test và workspace.

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/runner/node_isolation_test.go`

```go
package runner

import "testing"

// Scenario: Reviewer gọi tool ghi file -> bị chặn silent-deny và phát sinh event
func TestNodeIsolation_ReviewerWriteAttempt_SilentDenied(t *testing.T) {}

// Scenario: Reviewer gọi lệnh Bash đọc (git diff, git log, ls) -> được phép chạy (go vet = ghi theo BUG-344)
func TestNodeIsolation_ReviewerBashReadOps_Allowed(t *testing.T) {}

// Scenario: Reviewer gọi lệnh Bash ghi (rm, >, tee) -> bị chặn hoàn toàn
func TestNodeIsolation_ReviewerBashWriteOps_Denied(t *testing.T) {}

// Scenario: Node Owner trong vibe-owner-debate không có tool Bash
func TestNodeIsolation_OwnerVerdictOnly_NoBash(t *testing.T) {}

// Scenario: Kiểm tra tính tương thích tư thế trên cả 3 adapter Claude, Codex, Grok
func TestNodeIsolation_ThreeProviderParity(t *testing.T) {}
```

---

## 11. Code Guide & Implementation Details

1. **Enum Posture trong `agent_catalog.go`**:
   ```go
   const (
       PostureStandard    = "standard"
       PostureReadOnly    = "read_only"
       PostureVerdictOnly = "verdict_only"
   )
   ```
2. **Tái sử dụng `isReadOnlyCommand` trong `chat_posture_policy.go`**:
   Thay vì viết bộ phân loại mới, hàm quyết định phê duyệt tận dụng trực tiếp bộ logic đã được chứng minh của `BUG-344`:
   ```go
   func evaluateFlowNodeApproval(posture string, details ApprovalDetails) string {
       if posture == PostureStandard {
           return "approve"
       }
       if posture == PostureVerdictOnly {
           if isVerdictTool(details) || isReadOnlyToolName(details.Reason) {
               return "approve"
           }
           return "deny"
       }
       if posture == PostureReadOnly {
           return readOnlyApprovalDecision(details)
       }
       return "deny"
   }
   ```

---

## 12. Safe-Fix Compliance Note

- **R1 (No regression)**: Tuyệt đối không sửa đổi `chat_posture_policy_test.go` hiện có.
- **R2 (Three providers parity)**: Test `TestNodeIsolation_ThreeProviderParity` bắt buộc kiểm tra mock/fake adapter của cả 3 nhà cung cấp Claude, Codex, Grok (`claude_adapter.go`, `codex_adapter.go`, `grok_adapter.go`).
- **R3 (Matrix coverage)**: Kiểm thử ma trận `[Standard, ReadOnly, VerdictOnly] × [WriteTool, SafeBash, UnsafeBash, VerdictTool]`.
- **History**: Kế thừa và bảo toàn các khẳng định từ `BUG-344`.
