# CP-71: Run Worktree Isolation (Shared Manager + Toggle + Merge-Back)

## Metadata

- Document ID: `CP-71`
- Title: `Run Worktree Isolation — Shared internal/worktree, Start Toggle, Merge-Back Card, Recovery & GC`
- Feature Keys: `run-worktree`
- Phase: `coding_plan`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [SS-23: Run Isolation Worktree](../../05-System-Specs/SS-23-Run-Isolation-Worktree.md), [SD-27: Run Worktree Isolation](../../06-System-Tech-Design/SD-27-Run-Worktree-Isolation.md)
- Child Documents: `Task-407+ (created at implementation start)`
- Related Documents: [CP-60: Vibe Working Mode](../done/CP-60-Vibe-Working-Mode.md), [CP-51: Durable Turn Dispatch](../done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [CP-65: Tournament Harness](../done/CP-65-Multi-Candidate-Tournament-Harness.md), [Task-369](../../08-Task/done/Task-369-Worktree-Rollout-Manager.md)
- Replaces: `None`
- Tags: `worktree, isolation, vibe-mode, desktop, tui, runner`

## AI Quick View

### Summary

- Implement `SS-23`/`SD-27` in five slices: extract the proven tournament worktree lifecycle into `internal/worktree`, plumb `worktree:true` through start-run and provider cwd, add the Desktop/TUI toggle + badges, add the merge-back card + dedicated resolve endpoint, then recovery + GC + E2E.
- The heavy semantics (create-from-HEAD, `.base` sidecar, patch-based merge, `MergeConflictError`, idempotent no-orphan cleanup) already exist and are battle-tested by CP-65; this plan *generalizes and exposes* them, it does not invent them.
- Isolation is cwd-only: zero changes to `flowgate`, flow packs, provider prompts, or adapter semantics. Feature is opt-in per run, default off — `worktree:false` must be byte-for-byte today's behavior.
- Local-only persistence on the session record (CP-60 `working_mode` precedent); `X-Client: desktop|tui` gating identical to vibe; Admin Web untouched.

### Current Ask

- Land the five P-slices so a user can run N isolated vibe/harness runs on one project concurrently and merge each back explicitly. Each P is independently testable; `P-1`–`P-2` are pure Go, `P-3`–`P-4` are UX, `P-5` is hardening.

### Key Decisions

- `P-1` New `internal/worktree` package generalizes `tournament.WorktreeManager` and **`internal/tournament` delegates to it** (single implementation — worktree code is too subtle to fork; tournament suite + `WorktreePath` export are the compatibility oracle).
- `P-2` `StartRunInput.Worktree` + run-record fields + provider `cwd` — `worktree:true` accepted only from `X-Client: desktop|tui`, else `403 worktree_client_forbidden` (mirror `working_mode` gate); non-git repo → `worktree_unavailable`, never silent fallback. Base `HEAD` captured at `createRun` for all run kinds (chat + flow — `D-4b`, do not reuse flow-only `flowStartGitHead`). `createRun` resolves `worktreeOwnerID` (`D-8`): `chatId` for chat runs — a new leg first tries inheriting the chat's existing `active|merge_pending` binding (resident `s.runs` → `ListProviderSessionsByChat`, then `D-7` validation); parent flow `runId` for flow runs.
- `P-2b` Toggle persistence per chat (derived, `D-9`): `worktreeEnabled` on the **run record** (no chat record exists on the runner); reopening a chat pre-sets the toggle from its newest run (new chat = `false`; mid-run pinned). Toggle-off while a live binding exists → rejected with "merge or discard the worktree first".
- `P-3` Desktop toggle in `ChatPosturePanel` persisted **per chat** (new chat = off; reopen restores; mid-run pinned) + Navigator `worktree` badge; TUI start option + status badge.
- `P-4` Merge-back as dedicated `worktree_merge_requested` event/card + `POST /client/workflow-runs/{runId}/worktree/resolve {mode}` — modes `apply_patch|keep_branch|discard`; conflict → card with `conflictPaths` + patch artifact ref.
- `P-5` Recovery rehydrates binding and runs `SD-27 D-7` validation (`git worktree list --porcelain` + dir + `.base` sidecar) on every resume/attach/history-open; external deletion → `worktreeState=lost` + notice card (`archive chat` | `recreate empty from base`) — never silent recreate/duplicate; boot GC prunes only worktrees of deleted/corrupt runs — never `merge_pending` or resumable runs.

### Constraints

- `safe-fix-contract` governs every slice: additive tests only, old suite untouched and green (stop+report on any old failure), Claude+Codex+Grok parity proven or evidenced (manager is Case-1 agnostic — grep-verify zero `providerKey`), feature_key `run-worktree` + CA ledger entry per slice.
- No Supabase migration; no Admin Web work; no engine/gate/prompt changes; no auto-commit/merge/push in user repo (`BR-2`).
- Real git in tests (`t.TempDir()` + `git init`), no git mocks (Task-369 convention).
- GitNexus `impact` before editing `WorktreeManager`/`createRun` symbols; `detect_changes` before commit.

### Open Questions

- `Q-1` Resolved — `internal/tournament` delegates to `internal/worktree` (single implementation; CP-65 suite is the oracle).
- `Q-2` Resolved — dedicated `worktree_merge_requested` event/card + `POST .../worktree/resolve`.

### Source Refs

- `SS-23 AC-1..AC-8`, `BR-1..BR-7`, `E-1..E-7`; `SD-27 D-1..D-5`, §5 state machine, §6 contracts, §8 `F-1..F-6`; `internal/tournament/worktree_manager.go`; `internal/runner/interactive_handlers.go` (`handleStartRun`, `runHistoryItem`, `runSnapshotView`); `internal/runner/interactive_service.go` (`createRun`, `resumeRun`); `local_file_session_store.go`; `apps/desktop-flowpilot/src/components/ChatPosturePanel.tsx`, `Navigator.tsx`; TUI `working_mode.go` (toggle precedent).

## 1. Goal

Ship opt-in per-run git worktree isolation so concurrent vibe/harness runs on one project are safe, with explicit user-controlled merge-back and crash-safe recovery — implemented by generalizing proven machinery, not redesigning it.

## 2. Input Documents

- `SS-23` (full AC/BR/E set), `SD-27` (D-1..D-5, data model, contracts, failure table).
- `Task-369`/`CA-877` (manager semantics being generalized), `CP-60 P-1` (`working_mode` plumbing precedent: run-level field, client gate, session toggle), `CP-51`/`SD-25` (recovery contract the binding must honor).

## 3. Implementation Strategy

- **Overall approach:** bottom-up, additive, each P independently testable and revertible. Manager semantics are copied-proven, not re-derived; all UX rides existing card/toggle/event plumbing. The feature is dead code until `P-3` exposes the toggle — intermediate merges are safe.
- **Sequencing:** `P-1` (package) → `P-2` (plumbing) → `P-3` (start UX) → `P-4` (merge-back UX) → `P-5` (recovery/GC/E2E). `P-3` and `P-4` can parallelize once `P-2` lands.
- **Dependencies:** `P-4` needs `P-2` state machine; `P-5` needs `P-2` binding fields. No slice depends on Admin Web or Supabase.

## 4. Work Breakdown

- `P-1` **Shared `internal/worktree` package.** `Manager{Create,List,Resolve,Cleanup}` parameterized by `ownerID` + name prefix (tournament passes `candidate-`, runs pass none; paths `.flowpilot/worktrees/<prefix><ownerId>`) + slug + `.base` sidecar + `MergeConflictError` + per-repo apply mutex; `.flowpilot/worktrees/` gitignore append (additive). `internal/tournament` **delegates** to it — CP-65 tests + `WorktreePath` export are the compatibility oracle.
- `P-2` **Start plumbing + run record.** `StartRunInput.Worktree`, `X-Client` gate (`403 worktree_client_forbidden` for other clients), `createRun` resolves `worktreeOwnerID` per `D-8` (chatId with leg-inheritance lookup — resident `s.runs` → `ListProviderSessionsByChat` → `D-7` validate → reuse `active|merge_pending` binding; parent flow runId for flow runs), captures `HEAD` base for all run kinds, persists binding (`worktreeOwnerID`, path, branch, baseCommit, slug, state, `worktreeEnabled`), provider `cwd=worktreePath`, `runSnapshotView`/`runHistoryItem` `worktree*` fields, `worktreeState` transitions `none→active`.
- `P-3` **Start UX.** Desktop: "Run in worktree" toggle in `ChatPosturePanel` persisted per chat by derivation (`D-9`: restores from the chat's newest run; toggle-off rejected while binding live); Navigator badge `worktree` + slug on run items. TUI: start option + status badge. Toggle disabled with tooltip when project isn't a git repo.
- `P-4` **Merge-back UX — two triggers, one contract (`D-4c`).** Flow runs: terminal status with `state=active` → `merge_pending` → emit `worktree_merge_requested` card. Chat runs: user-initiated "Merge worktree back" control + same decision surfaced on chat delete. Shared: `POST .../worktree/resolve`; conflict → card with `conflictPaths` + patch artifact; serialized per-repo apply with `--check` pre-flight.
- `P-5` **Recovery + GC + E2E.** `resumeRun` rehydrates binding + `D-7` validation (missing dir/registration → `lost` notice: `archive chat` | `recreate empty from base`; new legs in a `lost` chat are blocked behind it — `E-8`); run deletion → merge decision first (`E-3`) then GC-eligible; boot GC scan; E2E: two concurrent vibe runs overlapping files → sequential merge, second conflicts → card; provider-switch leg keeps files; toggle-off regression probe.

## 5. Touched Areas

- files: `internal/worktree/*.go` (new), `internal/tournament/worktree_manager.go` (delegate or untouched), `internal/runner/{interactive_handlers,interactive_service,interactive_resume,local_file_session_store}.go`, `apps/desktop-flowpilot/src/components/{ChatPosturePanel,Navigator}.tsx` + new `WorktreeMergeCard.tsx`, `internal/tui/app/{working_mode.go analog, status bar}`, `HttpWsRunnerClient.ts`, `state/store.ts`.
- modules: `internal/worktree` (new); `internal/runner` (additive fields/handlers); desktop components.
- database: none.
- external systems: `git` CLI only.

## 6. Data or Migration Steps

- schema: none — local `sessions.ndjson` run record gains optional `worktree*` fields (`omitempty`); old records decode with `state=none`.
- data backfill: none.
- config updates: `.gitignore` additive append `.flowpilot/worktrees/` (best-effort, idempotent).

## 7. Validation Plan

- tests to add:
  - `internal/worktree`: create/list/resolve/cleanup/conflict/GC matrix on real temp repos; dirty-main pre-check; serialized double-apply.
  - `internal/runner`: start with `worktree:true` → binding + cwd assertion; `worktree:true` from admin client → 403; resume rehydrate + `D-7` validation → `lost` path; delete → merge decision first then GC-eligible; non-git repo → `worktree_unavailable`; **leg switch shares the chat worktree** (files persist across legs); **leg switch after restart** inherits via `ListProviderSessionsByChat`; **toggle-off with live binding rejected**; **flow-run owner = flow runId** (child agents inherit `parent.workspaceCwd`); leg in a `lost` chat blocked behind the notice.
  - Desktop/TUI: toggle renders + disables appropriately; badge on run item; merge card renders options and conflict paths.
  - E2E (`internal/runner/cp71_worktree_e2e_test.go`): full HTTP-driven suite on real git repos — start → binding → cwd → terminal → `worktree_merge_requested` in event log → `resolve apply_patch` → main workspace + cleanup; leg-switch shares worktree; conflict evidence + retry; restart resume; external-delete → `lost` + leg-blocked; client-gate 403; boot GC prunes orphans only; toggle-off byte parity. Manual/live probes on top: two concurrent vibe runs same project.
- manual checks: restart runner mid-run → binding intact; `keep_branch` leaves `run/<slug>` branch visible in `git branch`.
- failure cases: conflict card content; GC never touches `merge_pending`/resumable worktrees; `git apply` all-or-nothing.

## 8. Rollout and Fallback

- rollout order: `P-1`→`P-5` sequential merges; feature inert until `P-3`.
- fallback path: toggle off = today's behavior exactly; to disable shipped feature, hide toggle — no data migration needed since state is per-run additive.
- monitoring: `[worktree]` log lines on create/resolve/cleanup; GC scan summary at boot.

## 9. Risks

- `R-1` Tournament regression from manager extraction → delegate-or-untouched rule + full `internal/tournament` suite as gate.
- `R-2` Stale-worktree accumulation → Navigator badge + GC + delete notice (`R-2` in SD-27).
- `R-3` Patch-merge loses history → documented; `keep_branch` escape hatch.
- `R-4` Windows path/locking quirks in `git worktree remove --force` → reuse Task-369 cleanup ordering, test on Windows shell.

## 10. Definition of Done

- [ ] `SS-23 AC-1..AC-8` all demonstrable; `E-1..E-7` handled or documented.
- [ ] All pre-existing tests untouched and green; new tests cover the §7 matrix.
- [ ] Provider parity: evidence or tests for Claude/Codex/Grok (OpenCode/Devin noted if identical path).
- [ ] CA ledger entry per slice under `feature_key: run-worktree`; FEATURE-KEYS.md updated.
- [ ] `GitNexus detect_changes` confirms only expected symbols touched before each commit.
