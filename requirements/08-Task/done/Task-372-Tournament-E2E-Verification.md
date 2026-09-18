# Task-372: Tournament E2E Verification

## Metadata

- Document ID: `Task-372`
- Title: `Tournament E2E Verification`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-16`
- Completed: `2026-09-16` (CA-880)
- Parent Documents: [CP-65 P-5](../../07-Coding-Plan/done/CP-65-Multi-Candidate-Tournament-Harness.md)
- Child Documents: `None`
- Related Documents: [SS-19](../../05-System-Specs/SS-19-Engineering-Harness-Flow-Family.md), [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [CP-65](../../07-Coding-Plan/done/CP-65-Multi-Candidate-Tournament-Harness.md)
- Replaces: `None`
- Tags: `tournament, e2e, arbiter, worktree-merge, winner-selection`
- Feature Keys: `tournament-harness`

## AI Quick View

### Summary

- Slice cuối của CP-65: test E2E hội tụ P-1 → P-4 — 2 candidate mock giải cùng một bài toán trong 2 worktree, đúng 1 candidate pass test sạch, Arbiter tự pick và winner được merge về workspace chính thành công.
- Bao phủ cả failure case của CP-65 §7: tất cả candidate đều fail → Arbiter báo hòa (`NeedsHumanDecision`) + không merge gì cả.
- Đây là bằng chứng chốt Definition of Done CP-65: điểm khách quan theo ma trận trọng số, worktree cô lập + dọn sạch, escalation kích hoạt khi chạm trần, flow chạy ổn định độc lập.

### Current Ask

- Implement P-5 theo CP-65 §4: E2E test đóng gói toàn bộ chuỗi tournament. 2 test signatures phải xanh (winner + hòa) + failure case hòa phải được assert; toàn bộ DOD CP-65 tick được.

### Key Decisions

- `T-1` E2E dùng mock provider turn + temp git repo thật (same harness style như P-4 của CP-64 / Task-367): suite chạy thật trong temp repo, LSP/dependents stub qua các metric đã tổng hợp — E2E assert trên verdict + trạng thái git, không mock mờ kết quả merge.
- `T-2` Scenario chính: candidate A sửa đúng (suite green, 0 LSP error), candidate B sửa sai (suite đỏ) → `TestTournamentEndToEndWinnerSelectedAndMerged` khẳng định: winner = A, diff A đã về workspace chính, worktree B và A đã bị dọn sạch, run kết thúc done.
- `T-3` Scenario phụ (failure case §7): cả A lẫn B đều đỏ → verdict hòa `NeedsHumanDecision=true`, workspace chính KHÔNG đổi, worktree vẫn được dọn sạch (không rác dù thất bại), flow escalate ask_user.
- `T-4` Escalation leg được test ở mức wiring đã có trong Task-371; E2E này tập trung chuỗi tournament-harness độc lập — không nhân đôi bài test của P-4.

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-1 agnostic — E2E dùng mock candidate turn + temp git repo thật, suite chạy thật, LSP/dependents stub qua metric tổng hợp; không nhánh `providerKey` (grep verify 0 hit).
- Prior CA: none (new feature `tournament-harness`); đóng task phải kèm CA-NNN + tick toàn bộ DOD CP-65 §10.
- Deterministic: không sleep-based; suite trong temp repo chạy nhanh (< vài giây).
- Nếu E2E bóc bug thật ở P-1→P-3, sửa trong slice tương ứng — không patch trong test để qua.
- GitNexus impact analysis nếu phải đụng symbol của các slice trước.

### Open Questions

- None.

### Source Refs

- CP-65 §4 P-5, test signature 12, §7 (failure case hòa + lỗi merge), §10 Definition of Done.
- `internal/tournament/` (Task-368/369), `flows/tournament-harness.yaml` + `runner/tournament_behavior.go` (Task-370), escalation wiring (Task-371).

## 1. Goal

Chuỗi tournament end-to-end chứng minh được: 2 candidate cô lập trong worktree → Arbiter chọn đúng candidate tốt hơn theo ma trận trọng số → merge winner sạch → dọn dẹp sạch; và khi không có candidate khả dụng, hệ thống fail-closed về tay con người mà không hỏng workspace.

## 2. Parent Links

- coding plan: `CP-65-Multi-Candidate-Tournament-Harness.md` P-5
- tech design: `SD-19-Agent-Flow-Engine.md`
- system spec: `SS-19-Engineering-Harness-Flow-Family.md`

## 3. Trigger

P-1 → P-4 hoàn tất từng mảnh; cần một bài test hội tụ để chốt DOD CP-65 trước khi chuyển CP sang done — đặc biệt phần dọn dẹp worktree và hòa-candidate là thứ chỉ thấy được khi ghép chuỗi.

## 4. Exact Change

- `T-1` **`TestTournamentEndToEndWinnerSelectedAndMerged`** (`apps/local-runner/internal/runner/tournament_e2e_test.go` new, không sửa test cũ): temp git repo với 1 bug thật; mock 2 candidate turn — A sửa đúng, B sửa sai; chạy flow `tournament-harness` full; assert winner=A theo verdict, workspace chính chứa fix của A, worktree sạch, flow done.
- `T-2` **`TestTournamentTieRequiresHumanDecision`** (cùng file `tournament_e2e_test.go`, scenario hòa §7): cả 2 candidate đều đỏ → `NeedsHumanDecision=true`, workspace chính không đổi, worktree dọn sạch, flow escalate `ask_user`.
- `T-3` **DOD checklist run**: ghi nhận bằng chứng từng mục DOD CP-65 §10 trong completion notes khi hoàn tất.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/tournament_e2e_test.go` (new)
- modules: `runner` (test only)
- routes: none
- tables: none

## 6. Acceptance Check

- [x] AC-1: Winner đúng candidate có điểm cao hơn (suite green, ít LSP error, blast radius thấp hơn khi hòa điểm).
- [x] AC-2: Diff winner merge về workspace chính đúng nội dung, không rác worktree còn lại sau khi flow done lẫn fail.
- [x] AC-3: Hòa toàn bộ candidate → fail-closed: `NeedsHumanDecision`, không merge, escalate ask_user.
- [x] AC-4: 2 test signatures green:
  - `TestTournamentEndToEndWinnerSelectedAndMerged`
  - `TestTournamentTieRequiresHumanDecision`
- [x] AC-5: Toàn bộ DOD CP-65 tick được (arbiter đúng ma trận, worktree cô lập + dọn sạch, escalation kích hoạt khi chạm trần, tournament-harness ổn định độc lập, additive tests green).

## 7. Out of Scope

- Sửa logic scoring/worktree/flow/escalation (thuộc P-1→P-4; chỉ bóc bug và quay lại slice tương ứng).
- Perf benchmark 3 candidate song song trên máy yếu (R-1 đã có mitigation config ở P-3).
- Multi-workspace/remote worktree (ngoài scope CP-65).

## 8. Completion Notes

- result: done 2026-09-16 — CA-880; 2/2 E2E green on real repos+suites; DOD evidenced (see CA-880 map).
- follow-ups: live rollout-controller dispatch, 3rd-candidate expansion (parse-ready), untracked-child UI visibility.
- upstream docs updated:
