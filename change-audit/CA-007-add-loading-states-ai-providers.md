# CA-007 Visual Loading States and UI Sync for AI Providers settings

## Scope

Added visual loading feedback when refreshing the provider inventory or installing/refreshing individual providers on the AI Providers settings page (`/settings/ai-providers`). Additionally, fixed a state synchronization bug where manually removing a provider (e.g. Gemini) from the PATH/environment and clicking "Refresh inventory" did not reflect the new `NOT_INSTALLED` state in the UI.

## Completed

- Updated [ai-providers.tsx](file:///c:/working/flowpilot/apps/admin-web/src/routes/_authenticated/settings/ai-providers.tsx):
  - Added router pending state detection using `useRouterState` to show a spinning `RefreshCw` icon and "Refreshing..." text on the global "Refresh inventory" button while invalidation loader runs.
  - Added mutation variables tracking via `installProvider.variables` to identify which specific provider card triggered an install/refresh action.
  - Implemented conditional button disabling and inline visual loading states (`Refreshing...` or `Installing...` with a spinning `RefreshCw` loader) on the active action button during CLI invocation.
  - Disabled all provider buttons and global refresh button while any install/refresh is currently pending to prevent concurrent installations.
  - **Fixed UI Sync bug**: Removed the redundant React state `visibleProviders` and its `useEffect` listener. The component is now purely presentation-driven by the `providers` prop returned from the route's loader, resolving the bug where router loader invalidations failed to update local React state.
- Updated [ai-providers.test.tsx](file:///c:/working/flowpilot/apps/admin-web/src/routes/_authenticated/settings/ai-providers.test.tsx):
  - Mocked `useRouterState` from `@tanstack/react-router` to return `false` by default, preventing crashes caused by missing router context during test execution.
  - Created a React `TestWrapper` component in the test suite to simulate how `router.invalidate()` updates the route matches and changes the component's `providers` prop dynamically.

## Verification

- Executed the unit test suite in `apps/admin-web` via `npm run test` (all 129 tests passed successfully, including the `ai-providers.test.tsx` suite).
- Verified correct behavior using component visual states and mocked mutation responses.

## Residual Notes

- Visual transitions are handled cleanly with Lucide icons (`RefreshCw`) and standard Shadcn button styling.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CA-007
change_type: feature
summary: Visual Loading States and UI Sync for AI Providers settings
# --->8---
