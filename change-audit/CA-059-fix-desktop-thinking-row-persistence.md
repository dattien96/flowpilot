# CA-059: Fix Desktop Thinking Row Persistence

## Summary

- Fixed the desktop timeline reducer so `Thinking...` stays anchored below the latest generated response while a turn is still active.
- Removed the `tool_completed` reducer escape path that skipped normal finalization and dropped the live thinking row after the first finished tool.
- Extracted the timeline event reducer into a pure module so the streaming behavior can be regression-tested outside the Zustand store.
- Added focused reducer tests that verify:
  - `Thinking...` remains after `tool_completed`
  - `Thinking...` disappears only after `turn_completed`

## Changed Files

- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/state/timelineReducer.ts`
- `apps/desktop-flowpilot/src/state/timelineReducer.test.ts`

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot`
- `npx tsc src/state/timelineReducer.ts src/state/timelineReducer.test.ts src/types/contract.ts --outDir /tmp/desktop-flowpilot-reducer-test --module NodeNext --moduleResolution NodeNext --target ES2022 --lib ES2022,DOM --strict --esModuleInterop --skipLibCheck`
- `node --test /tmp/desktop-flowpilot-reducer-test/state/timelineReducer.test.js`
- `npm run build` in `apps/desktop-flowpilot`

## Residual Risk

- The desktop package still lacks a first-class frontend test runner, so this regression guard currently uses a focused reducer test compiled with one-off TypeScript output.
- GitNexus MCP tooling was not available in this session, so symbol impact and change-scope checks were done with local code search and diff inspection instead of graph analysis.
