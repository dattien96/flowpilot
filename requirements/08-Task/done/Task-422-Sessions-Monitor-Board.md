# Task-422 — Sessions Monitor Board (Cross-Project Run Overview)

- Document ID: `Task-422`
- Title: `Sessions Monitor Board`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: `CP-82 (Multi-Project Parallel Vibe Operations)`
- Child Documents: ``
- Related Documents: `Task-404 (attention queue), Task-405, CA-922`
- Replaces: ``
- Tags: `desktop, project-nav, monitor`

## AI Quick View

### Summary

- Devin-style board overlay listing every known run across ALL projects,
  grouped by project, fed by the already-warmed `projectHistoryById` +
  `attentionItems` — zero new backend polling.
- Rows show live status icon, run title, waiting-kind chip, worktree badge,
  relative time; click = `openRunAtAttention` (switch project + open run).
- Pure derivation model (`boardModel.ts`) keeps the component thin and the
  grouping logic unit-testable without mounting React.

### Current Ask

- Implement the board overlay + derivation model + header toggle, additive
  only.

### Key Decisions

- `T-1` Board is an overlay panel (not a route): toggled from a header icon
  next to the inbox, Esc/scrim closes. The chat workspace underneath never
  unmounts — no state loss.
- `T-2` Data source is `projectHistoryById` (warmed by
  `loadAllProjectHistories`, 30s refresh) + `attentionItems` for waiting
  kind. No additional fetch per row.
- `T-3` Row click reuses `openRunAtAttention(runId, chatId, projectId)` —
  same path as the inbox; no new navigation contract.

### Constraints

- Read-only surface: the board never writes run state; only navigation.
- Design tokens only (spacing/radius/motion); `styles.tokens.test.ts` must
  pass with `SessionsBoard.tsx` added to the guarded list.
- No provider-specific status handling — statuses map through the same
  `STATUS_ICON`/`WAITING_STATUS` tables already used by Navigator.

### Open Questions

- Default grouping order: by project registry order (chosen) vs most-recent
  activity. Registry order is stable and matches Navigator groups.

### Source Refs

- `CP-82 P-1`, `Task-404`, `CA-922`

## 1. Goal

One glance answers "which of my 8–12 runs need me, which are running, which
finished" across every project, with one-click jump into any run.

## 2. Parent Links

- coding plan: CP-82
- tech design: —
- system spec: SS-13 doc contract
- specific upstream ids: Task-404, Task-405, CA-922

## 3. Trigger

Multi-project parallel use case (4 projects × 2–3 flows): the single-focus
workspace is correct, but there is no overview surface — today the operator
must click each project group to discover state.

## 4. Exact Change

- `T-1` New pure module `src/state/boardModel.ts`: derives `BoardRow[]`
  grouped by project from store state.
- `T-2` New component `src/components/SessionsBoard.tsx`: overlay with
  per-project sections, row list, empty states; closes on Esc/scrim.
- `T-3` Header toggle: `BoardIcon` button in `App.tsx` header-actions
  (left of the inbox icon), local `useState` open flag.
- `T-4` Styles: `.board-overlay`, `.board-panel`, `.board-section`,
  `.board-row`, `.board-row-*` — token-based, viewport-safe
  (`max-width: min(720px, 100vw - 32px)`), `min-width: 0` on text.
- `T-5` Add `SessionsBoard.tsx` to the guarded file list in
  `styles.tokens.test.ts` (additive line).

## 5. Touched Areas

- files: `src/state/boardModel.ts` (new), `src/components/SessionsBoard.tsx`
  (new), `src/App.tsx`, `src/styles.css`, `src/styles.tokens.test.ts`,
  `src/components/icons.tsx` (BoardIcon)
- modules: desktop renderer only
- routes: none
- tables: none

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/state/boardModel.ts
export interface BoardRow {
  runId: string;
  chatId: string;
  projectId: string;
  projectName: string;
  runTitle: string;
  status: string;               // RunHistoryItem.status verbatim
  waitingKind?: AttentionKind;  // from attentionItems when waiting
  worktreeBound: boolean;       // worktreeState/worktreeSlug present
  updatedAt: string;
  isFocused: boolean;           // runId === store.runId
}
export interface BoardSection { projectId: string; projectName: string; rows: BoardRow[]; }
export function deriveBoardSections(
  historyByProject: Record<string, RunHistoryItem[]>,
  attention: AttentionItem[],
  projects: ProjectInfo[],
  focusedRunId: string | null,
): BoardSection[]
```

```tsx
// apps/desktop-flowpilot/src/components/SessionsBoard.tsx
export function SessionsBoard(props: { onClose: () => void }): JSX.Element
// Row click → void useStore.getState().openRunAtAttention(row.runId, row.chatId, row.projectId); props.onClose()
```

```ts
// apps/desktop-flowpilot/src/state/store.ts — unchanged (existing selectors only)
```

## 7. Test Signatures

- `test("board groups runs by project and marks focused row", ...)` — T-1/AC-1
- `test("board row carries waitingKind only for waiting runs", ...)` — T-2/AC-2
- `test("board orders sections by project registry order, runs by updatedAt desc", ...)` — AC-3
- `test("board shows worktreeBound when worktreeSlug present", ...)` — AC-4
- `test("board empty state when no project history loaded", ...)` — degraded input (AC-5)
- `test("board click calls openRunAtAttention with row projectId", ...)` — T-3/AC-6

## 8. Acceptance Check

- With 3+ projects warmed, overlay lists all runs grouped under project names
  without horizontal scroll at 960px window width.
- A waiting run shows its kind chip identical to the inbox badge.
- Clicking a row in a non-focused project switches focus and opens the run;
  board closes.

## 9. Out of Scope

- Inline actions on board rows (Task-423 owns those in the inbox; board may
  reuse them later).
- Spectator pane, real-time streaming into rows, server-side "active runs"
  endpoint.

## 10. Definition of Done

- [ ] All §6 signatures implemented exactly (or deviation documented in §11)
- [ ] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [ ] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [ ] Provider parity: statuses rendered verbatim + shared tables → provider-agnostic, evidence in CA (R2)
- [ ] `feature_key` = `project-nav`; CA ledger entry written
- [ ] §8 acceptance checks verified by hand or test
- [ ] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: implemented — boardModel.ts derivation + SessionsBoard overlay + header toggle; 7 tests green.
- follow-ups: real-time row streaming stays out of scope (poll-derived by design).
- upstream docs updated: CA-925.
