# CA-103: Navigator Sync Progress Indicator

## Scope

- `apps/desktop-flowpilot/src/components/Navigator.tsx`
- `apps/desktop-flowpilot/src/components/navigatorHistory.ts`
- `apps/desktop-flowpilot/src/components/navigatorHistory.test.ts`
- `apps/desktop-flowpilot/src/styles.css`
- `requirements/09-BugFix/done/BUG-088-Desktop-History-Sync-Actions-Missing-In-Progress-Indicator.md`

Implements `BUG-088`: when a chat sync starts, the Navigator now shows visible progress at both the row and project-sync-all levels.

## Completed

### I-1 — Project-level syncing helper

Added `isProjectSyncing(history, projectId)` in `navigatorHistory.ts` so the group header can derive whether any row in the project is actively syncing.

### I-2 — Sync-all loading indicator

Updated the project-level `Sync all` chip in `Navigator.tsx`:

- disables while batch sync is active
- swaps the static sync glyph for `history-status-spinner`
- changes the label from count-only to `Syncing…`

### I-3 — Normal row sync indicator

Updated the normal history row rendering in `Navigator.tsx`:

- when `item.syncStatus === "syncing"`, the row shows `history-status-spinner`
- row meta text changes to `Syncing to Drive…`

This makes progress visible after the confirmation flow exits selection mode.

### I-4 — Styling

Added a disabled state for `.project-history-sync-all` in `styles.css` so the loading button does not look clickable while a batch is already in flight.

### I-5 — Focused regression test

Added `navigatorHistory.test.ts` to verify:

- project syncing is detected when any row in the project is `syncing`
- rows from other projects and non-syncing rows do not trigger batch loading state

## Verification

- `npm --prefix apps/desktop-flowpilot run typecheck` → passed
- `npx tsx --test apps/desktop-flowpilot/src/components/navigatorHistory.test.ts` → passed

## Residual Notes

- The fix intentionally reuses the existing `runHistory.syncStatus` field from the store.
- No runner or Drive API behavior changed; this is a desktop-side UX correction only.

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: BUG-088
change_type: feature
summary: Navigator Sync Progress Indicator
# --->8---
