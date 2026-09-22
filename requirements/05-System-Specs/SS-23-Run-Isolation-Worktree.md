# SS-23: Run Isolation Worktree

## Metadata

- Document ID: `SS-23`
- Title: `Run Isolation Worktree — Per-Run Git Worktree For Parallel Vibe/Harness Runs`
- Phase: `system_spec`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [SS-18: Vibe Working Mode](./SS-18-Vibe-Working-Mode.md), [SS-16: Agent Flow Engine](./SS-16-Agent-Flow-Engine.md), [SS-19: Engineering Harness Flow Family](./SS-19-Engineering-Harness-Flow-Family.md)
- Child Documents: `SD-27 (planned)`, `CP-71 (planned)`
- Related Documents: [SS-08: Approve Gate](./SS-08-Approve-Gate.md), [SD-25: Recovery Ownership Linearization Closure](../06-System-Tech-Design/SD-25-Recovery-Ownership-Linearization-Closure.md), [SD-26: Chat Continuity SSOT](../06-System-Tech-Design/SD-26-Chat-Continuity-Ssot.md), [Task-369: Worktree Rollout Manager](../08-Task/done/Task-369-Worktree-Rollout-Manager.md)
- Replaces: `None`
- Tags: `worktree, isolation, vibe-mode, desktop, tui, parallel-runs`

## AI Quick View

### Summary

- Running multiple vibe (or harness) flows concurrently on the same project is blocked today by a shared working tree: two runs editing `requirements/` and source files in the same checkout clobber each other.
- This spec defines an **opt-in per-owner git worktree**: a toggle in the chat controls bar (next to working-mode / reasoning / yolo controls) that, when enabled at start, executes the run inside `.flowpilot/worktrees/<ownerId>` on its own slugified branch — where owner = the chat (all legs share) for chat mode, and the flow run for harness/vibe mode.
- At run end the user gets a merge-back choice — apply the worktree diff to the main workspace, keep the branch for later, or discard. Merge conflicts surface as a user card with the patch and conflicting paths as evidence; worktrees are never orphaned.
- The model mirrors Devin Desktop's per-session `gitWorktreePath` + `CreateWorktree`/`ResolveWorktreeChanges` lifecycle, implemented on top of the proven `tournament.WorktreeManager` semantics (Task-369).
- Backend already supports concurrent runs; this spec adds isolation so concurrency is *safe* on one project.

### Current Ask

- Define the user-facing contract for worktree-isolated runs: the start toggle, the worktree badge, the merge-back decision card, conflict handling, and recovery expectations — so `SD-27`/`CP-71` can implement without reopening product questions.

### Key Decisions

- `AC-1`/`AC-10` Worktree is **opt-in per chat**, toggle persisted on the run record: new chat defaults off; reopening a chat restores the toggle from its newest run; immutable once a run starts.
- `AC-1b`/`AC-6` On resume/reattach/history-open the binding is **validated** (`git worktree list` + dir + `.base` sidecar); externally-deleted worktrees mark the run `lost` — never silently recreated or duplicated.
- `AC-2`/`AC-7` Provider-agnostic: identical for Claude, Codex, Grok, OpenCode, Devin — the runner just gives the run a different cwd.
- `AC-3`/`AC-9` **One worktree owner ↔ one worktree**: all legs of a chat share it (provider switches keep files); flow runs own their own worktree.
- `AC-4`/`AC-5` Merge-back is an explicit user decision (apply patch / keep branch / discard) with conflict evidence — never auto-commit/auto-merge; survives restart per CP-51.

### Constraints

- Local-only feature: Desktop + TUI clients. Admin Web is out of scope (mirrors vibe-mode precedent, SS-18).
- No Supabase schema migration; worktree state rides on the local run/session record.
- Must not regress the CP-65 tournament worktree path — the shared manager is generalized, not moved/broken.
- `safe-fix-contract` applies to implementation: additive tests only, old suite stays green, all-provider parity proven or evidenced.

### Open Questions

- `Q-1` Resolved — toggle persisted **per chat**; new chats default off.
- `Q-2` Resolved — merge-back is a dedicated card in the chat timeline (patch preview + conflict paths), plus a Navigator badge.

### Source Refs

- `SS-18 AC-1..AC-10` (vibe entries and sprint loop), `SS-16`, `SS-19`, `SS-08` (approve-gate card surface), `Task-369 T-1..T-5` + `CA-877` (proven worktree lifecycle semantics), CP-51/SD-25 (durable dispatch + recovery), Devin Desktop worktree model (`gitWorktreePath`, `ResolveWorktreeChanges`, `worktreeFor`).

## 1. Goal

Let a user run many AI flows — especially vibe flows — concurrently on the same project without them overwriting each other's files, by giving each opted-in run its own git worktree and a clean, user-controlled merge-back path.

## 2. Problem

- Vibe mode (SS-18) is designed to be fire-and-forget: after the entry lock the user is only asked on `r-requirement` or owner-debate no-consensus. The natural usage is N vibe runs in parallel.
- Today all runs share one working tree. Two concurrent runs writing `requirements/**` or source files produce interleaved, silently corrupted state — worse than a slow workflow, it is a wrong-result workflow.
- The tournament harness (CP-65) already proved worktree isolation works inside our engine, but it is internal-only (candidate scratch dirs, auto-cleaned, never user-visible). There is no user-facing "run this in an isolated copy" capability.

## 3. Scope

- In scope:
  - Per-run worktree creation/reuse/cleanup for runs started from Desktop and TUI.
  - A chat-controls toggle ("Run in worktree") captured at run start.
  - Worktree visibility: badge/label in Navigator run items and TUI run status.
  - Merge-back decision surface at run end: apply patch to main workspace / keep branch / discard, including conflict card.
  - Recovery: worktree binding persists across runner/app restart and reattach; safe GC of stale worktrees.
- Out of scope:
  - Admin Web surface.
  - Multi-repo / cross-project worktrees.
  - Automatic conflict resolution, auto-merge, or auto-commit in the user's repo.
  - Changes to flow engine semantics, gate rules, or provider prompts (isolation is a working-directory concern only).

## 4. Non-Goals

- Not a replacement for the tournament harness's internal candidate worktrees (those remain auto-managed scratch).
- Not a CI/CD branching strategy — no PR creation, no remote push.
- Not a mechanism to share a worktree across **different** owners — legs of one chat share theirs (BR-1), but two chats / two flows never share.

## 5. User Stories or Primary Use Cases

- `US-1` As a vibe user, I want to start three vibe runs on one project at once, so that each builds its own slice without file collisions.
- `US-2` As a user, I want to see which runs are isolated (worktree badge) in the Navigator, so that I know which chats will need a merge-back decision.
- `US-3` As a user, when a worktree run finishes, I want to review and apply its diff to my real workspace — or keep it as a branch — so that I stay in control of what lands.
- `US-4` As a user, when merge-back conflicts with edits I made meanwhile, I want a card showing the conflicting paths and the patch, so that I can resolve manually without losing work.
- `US-5` As a TUI user, I want the same opt-in flag and merge-back prompt, so that isolation is not a desktop-only feature.

## 6. Acceptance Criteria

- `AC-1` A "Run in worktree" toggle is available in the chat controls/posture area on Desktop and as a start option on TUI; default off; once the run starts the setting is immutable for that run.
- `AC-2` With the toggle on, the run's entire file surface (provider cwd, flow artifacts, `requirements/` writes) is rooted at `.flowpilot/worktrees/<ownerId>` on a dedicated branch; the main workspace sees zero modifications until merge-back.
- `AC-3` Concurrent worktree runs on the same project do not interfere: two vibe runs started together each observe only their own file changes. The isolation contract is identical for chat-mode runs and flow-mode runs (harness + vibe) — the worktree is bound to the run/chat, not to the flow engine.
- `AC-4` Merge-back trigger differs by run kind, same decision contract: **flow runs** (harness + vibe) get the decision card automatically at terminal status; **chat runs** (ongoing, no natural end) get a user-initiated "Merge worktree back" action in the chat controls plus the same decision prompt when the chat is deleted. Both offer `apply patch`, `keep branch`, `discard`; the decision and outcome are recorded in the run timeline/audit.
- `AC-5` If `apply patch` fails due to workspace drift/conflict, a card presents the conflict paths and the generated patch; the worktree is preserved until the user resolves or discards (no silent loss), consistent with the no-orphan evidence policy.
- `AC-6` Worktree binding survives runner restart, app restart, and reattach/open-from-history; a recovered run reports the same worktree state — or `lost` when validation per `AC-1b` fails.
- `AC-7` Behavior is identical across Claude, Codex, Grok, OpenCode, and Devin providers; no provider-specific worktree logic exists.
- `AC-8` With the toggle off, behavior is byte-for-byte today's behavior (no worktree, main workspace cwd).
- `AC-9` One worktree owner ↔ one worktree: every leg of a chat (provider switches, reattach) shares the chat's worktree and keeps its files; a flow run is its own owner and its spawned children/agents run inside the parent's worktree. Worktree path is `.flowpilot/worktrees/<ownerId>` where owner = `chatId` (chat mode) or the flow run id (flow mode).
- `AC-10` The toggle value persists on the run record; reopening/switching a chat pre-sets it from the chat's newest run. A chat with a live (`active`/`merge_pending`) binding cannot toggle off until the user merges or discards the worktree — the control is rejected with an explanatory message.

## 7. Business Rules

- `BR-1` One **worktree owner** ↔ at most one worktree. Owner = `chatId` for chat-mode runs (all legs share it); owner = the flow run id for harness/vibe flow runs (spawned children inherit the parent's worktree). Worktrees are never shared between owners.
- `BR-2` The system never runs `git commit`, `git merge`, or `git push` in the user's main workspace; merge-back is patch application only.
- `BR-3` The worktree base is the run-start `HEAD` commit captured at `createRun` for **every** run kind (chat + flow); the existing `flowStartGitHead` discipline (CP-55) applies but is flow-only — worktree anchoring must not depend on it, not a moving branch tip.
- `BR-4` `.flowpilot/worktrees/` is gitignored so child worktrees never appear as untracked dirt.
- `BR-5` `discard` removes the worktree and its branch; `keep branch` removes the worktree but retains the branch; `apply patch` applies the diff then removes both.
- `BR-6` A run without a completed merge-back decision may keep its worktree indefinitely (user data), but boot-time GC may prune worktrees whose runs were deleted or are corrupt.
- `BR-7` The toggle is a start-time input only; there is no mid-run "move into worktree".

## 8. Edge Cases

- `E-1` Run started with toggle on inside a dirty workspace → worktree is created from `HEAD` (dirty files are not copied); the merge-back pre-check detects main-workspace drift before applying.
- `E-2` Runner killed mid-run → on restart the worktree dir is reattached to the run record (validated per `AC-1b`); GC must not remove worktrees of live or resumable runs.
- `E-2b` User manually deletes the worktree dir out-of-band → next resume/attach validation detects it → run marked `lost` with a notice offering `archive chat` or `recreate empty worktree from base commit` (explicit user choice — prior changes are gone and the system says so; no silent duplicate, no auto-recreate).
- `E-3` User deletes the run/chat before deciding merge-back → deletion first surfaces the pending merge-back decision (per `AC-4` chat rule); if the user still deletes, the worktree becomes GC-eligible and the notice says an isolated worktree will be removed.
- `E-4` Two worktree runs finish and both apply patches overlapping the same files → merge-back applies are serialized per repo; the second may hit the conflict path (`AC-5`).
- `E-5` Project directory is not a git repo (or `git` missing) → the toggle is disabled with an explanatory tooltip; starting with worktree is rejected with a clear error, never silently ignored.
- `E-6` Very large diffs → patch is stored as a file artifact, not inlined into the timeline event.
- `E-7` Nested/linked worktrees (project already inside a worktree) → behave normally; base detection uses the repo's own `HEAD`.
- `E-8` A new leg (provider switch / reattach) in a chat whose worktree validated `lost` → the leg is blocked behind the `lost` notice (`archive chat` | `recreate empty worktree from base`) — the run never proceeds against a missing tree.

## 9. Dependencies

- `internal/tournament` worktree lifecycle semantics (Task-369) as the proven base to generalize — not duplicated.
- `flowStartGitHead` / baseline fingerprint capture in `flow_executor.go` as the **pattern precedent** for base-commit anchoring (the anchor itself is captured at `createRun` per `BR-3`, not reused from the flow-only field).
- CP-51/SD-25 recovery + `sessions.ndjson` local run record for binding persistence.
- Desktop `ChatPosturePanel` / TUI start options for the toggle surface; Navigator for badges; existing decision/`user.confirm` card plumbing for merge-back.

## 10. Open Questions

- `Q-1` Resolved — the toggle is remembered **per chat** (not per project, not global): new chat defaults off; the persisted chat preference restores on reopen/switch.
- `Q-2` Resolved — dedicated `worktree_merge_requested` card/event (patch preview + conflict paths + 3 actions), not generic `user.confirm`.
- `Q-3` Resolved — `keep branch` prints a `git checkout <branch>` hint in the TUI timeline and a copy-branch button on the Desktop card; no external git-client integration.

## 11. Definition of Done

- This spec is complete when `SD-27` and `CP-71` are authored and trace every AC above, and the merge-back + conflict + recovery behaviors are implementable without new product decisions.
