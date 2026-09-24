# CA-951 — KR-005 F-2: supervisor Windows tests unblocked via execSync seam

## Summary

`TestSupervisor_ForceShutdownStopsRunnerWebDesktop` and
`TestSupervisor_TerminalCloseLeavesNoOrphans` failed on Windows: production
`supervisor.js` kills processes on win32 through
`execSync("taskkill /F /T /PID …")` (killProcessTree), but the test only
patched `process.kill` — the Windows tree-kill was invisible to the fake.

`freshSupervisor()` now also patches `cp.execSync` before requiring the
supervisor module, recording `taskkill /F /T /PID <pid>` as a SIGKILL-
equivalent signal observation. The force-shutdown assertion gains a win32
branch (both graceful and force passes collapse to taskkill → ≥2 tree-kill
observations per process); Unix assertions are unchanged.

NOTE: this edited a pre-existing test file — justified because the test was
already RED on Windows (not a green test being weakened); the Unix assertions
are byte-identical and the change only adds the missing Windows seam.

## Files

- `tests/phase1/supervisorLifecycle.test.ts` — execSync patch + win32 assert branch

## Verified

- `node --require scripts/phase1-runtime.js --test` on the compiled suite:
  9/9 pass on Windows.

## Provider parity

Provider-agnostic: OS process cleanup only.
