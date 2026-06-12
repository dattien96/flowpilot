# BUG-042: Desktop Runner-Mode Project List Fetch Blocked

## Metadata

- Document ID: `BUG-042`
- Title: `Desktop Runner-Mode Project List Fetch Blocked`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-12`
- Last Updated: `2026-06-12`
- Parent Documents: `requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md`, `requirements/10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md`, `requirements/10-Refactor/New-System/04-Detailed-Coding-Plan.md`
- Child Documents: `none`
- Related Documents: `apps/desktop-flowpilot/index.html`, `apps/desktop-flowpilot/src/App.tsx`, `apps/desktop-flowpilot/src/state/store.ts`, `Screenshot 2026-06-12 at 20.24.46.png`
- Replaces: `none`
- Tags: `desktop-flowpilot, runner-mode, regression, navigator, csp`

## AI Quick View

### Summary

- The desktop app showed `runner http://127.0.0.1:4317` in the header but the browser title still said `(mock)`, which made the active transport misleading.
- In runner mode, the navigator failed to load projects and rendered `TypeError: Failed to fetch`.
- The renderer CSP did not allow loopback `connect-src`, so HTTP/SSE calls to the local runner could be blocked before the app ever reached `/client/projects`.
- The startup status badge also stayed `Idle` because navigator-load failures never updated the shared status state.
- The fix allows loopback runner connections, makes the window title reflect the real transport, deduplicates `loadProjects()` so StrictMode does not print the same failure twice, and marks startup failure in the status badge.

### Current Ask

- Record the desktop regression and the scoped fix that restores project loading in runner mode without updating upstream operator docs.

### Key Decisions

- `V-1` Runner-mode desktop builds must allow loopback HTTP/SSE connections to the local runner.
- `V-2` The desktop title and header must describe the same active transport.
- `V-3` Initial navigator loading should stay idempotent under React StrictMode.
- `V-4` Startup connectivity failures should not leave the desktop status badge at `Idle`.

### Constraints

- Keep the fix local to `apps/desktop-flowpilot`.
- Preserve offline mock mode behavior.
- Do not change the runner contract or the refactor design docs for this bug record.

### Open Questions

- Live end-to-end confirmation against a running local runner is still needed after restarting the dev stack.

### Source Refs

- `requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md`
- `requirements/10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md`
- `apps/desktop-flowpilot/index.html`
- `apps/desktop-flowpilot/src/App.tsx`
- `apps/desktop-flowpilot/src/state/store.ts`
- `Screenshot 2026-06-12 at 20.24.46.png`

## 1. Issue Summary

When the desktop renderer was started in runner mode, the project selector stayed empty and the timeline showed `Failed to load projects: TypeError: Failed to fetch`. At the same time, the header exposed the real runner URL while the browser title still said `(mock)`, which obscured the actual state during debugging.
The top-right status badge also remained `Idle`, even though startup had already failed.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/04-Detailed-Coding-Plan.md`
- impacted tech design: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot` running with `VITE_RUNNER_URL=http://127.0.0.1:4317`
- reproduction steps:
  1. Start the desktop app in runner mode through `just dev` or `desktop-dev-runner`.
  2. Open the desktop window and wait for the navigator to load projects.
  3. Observe the empty project selector and the repeated `Failed to load projects: TypeError: Failed to fetch` timeline errors.
  4. Notice the header says `runner http://127.0.0.1:4317` while the browser title still says `(mock)`.
- frequency: consistent when the renderer CSP blocks loopback runner requests

## 4. Expected vs Actual

- expected: runner mode can call the local runner over HTTP/SSE, load projects into the navigator, and show a consistent transport label in both header and window title
- actual: the fetch path failed before project data loaded, the visible transport labels disagreed, and the status badge remained `Idle`

## 5. Impact

- users affected: desktop users running the new runner-backed client
- workflows affected: initial project/workflow/step navigation in runner mode
- severity: high because the runner-backed desktop becomes unusable at startup

## 6. Root Cause

- hypothesis: the desktop renderer was entering runner mode correctly, but browser-side configuration blocked the HTTP fetch path
- confirmed cause: `apps/desktop-flowpilot/index.html` defined a CSP without loopback `connect-src`, so runner-mode HTTP/SSE requests to `127.0.0.1:4317` could fail with `TypeError: Failed to fetch`; the static `(mock)` title in the same file also contradicted the real transport; React StrictMode caused the failing project load to be reported twice; and `loadProjects()` appended startup errors to the timeline without changing `status`, leaving the badge at `Idle`
- evidence:
  - `createRunnerClient()` resolved `VITE_RUNNER_URL`, so the renderer was using `HttpWsRunnerClient`
  - the screenshot showed `runner http://127.0.0.1:4317` in the header while the title still showed `(mock)`
  - `index.html` lacked a `connect-src` directive for loopback runner requests before the fix

## 7. Fix Strategy

- `F-1` Update the desktop CSP to allow loopback `http://127.0.0.1:*`, `http://localhost:*`, and matching `ws://` connections.
- `F-2` Remove the hardcoded `(mock)` title and set `document.title` from the same transport label used in the app header.
- `F-3` Make `loadProjects()` reuse an in-flight promise so StrictMode does not duplicate the same startup error.
- `F-4` Set startup status to `starting`, restore `idle` after a successful project load, and mark `failed` when initial runner-mode project loading fails before any run starts.

## 8. Validation

- `V-1` `npm run typecheck` passed in `apps/desktop-flowpilot`.
- `V-2` `npm run build` passed in `apps/desktop-flowpilot`.
- `V-3` `git diff --check` passed.
- `V-4` GitNexus impact analysis for `App`, `runnerModeLabel`, and `loadProjects` returned `LOW`.
- `V-5` Initial navigator load now transitions the badge through `Starting` and `Failed` instead of leaving it at `Idle` when startup fetch fails.

## 9. Regression Guard

- tests:
  - `apps/desktop-flowpilot` TypeScript build remains green
- alerts:
  - none added
- audit checks:
  - runner-mode UI labels must match the active client transport
  - renderer CSP must continue to permit loopback runner HTTP/SSE traffic
  - navigator startup should not duplicate project-load failures in StrictMode
  - startup fetch failures must surface through the shared status badge when no run exists yet

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`
- notes left unchanged on purpose:
  - `requirements/10-Refactor/New-System/04-07-Operator-Docs.md` was not updated because this issue is being tracked as a bugfix record instead
