# Task-337: Hợp đồng Precedence giữa các hệ thống Gate và Kết nối Runner

## Metadata

- Document ID: `Task-337`
- Title: `Hợp đồng Precedence giữa các hệ thống Gate và Kết nối Runner`
- Feature Keys: `zcode-parity, gate-precedence`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-12`
- Parent Documents: [CP-62: Nâng cấp Harness học từ ZCode](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md)
- Child Documents: `None`
- Related Documents: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [Task-335: Drift Detector](../../08-Task/done/Task-335-Drift-Wrong-Way-Detector-And-Correction-Ladder.md), [Task-331: r-dod-complete](../../08-Task/done/Task-331-DOD-Complete-Gate-And-Runner-Wiring.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `gate-precedence, flowgate, vibe-gate, safe-fix, cp-62, p-1`

## AI Quick View

### Summary

- Hiện thực hóa Slice `P-1` của [CP-62](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md): thiết lập bảng thứ bậc ưu tiên (Precedence Table) rõ ràng giữa 4 hệ thống kiểm soát: (1) Budget Packer, (2) Drift Ladder, (3) Vibe Owner-Debate, và (4) Các Gate Rule (`r-dod`, `r-requirement`, `r-scope`).
- Xử lý xung đột trọng yếu: Điểm drift $\ge 80$ trong Vibe Mode phải được định tuyến qua `classifyVibeGate` (đưa vào Owner Debate nếu không phải vi phạm yêu cầu) thay vì bắn Dev Card thô; Drift không được thu hẹp context khi đang trong lượt tranh luận Owner; `r-dod-complete` đánh giá trước `r-requirement`.
- Đảm bảo tính nguyên vẹn (invariance) của Dev Mode: Dev mode giữ nguyên 100% hành vi cũ (`AC-8`).

### Current Ask

- Thêm logic kiểm soát thứ tự ưu tiên trong `internal/flowgate/evaluate.go` và `runner/vibe_gate.go`.
- Cập nhật bảng hợp đồng Precedence vào tài liệu [SD-20](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md).
- Bổ sung bộ kiểm thử ma trận xác định `precedence_test.go` bảo đảm mọi kịch bản xung đột được phân giải chính xác.

### Key Decisions

- `T-1` **Quy tắc Requirement-Class luôn có độ ưu tiên tuyệt đối**: `r-requirement` hoặc bất kỳ quy tắc nào bị suy yếu thành `r-requirement` luôn do người dùng quyết định trực tiếp (`user-only`, tuân thủ `SS-18 BR-4`), không cho phép Owner tự giải quyết.
- `T-2` **Drift 80+ trong Vibe Mode chuyển hướng qua `classifyVibeGate`**: Khi `working_mode == "vibe"`, điểm drift $\ge 80$ kích hoạt logic phân loại vibe gate thay vì bật dev card thô.
- `T-3` **Không thu hẹp context trong turn Owner-Debate**: Khi node thuộc kiểu debate hoặc đang tranh luận với owner, engine bỏ qua cơ chế cắt giảm context của drift ladder để bảo toàn đầy đủ ngữ cảnh vi phạm cho cuộc tranh luận.
- `T-4` **Thứ tự đánh giá trên cùng turn done**: `r-dod-complete` (kiểm tra nghiệm thu hoàn tất) đánh giá trước `r-requirement`.
- `T-5` **Tuân thủ trạng thái Settle**: Phân định rành mạch `blocked-awaiting-user` $\ne$ `running` $\ne$ `failed` theo BUG-231/BUG-234.

### Constraints

- Tuân thủ nghiêm ngặt **safe-fix-contract**: Không sửa bất kỳ test cũ nào; nếu test cũ fail phải dừng và báo cáo ngay lập tức.
- Giữ vững tính bất biến của engine domain-free [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md): Thay đổi thuần túy là data + thứ tự wiring, không thêm role hay state tùy tiện vào engine core.
- Đảm bảo tính tương thích trên cả 3 provider: Claude, Codex, Grok.

### Open Questions

- Không còn open questions (đã giải quyết toàn bộ tại CP-62).

### Source Refs

- CP-62 §4 `P-1` (Precedence contract).
- SD-20 §3 (Rule semantics).
- SS-18 `BR-4`, `AC-8`.

---

## 1. Goal

Xác lập hợp đồng ưu tiên xác định (deterministic precedence contract) giữa các hệ thống cổng kiểm duyệt, ngăn chặn triệt để các tình huống xung đột (race/mâu thuẫn) giữa Budget Packer, Drift Ladder, Owner Debate và các cổng r-rules, đồng thời đảm bảo an toàn tuyệt đối cho dev mode.

---

## 2. Parent Links

- Coding Plan: [CP-62 Slice P-1](../../07-Coding-Plan/todo/CP-62-Zcode-Harness-Parity.md)
- System Tech Design: [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- System Specs: [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md)

---

## 3. Trigger

Khi kết hợp CP-23 (Drift & Budget Packer), CP-47 (DOD Gate) và CP-60 (Vibe Mode), xảy ra hiện tượng chồng chéo: Drift 80+ bắn ra Dev Card thô trong khi người dùng đang chạy Vibe Mode; hoặc Drift Ladder tự động cắt gọt prompt khiến Owner Debate thiếu dữ liệu tranh luận về vi phạm hợp đồng.

---

## 4. Exact Change

- `T-1` **Xây dựng Precedence Matrix trong `flowgate`**:
  Thêm cấu trúc giải quyết độ ưu tiên trong `apps/local-runner/internal/flowgate/evaluate.go`:
  ```text
  1. Requirement-Class Rules (r-requirement, SS-drift) -> Cao nhất (User-only)
  2. Drift 80+ in Vibe Mode -> classifyVibeGate (Owner Debate nếu non-requirement)
  3. r-dod-complete -> Đánh giá trước khi chốt hoàn thành turn
  4. Contract Rules (r-ca, r-tests, r-scope)
  5. Context-Reduction Ladder (Bỏ qua nếu đang chạy Owner-Debate)
  ```
- `T-2` **Tích hợp bộ điều hướng Vibe Gate (`runner/vibe_gate.go`)**:
  Cập nhật hàm `classifyVibeGate` để tiếp nhận tín hiệu `DriftScore >= 80`. Nếu vi phạm không thuộc nhóm thay đổi intent/SS, định tuyến sang lượt Owner Debate (`vibeGateOwnerDebate`) thay vì xuất thẻ dev card thô.
- `T-3` **Bảo toàn Context cho Turn Owner Debate**:
  Trong `runner/vibe_cp.go` và điểm gọi Budget Packer, kiểm tra nếu node hiện tại là `owner_debate` thì tắt cờ thu hẹp context của Drift Ladder.
- `T-4` **Cập nhật tài liệu SD-20**:
  Thêm bảng Precedence Table vào `requirements/06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md`.

---

## 5. Touched Areas

- `apps/local-runner/internal/flowgate/evaluate.go`
- `apps/local-runner/internal/flowgate/rules.go`
- `apps/local-runner/internal/flowgate/precedence_test.go` (Mới)
- `apps/local-runner/internal/runner/vibe_gate.go`
- `apps/local-runner/internal/runner/vibe_gate_precedence_test.go` (Mới)
- `requirements/06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md`

---

## 6. Acceptance Check

- Chạy `go test ./internal/flowgate/ -run TestPrecedence_` đạt PASS 100%.
- Kiểm thử ma trận `Mode × Gate × Drift`:
  - Ở `working_mode = dev`: Drift 80+ xuất hiện dev card như cũ; không có Owner Debate.
  - Ở `working_mode = vibe`: Drift 80+ chuyển sang `classifyVibeGate`, kích hoạt Owner Debate nếu không vi phạm requirement.
  - Khi có cả `r-dod-complete` và `r-requirement` vi phạm trên cùng turn, gate xử lý `r-dod-complete` trước để chốt trạng thái checklist, sau đó `r-requirement` chốt chặn người dùng.

---

## 7. Out of Scope

- Không thay đổi cấu trúc dữ liệu của `submit_review_outcome` (thuộc về Task-338).
- Không tạo tool thẻ hỏi người dùng mới (thuộc về Task-339).

---

## 8. Completion Notes

- Trạng thái: `done` (2026-09-12).
- Triển khai: `flowgate/precedence.go` (MỚI — tách file trong cùng package thay vì nhét vào `evaluate.go` cho gọn review): `ResolvePrecedence` (pure, 0-token) + `PrecedenceRouteUser/OwnerDebate/None` + `VibeDriftDebateThreshold=80` + `IsRequirementViolation`/`IsDODCompleteViolation`; settlement order T-4 (dod-complete trước, requirement sau) trong `PrecedenceResult.Ordered`.
- Runner wiring: `classifyVibeGateWithDrift` (legacy 2-arg `classifyVibeGate` giữ nguyên byte-stable — delegate với drift=0 để test cũ `vibe_gate_test.go` không phải sửa); `applyVibeGateResolver` đọc `latestVibeDriftScore` (từ `driftRunState.lastScore` mới); `applyVibeDriftOnlyResolver` gọi ở nhánh clean-gate của root gate (drift 80+ trên vibe hội đủ điều kiện escalate dù gate sạch — không bao giờ rơi vào pause path deferred của Task-335); `startVibeOwnerDebate` tách helper dùng chung.
- T-3: `stashVibeFlowForDebate` xóa pending ladder note/narrow; `recordDriftTelemetry` không stash ladder action khi run đang trong flow owner-debate.
- T-4: SD-20 §7 (bảng precedence + anchors) đã thêm.
- Tests: `flowgate/precedence_test.go` (9 test — 5 signature + 4 matrix edge) + `runner/vibe_gate_precedence_test.go` (7 test — classify wiring, requirement-beats-drift, drift-only, below-threshold legacy, lastScore read, stash-clears-ladder). Full flowgate suite xanh; agentpack xanh; runner full-suite chạy kèm trong CA-849 (failure set = pre-existing đã ghi nhận tại Task-331, không liên quan).
- Drift flag: mặc định OFF giữ nguyên (Task-335) — routing drift inert khi flag off, `latestVibeDriftScore` trả 0.
- Provider parity: provider-agnostic (pure Go routing, 0 LLM, không đụng adapter).
- GitNexus impact: classifyVibeGate (byte-stable delegate), applyVibeGateResolver (signature giữ nguyên, thêm drift nội bộ), driftRunState (+1 field), stashVibeFlowForDebate (thêm ladder drop) — tất cả LOW.

## 9. Definition of Done

- [x] Bảng Precedence được định nghĩa tường minh bằng mã Go trong `internal/flowgate/` (`precedence.go`, cùng package `evaluate.go`).
- [x] Tín hiệu Drift $\ge 80$ trong `vibe` mode được kết nối tới `classifyVibeGate` (qua `classifyVibeGateWithDrift` + `applyVibeDriftOnlyResolver` cho clean gate).
- [x] Bật cờ miễn trừ thu hẹp context (context pruning bypass) cho turn owner debate.
- [x] `r-dod-complete` được định tuyến đánh giá trước `r-requirement` trên cùng turn done (`PrecedenceResult.Ordered`, test pin `TestPrecedence_RDodCompleteEvaluatedBeforeRRequirement`).
- [x] Toàn bộ test cũ trong `flowgate` và `runner` giữ nguyên trạng thái xanh (untouched — flowgate full suite xanh; legacy `classifyVibeGate` byte-stable).
- [x] Bổ sung bộ test mới `precedence_test.go` + `vibe_gate_precedence_test.go` phủ ma trận `[Dev, Vibe] × [Drift < 80, Drift >= 80] × [Requirement, Non-Requirement]`.
- [x] Cập nhật SD-20 phản ánh hợp đồng Precedence (§7).

---

## 10. Test Signature Guide (TDD)

Tệp kiểm thử: `apps/local-runner/internal/flowgate/precedence_test.go`

```go
package flowgate

import "testing"

// Scenario: Requirement-class vi phạm thì luôn thắng mọi gate khác và chặn chờ user
func TestPrecedence_RequirementClassBeatsDriftAndDod(t *testing.T) {}

// Scenario: Trong Vibe mode, Drift score >= 80 định tuyến sang Owner Debate thay vì xuất dev card
func TestPrecedence_VibeDrift80RoutesToVibeGate(t *testing.T) {}

// Scenario: Trong Dev mode, Drift score >= 80 vẫn xuất hiện dev card bình thường (không bị phá vỡ)
func TestPrecedence_DevModeDrift80NonRegression(t *testing.T) {}

// Scenario: Turn Owner Debate không bị thu hẹp context dù drift score cao
func TestPrecedence_OwnerDebateBypassesContextReduction(t *testing.T) {}

// Scenario: Trên cùng một turn done, r-dod-complete được đánh giá trước r-requirement
func TestPrecedence_RDodCompleteEvaluatedBeforeRRequirement(t *testing.T) {}
```

Tệp kiểm thử wiring (runner): `apps/local-runner/internal/runner/vibe_gate_precedence_test.go`

```go
package runner

import "testing"

// Scenario: Wiring runner — classifyVibeGate nhận drift >= 80 non-requirement trên run vibe -> vibeGateOwnerDebate
func TestVibeGatePrecedence_Drift80RoutesToOwnerDebate(t *testing.T) {}

// Scenario: Wiring runner — dev mode không bao giờ vào nhánh owner debate dù drift cao
func TestVibeGatePrecedence_DevModeNeverRoutesToOwnerDebate(t *testing.T) {}
```

---

## 11. Code Guide & Implementation Details

1. **Trong `flowgate/evaluate.go`**:
   Định nghĩa hàm `ResolvePrecedence(violations []Violation, mode WorkingMode, isOwnerDebate bool) EvaluationResult`:
   - Lọc các vi phạm nhóm `RequirementClass`. Nếu có -> trả về ngay trạng thái `BlockedAwaitingUser`.
   - Nếu `mode == ModeVibe` và có tín hiệu drift $\ge 80$: kiểm tra nếu là non-requirement thì đánh dấu `RouteToVibeOwnerDebate`.
   - Nếu `isOwnerDebate == true`: cờ `AllowContextPruning = false`.
2. **Trong `runner/vibe_gate.go`**:
   Bổ sung nhánh xử lý khớp enum `vibeGateOwnerDebate`:
   ```go
   if driftScore >= 80 && !isRequirementViolation {
       return vibeGateOwnerDebate, nil
   }
   ```

---

## 12. Safe-Fix Compliance Note

- **R1 (No regression of the old suite)**: Tuyệt đối không chỉnh sửa các file test cũ (`contract_rules_test.go`, `r_dod_complete_test.go`, `r_requirement_test.go`, `driftdetect_test.go`). Nếu test cũ fail, lập tức dừng lại, báo cáo nguyên nhân và sửa đổi logic code mới.
- **R2 (Three providers parity)**: Xác minh tính tương thích trên cả 3 adapter (Claude, Codex, Grok). Vì logic Precedence nằm ở tầng pure Go engine (`flowgate`), cần đảm bảo không phụ thuộc vào định dạng riêng của bất kỳ model nào.
- **R3 (Matrix coverage)**: Bộ test mới bao phủ đủ ma trận `[Dev, Vibe] × [Drift < 80, Drift >= 80] × [Requirement, Non-Requirement]`.
- **History & Context Discipline**: Đọc kỹ lịch sử `CA-791`, `CA-793` trước khi điều chỉnh liên kết `vibe_gate.go`.
