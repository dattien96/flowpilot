# CA-145: Desktop First-Start Runner Retry

Fix the first-start hang where the desktop app fires a single bootstrap probe before the Go runner has compiled and started, then silently falls back to the Supabase settings tab with no retry.

## Changes

- `apps/desktop-flowpilot/electron/main.ts`: Added 8 s `AbortController` timeout to the `http:request` IPC handler so fetch to a slow-starting runner cannot hang indefinitely.
- `apps/desktop-flowpilot/src/App.tsx` (`refreshBootstrap`): Added retry loop (15 × 2 s) that waits while `runnerReachable === false` before giving up and rendering the error state.
- `apps/desktop-flowpilot/src/App.tsx` (unauthenticated `SettingsShell`): Changed `visibleSections` to include `"runner"` when the runner is offline so the user sees the Runner health panel instead of an inert Supabase form.

# ---8<--- flowpilot:change-ledger
feature_key: terminal-session
source_doc_id: BUG-150
change_type: bugfix
summary: add bootstrap retry loop and IPC timeout to fix first-start desktop hang on Supabase tab
# --->8---
