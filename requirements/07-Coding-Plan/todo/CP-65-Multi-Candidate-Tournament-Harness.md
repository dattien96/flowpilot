# CP-65: Multi-Candidate Tournament & PDR Escalation Harness

## Metadata

- Document ID: `CP-65`
- Title: `Multi-Candidate Tournament & PDR Escalation Harness`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot Architecture`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-16`
- Parent Documents: [SS-19: Engineering Harness Flow Family](../../05-System-Specs/SS-19-Engineering-Harness-Flow-Family.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-24: Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md)
- Child Documents: [Task-368](../../08-Task/todo/Task-368-Tournament-Arbiter-Scoring-Engine.md) (P-1), [Task-369](../../08-Task/todo/Task-369-Worktree-Rollout-Manager.md) (P-2), [Task-370](../../08-Task/todo/Task-370-Tournament-Harness-Flow-Definition.md) (P-3), [Task-371](../../08-Task/todo/Task-371-Escalation-Fallback-Wiring-Review-Loop.md) (P-4), [Task-372](../../08-Task/todo/Task-372-Tournament-E2E-Verification.md) (P-5)
- Related Documents: [CP-36: Agent Review Loop](../done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [CP-62: Zcode Harness Parity](../done/CP-62-Zcode-Harness-Parity.md), [CP-63: IDE-Grade LSP Runtime](CP-63-IDE-Grade-LSP-Runtime.md), [CP-64: Reproduce-First TDD Gate](CP-64-Reproduce-First-TDD-Gate.md)
- Replaces: `None`
- Tags: `tournament, pdr, parallel-rollout, multi-candidate, escalation, arbiter, swarms`
- Feature Keys: `tournament-harness`
- Implementation Owner: `Claude Sonnet MAX`

---

## AI Quick View

### Summary

- Sequential retry loop khi gặp bài toán khó thường dẫn đến cognitive lock-in, context dilution và vướng trần lặp (`review_cap_exceeded`, `owner_debate_stalled`).
- Áp dụng kỹ thuật PDR (Parallel-Distill-Refine) & Tournament Voting: Chạy song song 2-3 nhánh độc lập (Claude, Codex, Grok), dùng trọng tài khách quan (Test Suite + LSP + GitNexus) để chấm điểm và tự động chọn phương án tốt nhất.
- Tích hợp theo 2 hướng: (1) Flow độc lập `tournament-harness.yaml` cho các bài toán hóc búa được chỉ định trước; (2) Chốt thoát hiểm (Escalation Fallback Leg) tự động kích hoạt khi các flow thông thường bị kẹt bế tắc.

### Current Ask

- Triển khai 5 slice công việc (P-1 đến P-5) xây dựng Tournament Arbiter, Worktree Rollout Isolation, Flow Topology và Escalation Wiring.

### Key Decisions

- `P-1` Trọng tài (Tournament Arbiter) hoạt động 100% bằng code Go xác định (deterministic), không dùng LLM để chấm điểm, tính theo công thức: 50% Test Pass Rate + 30% LSP Diagnostics + 20% GitNexus Blast Radius.
- `P-2` Mỗi candidate chạy trong một Git Worktree tạm thời (`.flowpilot/worktrees/candidate-<id>`), cô lập tuyệt đối file system giữa các ứng viên.
- `P-3` Khi review loop chạm trần cap, Runner tự động chuyển trạng thái sang `tournament_escalation` thay vì đánh dấu failed/stopped.
- `Retry` Tournament tối đa 2 attempts qua back-edge `tournament_arbiter` → `parallel_rollout` (`when: retry`) — cùng pattern vòng lặp `synthesis` → `coder` của review-loop; lần 2 GIỮ NGUYÊN candidate configs/model, chỉ spawn sub-agent mới (context sạch, `lifecycle: spawn` mới mỗi lượt) + brief chưng cất fail lần 1 do behavior đóng dấu; chỉ retry khi `auto_pick=true` và verdict hòa/tất cả đỏ (suy ra từ `NeedsHumanDecision`, không đổi P-1); merge-conflict → ra human ngay, không retry.

### Constraints

- Không kích hoạt Tournament mặc định cho các task thông thường để tránh tốn token (chỉ kích hoạt qua flow chuyên dụng hoặc khi chạm trần bế tắc).
- Additive tests only — không sửa test cũ.
- GitNexus impact analysis trước mỗi symbol edit.

### Open Questions

- `Q-1` — CHỐT (Task-370): có cờ config `auto_pick: true/false` trên node `tournament_arbiter` — false hoặc hòa → escalate `ask_user` kèm decision card ranking.

### Source Refs

- SS-19; SD-19; SD-24.
- DeepSeek-Coder PDR (Parallel-Distill-Refine) methodology.
- SWE-bench test-time scaling research.

---

## 1. Goal

Cung cấp cơ chế giải quyết bài toán khó và bế tắc tự động bằng mô hình Đấu trường ứng viên song song (Multi-Candidate Tournament), giải phóng hệ thống khỏi bẫy lặp tuần tự và tối ưu hóa tỷ lệ thành công của các task kỹ thuật phức tạp.

---

## 2. Input Documents

### 2.1 Governing documents
- SS-19: Engineering Harness Flow Family.
- SD-19: Agent Flow Engine.
- SD-24: Durable Turn Dispatch.

### 2.2 Implemented foundations
- CP-36: Multi-Agent Review Loop & Main Hub.
- CP-62: Node Isolation & Structured Decision Cards.
- CP-63: IDE-Grade LSP Runtime (cung cấp tín hiệu chấm điểm Compiler).

### 2.3 Current code locations
- `apps/local-runner/internal/runner/agent_orchestrator.go`: Điều phối vòng lặp agent.
- `apps/local-runner/internal/runner/provider_registry.go`: Quản lý các provider (Claude, Codex, Grok).
- `apps/local-runner/internal/flowgate/rules.go`: Luật cổng kiểm soát.

---

## 3. Implementation Strategy

### 3.1 Cấu trúc Đấu Trường (Tournament Architecture)
```text
Parent Hub Node (Tournament Controller)
  ├── Spawn Candidate A (Claude)  ──► Worktree A ──► Test & LSP
  ├── Spawn Candidate B (Codex)   ──► Worktree B ──► Test & LSP
  └── Arbiter Node (Go Code)
        ├── Thu thập điểm số 3 ứng viên
        ├── Chọn ứng viên đạt điểm cao nhất
        └── Merge Worktree thắng cuộc về Workspace chính
↩ retry → parallel_rollout khi hòa/tất cả đỏ (tối đa 2 attempts, cùng model spawn mới + brief chưng cất; merge-conflict thì ra human, không retry)
```

### 3.2 Công thức chấm điểm (Arbiter Formula)
$$\text{TotalScore} = (S_{\text{tests}} \times 0.5) + (S_{\text{lsp}} \times 0.3) + (S_{\text{blast}} \times 0.2)$$
- $S_{\text{tests}}$: Tỷ lệ test pass (bị phạt 0 điểm nếu gãy test cũ).
- $S_{\text{lsp}}$: 1.0 nếu 0 error, trừ điểm theo số lượng lỗi cú pháp.
- $S_{\text{blast}}$: 1.0 (Low, 0 dependents), 0.7 (Medium, 1–3), 0.3 (High, 4–10), 0.0 (Critical, >10) — ngưỡng chốt Task-368.

---

## 4. Work Breakdown

### P-1: Tournament Arbiter Scoring Engine

**Status: done** (Task-368, CA-876)

**Production changes**
- `internal/tournament/arbiter.go` (**new**): `TournamentArbiter` tính toán điểm số từ kết quả Test, LSP và GitNexus.
- `internal/tournament/types.go` (**new**): Định nghĩa `CandidateResult`, `CandidateScore`, `TournamentVerdict`.

**Test signatures**
```go
func TestArbiterPrefersCandidateWithHigherTestPassRate(t *testing.T)
func TestArbiterPenalizesLSPCompilerErrors(t *testing.T)
func TestArbiterPrefersLowerBlastRadiusOnTie(t *testing.T)
func TestArbiterDisqualifiesCandidateBreakingExistingTests(t *testing.T)
```

---

### P-2: Worktree Rollout Manager

**Status: draft**

**Production changes**
- `internal/tournament/worktree_manager.go` (**new**): Quản lý tạo, đồng bộ và dọn dẹp các Git Worktrees tạm thời cho các ứng viên chạy song song mà không xung đột file system.

**Test signatures**
```go
func TestWorktreeManagerCreatesIsolatedWorktrees(t *testing.T)
func TestWorktreeManagerCleansUpAfterTournament(t *testing.T)
func TestWorktreeManagerMergesWinningCandidate(t *testing.T)
```

---

### P-3: Flow Definition `tournament-harness.yaml`

**Status: draft**

**Production changes**
- `internal/agentpack/flow-pack/flows/tournament-harness.yaml` (**new**): Khai báo flow đấu trường độc lập với các node: `problem_scout` $\rightarrow$ `parallel_rollout` $\rightarrow$ `tournament_arbiter` $\rightarrow$ `merge_and_audit`, cộng back-edge `tournament_arbiter` $\rightarrow$ `parallel_rollout` (`when: retry`, tối đa 2 attempts — lần 2 giữ nguyên candidate configs, spawn sub-agent mới context sạch + brief chưng cất fail lần 1).
- `internal/agentpack/flow-pack/prompts/tournament-candidate.md` (**new**): prompt riêng cho candidate (tái dùng persona `agents/coder.md`).
- `internal/runner/tournament_behavior.go` (**new**): `behaviorTournamentArbiter` + `behaviorTournamentMerge` (thu metric test/LSP/dependents trong từng worktree, gọi Arbiter/WorktreeManager).
- `internal/runner/behavior_registry_builtin.go` + `behaviors/registry.yaml` (modified): đăng ký `tournament.arbiter` + `tournament.merge`; `manifest.yaml` đăng ký flow + prompt mới.

**Test signatures**
```go
func TestTournamentHarnessTopologyValid(t *testing.T)
func TestTournamentHarnessParsesCandidateConfigs(t *testing.T)
```

---

### P-4: Escalation Fallback Wiring trong Review Loop

**Status: draft**

**Production changes**
- `internal/runner/agent_orchestrator.go` (modified): vùng round-cap — khi flag bật và flow hiện tại không phải `tournament-harness`, set `LoopStatusTournamentEscalation` (const mới trong `flow_step_runtime.go`) + dispatch tournament.
- `internal/runner/interactive_service.go` (modified): helper `escalateToTournament(parentRunID, intent, frozen)` spawn run con kế thừa workspace/contract, chống đệ quy, theo dõi attempt (attempt/max_attempts trên run state; hết lượt → ask_user).
- `internal/runner/cohort_stall.go` + `internal/runner/vibe_debate.go` (modified): stall sweep/owner-fail path cùng điều kiện flag, cùng `escalateToTournament`; return path qua `applyFlowControl` hiện có.

**Test signatures**
```go
func TestReviewLoopTriggersTournamentOnCapExceeded(t *testing.T)
func TestVibeDebateTriggersTournamentOnStall(t *testing.T)
```

---

### P-5: E2E Tournament Verification

**Status: draft**

**Production changes**
- Viết test E2E mô phỏng 2 candidate giải cùng 1 bài toán, 1 candidate pass test sạch và được Arbiter tự động pick và merge thành công.

**Test signatures**
```go
func TestTournamentEndToEndWinnerSelectedAndMerged(t *testing.T)
func TestTournamentTieRequiresHumanDecision(t *testing.T)
```

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/tournament/` (new package)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/tournament-harness.yaml` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/tournament-candidate.md` (new)
  - `apps/local-runner/internal/runner/tournament_behavior.go` (new)
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go` (modified — tournament behaviors)
  - `apps/local-runner/internal/runner/agent_orchestrator.go` (modified — escalation trigger)
  - `apps/local-runner/internal/runner/interactive_service.go` (modified)
  - `apps/local-runner/internal/runner/cohort_stall.go` (modified)
  - `apps/local-runner/internal/runner/vibe_debate.go` (modified)
  - `apps/local-runner/internal/runner/flow_step_runtime.go` (modified — LoopStatusTournamentEscalation)
- modules: `tournament`, `runner`, `agentpack`

## 6. Data or Migration Steps

- schema: none
- data backfill: none

## 7. Validation Plan

- tests to add: ~15 tests (4 arbiter + 3 worktree + 4 flow + 2 escalation + 2 E2E winner/tie)
- failure cases: Tất cả các candidate đều fail (Arbiter báo hòa và yêu cầu con người can thiệp — `TestTournamentTieRequiresHumanDecision`), lỗi merge worktree.

## 8. Rollout and Fallback

- Rollout: P-1 $\rightarrow$ P-2 $\rightarrow$ P-3 $\rightarrow$ P-4 $\rightarrow$ P-5.
- Fallback: Cờ `FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION=false` sẽ giữ nguyên hành vi dừng báo fail cũ.

## 9. Risks

- `R-1` **Tài nguyên máy (CPU/RAM):** Chạy 3 model cùng lúc trên máy yếu có thể làm chậm OS. Mitigation: Mặc định 2 candidates (Claude + Codex), có thể cấu hình chạy tuần tự các candidate nếu RAM < 8GB.

## 10. Definition of Done

- [ ] Arbiter tính toán điểm số khách quan, chính xác theo ma trận trọng số.
- [ ] Worktrees được cô lập và dọn dẹp sạch sẽ sau khi hoàn tất.
- [ ] Chốt thoát hiểm tự động kích hoạt khi review chạm trần.
- [ ] `tournament-harness.yaml` hoạt động độc lập ổn định.
- [ ] Toàn bộ test additive đều green.

---

## Review Protocol

1. Implementer hoàn thành từng slice và tạo Task + CA doc.
2. Reviewer verify thuật toán chấm điểm của Arbiter và cơ chế dọn dẹp Git Worktree.
