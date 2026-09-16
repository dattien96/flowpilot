# Task-368: Tournament Arbiter Scoring Engine

## Metadata

- Document ID: `Task-368`
- Title: `Tournament Arbiter Scoring Engine`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-16`
- Parent Documents: [CP-65 P-1](../../07-Coding-Plan/done/CP-65-Multi-Candidate-Tournament-Harness.md)
- Child Documents: `None`
- Related Documents: [SS-19](../../05-System-Specs/SS-19-Engineering-Harness-Flow-Family.md), [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [CP-63](../../07-Coding-Plan/todo/CP-63-IDE-Grade-LSP-Runtime.md), [CP-64](../../07-Coding-Plan/todo/CP-64-Reproduce-First-TDD-Gate.md)
- Replaces: `None`
- Tags: `tournament, arbiter, scoring, deterministic, multi-candidate`
- Feature Keys: `tournament-harness`

## AI Quick View

### Summary

- Slice đầu tiên của CP-65: package mới `internal/tournament` chứa trọng tài (Tournament Arbiter) — chấm điểm các candidate bằng code Go thuần deterministic, KHÔNG dùng LLM (CP-65 P-1 key decision).
- Công thức: `TotalScore = 0.5×S_tests + 0.3×S_lsp + 0.2×S_blast`; candidate làm gãy test cũ bị loại ngay (disqualified, điểm 0) — cùng tinh thần regression safety với oracle `r-reg` của CP-64.
- Ba tín hiệu đầu vào đều đã có hạ tầng: test pass ratio (suite run), LSP diagnostics (`lsp.ServerSet.CheckFiles` từ CP-63), blast radius (`structure.Provider.Dependents` qua `npx gitnexus impact` + `changecontract.GitNexusQueryTargetsForPath`).
- Arbiter là scorer thuần trên `CandidateResult` đã thu thập — việc chạy test/LSP/dependents trong worktree thuộc glue của P-2/P-3; package này không spawn process nào.

### Current Ask

- Implement P-1 theo CP-65 §4: `types.go` + `arbiter.go` trong package `internal/tournament`. 4 test signatures cho sẵn phải xanh.

### Key Decisions

- `T-1` Package mới `internal/tournament` tự định nghĩa types (`CandidateResult`, `CandidateScore`, `TournamentVerdict`) — không import `flowgate`/`lsp` để tránh phụ thuộc; các metric đi vào `CandidateResult` dưới dạng số liệu đã tổng hợp (test pass ratio, error count, dependents count).
- `T-2` `S_tests` = tỷ lệ test pass trên tổng số test chạy; candidate có `BrokeExistingTests=true` → disqualified (bị loại khỏi ranking dù điểm khác cao) — theo CP-65 §3.2 "bị phạt 0 điểm nếu gãy test cũ".
- `T-3` `S_lsp` = 1.0 khi 0 error, giảm tuyến tính theo số lỗi cú pháp (floor 0) — tín hiệu lấy từ kết quả diagnostics CP-63 đã thu thập.
- `T-4` `S_blast` bucket theo `DependentsSummary.Count` của các path candidate thay đổi (Low 1.0 / Medium 0.7 / High 0.3 / Critical 0.0) — mapper tái dùng `changecontract.GitNexusQueryTargetsForPath` ở tầng glue; trong package, bucket là hàm thuần nhận `count int`.
- `T-5` Tất cả candidate đều fail/disqualified → `TournamentVerdict` báo hòa + cờ yêu cầu con người can thiệp (CP-65 §7 failure case), KHÔNG pick ngẫu nhiên. Q-1 (`auto_pick` config) sẽ được wire ở P-3; engine trả verdict trung lập với cờ `NeedsHumanDecision`.

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-1 agnostic — scorer pure trên `CandidateResult` số liệu tổng hợp, không nhận `providerKey`, không import `flowgate`/`lsp` (grep verify 0 hit); `ProviderKey` trong types chỉ là label, không nhánh logic.
- Prior CA: none (new feature `tournament-harness`); đóng task phải kèm CA-NNN ledger entry.
- Scoring phải deterministic tuyệt đối: cùng input → cùng verdict (unit test có thể assert điểm số chính xác).
- Package mới không import ngược từ `tui/`, không tạo cycle dependency (same contract as `internal/lsp`).
- GitNexus impact analysis trước khi tạo symbol mới (package mới — d=1: none, nhưng phải chạy `gitnexus_detect_changes` sau).

### Open Questions

- None (ngưỡng S_blast đã chốt ở T-3: 0 / 1–3 / 4–10 / >10).

### Source Refs

- CP-65 §3.2 (công thức chấm), §4 P-1, test signatures 1–4, §7 (failure case hòa).
- `internal/structure/provider.go` (`Provider.Dependents`, `DependentsSummary`), `internal/changecontract/impact_targets.go` (`GitNexusQueryTargetsForPath`), `internal/lsp/runner_hook.go` (`ServerSet.CheckFiles`).

## 1. Goal

Một scoring engine deterministic: nhận kết quả test + diagnostics + dependents của N candidate, trả về ranking có trọng số 50/30/20, loại candidate gãy test cũ, và báo hòa kèm `NeedsHumanDecision` khi không có candidate khả dụng.

## 2. Parent Links

- coding plan: `CP-65-Multi-Candidate-Tournament-Harness.md` P-1
- tech design: `SD-19-Agent-Flow-Engine.md`
- system spec: `SS-19-Engineering-Harness-Flow-Family.md`

## 3. Trigger

CP-65 phê duyệt mô hình PDR/Tournament để thoát bẫy lặp tuần tự (cognitive lock-in, `review_cap_exceeded`). P-1 là bộ não chấm điểm — mọi slice sau (worktree, flow, escalation) đều tiêu thụ verdict của Arbiter.

## 4. Exact Change

- `T-1` **`internal/tournament/types.go`** (new): `CandidateResult{CandidateID, Label, ProviderKey, TotalTests, PassedTests, BrokeExistingTests, LSPErrors, ChangedPaths, DependentsCount}`, `CandidateScore{CandidateID, STests, SLsp, SBlast, Total, Disqualified}`, `TournamentVerdict{WinnerCandidateID, Ranking []CandidateScore, NeedsHumanDecision bool, Reason string}`.
- `T-2` **`internal/tournament/arbiter.go`** (new): `TournamentArbiter` với `Score(result CandidateResult) CandidateScore` (công thức 0.5/0.3/0.2, disqualified khi `BrokeExistingTests`) và `Decide([]CandidateResult) TournamentVerdict` (sort theo Total, skip disqualified, hòa trên điểm → prefer S_blast cao hơn, vẫn hòa → `NeedsHumanDecision=true`).
- `T-3` **`internal/tournament/arbiter.go`**: `BlastBucket(count int) float64` — CHỐT ngưỡng: 0 → Low 1.0; 1–3 → Medium 0.7; 4–10 → High 0.3; >10 → Critical 0.0 (theo đề xuất CP-65 §3.2).
- `T-4` **`internal/tournament/arbiter_test.go`** (new): 4 test signatures dưới đây.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/tournament/types.go` (new)
  - `apps/local-runner/internal/tournament/arbiter.go` (new)
  - `apps/local-runner/internal/tournament/arbiter_test.go` (new)
- modules: `tournament` (new)
- routes: none
- tables: none

## 6. Acceptance Check

- [x] AC-1: Candidate có test pass rate cao hơn đạt điểm `S_tests` và tổng điểm cao hơn (50% trọng số).
- [x] AC-2: Candidate nhiều LSP error hơn bị trừ điểm `S_lsp` rõ rệt; 0 error → 1.0.
- [x] AC-3: Hai candidate hòa tổng điểm → candidate blast radius thấp hơn thắng.
- [x] AC-4: Candidate gãy test cũ → `Disqualified=true`, không bao giờ được chọn làm winner dù điểm thành phần cao.
- [x] AC-5: Tất cả candidate disqualified → verdict `NeedsHumanDecision=true`, không crash, không pick winner.
- [x] AC-6: 4 test signatures green:
  - `TestArbiterPrefersCandidateWithHigherTestPassRate`
  - `TestArbiterPenalizesLSPCompilerErrors`
  - `TestArbiterPrefersLowerBlastRadiusOnTie`
  - `TestArbiterDisqualifiesCandidateBreakingExistingTests`

## 7. Out of Scope

- Tạo/quản lý worktree (P-2 / Task-369).
- Flow YAML + đăng ký behavior arbiter (P-3 / Task-370).
- Escalation wiring khi chạm cap (P-4 / Task-371).
- Thu thập metric trong worktree thật (chạy suite/LSP/dependents) — glue thuộc P-3 execution behavior.

## 8. Completion Notes

- result: done 2026-09-16 — CA-876; 5/5 tests green (4 signatures + all-disqualified extra); gofmt/vet clean; provider-parity grep 0 hit.
- follow-ups: P-2 worktree (Task-369), P-3 flow glue collects CandidateResult metrics.
- upstream docs updated:
