# CA-907: CP-71 Run Worktree Isolation + Desktop Attention Queue + UI Tokens

## Summary

- CP-71 (SS-23/SD-27): opt-in git worktree isolation for runs. New shared
  package `internal/worktree` (`Manager.Create/List/Diff/Apply/Cleanup/
  Validate`, `Info`, `MergeConflictError`, `Slugify`); `internal/tournament`
  delegates to it (prefix `candidate-`, strict HEAD-drift check preserved —
  Task-369 oracle) while run worktrees use `git apply --check` as the merge
  oracle so unrelated main-workspace drift still merges and conflicts are
  retryable.
- Worktree owner model: 1 owner ↔ 1 worktree. Chat mode owner = `chatId`
  (provider-switch legs share the worktree — verified via
  `ListProviderSessionsByChat` lookup); flow mode owner = flow `runId`
  (children inherit `workspaceCwd`). Binding persists on the run record
  (`worktreeEnabled`, `worktreePath`, `worktreeState`, `worktreeSlug`,
  `baseCommit`); resume validates via `git worktree list` + dir + `.base`
  sidecar → `lost` state + `worktree_lost` event; never silently recreates.
- Merge-back: terminal completion emits `worktree_merge_requested`;
  `POST /client/workflow-runs/{id}/worktree/resolve` accepts
  `apply_patch | keep_branch | discard`; conflicts return 409 with
  `conflictPaths` + `patchArtifactRef` and keep `merge_pending` for retry.
  Boot GC prunes only orphaned worktrees. Client gate rejects
  `worktree:true` from unsupported clients (403 `worktree_client_forbidden`).
- Task-409: desktop `RunInWorktreeToggle` in `ChatPosturePanel` (per-chat
  restore from latest run binding, disabled for non-git projects and while
  a live binding exists), Navigator worktree badge, TUI `/worktree` toggle +
  status-bar badge, `isGitRepo` IPC via electron preload.
- Task-404: desktop attention queue — `state/attentionQueue.ts` singleton
  observer (`deriveAttentionItems` + `ingestHistory/ingestSnapshot/
  ingestDispatch/subscribe`), `AttentionQueue` component pinned above
  Navigator history groups, store fields `attentionItems` +
  `openRunAtAttention` (reuses `openHistoryRun`). `waiting_user_approval`
  added to the desktop `RunStatus` union (real backend status).
- Task-405: design-token layer — `--space-1..6`, `--radius-*`, `--elev-*`,
  `--dur-*`, `--ease-standard`, `--font-size-*`, `--line-*` in `:root`;
  97 declarations migrated across navigator/history/attention-queue/
  card/posture regions; shared hover transition; `STYLE-TOKENS.md`
  guardrail doc + `styles.tokens.test.ts` enforcing token usage in
  migrated sections.

## Files

- `apps/local-runner/internal/worktree/` (new package + tests)
- `apps/local-runner/internal/tournament/worktree_manager.go` (delegation)
- `apps/local-runner/internal/runner/run_worktree.go`,
  `run_worktree_merge.go`, `interactive_handlers.go`,
  `interactive_service.go`, `interactive_resume.go`,
  `workflow_store.go`, `local_file_session_store.go`,
  `provider_event.go`, `flow_step_runtime.go`,
  `cp71_worktree_e2e_test.go` (8 live HTTP/event-log E2E tests)
- `apps/local-runner/internal/tui/app/` (`worktree_toggle.go`, status bar,
  model, chat switch) + tests
- `apps/desktop-flowpilot/src/state/attentionQueue.ts` (new),
  `attention_queue.test.ts`, `cp71_worktree_toggle.test.ts`,
  `styles.tokens.test.ts`, `store.ts`, `styles.css`,
  `types/contract.ts`, `components/AttentionQueue.tsx` (new),
  `Navigator.tsx`, `ChatPosturePanel.tsx`, `DispatchAttentionCard.tsx`,
  `RunStatus.tsx`, `client/ideBridge.ts`, `electron/main.ts`,
  `electron/preload.ts`, `STYLE-TOKENS.md`

## Verification

- `go test ./internal/worktree ./internal/tournament` — PASS
- `go test ./internal/runner -run 'TestE2EWorktree|TestWorktree'` — PASS
  (8/8 CP-71 E2E: lifecycle HTTP, leg-switch share, conflict evidence +
  retry, resume-after-restart, external-delete lost, client gate, boot GC,
  off-byte-parity)
- `go test ./internal/tui/app -run TestTUI_Worktree` — PASS (3/3)
- `go test ./internal/runner -run 'TestDevin|TestE2EDevin'` — PASS
- Desktop phase-1: `attention_queue.test` 8/8, `cp71_worktree_toggle.test`
  6/6, `styles.tokens.test` 4/4 — all PASS
- `tsc --noEmit` (desktop app) — clean except pre-existing
  `store.chat-mode-persist.test.ts` arity error (baseline).

## Out of Scope / Known Issues

- Pre-existing `internal/runner` suite failures on HEAD (reproduced on a
  clean worktree at 435e336, unrelated): `TestListRemoteChatSessions…`,
  `TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory`,
  `TestTryAdvanceFlowFromNodeBailsOnNonDelegateTarget`,
  `TestFlowEngineSynthesisPromptIncludesJoinedReviewerNote`,
  `TestLiveCodexChatResumesAcrossProviderAccountSwitches`,
  `TestStartResolvedFlowSpawnsPackAgentEvenWhenProjectShadowsItsName`;
  full suite also exceeds the 10m timeout under load.
- Live provider tests: Grok live suite blocked by 402 "usage balance
  exhausted" (account credit, not code); detection tests PASS. Devin CLI
  on this machine is not logged in — real SWE-2 live calls cannot run;
  Devin coverage is via the fake-ACP E2E suite (incl. swe-2-* model ids).
  Claude live gate unusable (broken npm shim).
- GitNexus MCP was unreachable during implementation; `detect_changes`
  deferred — impact assessed manually (new package is leaf; tournament
  delegation keeps signatures).

# ---8<--- flowpilot:change-ledger
feature_key: run-worktree
source_doc_id: CP-71
change_type: feature
summary: CP-71 chat-scoped git worktree isolation (internal/worktree, merge-back resolve endpoint, lost-state recovery, desktop+TUI toggle) + Task-404 attention queue + Task-405 design tokens
# --->8---
