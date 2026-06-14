# BUG-048: Desktop Dev Used Mock Mode

## Metadata

- Document ID: `BUG-048`
- Title: `Desktop Dev Used Mock Mode`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-14`
- Last Updated: `2026-06-14`
- Parent Documents: `requirements/10-Refactor/New-System/04-07-Operator-Docs.md`, `requirements/10-Refactor/New-System/04-08-Phase8-Cutover-And-Live-Acceptance.md`
- Child Documents: `none`
- Related Documents: `Justfile`, `scripts/supervisor.js`, `apps/desktop-flowpilot/src/client/createRunnerClient.ts`
- Replaces: `none`
- Tags: `desktop-flowpilot, justfile, runner-mode, mock-mode, regression`

## AI Quick View

### Summary

- `just desktop-dev` used to start the desktop app directly, which meant the renderer launched without `VITE_RUNNER_URL` and fell back to `MockRunnerClient`.
- That made the command behave differently from `just dev`, even though both were expected to represent the real desktop transport path.
- The fix reroutes `desktop-dev` through the supervisor in runner-only desktop mode and preserves the offline entry point as `desktop-dev-mock`.

### Current Ask

- Record the bug where `desktop-dev` started mock mode instead of real runner mode and document the command fix.

### Key Decisions

- `V-1` `desktop-dev` should launch the real runner-backed desktop transport, matching the `just dev` desktop path.
- `V-2` Offline mock mode remains available, but only through an explicit `desktop-dev-mock` command.

### Constraints

- Keep the fix local to developer command wiring.
- Do not change the renderer transport selection logic beyond what is necessary to make the command use real mode.
- Preserve the existing `just dev` behavior for the full stack.

### Open Questions

- None for this recorded bug.

### Source Refs

- `Justfile`
- `scripts/supervisor.js`
- `apps/desktop-flowpilot/src/client/createRunnerClient.ts`
- `requirements/10-Refactor/New-System/04-07-Operator-Docs.md`
- `requirements/10-Refactor/New-System/04-08-Phase8-Cutover-And-Live-Acceptance.md`

## 1. Issue Summary

`just desktop-dev` started the desktop app directly, which left `VITE_RUNNER_URL` unset. The renderer then selected `MockRunnerClient`, so the command behaved like an offline demo launch instead of the real desktop transport path.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/04-08-Phase8-Cutover-And-Live-Acceptance.md`
- impacted tech design: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop app launched from the repository `just` recipes
- reproduction steps:
  1. Run `just desktop-dev`.
  2. Observe the desktop app start without a runner URL.
  3. Inspect the renderer transport label and confirm it is using mock mode.
- frequency: always, before the command wiring fix

## 4. Expected vs Actual

- expected: `desktop-dev` launches the desktop in real runner mode, matching the desktop transport used by `just dev`
- actual: `desktop-dev` launched the renderer directly, so the app used `MockRunnerClient`

## 5. Impact

- users affected: desktop developers using the convenience command
- workflows affected: local desktop smoke testing, login/session verification, runner-backed navigation
- severity: medium because it caused a misleading developer entry point and masked real transport issues

## 6. Root Cause

- hypothesis: the `desktop-dev` recipe was still wired to the offline renderer-only path from the mock MVP phase
- confirmed cause: `Justfile` called `npm run dev` inside `apps/desktop-flowpilot` without setting `VITE_RUNNER_URL` or starting the runner
- evidence:
  - `createRunnerClient()` falls back to `MockRunnerClient` when `VITE_RUNNER_URL` is absent
  - `just dev` already sets the runner URL through `scripts/supervisor.js`

## 7. Fix Strategy

- `F-1` Rewire `desktop-dev` to the supervisor so it starts the desktop against the local runner.
- `F-2` Add `desktop-dev-mock` to preserve the old offline mock entry point for UI-only work.

## 8. Validation

- `V-1` `just --list` shows `desktop-dev` as the real runner mode and `desktop-dev-mock` as the offline mock mode.
- `V-2` `node --check scripts/supervisor.js` passes after adding the runner-only desktop mode flag.
- `V-3` `npm run typecheck` in `apps/desktop-flowpilot` passed during the related auth/session verification work.

## 9. Regression Guard

- tests: no new runtime test harness exists for `Justfile`; command behavior is guarded by `just --list` and the supervisor syntax check
- alerts: none
- audit checks: `desktop-dev` must continue to represent real runner mode, and `desktop-dev-mock` must remain available for offline renderer work

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose:
  - the command split is intentional so developers can choose between real transport and offline mock mode explicitly
