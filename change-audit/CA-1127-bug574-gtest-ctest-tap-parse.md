# CA-1127 — BUG-574: suite result parser blind to gtest/ctest/TAP

## Why

Live run-100368 (PrivateVault): the test suite ran via a bash/ctest harness
over a gtest binary. `parseSuiteTestNames` only had `go test` / `npm` /
`pytest` branches, so a red suite produced zero parsed names →
`oracle.Failed` empty. On a red-at-capture baseline
(`test_baseline.json` had `suite_passed: false`), `gate_hook`'s
`failedTests` fills only from `oracle.Failed` — so the still-red suite
produced no failure signal at all and read green at the rules level
(gate_blind only blocks when the turn has code changes; zero-delta turns
passed silently).

## What changed

`apps/local-runner/internal/flowgate/oracle.go`:

- New `parseGtestCtestTapLine` in a `default` branch of
  `parseSuiteTestNames`, covering three standard shapes:
  - gtest `[       OK ] Suite.Test` / `[  FAILED  ] Suite.Test` —
    suite-summary lines (`[  PASSED  ] 2 tests.`, `[  FAILED  ] 1 test`)
    are skipped because the token after the marker is a count.
  - ctest `N/M Test #K: name ... Passed/Failed`.
  - TAP `ok N name` / `not ok N name`.
- Anything unrecognized still returns ok=false — additive granularity only;
  the suite verdict remains the process exit code (unchanged).

## Invariant

A red suite must surface a failure signal on every supported output
format; unknown formats degrade to the exit-code verdict, never to a
fabricated green.

## Tests

`bug573_574_cpp_test.go` (red → green):

- `TestParseSuiteTestNamesGtest` — gtest output parsed under `ctest`,
  `./binary`, and `bash` commands; `VaultCoreTest.Zeroize` fails named.
- `TestParseSuiteTestNamesCtest` — ctest summary lines parse 2 passed +
  `audit_log_test` failed.
- `TestParseSuiteTestNamesTap` — TAP `ok`/`not ok` lines parse.
