# CP-82 — Multi-Project Parallel Vibe Operations (Monitor Board + Inbox Actions)

- Document ID: `CP-82`
- Title: `Multi-Project Parallel Vibe Operations`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: `Task-404 (attention queue), Task-405 (desktop UI consistency), SS-23/SD-27 (worktree isolation), CA-922`
- Child Documents: `Task-422 (board), Task-423 (inbox actions), Task-424 (worktree proofs), Task-425 (spectator), CP-82-Test-Steps`
- Related Documents: `Task-421 (windowed timeline), CP-71 (worktree isolation)`
- Replaces: ``
- Tags: `desktop, project-nav, attention-queue, chat-ui, parallelism`

## AI Quick View

### Summary

- Real target use case: ~4 projects × 2–3 concurrent vibe flows (8–12 live
  runs). Runner-side concurrency is already designed in (`runs map[string]`
  per-run provider processes); the gap is entirely on the **awareness and
  action** layer of the desktop UI.
- Plan keeps the single-focused-workspace model (one attached run, one
  composer) and adds a multi-project awareness layer: sessions monitor board,
  inline attention actions, and a read-only spectator pane — deliberately
  deferring true dual-interactive panes.
- Worktree isolation is the user's responsibility to enable; this plan only
  verifies the code-level guarantee that parallel runs can never share one
  worktree, and surfaces worktree state better in the UI.

### Current Ask

- Deliver a monitor board + inline inbox actions so an operator can supervise
  8–12 parallel runs without losing track, plus tests proving worktree
  uniqueness for parallel same-project runs.

### Key Decisions

- `P-1` Single attached run stays: `selectedProjectId`/`runId`/`timeline`
  keep their single-focus semantics. All new surfaces are read-only or
  action-at-a-distance; no second live stream attach in this CP.
- `P-2` The monitor board is fed by the already-warmed
  `projectHistoryById` + `attentionQueue` slices — no new backend polling
  beyond the existing 30s warm + active-project poll.
- `P-3` Worktree uniqueness is verified by code + tests (ownerID = chatId for
  chat runs / runId for flow runs → path `.flowpilot/worktrees/<ownerID>`,
  branch `fp/<slug>-<owner-suffix>`; `Create` fails closed on collision),
  not left to user discipline.
- `P-4` Spectator pane is read-only (status + latest activity via snapshots),
  promoted-to-focus on click. No composer, no stream attach.

### Constraints

- No changes to run/session durability contracts; monitor data is derived,
  never authoritative.
- Inline approve must go through the exact same `approve`/`answer` store
  actions as the in-chat cards — no parallel approval path.
- Provider parity: board/actions must not assume one provider's status set.

### Open Questions

- Does the runner expose a cheap "all runs status" endpoint, or is per-project
  `listRunHistory` polling sufficient at 8–12 runs? (Measure payload size
  first; add `/client/runs/active` only if polling cost is real.)
- Spectator pane data source: reuse `_runSnapshots` cache vs a snapshot
  endpoint — decide at P-4 planning.

### Source Refs

- `Task-404` (attention queue), `Task-405` (desktop UI), `SS-23`/`SD-27`
  (worktree D-7/D-8 binding rules), `CA-922`

## 1. Goal

An operator can run 8–12 concurrent vibe flows across ~4 projects and, at a
glance: see which runs are live/waiting/done, approve gates without
context-switching, and drop into any run in one click — without weakening the
single-focused-workspace model or worktree isolation.

## 2. Input Documents

- `Task-404` — attention queue semantics this CP extends cross-project.
- `Task-405` / CA-919 — design-token system the board must consume.
- `SS-23` / `SD-27` — worktree isolation contract (D-7 validate, D-8 binding
  ownership); P-3 tests these guarantees.
- `CA-922` — the UI pass that built `projectHistoryById`, per-project
  attention slices, and the header inbox this CP builds on.

## 3. Implementation Strategy

- overall approach: additive awareness layer. Everything reads existing
  derived state (`projectHistoryById`, `attentionItems`, `agentRuns`); the
  only writes are user actions routed through existing store actions.
- sequencing logic: P-1 board first (biggest visibility win, zero risk);
  P-2 inbox actions next (act on what the board surfaces); P-3 worktree
  guarantees are test-only verification; P-4 spectator last (nicest, most
  optional).
- dependencies: none on runner changes unless Open Question 1 proves a
  multi-run status endpoint is needed.

## 4. Work Breakdown

- `P-1` **Sessions monitor board** — a toggleable view (header icon or left
  rail section) listing every known run across all projects, grouped by
  project, each row showing live status icon, title, waiting-kind chip, and
  relative time. Clicking a row = `selectProject` + `openHistoryRun` (the
  `openChatInProject` path already built). Data: `projectHistoryById`
  (warmed by `loadAllProjectHistories`) + `attentionItems`.
- `P-2` **Inline attention actions (no focus switch)** — inbox popover items
  get context-appropriate quick actions rendered from the item's own pending
  payload (approve/deny for approvals, option chips for questions/decisions).
  Verified feasible without switching: `client.submitApproval(approvalId)` /
  `client.answerQuestion(questionId)` are ID-scoped RPCs — approval/question
  IDs are globally unique server-side, so the call itself needs no project or
  run context. What must change is the optimistic-update layer: existing
  `approve()`/`answer()` write to the *focused* slot (`pendingApprovals`,
  `status`, `timeline`), so this slice adds a run-scoped variant
  (e.g. `approveAttentionItem(item, decision)`) that calls the client
  directly, evicts the item from `attentionQueue`, and toasts on failure
  without touching focused-run state. Fallback "Open" still switches + opens.
  Open scope: a "quick prompt" mini-composer targeting a non-focused run is
  possible server-side (`sendTurn` is run-scoped) but `sendPrompt` is wired
  to focused state — defer unless a scoped variant proves cheap.
- `P-3` **Worktree uniqueness verification (code-level)** — tests proving two
  parallel runs in one repo can never share a worktree: distinct chatIds →
  distinct `ownerID` → distinct `Path`; same chat's legs inherit one binding
  by design; `Create` fails `worktree_create_failed` on path collision rather
  than silently sharing. Plus UI: worktree badge on monitor rows.
- `P-4` **Spectator pane** — read-only mini view of one non-focused run
  (status, last activity, attention state) docked beside the chat; click to
  promote. Explicitly no composer/stream attach — promotion reuses
  `openHistoryRun`.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/` (new `SessionsBoard`,
  `AttentionInbox` actions, spectator pane), `src/state/store.ts`
  (derived selectors only), `src/styles.css`
- modules: desktop renderer only for P-1/P-2/P-4; P-3 adds Go tests under
  `apps/local-runner/internal/worktree` + `internal/runner`
- database: none
- external systems: none

## 6. Data or Migration Steps

- schema: none.
- data backfill: none.
- config updates: none.

## 7. Validation Plan

- tests to add:
  - `test("monitor board groups runs by project across cached histories", …)`
  - `test("inbox inline approve calls store approve without project switch", …)`
  - `test("inbox quick action falls back to open for unactionable kinds", …)`
  - `TestWorktree_ParallelRunsGetDistinctPaths` (Go) — two chat runs with
    different chatIds in one repo create distinct worktrees.
  - `TestWorktree_CreateFailsOnPathCollision` (Go) — second create with the
    same ownerID fails closed.
- manual checks: 4 projects × parallel flows — board stays accurate during
  stream, inbox actions resolve without stealing focus, spectator never
  attaches a stream.
- failure cases: run finishes while its row is rendering; attention item
  resolved elsewhere must disappear from board+inbox consistently.

## 8. Rollout and Fallback

- rollout order: board ships hidden behind no flag (additive nav surface);
  inbox actions additive; spectator opt-in via row action.
- fallback path: every surface is read-only/additive — removing the board
  loses zero function.
- monitoring: none (desktop local app).

## 9. Risks

- `R-1` Inline actions on stale snapshot state → approve call may 409;
  mitigate by surfacing the error toast and refreshing history.
- `R-2` 30s warm staleness makes the board lag; acceptable for v1 — the row
  opens the live run on click regardless.
- `R-3` Scope creep into true multi-pane; P-4 is the ceiling — anything more
  needs a new CP with `timelineByRunId` namespacing.

## 10. Definition of Done

- [ ] Board shows all known runs grouped by project with correct status.
- [ ] Inbox quick actions work for approval/question kinds; unactionable kinds
      degrade to "Open".
- [ ] Worktree uniqueness tests green; no code change needed unless a gap is
      found.
- [ ] Spectator pane is read-only and promotes via `openHistoryRun`.
- [ ] All additive tests; existing suite green.
- [ ] CA entries per slice; commit format `[Feature][project-nav|attention-queue]`.
