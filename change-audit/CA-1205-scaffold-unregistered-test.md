# CA-1205 — r-scaffold-red flags C/C++ test files never wired into the build (live run-183756)

## Evidence
Ledger fingerprint `scaffold-gate-falsepos|run-183756|tdd` (medium): the
tdd scaffold turn wrote `vault_metadata_test.cpp` but never registered it in
CMakeLists.txt. The suite ran green on already-landed Task-032..037 tests,
and r-scaffold-red fired its all-green signal — "stubs contain real
implementation" — reprompting a revert of stubs that were correct. The gate
was right that something was off, wrong about what: an unregistered test
file is a build-wiring gap, not smuggled logic.

## Root cause
The evaluator had no registration signal: `Tests.Ran + Failed==[]` is
all-green whether or not the new tests were compiled. The runner knows the
turn's `WrittenPaths`; nothing checked those test files against the build
manifests that compile them.

## Fix
- `TurnResult.ScaffoldUnregisteredTests` — caller-computed like the other
  scaffold signals; flowgate stays I/O-free.
- `scaffoldUnregisteredTestFiles(cwd, written)` (runner, scaffold_gate.go):
  C/C++ test files (`_test`/`test_` `.c/.cc/.cpp/.cxx`) whose basename no
  CMakeLists.txt / *.cmake under the workspace references. Deliberately
  CMake-only — Gradle sourceSets, `go test`, and cargo auto-discover tests,
  and no manifest text means no signal (fail-open). Build output, VCS, and
  vendored dirs are skipped; oversized manifests ignored.
- `checkScaffoldRedRule`: the unregistered-test violation lands after the
  pre-existing-touched check and BEFORE both the waived path and the
  all-green signal — registration is a precondition for either outcome
  proving anything. Detail names the files and the wiring fix.
- `ScaffoldSatisfied` returns false for unregistered tests — a suite that
  never saw them cannot satisfy the contract.

## Tests
- `scaffold_red_rule_test.go`: unregistered → violation naming file + build
  manifest; takes precedence over the all-green detail; never satisfied.
- `ca1205_unregistered_scaffold_test_test.go`: unregistered flagged,
  registered clean, no-CMake fail-open, non-C/C++ test files skipped.
