# Task-369: Worktree Rollout Manager

## Metadata

- Document ID: `Task-369`
- Title: `Worktree Rollout Manager`
- Phase: `task`
- Status: `draft`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-16`
- Parent Documents: [CP-65 P-2](../../07-Coding-Plan/todo/CP-65-Multi-Candidate-Tournament-Harness.md)
- Child Documents: `None`
- Related Documents: [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SS-19](../../05-System-Specs/SS-19-Engineering-Harness-Flow-Family.md), [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- Replaces: `None`
- Tags: `tournament, git-worktree, isolation, merge, cleanup`
- Feature Keys: `tournament-harness`

## AI Quick View

### Summary

- Slice 2 của CP-65: `WorktreeManager` quản lý vòng đời Git worktree tạm cho các candidate chạy song song — mỗi candidate một bản sao working tree riêng tại `.flowpilot/worktrees/candidate-<id>`, cô lập tuyệt đối file system (CP-65 P-2 key decision).
- Vòng đời: `Create` (git worktree add từ flow-start HEAD) → candidate chạy trong worktree → `MergeWinner` (đưa diff của candidate thắng về workspace chính) → `Cleanup` (worktree remove + prune, không để rác).
- Merge theo kiểu patch-based (`git diff` từ worktree rồi apply vào workspace chính) để không ô nhiễm branch/ref state của repo chính; merge conflict → giữ worktree + báo lỗi cho con người (CP-65 §7 failure case).
- Điểm neo sẵn có: `flow_executor.go` đã capture `flowStartGitHead` một lần cho mỗi flow run — worktree sẽ mọc từ commit đó để tất cả candidate cùng xuất phát điểm.

### Current Ask

- Implement P-2 theo CP-65 §4: `worktree_manager.go` trong package `internal/tournament`. 3 test signatures cho sẵn phải xanh.

### Key Decisions

- `T-1` Worktree path `.flowpilot/worktrees/candidate-<id>`; thư mục `.flowpilot/worktrees/` phải được thêm vào `.gitignore` (additive) để worktree con không bị git của repo chính nhìn thấy là untracked.
- `T-2` Base commit = flow-start HEAD (tương thích CP-55 baseline fingerprint); không dùng branch HEAD trôi nổi.
- `T-3` Merge winner = patch-based: `git -C worktree diff` sinh patch, `git apply` vào workspace chính; chỉ apply khi workspace chính chưa trôi (pre-check HEAD + dirty fingerprint như `baselineWorktreeFingerprint`); - `T-3` Merge winner = patch-based: `git -C worktree diff` sinh patch, `git apply` vào workspace chính; chỉ apply khi workspace chính chưa trôi (pre-check HEAD + dirty fingerprint như `baselineWorktreeFingerprint`); conflict → `MergeConflictError` kèm patch + danh sách path conflict để behavior P-3 gắn vào card `ask_user` RỒI DỌN worktree (policy chốt 2026-09-16: worktree thua xóa ngay tại verdict, worktree thắng xóa khi flow done — KHÔNG giữ worktree điều tra, bằng chứng nằm trong card/audit, tuyệt đối không mồ côi).
- `T-4` Cleanup idempotent: `git worktree remove --force` + `git worktree prune`; xóa worktree THUA ngay tại verdict, xóa worktree THẮNG khi flow done (sau merge xong); được gọi cả trong path thành công lẫn path lỗi (defer), và an toàn khi chạy lại (worktree đã mất → no-op).
- `T-5` R-1 (CP-65 §9): manager hỗ trợ chạy tuần tự các candidate qua API create-run-cleanup từng cái — quyết định song song/serial thuộc P-3 config, manager chỉ cung cấp khả năng.

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-1 agnostic — `WorktreeManager` chỉ chạy `git worktree/diff/apply`, không nhận `providerKey` (grep verify 0 hit); candidate nào chạy trong worktree là việc của P-3.
- Prior CA: none (new feature `tournament-harness`); đóng task phải kèm CA-NNN ledger entry.
- Test dùng temp git repo thật (`git init` trong t.TempDir()) — không mock git.
- Không đụng workspace chính ngoài `git apply` patch của winner; tuyệt đối không commit tự động trong manager.
- GitNexus impact analysis trước symbol edit (package mới từ Task-368).

### Open Questions

- None.

### Source Refs

- CP-65 §3.1 (kiến trúc worktree), §4 P-2, test signatures 5–7, §7 (lỗi merge worktree), §9 (R-1 tài nguyên).
- `internal/runner/flow_executor.go` (`flowStartGitHead`, `baselineWorktreeFingerprint`), `internal/tournament/` (package nền từ Task-368).

## 1. Goal

WorktreeManager hoạt động trọn vòng đời trên repo git thật: tạo N worktree cô lập từ cùng base commit, merge diff của winner về workspace chính sạch sẽ, và dọn dẹp toàn bộ worktree tạm — kể cả khi tournament thất bại giữa chừng.

## 2. Parent Links

- coding plan: `CP-65-Multi-Candidate-Tournament-Harness.md` P-2
- tech design: `SD-19-Agent-Flow-Engine.md`
- system spec: `SS-19-Engineering-Harness-Flow-Family.md`

## 3. Trigger

P-1 có scorer nhưng chưa có môi trường chạy cô lập — chạy nhiều candidate trên cùng working tree sẽ ghi đè lẫn nhau. Worktree là cơ chế cô lập cấp git chuẩn, không cần container.

## 4. Exact Change

- `T-1` **`internal/tournament/worktree_manager.go`** (new): `WorktreeManager` với `Create(repoDir, baseCommit, candidateID string) (worktreePath, error)` — `git worktree add .flowpilot/worktrees/candidate-<id> <baseCommit>`; lỗi khi repo không phải git hoặc worktree đã tồn tại.
- `T-2` **`internal/tournament/worktree_manager.go`**: `Diff(repoDir, candidateID string) (patch []byte, error)` — `git -C worktree diff` (untracked mới được `git add -N` trước để vào diff); `MergeWinner(repoDir, candidateID string) error` — pre-check HEAD/fingerprint chưa trôi rồi `git apply`; conflict → `MergeConflictError` (typed), giữ worktree.
- `T-3` **`internal/tournament/worktree_manager.go`**: `Cleanup(repoDir string, candidateIDs []string)` idempotent — remove + prune; gọi được nhiều lần; thêm `.flowpilot/worktrees/` vào `.gitignore` repo (additive, best-effort).
- `T-4` **`internal/tournament/worktree_manager_test.go`** (new): 3 test signatures dưới đây trên temp git repo thật.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/tournament/worktree_manager.go` (new)
  - `apps/local-runner/internal/tournament/worktree_manager_test.go` (new)
- modules: `tournament`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: Tạo 2 worktree từ cùng base commit — 2 đường dẫn tồn tại độc lập, ghi file vào worktree A không thấy ở worktree B và ở workspace chính.
- [ ] AC-2: Sau tournament (dù thành công hay lỗi), không còn worktree tạm nào: `git worktree list` sạch, thư mục `.flowpilot/worktrees/` rỗng hoặc biến mất; gọi Cleanup lần 2 là no-op.
- [ ] AC-3: Merge winner đưa đúng diff về workspace chính (file nội dung khớp worktree winner), không tạo commit mới, không đụng branch/ref.
- [ ] AC-4: Workspace chính trôi (HEAD đổi / dirty trùng vùng) → `MergeConflictError` mang patch + path conflict vào card `ask_user`, worktree vẫn được dọn (không giữ điều tra, không mồ côi).
- [ ] AC-5: 3 test signatures green:
  - `TestWorktreeManagerCreatesIsolatedWorktrees`
  - `TestWorktreeManagerCleansUpAfterTournament`
  - `TestWorktreeManagerMergesWinningCandidate`

## 7. Out of Scope

- Scoring/verdict của Arbiter (P-1 / Task-368).
- Flow YAML + spawn candidate vào worktree (P-3 / Task-370).
- Escalation wiring (P-4 / Task-371).
- Auto-resolve merge conflict hoặc auto-commit (chủ đích để con người xử lý).

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
