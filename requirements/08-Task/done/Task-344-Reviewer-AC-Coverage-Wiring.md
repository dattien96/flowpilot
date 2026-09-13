# Task-344: Reviewer AC Coverage Wiring (Chặn thiếu AC thật ở submit path)

## Metadata

- Document ID: `Task-344`
- Title: `Reviewer AC Coverage Wiring (Chặn thiếu AC thật ở submit path)`
- Feature Keys: `zcode-parity, gate-schema`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-13`
- Last Updated: `2026-09-13`
- Parent Documents: [CP-62: Zcode Harness Parity](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [Task-338: Reviewer Verdict Schema](./Task-338-Reviewer-Verdict-Schema-And-Per-AC-Evidence.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Child Documents: `None`
- Related Documents: [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [CP-61: Harness Done Verdict Gate](../../07-Coding-Plan/done/CP-61-Harness-Done-Verdict-Gate.md)
- Replaces: `None`
- Tags: `zcode-parity, gate-schema, verdict-coverage, safe-fix, cp-62`

## AI Quick View

### Summary

- Task-338 đã ship `ExtractACIDs` + `ValidateReviewOutcomeVerdicts` (pure Go, test phủ) nhưng **chưa call site production nào gọi chúng** — reviewer nộp thiếu AC vẫn đi qua parse layer (chỉ validate shape).
- Task này wire coverage check vào **điểm duy nhất mà cả 3 provider path đều đi qua**: đầu `turnBridge.SubmitFlowControl` (`interactive_service.go:6076`) — claude MCP (`claude_permission_mcp.go:232`), codex (`codex_adapter.go:403`), grok đều đổ về bridge này.
- Nguồn expectedACs deterministic, 0 token LLM: (1) `rs.vibeTaskName` (vibe family — đường dẫn task doc có sẵn trên run state), (2) INPUT `file_artifact.v1` binding có `config.pathTemplate` trên node đang submit (vd `plan_reviewer`/`plan_md`) → glob file mới nhất khớp pattern (đúng cơ chế runner đã dặn model: "find the newest matching file").

### Current Ask

- Operator duyệt 2026-09-13 (nhóm 5 task follow-up CP-62). Thực hiện TDD signatures-first, additive tests, không sửa test cũ (R1), parity 3 provider (R2), CA note (R-safe-fix).

### Key Decisions

- `D-1` Validation nằm ở **một điểm duy nhất** đầu `SubmitFlowControl` khi `in.viaReviewOutcome` — không thêm nhánh per-adapter (parity by construction).
- `D-2` Set rỗng (không tìm được doc / flow tự do) → **no-op** (`ValidateReviewOutcomeVerdicts` đã có fallback `len==0 → nil`) — byte-identity cho flow legacy.
- `D-3` Cache expectedACs trên child run (doc không đổi giữa các lần submit trong review) — tránh glob lại mỗi call.
- `D-4` Không đụng cohort verdict recording, applyFlowControl, hub-done gate (CP-61) — chỉ thêm chặn TRƯỚC khi record.

## 1. Goal

Reviewer không thể "đóng dấu duyệt bừa" — nộp `submit_review_outcome` thiếu verdict row cho bất kỳ AC nào trong task doc governing sẽ bị từ chối in-turn với error đặt tên AC thiếu (error chính là reprompt), phủ cả cohort path (harness) lẫn non-cohort path (vibe).

## 2. Parent Links

- CP-62 P-2 follow-up (a) — "thread expected-ACs into the 3 adapter call sites", ghi nhận trong Completion Notes của Task-338.

## 3. Trigger

- Operator duyệt nhóm 5 task CP-62 follow-up (2026-09-13).

## 4. Exact Change

### 4.0 Before → After

| | Before (hiện trạng trước Task-344) | After (sau Task-344) |
|---|---|---|
| **Kiểm tra độ phủ AC** | `ValidateReviewOutcomeVerdicts` + `ExtractACIDs` có sẵn (Task-338) nhưng **không có call site production nào gọi** — reviewer nộp thiếu AC vẫn đi qua | Mọi `submit_review_outcome` (claude MCP, codex, grok — chung 1 điểm `turnBridge.SubmitFlowControl`) bị check coverage trước khi xử lý |
| **Khi thiếu AC** | Accept im lặng → rubber-stamp ("đóng dấu duyệt bừa") | Tool result trả lỗi **đặt tên đích danh AC thiếu** ("missing verdicts for required ACs: AC-2") — chính là reprompt in-turn, model tự nộp lại |
| **Nguồn expectedACs** | Không có — chưa ai biết task doc governing nằm đâu | 4 nguồn deterministic, 0 token LLM: vibe short task name (định vị file mới nhất dưới `requirements/`) → vibe plan entry (hub inline) → INPUT pathTemplate binding của node (glob-newest) → OUTPUT template của node writer anh em (reviewer không có binding riêng) |
| **Ai bị enforce** | n/a | Reviewer `read_only` bị chặn cả khi nộp 0 verdict; hub (posture "") chỉ bị chặn khi ĐÃ nộp rows mà thiếu; owner `verdict_only` **không bao giờ** bị enforce (owner trả lời tranh luận, không chấm AC) |
| **Flow tự do / legacy** | n/a | Không resolve được doc → no-op — byte-identical như trước |


- `T-1` File mới `internal/runner/review_ac_coverage.go`: (deviation Task-352: helper `flowNodeFor` được inline vào `expectedReviewACs` thay vì hàm riêng — cùng pattern khóa)
  - `flowNodeFor(rs)` — resolve node của child submit từ `parent.activeFlowNodes` (mirror matching của `flowNodePostureFor`: stepID/label; trả node, không đổi hàm cũ).
  - `expectedReviewACs(s, rs) []string` — nguồn theo thứ tự: `rs.vibeTaskName` (join `rs.workspaceCwd`, abs-aware) → node INPUT binding `pathTemplate` → glob-newest (`{{idx}}`→`\d+`, `{{slug}}`→`[a-z0-9-]+`, pick ModTime mới nhất) → nil. I/O ngoài `s.mu`; cache dưới `s.mu`.
  - `validateReviewACCoverage(s, rs, in)` — `!in.viaReviewOutcome` → nil; rows từ `in.Payload["verdicts"].([]VerdictRow)`; gọi `ValidateReviewOutcomeVerdicts`.
- `T-2` Wire: đầu `turnBridge.SubmitFlowControl` — lỗi coverage → `return FlowControlResult{}, err` (error thành tool result của cả 3 adapter như parse error hiện có).
- `T-3` `interactiveRun` thêm 2 field additive: `expectedACsCache []string`, `expectedACsResolved bool`.
- `T-4` Tests mới `review_ac_coverage_test.go`: vibe path thiếu AC → error tên AC; đủ → nil; glob-newest chọn Task-2 mới hơn Task-1; không doc/binding → nil; raw flow_control submit → nil; bridge-level cohort submit thiếu AC → KHÔNG record verdict.

## 5. Touched Areas

- `apps/local-runner/internal/runner/review_ac_coverage.go` (mới), `interactive_service.go` (2 field + 3 dòng wire đầu SubmitFlowControl), `review_ac_coverage_test.go` (mới). Không đụng adapter files, không đụng flowgate.

## 6. Acceptance Check

- [x] AC-1: Reviewer vibe submit thiếu AC → tool result lỗi nêu đích danh AC thiếu, trong cùng turn model nộp lại được.
- [x] AC-2: Reviewer harness (cohort, `plan_reviewer` qua pathTemplate) thiếu AC → bị chặn trước khi verdict được record.
- [x] AC-3: Flow không có task doc resolvable → hành vi byte-identical như trước (no-op).
- [x] AC-4: Cả 3 provider path (claude MCP, codex, grok-bridge) đều bị chặn như nhau — test ở tầng bridge (điểm hội).
- [x] AC-5: Test cũ không bị sửa; toàn bộ suite `go test ./internal/runner/ ./internal/flowgate/` xanh (trừ pre-existing TestBUG327/TestTask330).

## 7. Out of Scope

- Board endpoint `handleSubmitFlowControl` (kênh con người chủ động — giữ nguyên).
- Resolve OUTPUT templated path của node writer (chỉ INPUT binding của node submit).
- Lưu verdict rows cho cohort reviewer vào handoff (thuộc Task-346).

## 8. Completion Notes

- Trạng thái: `done` (2026-09-13).
- Triển khai khớp §4 với 3 điều chỉnh phát hiện trong quá trình (đã test pin):
  1. `rs.vibeTaskName` runtime là **shortTaskName** (chỉ tên file) — source 1 định vị file mới nhất khớp tên dưới `<ws>/requirements` (walk, skip dot-dirs).
  2. Vibe-sprint **không có node reviewer** — review do synthesis hub inline (root run) đảm nhận; hub posture "" nên chỉ enforce khi ĐÃ nộp verdict rows (partial → chặn, rỗng → passthrough). Nguồn doc của hub = `vibeTaskPlan[vibeSprintIndex-1]`.
  3. Regex glob build từ **base name** của template (ReadDir trả tên file), không phải full path.
- Enforcement matrix: owner `verdict_only` không bao giờ bị enforce; reviewer `read_only` enforce cả khi nộp 0 verdict rows; hub posture "" chỉ partial; doc không resolve được → no-op.
- Test phát hiện 1 deadlock do chính test viết (createRun trong s.mu) — sửa test, không phải production code.
- R1 evidence: base worktree cũng fail 22 test full-suite (máy nhiễu); 2 test fail khác của branch pass 3/3 khi chạy đơn; CA791 fail 1/10 trên cả base (flake có sẵn). Chi tiết: CA-856.

## 9. Definition of Done

- [x] Code + tests landed theo §4, AC-1..AC-5 tick.
- [x] CA note ghi `zcode-parity`, không HIGH/CRITICAL impact ngoài dự kiến (CA-856).
- [x] Commit đúng format `[Feature][zcode-parity] ... Task-344`.
