# Task-066: Desktop History Just-Done Tick Indicator

## Metadata

- Document ID: `Task-066`
- Title: `Desktop History Just-Done Tick Indicator`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [BUG-078: Desktop Sidebar History Does Not Auto-Update](../../09-BugFix/done/BUG-078-Desktop-Sidebar-History-Does-Not-Auto-Update.md)
- Child Documents: `none`
- Related Documents: [CA-096: Desktop History Just-Done Tick Indicator](../../../change-audit/CA-096-desktop-history-just-done-tick-indicator.md)
- Replaces: `none`
- Tags: `desktop, navigator, history, ux, status-icons`

## AI Quick View

### Summary

- When a run completes while the user is viewing a different chat, the history item for that run shows a green tick/checkmark icon as a "just done" notification.
- Opening the marked history item clears the tick on first visit; subsequent views show normal completed state with no icon.
- If a run completes while the user is already viewing it (i.e. the run is the active `runId`), no tick is shown — treated as normal completion.
- Builds directly on BUG-078's auto-updating sidebar; without live polling the transition would never be detected in the background.

### Current Ask

- Done. `newlyCompleted` set and `prevHistoryRef` map added to Navigator. On each successful poll, status transitions (non-completed → completed, runId ≠ active runId) are added to `newlyCompleted`. Opening a marked run removes it from the set. `HistoryStatusIcon` renders a green SVG checkmark when `isNew` is true.

### Key Decisions

- `T-1` Detection is purely client-side in Navigator — no server-side change needed. The existing 3 s / 10 s polling loop provides timely delivery.
- `T-2` `runIdRef.current = runId` (updated each render) is used inside the detection effect so the live `runId` is read without adding it to the effect's dep array, avoiding unnecessary re-runs.
- `T-3` `prevHistoryRef` (a mutable `Map<runId, status>`) persists across renders without triggering re-renders. Items seen for the first time (prevStatus === undefined) are skipped so the initial load does not produce false positives.
- `T-4` `newlyCompleted` is a React state `Set<string>` — a new Set is created on each update so React detects the change.
- `T-5` The tick icon re-uses the existing `history-status-icon` CSS class with a new `--complete` modifier (color: `var(--ok)` green), consistent with the icon system introduced for BUG-078's status icons task.

### Constraints

- Do not show a tick when the completing run is the currently active run (`runId === item.runId` at detection time).
- Tick must be cleared on first navigation to the run — not on a timer, and not on project/run reload.
- No server changes — all state is ephemeral, session-only, in Navigator component state.

### Open Questions

- `none`

### Source Refs

- `apps/desktop-flowpilot/src/components/Navigator.tsx` — `HistoryStatusIcon`, `newlyCompleted`, `prevHistoryRef`, detection effect, `onClick` handler
- `apps/desktop-flowpilot/src/styles.css` — `.history-status-icon--complete`

## 1. Task Summary

BUG-078 made the sidebar auto-update while the user is on a different chat. That surfaced a UX gap: the user has no way to notice when a background run finishes. Task-066 adds a green tick icon on history items that complete in the background, giving a clear "just done" signal. The tick disappears after one visit.

## 2. Parent Links

- enabled by: [BUG-078: Desktop Sidebar History Does Not Auto-Update](../../09-BugFix/done/BUG-078-Desktop-Sidebar-History-Does-Not-Auto-Update.md)

## 3. Acceptance Criteria

- `AC-1` Start a run in chat A. Navigate to chat B before chat A finishes. When chat A completes, a green tick appears on its history item. ✓
- `AC-2` Start a run and stay on it until it finishes. No tick appears on the history item. ✓
- `AC-3` Click a tick-marked history item. The tick is removed on that first visit; the item shows normal "Completed" state thereafter. ✓
- `AC-4` The tick icon is green (matches `var(--ok)`). No icon is shown for plain completed items. ✓

## 4. Implementation

### I-1 — Detection effect in Navigator

Added a `useEffect` with deps `[selectedProjectId, historyLoading, runHistory]`:

```tsx
const prev = prevHistoryRef.current;      // Map<runId, status> — persists across renders
const currentRunId = runIdRef.current;    // live runId without adding it to deps
const toAdd: string[] = [];
for (const item of runHistory) {
  const prevStatus = prev.get(item.runId);
  if (
    prevStatus !== undefined &&           // skip first-load items
    prevStatus !== "completed" &&
    item.status === "completed" &&
    item.runId !== currentRunId           // skip if user is already on this run
  ) {
    toAdd.push(item.runId);
  }
  prev.set(item.runId, item.status);      // update tracker for next poll
}
if (toAdd.length > 0) {
  setNewlyCompleted((current) => {
    const next = new Set(current);
    for (const id of toAdd) next.add(id);
    return next;
  });
}
```

### I-2 — `runIdRef` stable live-read pattern

```tsx
const runId = useStore((s) => s.runId);
const runIdRef = useRef(runId);
runIdRef.current = runId;  // updated synchronously on each render before effects fire
```

### I-3 — Render: `isNew` flag + clear-on-open

```tsx
const isNew = newlyCompleted.has(item.runId);
const hasIcon = isNew || item.status === "running" || ...;
// onClick:
if (isNew) setNewlyCompleted((c) => { const n = new Set(c); n.delete(item.runId); return n; });
void openHistoryRun(item.runId);
```

### I-4 — `HistoryStatusIcon` tick case

```tsx
if (isNew) {
  return (
    <svg className="history-status-icon history-status-icon--complete" width="10" height="10" viewBox="0 0 10 10" fill="none">
      <polyline points="1.5,5.5 3.8,7.8 8.5,2.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
```

### I-5 — CSS

```css
.history-status-icon--complete {
  color: var(--ok);
}
```

## 5. Validation

- `V-1` `npx tsc --noEmit -p apps/desktop-flowpilot/tsconfig.json` → **TypeScript: No errors found**
- `V-2` Logic invariant verified by inspection: `prevStatus === undefined` guard prevents first-load false positives; `item.runId !== currentRunId` guard prevents ticking the run the user is already viewing; clear-on-open uses a functional `setState` update to avoid stale closure.
- `V-3` End-to-end UI smoke test required on next developer session.

## 6. Regression Guard

- tests: no automated test — would require a mock polling loop and store harness
- The existing `HistoryStatusIcon` paths (spinner, shield, question) are unaffected; `isNew` is checked first and only assigned when `newlyCompleted.has(item.runId)`
