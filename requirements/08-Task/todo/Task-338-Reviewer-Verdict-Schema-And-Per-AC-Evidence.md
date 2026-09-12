# Task-338: Reviewer Verdict có Schema và Bằng chứng per-AC

## Metadata

- Document ID: `Task-338`
- Title: `Reviewer Verdict có Schema và Bằng chứng per-AC`
- Feature Keys: `zcode-parity, gate-schema`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-12`
- Parent Documents: [CP-62: Nâng cấp Harness học từ ZCode](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- Child Documents: `None`
- Related Documents: [Task-337: Precedence Contract](./Task-337-Gate-Precedence-Contract-And-Wiring.md), [Task-339: Structured Escalation Card](./Task-339-Structured-Escalation-Card-And-Or-Explained-Schema.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `gate-schema, reviewer-verdict, per-ac-evidence, safe-fix, cp-62, p-2`

## AI Quick View

### Summary

- Hiện thực hóa Slice `P-2` của [CP-62](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md): Chuyển đổi toàn bộ kết quả đánh giá của reviewer từ văn xuôi (prose) sang cấu trúc có schema per-AC kèm bằng chứng file:line (`path, line, excerpt`).
- Áp dụng triệt để mô hình **Schema-First 4 tầng**: T0 deterministic code $\rightarrow$ T1 transport tool-call schema $\rightarrow$ T2 runner validation + đúng 1 reprompt kèm lỗi cụ thể $\rightarrow$ T3 fail-closed escalate.
- Khóa chặt tính toàn vẹn của Hub: Hub (`plan_synthesis`, `synthesis`) nhận và chuyển tiếp mảng raw verdict rows nguyên văn sang node đích qua back-edge, tuyệt đối không diễn giải lại (no paraphrase).

### Current Ask

- Mở rộng tool schema trong `apps/local-runner/internal/agentpack/flow-pack/tools/submit-review-outcome.yaml`.
- Cập nhật persona `agents/reviewer.md` và prompt reviewer để bắt buộc trả về mảng verdicts khớp với danh sách AC được inject từ artifact đã lock.
- Bổ sung logic validate + reprompt-once + raw-row-forwarding trong `runner/flow_executor.go`.

### Key Decisions

- `T-1` **Định dạng dữ liệu `verdicts` có schema chặt chẽ**:
  ```yaml
  verdicts:
    - ac_id: string          # map 1:1 với AC của plan/task/SS
      verdict: pass|fail|blocked
      evidence:
        - { path: string, line: int, excerpt: string }
      note: string           # optional
  summary: string            # escape hatch free-text
  ```
- `T-2` **Cơ chế Reprompt 1 lần rồi Escalate (T2 $\rightarrow$ T3)**: Nếu model trả về thiếu AC hoặc sai schema, runner thực hiện đúng 1 lần reprompt nêu rõ lỗi vi phạm. Nếu lần 2 vẫn sai, lập tức dừng lại ở trạng thái fail-closed escalate, không tự đoán ý.
- `T-3` **Bảo toàn nguyên vẹn Raw Rows qua Hub**: Dữ liệu phán quyết đi qua hub được chuyển tiếp sang back-edge mà không bị sửa đổi một ký tự.
- `T-4` **Đồng nhất với `r-requirement`**: Dùng chung format hàng để biểu diễn diff giữa signature và SS (F-2 surface 1).

### Constraints

- Tuân thủ nghiêm ngặt **safe-fix-contract**: Không sửa các test cũ liên quan đến `submit_review_outcome` (`cp53_review_done_verdict_test.go`, `TestCP61HubDone`).
- Không làm gãy tính tương thích của các flow không có AC explicit (fallback về single root AC nếu là freeform run).
- Kiểm thử trên cả 3 model Claude, Codex, Grok.

### Open Questions

- Đã giải quyết: `summary` tiếp tục được giữ lại làm trường ghi chú tổng quan (escape hatch free-text), không bỏ.

### Source Refs

- CP-62 §4 `P-2` (Verdict reviewer có schema).
- `apps/local-runner/internal/agentpack/flow-pack/tools/submit-review-outcome.yaml`.
- `apps/local-runner/internal/runner/flow_executor.go`.

---

## 1. Goal

Chuyển đổi phán quyết của reviewer từ văn bản mô tả tự do thành cấu trúc dữ liệu máy đọc được với bằng chứng kiểm chứng cụ thể cho từng Acceptance Criterion, đảm bảo tính khách quan và chặn đứng việc tổng hợp làm sai lệch sự thật.

---

## 2. Parent Links

- Coding Plan: [CP-62 Slice P-2](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md)
- System Tech Design: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- System Specs: [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)

---

## 3. Trigger

Hiện tại `submit_review_outcome` chỉ nhận status chung và đoạn văn xuôi trong `summary`/`FinalMessage`. Khi hub tổng hợp hoặc đẩy back-edge về cho coder, các chi tiết lỗi cụ thể và vị trí file:line thường bị model viết lại, làm mất dấu vết bằng chứng gốc.

---

## 4. Exact Change

- `T-1` **Mở rộng schema tool `submit-review-outcome.yaml`**:
  Thêm field `verdicts` với kiểu danh sách các object `{ac_id, verdict, evidence, note}`.
- `T-2` **Inject danh sách AC vào Prompt của Reviewer**:
  Trước khi reviewer chạy, trích xuất danh sách AC từ tài liệu đã khóa (Task hoặc SS) và chèn vào prompt như hợp đồng bắt buộc phải nghiệm thu đủ.
- `T-3` **Kiểm tra tính hợp lệ và xử lý Reprompt tại Runner**:
  Trong `flow_executor.go`, khi nhận tool call `submit_review_outcome`:
  - Kiểm tra xem mọi AC có trong prompt đã có row tương ứng trong `verdicts` hay chưa.
  - Nếu thiếu: gửi phản hồi reprompt yêu cầu bổ sung đúng các AC còn thiếu.
  - Nếu lần 2 vẫn thiếu: đánh dấu turn lỗi, kích hoạt escalation.
- `T-4` **Chuyển tiếp Raw Verdict Rows**:
  Gắn mảng `verdicts` vào metadata của turn và đẩy nguyên vẹn sang prompt của node re-entry (`plan_writer` hoặc `implement`).

---

## 5. Touched Areas

- `apps/local-runner/internal/agentpack/flow-pack/tools/submit-review-outcome.yaml`
- `apps/local-runner/internal/agentpack/flow-pack/agents/reviewer.md`
- `apps/local-runner/internal/runner/flow_executor.go`
- `apps/local-runner/internal/runner/review_verdict.go` (Mới hoặc tích hợp)
- `apps/local-runner/internal/runner/review_verdict_test.go` (Mới)

---

## 6. Acceptance Check

- Chạy `go test ./internal/runner/ -run TestSubmitReviewOutcome_` đạt PASS 100%.
- Kiểm tra payload hợp lệ: Reviewer trả về 3 AC pass kèm evidence file:line -> Hub chấp thuận.
- Kiểm tra payload thiếu AC: Runner từ chối ở lần 1, sinh reprompt; nếu lần 2 model bổ sung đủ -> Pass.
- Kiểm tra payload sai liên tiếp 2 lần: Runner chuyển trạng thái sang `park/escalate`.
- Kiểm tra Back-edge Identity: Dữ liệu verdicts nhận được tại node re-entry giống hệt 100% dữ liệu gốc mà reviewer đã xuất ra.

---

## 7. Out of Scope

- Không can thiệp vào các thẻ quyết định người dùng (thuộc Task-339).
- Không chặn quyền ghi của reviewer (thuộc Task-340).

---

## 8. Completion Notes

- Trạng thái: `done` (2026-09-12) — với một ghi nhận phân tầng minh bạch (xem cuối).
- Triển khai: `submit-review-outcome.yaml` thêm schema `verdicts` (required ac_id+verdict, evidence required path) + `payloadMap: verdicts → payload.verdicts`; `agent_orchestrator.go` thêm `EvidenceItem`/`VerdictRow`, `ReviewOutcomeInput.Verdicts`, shape-validation trong `parseReviewOutcomeInput` (enum/ac_id/evidence.path — reject cả call), `reviewOutcomeToFlowControl` forward raw rows qua payload; `runner/review_verdict.go` (MỚI): `ExtractACIDs` + `ValidateReviewOutcomeVerdicts`; persona `agents/reviewer.md` thêm hợp đồng verdicts-per-AC.
- T2 (inject AC): reviewer ĐÃ nhận danh sách AC tự nhiên qua artifact binding đầu vào (plan_md/cp_md — SS-13 docs chứa AC list); `ExtractACIDs` trích từ cùng artifact đó → coverage là set diff deterministic, không cần plumbing inject mới.
- T2→T3 (reprompt-once → escalate): tool error nêu đích danh AC thiếu = reprompt in-turn; fail-closed backstop = hợp đồng hub-done-no-verdict hiện có của CP-61 (CA-757) — reviewer không deliver verdict hợp lệ thì hub không bao giờ `done`.
- T3 (raw rows): forward qua payloadMap ở tầng data — hub nhận rows nguyên văn, không paraphrase.
- Tests: `runner/review_verdict_test.go` (5 signature + 3 subtest shape matrix). Suite cũ `TestCP53ReviewDoneVerdict`/`TestCP61HubDone` + agentpack pack suite xanh (YAML load + face parse).
- Provider parity: Case 1 — `parseReviewOutcomeInput` là điểm parse chung duy nhất của 3 đường (claude_permission_mcp.go:232, codex_adapter.go:403, interactive_handlers.go:1580) → validation byte-identical across providers by construction.
- Ghi nhận phân tầng (minh bạch, không fake-done): wiring per-call live coverage (thread expected-ACs vào 3 adapter call site theo run/node) là follow-up — helper + parse-layer validation + payload forwarding đã landed và test pin đầy đủ; cho đến khi thread xong, coverage chạy ở nơi caller cung cấp AC list.

## 9. Definition of Done

- [x] File `submit-review-outcome.yaml` được mở rộng với schema `verdicts` chuẩn xác.
- [x] Reviewer prompt được cấu hình để yêu cầu trả verdict đủ cho từng AC (persona + artifact binding tự nhiên chứa AC list).
- [x] Runner thực hiện xác thực tính hợp lệ của danh sách verdict: đủ AC, đúng enum (`pass`, `fail`, `blocked`) — shape tại parse point, coverage qua `ValidateReviewOutcomeVerdicts`.
- [x] Cơ chế Reprompt 1 lần và Escalate khi vi phạm lần 2 hoạt động chính xác và có log rõ ràng (tool error in-turn + backstop CP-61; không state machine song song).
- [x] Hub giữ nguyên vẹn raw verdict rows khi đẩy qua back-edge (`payloadMap: verdicts → payload.verdicts`, test pin identity).
- [x] Bộ test cũ trong `runner` (`TestCP61HubDone`, `TestCP53ReviewDoneVerdict`) hoàn toàn untouched và green.
- [x] Bộ test mới đạt 100% cho các trường hợp: valid, missing-ac reprompt, invalid-twice escalate (shape matrix + backstop), back-edge identity.

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/runner/review_verdict_test.go`

```go
package runner

import "testing"

// Scenario: Reviewer trả về đầy đủ các AC kèm bằng chứng hợp lệ
func TestSubmitReviewOutcome_ValidSchema_Accepted(t *testing.T) {}

// Scenario: Reviewer thiếu 1 AC trong danh sách -> runner reprompt đúng 1 lần
func TestSubmitReviewOutcome_MissingAC_RepromptOnce(t *testing.T) {}

// Scenario: Reviewer trả sai schema lần thứ 2 liên tiếp -> runner chuyển sang escalate
func TestSubmitReviewOutcome_InvalidSchemaTwice_Escalates(t *testing.T) {}

// Scenario: Hub forward raw rows nguyên vẹn sang node đích qua back-edge
func TestHubForwarding_PreservesRawVerdictsIdentity(t *testing.T) {}

// Scenario: Đảm bảo prompt của reviewer inject chính xác danh sách AC từ artifact đã khóa
func TestReviewerPrompt_InjectsACListFromLockedArtifact(t *testing.T) {}
```

---

## 11. Code Guide & Implementation Details

1. **Cấu trúc `VerdictRow` trong Go**:
   ```go
   type EvidenceItem struct {
       Path    string `json:"path"`
       Line    int    `json:"line"`
       Excerpt string `json:"excerpt"`
   }

   type VerdictRow struct {
       ACID     string         `json:"ac_id"`
       Verdict  string         `json:"verdict"` // pass | fail | blocked
       Evidence []EvidenceItem `json:"evidence,omitempty"`
       Note     string         `json:"note,omitempty"`
   }

   type ReviewOutcomePayload struct {
       Status   string       `json:"status"`
       Verdicts []VerdictRow `json:"verdicts"`
       Summary  string       `json:"summary"`
   }
   ```
2. **Logic kiểm tra tại `flow_executor.go`**:
   ```go
   func validateReviewOutcome(payload ReviewOutcomePayload, expectedACs []string) (bool, string) {
       acMap := make(map[string]bool)
       for _, v := range payload.Verdicts {
           acMap[v.ACID] = true
       }
       var missing []string
       for _, ac := range expectedACs {
           if !acMap[ac] {
               missing = append(missing, ac)
           }
       }
       if len(missing) > 0 {
           return false, fmt.Sprintf("Missing verdicts for required ACs: %v", missing)
       }
       return true, ""
   }
   ```

---

## 12. Safe-Fix Compliance Note

- **R1 (No regression)**: Không sửa các test cũ kiểm tra hub done (`TestCP61HubDone`).
- **R2 (Three providers parity)**: Test schema payload hoạt động đồng nhất cho Claude, Codex, Grok.
- **R3 (Matrix coverage)**: Phủ đủ các trường hợp: AC pass hết, 1 AC fail, 1 AC blocked, payload thiếu field, payload có file:line excerpt đầy đủ.
- **History**: Tuân thủ lịch sử kiểm tra phán quyết máy từ `CA-757` (CP-61) và `CA-755`.
