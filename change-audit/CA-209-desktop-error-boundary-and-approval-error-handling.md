# CA-209: Desktop Error Boundary And Approval Error Handling

## Summary

Fixed `BUG-172`: approving a pending step in a Flow Mode + YOLO run could leave the entire FlowPilot Desktop window blank/black with no error message, recoverable only by restarting the app. Root cause was structural — `apps/desktop-flowpilot/src` had no React error boundary anywhere, so any uncaught render exception during the post-approval state churn unmounted the whole React tree, leaving only the dark theme's `--bg: #0e1117` body background visible (near-black) alongside the still-present native menu bar.

## What Changed

- `apps/desktop-flowpilot/src/components/ErrorBoundary.tsx` (new): a class component that catches render errors anywhere below it and renders a recoverable "Something went wrong" fallback card (reusing existing `.status-shell`/`.status-card` styles) with the error message and a Reload button, logging the error + component stack via `console.error`.
- `apps/desktop-flowpilot/src/main.tsx`: wraps `<App />` in `<ErrorBoundary>` at the root render call.
- `apps/desktop-flowpilot/src/state/store.ts`: `approve()` and `answer()` now wrap their `client.submitApproval`/`client.answerQuestion` calls in `try/catch`, mirroring `sendPrompt`'s existing error handling — on failure they set `status: "failed"`, `recoverable: true`, and append a `system`/`error` timeline entry instead of leaving an unhandled promise rejection.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Started the Vite dev server and loaded the renderer in a browser preview — boots cleanly with no console errors (no runner backend available in this environment, so could not proceed past bootstrap to a live Flow Mode run).
- Not verified live against the original repro (approve a Flow+YOLO pending step) — flagged in `BUG-172` (`V-3`); the exact throwing component/line is still unconfirmed. The new boundary will surface the real error and component stack if it recurs.

## Notes

- GitNexus MCP tools were unavailable in this session; both edits are additive (new component, added `try/catch`) and don't change existing signatures or call sites, so proceeded with manual inspection per the skill's fallback rule.
- Related to the same user report thread as `BUG-168`/`BUG-169`/`BUG-170` (Flow Mode approval/resume UX), but is a distinct, previously undocumented defect.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-172
change_type: bugfix
summary: Add a root-level React error boundary and harden approve/answer error handling so an uncaught render error during Flow Mode approval no longer blanks the entire desktop window
# --->8---
