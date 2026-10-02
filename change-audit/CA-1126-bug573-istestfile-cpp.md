# CA-1126 — BUG-573: IsTestFile misses C/C++ test shapes

## Why

Live run-100368 (PrivateVault, NDK project), owner_2 finding: `IsTestFile`
recognized go/js/py/dart/kotlin/java/swift test files but no C/C++ shapes.
`vault_core_test.cpp` was classified as a production file, so the scaffold
bounded-stub gate froze it read-only and the reproduce/test-tamper paths
never saw it as a test — a stub-whitelist false positive.

## What changed

`apps/local-runner/internal/flowgate/baseline.go`:

- New `reTestCpp` covers `_test.cpp|cc|cxx`, `_tests.*`, gtest
  `FooTest.cpp|cc|cxx` / `FooTests.*` (capital-T required so `latest.cpp` /
  `contest.cpp` do not false-match), and `test_foo.*` prefixes.
- `IsTestFile` includes it — one shared classifier, so every downstream
  consumer (oracle tamper detection, reproduce lock, scaffold read-only
  exclusion) sees the same answer.

## Invariant

Test-file classification is ecosystem-complete for the languages the
oracle can host; a test file must never masquerade as a production file on
any lock/tamper path.

## Tests

`bug573_574_cpp_test.go` (red → green):

- `TestIsTestFileCpp` — `_test`/`*Test`/`*Tests`/`test_*` cpp shapes true;
  `VaultCore.cpp`, `TestHelper.cpp`, `contest.cpp`, `latest.cpp` false.
