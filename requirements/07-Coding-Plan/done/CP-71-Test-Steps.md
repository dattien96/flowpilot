# CP-71 Test Steps — Run Worktree Isolation

- Document ID: `CP-71-Test-Steps`
- Title: `CP-71 Test Steps`
- Phase: `coding-plan`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-24`
- Last Updated: `2026-09-24`
- Parent Documents: `CP-71 (Run Worktree Isolation)`
- Child Documents: ``
- Related Documents: `Task-407, Task-408, Task-409, Task-410, Task-411, Task-424, Task-426, Task-435, Task-436, Task-437, KR-005-cp71-cp84-test-gap-audit`
- Replaces: ``
- Tags: `verification, run-worktree, isolation, merge-back`

## AI Quick View

### Summary

- Kế hoạch verify CP-71 (written retrospectively under KR-005): automated
  Go tests cover the full lifecycle HTTP surface + manager package +
  resolve modes + LOW-gap fills; manual/live checks for toggle UX and
  real-runner merge conflicts.

### Current Ask

- Đã chạy xong trong KR-005 round: tất cả §2 xanh trên
  `livetest/cp-81-82-83-84`; live matrix 8/8 verify trên runner thật
  (`:4317`). Manual §4 còn một số ô cần operator trên build thật.

### Key Decisions

- Mọi scenario resolve đi qua HTTP thật (`POST .../worktree/resolve`),
  không gọi manager trực tiếp — wire contract là thứ desktop/TUI consume.
- Confirm-gate (`requiresConfirm` + file list) là fail-closed: quên
  `confirm:true` → 409, không bao giờ auto-proceed.
- `runSnapshot` không expose `chatId` — test đọc owner từ state, không
  giả vờ qua DTO.

### Constraints

- Baseline suite fail khớp `main` HEAD — fail mới = STOP + report.
- Không edit test cũ; mọi gap-fill là file additive mới.
- Live test giữ runner chạy; worktree fixtures nằm dưới `%TEMP%`.

### Source Refs

- `CP-71 §P-1..P-5`, `SS-23 AC-1..8`, `SD-27 D-1..D-9`,
  `requirements/reviews/KR-005-cp71-cp84-test-gap-audit.md`

## 1. Goal

Chứng minh worktree isolation hoạt động end-to-end: `worktree:true`
provision binding đúng owner, agent cwd vào worktree, merge-back qua 3
mode với confirm/conflict gates đúng contract, recovery detect `lost`,
GC chỉ prune orphan.

## 2. Automated Verification

```bash
cd apps/local-runner && go test ./internal/runner/ -run 'TestE2EWorktree|TestMarkChatWorktreeState' -count=1
cd apps/local-runner && go test ./internal/worktree/ -count=1
cd apps/local-runner && go test -race ./internal/runner/ -run 'TestE2EWorktree' -count=1
cd apps/local-runner && go test ./internal/tui/... -count=1   # /wt-merge surface (Task-437)
cd apps/desktop-flowpilot && npm run test:phase1               # worktreeMergeConfirm.test.ts (10)
```

| Nhóm | Test bắt buộc (đủ tên) | Pass criteria |
|---|---|---|
| Lifecycle HTTP | `TestE2EWorktree_FullLifecycleHTTP`, `TestE2EWorktree_KeepBranchHTTP`, `TestE2EWorktree_LostBindingResolutionsHTTP`, `TestE2EWorktree_InvalidModeRejectedHTTP`, `TestE2EWorktree_ClientGateHTTP`, `TestE2EWorktree_StartOnNonGitRepoReturnsUnavailable` | create→merge→cleanup; lost→archive/recreate; mode validation; X-Client 403; non-git → `worktree_unavailable` |
| Binding semantics | `TestE2EWorktree_LegSwitchSharesWorktreeHTTP`, `TestE2EWorktree_ResumeAfterRestartHTTP`, `TestE2EWorktree_ExternalDeleteMarksLostHTTP`, `TestE2EWorktree_PostBindingSessionWriteKeepsBindingHTTP`, `TestE2EWorktree_OffByteParityHTTP`, `TestE2EWorktree_DeleteGateBlocksAndInlineResolves`, `TestE2EWorktree_BootGCPrunesOnlyOrphansHTTP`, `TestE2EWorktree_ProviderWorkspaceCwdParity` | leg inherit (D-8); rehydrate + D-7 validate; lost detect; `worktree:false` byte-parity; delete gated; GC no-overreach; cwd = worktree path mọi provider |
| Confirm gates | `TestE2EWorktree_KeepBranchWithUncommittedRequiresConfirm`, `TestE2EWorktree_DiscardWithUntrackedRequiresConfirm` | dirty → 409 `requiresConfirm` + file list; `confirm:true` proceeds; conflict ≠ confirm |
| Conflict path | `TestE2EWorktree_ConflictEvidenceAndRetryHTTP`, `TestE2EWorktree_MergeFailedOnMissingBaseSidecar` | 409 bare `{conflict:true, conflictPaths, patchArtifactRef}` (no `error.code` wrapper); retry after fix → `applied:true`; non-conflict failure → 500 `worktree_merge_failed`, state not parked |
| Manager unit | `TestManager_CreateIsolatesOwners`, `TestManager_CreateDistinctOwnersGetDistinctPaths`, `TestManager_CreateFailsClosedOnExistingPath`, `TestManager_CreateDetachedWhenNoSlug`, `TestManager_CreateRejectsInvalidOwnerID`, `TestManager_BranchNamesIncludeOwnerSuffix`, `TestManager_DiffIncludesUntracked`, `TestManager_DiffIncludesCommittedBranchWork`, `TestManager_ApplyCarriesCommittedBranchWork`, `TestManager_ResolveApplyPatchClean`, `TestManager_ResolveConflictErrorCarriesEvidence`, `TestManager_CleanupKeepBranch`, `TestManager_CleanupIdempotent`, `TestManager_ValidateDetectsExternalDelete`, `TestManager_SerializedApplyMutex`, `TestSlugify`, `TestEnsureGitignore_AppendsWhenMissing`, `TestEnsureGitignore_AppendsAfterNoTrailingNewline`, `TestEnsureGitignore_Idempotent` | create/diff/apply/cleanup semantics; `.base` sidecar anchors diff; serialized apply; `.gitignore` written once |
| Session/leg state | `TestMarkChatWorktreeState_PropagatesToLegsAndSessions`, `TestE2EWorktree_GitignoreEntryWrittenOnce` | state lands on every leg + persisted `ProviderSessionState.WorktreeState`; one `.gitignore` entry across runs |
| Desktop surface | `worktreeMergeConfirm.test.ts` (10 tests): per-action descriptions, 409 `requiresConfirm` → confirm card, `confirm:true` resend, dismiss restores, non-confirm 409 stays error, conflict card (paths + patchRef + Retry/Cancel), **wire-shape test** (bare `conflict:true` body without `code` still flips card) | confirm/confirm-cards render from real wire shapes; fail-closed on unknown 409 |
| TUI surface | `tui/app` worktree merge tests (7), `tui/client` `ResolveWorktree` tests (3): `Details` map populated from raw 409 body, conflict vs requiresConfirm discrimination | `/wt-merge`, `/wtm`, `/wt merge <mode>` resolve; `--confirm` flag; conflict shown with paths + patchRef |

## 3. Manual Test Prep

1. `local_runner` thật đang chạy (`:4317`), desktop build thật
   (`pnpm dev` / packaged).
2. Một project thật là git repo sạch + một project **không** phải git
   repo (cho negative case).
3. Terminal `curl` sẵn; DevTools console mở đọc attention-queue logs.

## 4. Manual UI Verification (desktop app)

| ID | Steps | Expected |
|---|---|---|
| `M-1` | Chat posture panel → bật "Run in worktree" → gửi turn | Run badge `worktree` + slug hiện trên Navigator; agent sửa file trong `.flowpilot/worktrees/<id>`, project chính sạch |
| `M-2` | Toggle trên project **không** phải git repo | Toggle disabled + tooltip; start với `worktree:true` qua API → `worktree_unavailable`, không silent fallback |
| `M-3` | Turn xong → card "Merge worktree" hiện | 3 nút `[Apply patch] [Keep branch] [Discard]` + description từng nút |
| `M-4` | Bấm `Keep branch` khi worktree còn uncommitted | Card flip sang confirm: list file sẽ mất + `[Cancel] [Keep branch anyway]`; Proceed → `kept_branch`, branch `fp/*` còn trong repo |
| `M-5` | Bấm `Apply patch` khi main đã đổi trùng chỗ | Card flip sang conflict state: `conflictPaths` + patch ref + `[Cancel] [Retry]`; sửa main rồi Retry → `applied`, worktree+branch dọn sạch |
| `M-6` | `Discard` trên worktree có untracked files | Confirm card liệt kê untracked; Proceed → xóa hết |
| `M-7` | Toggle-off worktree trong khi binding còn live | Bị reject với "merge or discard the worktree first" |
| `M-8` | Xóa thư mục `.flowpilot/worktrees/<id>` bằng tay rồi resume | State `lost` + notice `[Archive] [Recreate empty]`; Recreate → worktree sạch từ base, `priorChangesLost` báo rõ |
| `M-9` | Provider switch (leg mới) trong chat có worktree | Leg mới share cùng worktree; file agent tạo ở leg trước còn nguyên |
| `M-10` | TUI: `/wt-merge` trên run có `merge_pending` | In binding state + usage; `/wt-merge keep_branch` dirty → 409 list + `--confirm` hint |

## 5. Live REAL Tests — qua HTTP, không mock

(Đã chạy 8/8 trong KR-005 round — runner `:4317`, workspace `C:\temp\fp-live-ws`.)

| ID | Steps | Expected |
|---|---|---|
| `L-1` | `POST /client/workflow-runs {worktree:true}` → đọc `worktree` block | `state:active`, `ownerId`, `path`, `branch fp/*`, `baseCommit` đầy đủ |
| `L-2` | Tạo file uncommitted trong worktree → `resolve keep_branch` | 409 `{requiresConfirm:true, uncommitted:[...]}` — bare body, không `code` |
| `L-3` | Retry L-2 với `confirm:true` | 200 `kept_branch`; `git branch` còn `fp/run-*`; worktree dir xóa |
| `L-4` | Tạo untracked file → `resolve discard` | 409 `{requiresConfirm:true, untracked:[...]}`; confirm → `discarded`, branch xóa |
| `L-5` | Clean worktree → `resolve keep_branch` | 200 ngay, không cần confirm |
| `L-6` | Main sửa trùng file worktree đổi → `resolve apply_patch` | 409 `{conflict:true, conflictPaths, patchArtifactRef}`; **không có** `requiresConfirm` |
| `L-7` | Sửa main xong → retry `apply_patch` | 200 `applied:true`, `merged`; nội dung worktree land vào main |
| `L-8` | Dirty worktree (uncommitted) → `resolve apply_patch` | 200 không cần confirm — uncommitted work được preserve |

## 6. Log & Audit Evidence

- `change-audit/` entries: CA-877 (manager base), KR-005 round CAs
  (resolve-mode coverage + `recreate_empty` fix, `patchArtifactRef` key
  fix, CA-958 `keep_branch` uncommitted guard, CA-961 conflict-card
  wire-shape, CA-962 TUI surface).
- `gitnexus_detect_changes` trước commit: chỉ worktree/runner/tui/desktop
  attention symbols.
- Live evidence: `C:\temp\fp-live-ws` matrix log (8/8) trong KR-005 §7.

## 7. Verification Complete When

- [x] §2 automated xanh đủ bảng (Go + desktop phase1); baseline fail
      byte-identical HEAD nếu có.
- [ ] `M-1` → `M-10` ticked bởi operator trên build thật (desktop UI
      confirm/conflict cards — automation covers store-level contract,
      pixel render chưa được operator xác nhận).
- [x] `L-1` → `L-8` ticked trên runner thật (KR-005 live matrix 8/8).
- [x] D-* trong CP-71 map đủ sang checklist — mỗi D có ít nhất một
      M/L/auto proof (D-8 leg-inherit → auto; D-7 validate → auto;
      D-4c merge-back contract → auto + live; D-9 toggle persistence → M-7).
- [x] Không test cũ bị sửa; fail-closed confirm/conflict chứng minh qua
      auto + live (L-2/L-4/L-6).
