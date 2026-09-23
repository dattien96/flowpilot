# CA-925 — Multi-project parallel vibe operations (CP-82 / Task-422..425)

## Context

The operator's real use case is 4 projects × 2–3 concurrent vibe flows. The
architecture keeps one focused workspace (correct — the run executes on the
runner regardless of UI focus) but lacked the awareness layer: no overview of
all runs, inbox actions required a context switch each, worktree collision
safety was reviewed-but-unproven, and there was no way to glance at another
run without losing focus.

CP-82 adds that layer — deliberately WITHOUT touching `selectedProjectId`
semantics or namespacing timeline state (true dual-pane stays out of scope).

## Changes

### Task-422 — sessions monitor board

- `src/state/boardModel.ts` `deriveBoardSections`: groups every warmed
  `projectHistoryById` slice under project registry order, runs sorted
  `updatedAt` desc; merges `attentionItems` waiting-kind; marks `isFocused`;
  `worktreeBound` from `worktreeState|worktreeSlug|worktreePath`. Pure — no
  fetch, no writes.
- `src/components/SessionsBoard.tsx`: overlay (Esc/scrim close, never
  unmounts the workspace), sectioned rows with status chip OR waiting chip,
  worktree badge, relative time; row click → `openRunAtAttention`.
- `App.tsx`: `BoardIcon` header toggle left of the inbox.

### Task-423 — inbox inline actions

- `AttentionItem.pending?: { approvals, questions }` — populated in
  `deriveAttentionItems` from the same snapshot that refined `kind` (single
  source; absent → Open-only, never guesses).
- `attentionQueue.resolvedPending(runId, {approvalId|questionId})` shrinks the
  cached snapshot payload; `evict(runId)` adds the run to a suppression set
  that auto-clears when the run's next observed status leaves waiting.
- Store actions `approveAttentionItem`/`answerAttentionItem`: direct
  ID-scoped `submitApproval`/`answerQuestion` RPCs + queue update; **never**
  write the focused run's `pendingApprovals`/`status`/`timeline`. Success
  evicts only when it was the last pending; failure returns false, item stays.
- `AttentionInbox`: per-item decision chips from `details.decisions`
  (fallback Approve/Deny), question option chips (multiSelect → Open-only),
  per-item busy state, inline error line.

### Task-424 — worktree uniqueness proof (test-only)

- `internal/worktree/manager_uniqueness_test.go`: distinct owners → distinct
  paths+branches under identical slugs; second create same owner fails closed
  ("already exists", original untouched); `fp/<slug>-<owner-suffix>` branch
  naming.
- `internal/runner/run_worktree_parallel_test.go`: `worktreeOwnerIDFor`
  contract (chat→chatId, flow→runId, never ""), 6-way concurrent
  `provisionRunWorktree` → 6 distinct registered trees, chat legs inherit one
  binding with no second Create.
- No production change needed — the guarantee held under test.

### Task-425 — spectator pane

- Store: `spectatorRunId`/`spectatorProjectId` + `openSpectator` (refuses the
  focused run) / `closeSpectator`; `openHistoryRun` auto-clears when the
  watched run is promoted.
- `boardModel.deriveSpectatorView`: read-only projection (status, waiting
  kind, last line, project name, updated-ago).
- `SpectatorPane.tsx` docked right of the chat column; `<1200px` collapses to
  a compact absolute chip. Watch (eye) affordances on board rows and inbox
  items. Zero stream attaches, zero writes.

## Safe-fix compliance

- Zero existing tests edited. Guard-list additions in `styles.tokens.test.ts`
  are additive coverage (TerminalPanel, SessionsBoard, SpectatorPane).
- New tests: `boardModel.test.ts` (7) + `attentionQueue.inline.test.ts` (5) +
  `store.attention-actions.test.ts` (5) + `store.spectator.test.ts` (7) +
  Go 8 (worktree uniqueness + parallel provisioning). All green.
- Full desktop suite: **535 tests, 14 failures = byte-identical to the
  recorded HEAD baseline** (history-replay order ×3, selectProject,
  openHistoryRun error, sendPrompt abort, workflow handoff, stop flowRef,
  terminal replay ×3-providers, jira dup, client-core boundary,
  HttpRunnerRepository health, TestSupervisor ×2). Zero regressions.
- Provider parity: all changes are provider-agnostic — statuses verbatim,
  shared WAITING_STATUS/kind tables, ID-scoped RPCs, worktree is
  provider-independent by construction.

## Known edges (accepted)

- Inline approve suppression is optimistic: if the server keeps reporting a
  waiting status after a successful resolve, the item stays hidden until the
  status flips — spec'd behavior (T-1), the board still shows the run.
- Board/pane freshness is poll-derived (30s history warm) — intentional;
  real-time row streaming is out of scope.
- `provisionRunWorktree` concurrent test mirrors production locking (s.mu);
  `Manager.Create` itself relies on `git worktree add` + fs-exists check —
  a stat→create race would need a mutex, but the caller serializes, so none
  added (tested under s.mu contention, zero errors).
- Windows flake noted: `TestRunContractFreezeNodeAllowsPreExistingDirtyWorktree`
  TempDir cleanup failed once under parallel load (file lock); passes
  standalone — environmental, pre-existing.

## GitNexus

MCP unreachable — manual blast-radius: new modules (boardModel, SessionsBoard,
SpectatorPane) are leaf additions; store gained optional fields + 4 additive
actions; `deriveAttentionItems` gained an optional 4th param (only caller:
`recompute`); `openHistoryRun` gained an early-return guard.

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: CP-82
change_type: feature
summary: Multi-project parallel ops — sessions monitor board, inbox inline approve/answer without focus switch, worktree uniqueness proofs, read-only spectator pane; single-focus workspace semantics preserved
# --->8---
