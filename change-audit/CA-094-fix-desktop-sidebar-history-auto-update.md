# CA-094: Fix Desktop Sidebar History Auto-Update

## Scope

- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/components/RunStatus.tsx`
- `requirements/09-BugFix/done/BUG-078-Desktop-Sidebar-History-Does-Not-Auto-Update.md`

Fixes BUG-078: the Navigator sidebar history panel was permanently stale until the user manually pressed the "HISTORY" button in the top-right header. Also removes that button as it duplicated history already shown in the sidebar.

## Completed

### F-1 — Always refresh history after sendPrompt

Removed the `historyOpen` guard in `store.ts` `sendPrompt` finally block. Previously: `if (get().historyOpen) { void get().loadRunHistory(); }`. Now: `void get().loadRunHistory()`. History refreshes after every run turn regardless of whether the old popover was open.

### F-2 — Navigator polling

Added a polling `useEffect` to `Navigator.tsx`. When the current run is active (`running | starting | waiting_approval | waiting_question`) it polls every 3 s; when idle it polls every 10 s. The interval is torn down and restarted when `status` or `selectedProjectId` changes, so a transition from active to idle immediately switches cadence.

Added `status` from the store as a new subscription in Navigator.

### F-3 — Remove History button and historyOpen state

Removed from `store.ts`:
- `historyOpen: boolean` state field and its initial value
- `toggleRunHistory(): Promise<void>` action and implementation
- All `historyOpen: false` resets in `selectProject`, `openHistoryRun`, and `resetRun`

Rewrote `RunStatus.tsx` to remove the History button, its popover, the `useRef`/`useEffect` click-outside handler, and all history-related state subscriptions. Component now renders only: status dot, status label, run ID, Stop button (when active), and New run button (when a runId exists).

## Verification

- `npx tsc --noEmit -p apps/desktop-flowpilot/tsconfig.json` → **TypeScript: No errors found**
- Grep for `historyOpen` and `toggleRunHistory` in `apps/desktop-flowpilot/src` → **No matches found**
- Manual Electron smoke test not run (no preview server in this session).

## Residual Notes

- The existing `_historyLoadSeq` stale-response guard (BUG-060 F-3) remains active and protects against overlapping `loadRunHistory` calls that the new polling may cause.
- Polling cadence (3 s / 10 s) matches the admin-web active-session polling pattern. Adjust only after confirming server load.

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: BUG-078
change_type: fix
summary: Fix Desktop Sidebar History Auto-Update
# --->8---
