# CA-1181 — oracle blind to Gradle/JUnit test shapes (BUG-1181)

## Defect

`parseSuiteTestNames` only knew go-test / npm / pytest / gtest / ctest / TAP
shapes. A JVM suite (Gradle, Kotlin Multiplatform, Android unit tests) prints
`ClassName > test name FAILED` console lines and writes JUnit XML under
`<module>/build/test-results/` — neither was parsed. `OracleResult.Failed`
stayed empty, so an expected-red scaffold suite surfaced as the unnamed
`suite_regressed` placeholder instead of named red tests, misclassifying
scaffold turns as regressions (live: PrivateVault `run-204891`, Task-039).

## Fix

- `parseGradleConsoleLine` — recognises `X > y FAILED|PASSED|SKIPPED` in the
  default (unknown-runner) parse branch; name keeps the `Class > test` pair.
- `junitXMLTestNames(dir, since)` — when console parsing yields zero names,
  `executeSuite` walks `cmd.Dir` for `test-results/**/*.xml` files modified at
  or after suite start (stale results excluded) and extracts
  `classname.testname` pass/fail. Heavyweight dirs (.git, node_modules,
  .gradle, DerivedData, .idea) are pruned; 512-file / 4MiB caps bound cost.

The fallback only runs when console parsing found nothing, so per-suite name
shape stays stable for baseline diffing.

## Regression tests

`bug1181_gradle_junit_parse_test.go` — console line parse, XML fallback
naming, stale-XML exclusion, end-to-end `executeSuite` with a silent
failing suite that only wrote XML.

## Note

The target project keeps its `native_host_tests.sh` TAP workaround — it is
now redundant but harmless (TAP lines still parse first).
