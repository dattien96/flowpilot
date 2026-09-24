# CA-952 — KR-005 F-3: Test-Steps docs pointed at the wrong runner (vitest)

## Summary

CP-82/83/84 Test-Steps docs told operators to run `npx vitest`, but the repo
runs phase-1 tests through `tsc -p tsconfig.phase1-tests.json` +
`node --require scripts/phase1-runtime.js --test <compiled .js files>`.
`npx vitest` downloads an unrelated vitest build (vitest is not a repo dep)
and its directory-arg behaviour does not match `node --test` on this Node
version, so the documented commands either fail or test nothing.

Docs now give the real commands (`npm run test:phase1`, plus explicit
compiled-file invocation) and dropped references to component test files that
do not exist (`SessionsBoard.test.ts`, `SpectatorPane.test.ts`) — component
logic is covered by `boardModel`/`store.spectator` state tests; the repo has
no React-render test harness.

## Files

- `requirements/07-Coding-Plan/done/CP-82-Test-Steps.md`
- `requirements/07-Coding-Plan/done/CP-83-Test-Steps.md`
- `requirements/07-Coding-Plan/done/CP-84-Test-Steps.md`

## Verified

- `npm run test:phase1` compiles clean; explicit-file `node --test` runs green.

## Provider parity

Provider-agnostic: documentation only.
