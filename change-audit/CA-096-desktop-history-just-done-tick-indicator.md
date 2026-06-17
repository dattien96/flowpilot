# CA-096: Desktop History Just-Done Tick Indicator

## Scope

- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/styles.css`
- `requirements/08-Task/done/Task-066-Desktop-History-Just-Done-Tick-Indicator.md`

Implements Task-066: when a run completes while the user is on a different chat, a green tick icon appears on that history item. Clears on first visit.

## Completed

### I-1 — `newlyCompleted` state + `prevHistoryRef` tracker

Added `useState<Set<string>>(new Set())` for the set of run IDs that completed in the background. Added `useRef<Map<string, RunHistoryItem["status"]>>(new Map())` to track each run's last-seen status across polls without triggering re-renders.

### I-2 — `runIdRef` live-read pattern

```tsx
const runId = useStore((s) => s.runId);
const runIdRef = useRef(runId);
runIdRef.current = runId;
```

Updated synchronously each render so the detection effect reads the live `runId` without depending on it (avoids re-running the effect every time the user switches runs).

### I-3 — Detection effect

New `useEffect` on `[selectedProjectId, historyLoading, runHistory]`. Compares each item against `prevHistoryRef`. Adds the run ID to `newlyCompleted` when: `prevStatus !== undefined` (not first load) AND `prevStatus !== "completed"` AND `item.status === "completed"` AND `item.runId !== currentRunId` (not the active run).

### I-4 — Clear-on-open

`onClick` for each history item: if `newlyCompleted.has(item.runId)`, removes it from the set before calling `openHistoryRun`. Uses a functional `setState` updater to avoid stale closure.

### I-5 — `HistoryStatusIcon` tick case

Added `isNew?: boolean` prop. When true, renders a green SVG checkmark (`polyline` path) using `history-status-icon--complete` class. The `isNew` check runs before all existing status checks so it takes priority.

### I-6 — CSS

```css
.history-status-icon--complete {
  color: var(--ok);
}
```

Added after `.history-status-icon--question` in `styles.css`.

## Verification

- `npx tsc --noEmit -p apps/desktop-flowpilot/tsconfig.json` → **TypeScript: No errors found**
- Logic invariant verified by inspection (see Task-066 V-2).
- End-to-end UI smoke test not run in this session.

## Residual Notes

- `newlyCompleted` is session-only / ephemeral — not persisted to localStorage or the store. If the app is reloaded, no ticks are shown regardless of background completions.
- `prevHistoryRef` is keyed by `runId` (globally unique UUIDs) so there is no collision between projects.
