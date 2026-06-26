# CA-062: Fix Desktop Stale Run Stream Events

## Scope

- Corrected desktop live stream folding when users switch between runs while another run is still streaming.
- Kept the fix in the desktop store/reducer layer; no local-runner event persistence, provider adapter, or replay endpoint behavior changed.

## Completed

- Added an active-run guard so provider stream events apply only when the stream run id matches the visible run id.
- Passed the expected run id into `consumeStream()` for direct sends, reconnect, and history-run reopen flows.
- Guarded late send errors from mutating a different visible run.
- Added `BUG-052` and a focused reducer-level regression test for active-run event matching.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot`
- `npx tsc src/state/timelineReducer.ts src/state/timelineReducer.test.ts src/types/contract.ts --outDir C:/working/flowpilot/.tmp-desktop-reducer-test --module NodeNext --moduleResolution NodeNext --target ES2022 --lib "ES2022,DOM" --strict --esModuleInterop --skipLibCheck` in `apps/desktop-flowpilot`
- `node --test C:/working/flowpilot/.tmp-desktop-reducer-test/state/timelineReducer.test.js`

## Residual Notes

- Refresh/reopen behavior was already correct, so no runner persistence fix was needed.
- The desktop package still uses a focused one-off reducer test compile/run for this reducer test rather than a first-class package test script.
- GitNexus MCP tooling was not available in this thread, so impact and scope checks were done with local search and targeted tests.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: BUG-052
change_type: fix
summary: Fix Desktop Stale Run Stream Events
# --->8---
