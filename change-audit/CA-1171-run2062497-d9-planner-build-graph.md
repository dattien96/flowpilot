# CA-1171 — run-2062497 D9: planner must declare build-graph files for new test targets

## What changed

`apps/local-runner/internal/agentpack/flow-pack/agents/contract-planner.md`:

- The `declared_paths` instruction now states explicitly: when declaring a
  NEW test file, the planner MUST also declare the build-graph file that
  compiles/registers it (`CMakeLists.txt`, `build.gradle.kts`, …) — a test
  with no build wiring can never run, so the oracle stays blind and the
  contract forces an amend round.

Live: all four CP-03 sprint contracts declared new `*_test.cpp` files but
never `CMakeLists.txt`; `r-scaffold-red` could not see red, and every
sprint needed an operator wiring + amend round. (Task-031's planner did
declare it — regression since.)

Prompt-only change, per contract §1: role semantics live in pack data.

## Invariant

A contract that declares a test target also declares the file that wires it
into the suite the oracle runs.

## Tests

Pack-data change — verified by prompt text review; no engine behavior
changed.
