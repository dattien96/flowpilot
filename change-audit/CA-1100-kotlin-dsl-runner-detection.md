# CA-1100: test-runner detection blind to Kotlin DSL — Android repo pinned `go test` as its baseline suite

Date: 2026-10-02
Refs: PrivateVault run-3362. The repo is a Kotlin-DSL Android project
(`settings.gradle.kts` + `build.gradle.kts` + `gradlew` at root, no
`build.gradle`). During the vibe-sprint TDD leg the baseline captured
`test_command: go test -v .` against a scratch Go harness
(`requirements/.flowpilot/vibe/appbootstrap/go.mod`) that the scaffold
agent had generated, so the contract froze `test_file: *_test.go` +
`verify: go test` for a Kotlin codebase.

## Root cause

`flowgate.detectRunnerInDir` recognised Gradle only via the Groovy-DSL
marker `build.gradle`. Kotlin DSL (`build.gradle.kts` / `settings.gradle.kts`)
— the default for modern Android — produced no root hit, so
`DetectTestRunner` dropped into `detectNestedRunner`, where Go outranks
Gradle (`runnerRank` 0 vs 3) and any nested `go.mod` wins.

A missing native toolchain amplified it on the live run (no usable JDK on
PATH that day → `./gradlew test` could not satisfy the TDD gate → the
agent reached for `go`), but the detection gap is a real engine bug on its
own: even with a working JDK the root marker would still have been missed.

## Fix

`internal/flowgate/baseline.go` — the Gradle branch now probes all four
markers (`build.gradle`, `build.gradle.kts`, `settings.gradle`,
`settings.gradle.kts`) before requiring `gradlew`/`gradlew.bat`. A
Kotlin-DSL root therefore hits the fast path and returns
`./gradlew test` with `Dir=""`, so the nested walk never runs and foreign
nested modules cannot outrank the repo's real suite.

`settings.gradle(.kts)` counts as a marker on purpose: multi-module roots
exist whose only root-level Gradle file is the settings file (module build
files live in subdirectories). The wrapper requirement is unchanged —
marker without `gradlew`/`gradlew.bat` still yields no detection.

Additive: Groovy-DSL behaviour is byte-identical; no rank order changes.

## Tests

`ca1100_kotlin_dsl_runner_test.go` (new, additive) —
`TestCA1100DetectKotlinDslGradleRunner`:

- `build.gradle.kts at root detects gradlew` — RED pre-fix ("" instead of
  `./gradlew test`).
- `settings.gradle.kts alone still detects gradlew` — RED pre-fix.
- `nested go.mod does not outrank kotlin-dsl root` — RED pre-fix (returned
  `go test -v ./...` at `tools/harness`); asserts the root fast path wins
  over a nested foreign module.

## Verify

- Red: all three subtests fail pre-fix.
- Green: `go test -count=1 ./internal/flowgate/` — whole package pass
  (includes `TestDetectGradleRunner`, `TestDetectGradleRunnerBatFallback`,
  `TestDetectTestRunnerNestedGoMod` unchanged).
- `go vet ./internal/flowgate/` clean.

## Not covered here (follow-ups)

- `prompts/scaffold-contract-tdd.md` still does not constrain the test
  suite to the production language/source set, and the frozen contract's
  `declared_paths` does not include test files — a separate prompt/contract
  fix.
- Baselines already pinned to a foreign suite (e.g. PrivateVault's
  `test_baseline.json`) are refreshed on the next trusted recapture, which
  now detects `./gradlew test`; deleting the stale pin + scratch harness
  is a per-repo cleanup decision.
- No CMake/CTest runner exists yet for NDK modules (`core:vault-core` etc.
  have no C++ sources today, so nothing to detect yet).
