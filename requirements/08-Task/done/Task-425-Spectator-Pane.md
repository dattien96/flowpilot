# Task-425 — Spectator Pane (Read-Only Peek at a Non-Focused Run)

- Document ID: `Task-425`
- Title: `Spectator Pane`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: `CP-82 (Multi-Project Parallel Vibe Operations)`
- Child Documents: ``
- Related Documents: `Task-422, Task-404, CA-922`
- Replaces: ``
- Tags: `desktop, project-nav, monitor`

## AI Quick View

### Summary

- A read-only mini pane docked beside the chat showing one non-focused run:
  project, title, live status, last activity line, waiting chip — refreshed
  from `projectHistoryById` + snapshot cache, never a stream attach.
- Clicking the pane promotes the run via `openRunAtAttention` (same path as
  inbox/board). No composer, no timeline replay, no stream subscription.

### Current Ask

- Implement `spectatorRunId` store field + open/close actions, the pane
  component, and a "Watch" affordance on board rows / inbox items.

### Key Decisions

- `T-1` Spectator state is a single optional field — at most one watched run.
  This keeps the mental model "1 focused + 1 glance", not multi-pane.
- `T-2` Data is poll-derived (30s history warm + snapshot cache) — explicitly
  stale-tolerant; the pane labels its freshness ("updated 12s ago").
- `T-3` If the watched run becomes focused (user opens it), the pane
  auto-closes — never show the focused run as spectator.

### Constraints

- Zero stream/subscription attachments; zero writes to run state.
- Pane must not steal layout space at <1200px width — collapses to a header
  chip.

### Open Questions

- Whether last-activity line needs a snapshot endpoint beyond
  `listRunHistory` fields (`lastPrompt`/`lastMessage` suffice for v1).

### Source Refs

- `CP-82 P-4`, `Task-422`

## 1. Goal

While working in project A, keep one other run visible — know the moment run
B finishes or starts waiting without polling clicks.

## 2. Parent Links

- coding plan: CP-82
- tech design: —
- system spec: SS-13
- specific upstream ids: Task-422, Task-404

## 3. Trigger

Devin-style pinned session glance; covers the "watch run B while doing A"
need without paying for true dual-pane architecture.

## 4. Exact Change

- `T-1` `store.ts`: `spectatorRunId: string | null`, `spectatorProjectId`,
  `openSpectator(runId, projectId)`, `closeSpectator()`; auto-clear inside
  `openRunAtAttention`/`openHistoryRun` when target === spectator.
- `T-2` `src/components/SpectatorPane.tsx`: card showing status icon, title,
  project, waiting chip, last message preview, updated-ago label, "Open"
  button, close `×`.
- `T-3` "Watch" icon action on `SessionsBoard` rows and inbox items.
- `T-4` Styles `.spectator-pane` docked right of chat column; <1200px →
  compact chip in header.

## 5. Touched Areas

- files: `src/state/store.ts`, `src/components/SpectatorPane.tsx` (new),
  `src/components/SessionsBoard.tsx`, `src/components/AttentionInbox.tsx`,
  `src/styles.css`, `src/App.tsx`
- modules: desktop renderer
- routes: none
- tables: none

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/state/store.ts
interface AppState {
  spectatorRunId: string | null;
  spectatorProjectId: string | null;
  openSpectator(runId: string, projectId: string): void;
  closeSpectator(): void;
}
// derive view: find run in projectHistoryById[spectatorProjectId] +
// attentionItems match → status/kind/title/lastMessage/updatedAt.
```

```tsx
// apps/desktop-flowpilot/src/components/SpectatorPane.tsx
export function SpectatorPane(): JSX.Element | null
// null when spectatorRunId unset or === focused runId (T-3)
```

## 7. Test Signatures

- `test("openSpectator sets fields; closeSpectator clears", ...)` — T-1/AC-1
- `test("spectator auto-clears when its run becomes focused", ...)` — T-3/AC-2
- `test("spectator pane renders status + waiting chip from cached data", ...)` — AC-3
- `test("spectator never attaches a stream or writes run state", ...)` — spy on client subscriptions (AC-4)
- `test("spectator of a finished run shows terminal status, no chip", ...)` — AC-5

## 8. Acceptance Check

- Watching a running flow in project B while chatting in A: pane updates on
  the 30s poll, shows waiting chip when B blocks, Open promotes correctly.
- App at 960px width: pane degrades to chip without breaking layout.

## 9. Out of Scope

- True dual interactive panes, `timelineByRunId` namespacing, live stream
  into the pane, multiple watched runs.

## 10. Definition of Done

- [ ] All §6 signatures implemented (or deviation in §11)
- [ ] All §7 tests exist, green, additive-only
- [ ] Pre-existing tests green — old failure → STOP (R1)
- [ ] Provider-agnostic evidenced (R2)
- [ ] `feature_key` = `project-nav`; CA entry written
- [ ] §8 verified; `detect_changes` clean

## 11. Completion Notes

- result: implemented — spectatorRunId/spectatorProjectId + open/close actions, deriveSpectatorView, SpectatorPane docked right of chat (chip under 1200px), watch affordances on board rows and inbox items, auto-clear on promote. 7 tests green.
- follow-ups: true dual interactive panes remain a separate CP (timelineByRunId namespacing).
- upstream docs updated: CA-925.
