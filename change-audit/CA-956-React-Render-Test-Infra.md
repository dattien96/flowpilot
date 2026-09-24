# CA-956 — KR-005 CP-82: React-render test infra (jsdom harness) + board/spectator coverage

## Summary

The CP-82 render gap is now closed. Root cause of "can't test components":
the phase1 suite runs compiled JS under `node --test` with no DOM and no
guarantee of a single React copy. Two additive infra pieces fix that:

- `src/testhelpers/domHarness.ts` — `setupDom()` installs jsdom globals
  (document, events, HTMLElement…) and returns a restore fn; sets
  `IS_REACT_ACT_ENVIRONMENT` so `act()` works outside a test runner.
- `scripts/phase1-runtime.js` — pins `react`/`react-dom`/`zustand`/`scheduler`
  to `apps/desktop-flowpilot/node_modules`. Compiled tests live under
  `.phase1-tests/`, where Node's walk-up could hit the ROOT node_modules and
  load a second React → "Invalid hook call / useCallback null". One instance
  now guaranteed.
- `tsconfig.phase1-tests.json` — include `*.test.tsx` + `testhelpers/`.
- Dev-deps: `jsdom@29.1.1` (published 2026-04), `@types/jsdom` — the only new
  packages; react/react-dom already shipped as app deps.

Coverage added (9 tests, all green):
- `SessionsBoard.render.test.tsx` — section grouping by project, waiting
  chip, row click → `openRunAtAttention` + close, Escape → close, empty
  state, focused-row class.
- `SpectatorPane.render.test.tsx` — null for focused run, status/title/
  last-line render, body click → `openRunAtAttention`, close →
  `closeSpectator`, missing-data placeholder.

## Files

- `apps/desktop-flowpilot/src/testhelpers/domHarness.ts` — NEW
- `apps/desktop-flowpilot/src/components/SessionsBoard.render.test.tsx` — NEW
- `apps/desktop-flowpilot/src/components/SpectatorPane.render.test.tsx` — NEW
- `scripts/phase1-runtime.js` — React-family resolution pin
- `tsconfig.phase1-tests.json` — include .test.tsx + testhelpers
- `apps/desktop-flowpilot/package.json` — +jsdom, +@types/jsdom

## Verified

- `node --test` on both new files: 9/9 pass.
- Full suite re-run for resolver-change regression.

## Provider parity

Provider-agnostic: test infrastructure only.
