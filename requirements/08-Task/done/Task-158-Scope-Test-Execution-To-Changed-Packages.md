# Task-158: Scope Test Execution to Changed Packages

## Metadata

- Document ID: `Task-158`
- Title: `Scope Test Execution to Changed Packages`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: [Task-172: Scope Test Execution Phase 2 — Cross-Dir Resolution and Transitive Module Coverage](../todo/Task-172-Scope-Test-Execution-Phase2-CrossDir-And-Transitive.md)
- Related Documents: [Task-156: Regression Oracle — Polyglot Signal And Baseline Cost](../done/Task-156-R-Test-Performance.md), [Task-100: Regression Suite And Oracle Rule](../done/Task-100-Regression-Suite-And-Oracle-Rule.md)
- Replaces: `None`
- Tags: `flowgate, oracle, regression, performance, scope, android, ios, gradle, xcode, go, pytest, jest`

## AI Quick View

### Summary

- Today `RunOracle` always runs the **full** test suite after every turn (`executeSuite` runs `go test -v ./...` / `./gradlew test` / `xcodebuild test` unconditionally). On Android/iOS this means waiting 5–30 minutes per turn — completely unusable.
- The git diff is already available in `runFlowGate` and `RunOracle`. We can use it to **scope the test command to only the packages/modules that were actually changed**, cutting runtime from minutes to seconds in most cases.
- This implements `Task-156 T-4` ("scope-to-changed-packages where the ecosystem allows; fall back to full suite when scoping is not safe/available").
- **Baseline capture always uses the full suite** (complete picture needed). Scoping only applies to the **oracle run** (turn end), where the question is "did _this_ change break anything?"

### Current Ask

- Implement `scope.go` with `scopeTestCommand` for Go, Gradle, xcodebuild, pytest, and Jest ecosystems.
- Wire it into `RunOracle` (one call before `executeSuite`). Leave `CaptureBaseline` untouched.
- Cover all DOD items with `scope_test.go` and verify `go test ./internal/flowgate/...` stays green.

### Key Decisions

- `T-1` Scoping is applied **only to the oracle run** (`RunOracle`), never to baseline capture. Baseline always needs the full picture.
- `T-2` **Fallback to full suite** when: (a) diff is empty (baseline capture context), (b) diff touches too many distinct packages (> threshold), (c) ecosystem-specific scope computation fails or returns nothing, (d) the `test_command` in `TestConfig` explicitly includes a package/path argument already.
- `T-3` **Known trade-off — false negatives:** if you change `pkg/util` and the regression is in `pkg/feature` which imports it, a scoped run of `./pkg/util/...` won't catch `pkg/feature`'s breakage. This is the same trade-off as partial CI runs. Acceptable because: (a) it's far better than timing out and generating false positives for every turn on mobile; (b) the next turn's oracle will catch the transitive regression if the change reaches that package.
- `T-4` Scoping is ecosystem-detected from `testCmd` prefix, matching the existing `executeSuite` / `DetectTestRunner` convention.
- `T-5` **Max-scope threshold:** if the scoped run would cover > 10 distinct Go packages or > 5 Gradle modules, fall back to the full suite. Prevents pathological cases (a change to a shared utility that appears in 50 packages) from running a command so long it may as well be the full suite.

### Constraints

- Must not weaken oracle integrity (`SS-14 AC-6`): if the scoped run says "suite passed", we treat that as the signal. The trade-off is documented. Do not hide the coarse-mode sentinel from the result.
- `CaptureBaseline` is out of scope — it always runs the full suite, unchanged.
- Do not change the `TestConfig` / `test_command` user-facing config — scoping is an internal optimization, invisible to the user unless they look at logs.

### Open Questions

- `Q-1` For Gradle: use `testDebugUnitTest` or `test`? (`test` covers all build variants; `testDebugUnitTest` is faster but variant-specific.) Start with `test` for safety, document override via `TestConfig.test_command`.
- `Q-2` For iOS/xcodebuild: scheme-to-target mapping is not reliably derivable from file paths alone. Start with a **heuristic** (`SourceDir/FeatureX` → target `FeatureXTests`) with a full-suite fallback when heuristic fails.
- `Q-3` For the threshold in `T-5`: is 10 packages / 5 modules right? Tune after seeing real project data.

### Source Refs

- `Task-156 T-4`; `SD-20 §5`/`Q-1`; `CP-35 §4.5`. Code: `apps/local-runner/internal/flowgate/{oracle.go,baseline.go}`, new `scope.go`.

---

## 1. Goal

Make the post-turn oracle run fast enough to be practical on Android/iOS/large-Go projects by scoping the test command to only the packages changed in the current turn's diff, while keeping oracle integrity and falling back to full-suite when scope is ambiguous.

## 2. Parent Links

- coding plan: `CP-35 §4.5` (P-5 oracle/regression)
- tech design: `SD-20 §5` (deferred cost), `Q-1` (scope strategy)
- system spec: `SS-14 AC-6` (oracle integrity), `AC-9` (non-fatal)
- implements: `Task-156 T-4`

## 3. Trigger

### Current cadence (after Task-156)

| Event | What runs | Blocks? |
|-------|-----------|---------|
| Chat open (HEAD changed) | `executeSuite` full suite — async goroutine | No |
| Turn end — every turn | `executeSuite` full suite — synchronous | **Yes — blocks finalization** |

For a Go project with 500 tests: ~5s. Fine.
For an Android project with Gradle (`./gradlew test`): 10–30 min. Kills the turn.
For iOS (`xcodebuild test -scheme App`): 5–20 min. Same.

### Desired cadence

| Event | What runs | Blocks? |
|-------|-----------|---------|
| Chat open (HEAD changed) | full suite — async | No |
| Turn end | scoped suite (changed pkgs only) — synchronous | **Yes, but <60s** |

## 4. Exact Change

### New file: `apps/local-runner/internal/flowgate/scope.go`

```go
// scopeTestCommand returns a test command scoped to the packages/modules covered
// by the diff. Returns "" when scoping is not possible or the fallback threshold
// is exceeded — callers must use the original testCmd in that case.
// testDir is the runner's working directory relative to repoDir (may be "").
func scopeTestCommand(repoDir, testDir, testCmd string, diff []ChangedFile) string
```

Ecosystem dispatch inside `scopeTestCommand`:

| Prefix of `testCmd` | Scoping strategy |
|---------------------|-----------------|
| `go test` | Map changed `.go` files → Go package dirs → `go test -v ./pkg1/... ./pkg2/...` |
| `./gradlew` or `gradlew` | Walk up each changed file → nearest `build.gradle[.kts]` → module name → `./gradlew :mod1:test :mod2:test` |
| `xcodebuild` | Heuristic: `Sources/FeatureX/` → `-only-testing:FeatureXTests`; fallback `""` |
| `pytest` | Collect unique dirs of changed `.py` files → `pytest -v dir1/ dir2/` |
| `jest` or `npx jest` | `--testPathPattern=dir1\|dir2` from changed `.ts/.tsx/.js/.jsx` files |
| anything else | `""` (fall back to full suite) |

#### Go scoping detail

```
Changed files:
  internal/flowgate/oracle.go   → package dir: internal/flowgate
  internal/runner/gate_hook.go  → package dir: internal/runner
  apps/web/README.md            → not .go, skip

Scoped packages: ["./internal/flowgate/...", "./internal/runner/..."]
Scoped command:  "go test -v ./internal/flowgate/... ./internal/runner/..."

If > 10 distinct packages → return "" (use full suite)
```

#### Gradle scoping detail

```
Changed file: feature-auth/src/main/java/.../AuthRepo.kt
Walk up:
  feature-auth/src/main/java/.../  → no build.gradle
  feature-auth/src/main/           → no build.gradle
  feature-auth/src/                → no build.gradle
  feature-auth/                    → build.gradle FOUND
  Module path: feature-auth → Gradle module: ":feature-auth"

Changed file: core/network/src/.../HttpClient.kt
  core/network/ → build.gradle FOUND → ":core:network"

Scoped command: "./gradlew :feature-auth:test :core:network:test"

If > 5 distinct modules → return "" (use full suite)
```

**Gradle separator:** Gradle uses `:` to separate nested modules. The path `core/network` becomes `:core:network`. Use `strings.ReplaceAll(relPath, "/", ":")` where `relPath` is the directory containing `build.gradle` relative to the repo root.

#### iOS/xcodebuild scoping detail

xcodebuild does not have a simple file→target mapping without parsing the `.xcodeproj`/`.xcworkspace`. Use a naming heuristic only:

```
Changed file: Sources/FeatureAuth/AuthViewModel.swift
Heuristic:   parent dir name = "FeatureAuth" → look for test target "FeatureAuthTests"
             Check if <repoDir>/<anything>FeatureAuthTests exists as a directory → if yes, use it
             Scoped: xcodebuild test -scheme <scheme> -only-testing:FeatureAuthTests

If heuristic fails or multiple unrelated targets → return "" (full suite)
```

For iOS the fallback rate will be high — that's fine. Even a partial win (auth-only change → auth tests only) saves 15+ minutes.

**Scheme name:** Read from `TestConfig.TestDir` (the xcodebuild invocation typically sets `-scheme`). Or require the user to set `test_command` explicitly per `TestConfig`; scoping only appends `-only-testing:Target`.

#### pytest scoping detail

```
Changed files: src/auth/service.py, src/auth/models.py, tests/conftest.py
Dirs (skip test infra like conftest): {"src/auth"}
Scoped command: "pytest -v src/auth/"

If conftest.py or shared fixture changed → return "" (full suite, scope unreliable)
```

#### Jest scoping detail

```
Changed files: src/components/Button.tsx, src/utils/format.ts
Dirs: {"src/components", "src/utils"}
Scoped command: "jest --testPathPattern=src/components|src/utils"

If > 8 dirs → return "" (full suite)
```

### Integration point: `oracle.go`

```go
func RunOracle(repoDir string, baseline *Baseline, diff []ChangedFile, overrides map[string]Override) OracleResult {
    // ...
    testCmd := baseline.TestCmd
    if scoped := scopeTestCommand(repoDir, baseline.TestDir, testCmd, diff); scoped != "" {
        testCmd = scoped
        // Log: "[gate] scoped test command: %s" 
    }
    suitePassed, nowPassed, nowFailed := executeSuite(repoDir, testCmd, baseline.TestDir)
    // ...
}
```

That's the entire integration — one new variable assignment before `executeSuite`. No change to `CaptureBaseline`.

### Logging

Add a log line when scoping fires so it's debuggable:

```
[gate] scoped oracle run: "go test -v ./internal/flowgate/... ./internal/runner/..." (2 packages, full would be ./...)
[gate] scope threshold exceeded (12 packages), falling back to full suite
[gate] scoping not applicable for testCmd="./gradlew lint", using full suite
```

## 5. Touched Areas

- **new**: `apps/local-runner/internal/flowgate/scope.go`
- **modified**: `apps/local-runner/internal/flowgate/oracle.go` (call `scopeTestCommand` before `executeSuite`)
- **no change**: `baseline.go`, `gate_hook.go`, `override.go`, any frontend file

## 6. Acceptance Check

- On a Go project: a turn touching `internal/flowgate/oracle.go` runs `go test -v ./internal/flowgate/...`, not `./...`. Suite finishes in < 5s.
- On a Gradle project: a turn touching `feature-auth/src/...` runs `./gradlew :feature-auth:test`, not `./gradlew test`. Module builds in < 90s.
- When diff touches > 10 Go packages or > 5 Gradle modules → falls back to full suite (log line confirms).
- When `testCmd` is not a recognized ecosystem → falls back to full suite.
- Baseline capture (`CaptureBaseline`) is unchanged — always runs full suite.
- `go test ./internal/flowgate/...` passes.

## 7. Out of Scope

- Dependency-graph-aware affected-test selection (GitNexus-level; future).
- iOS full `-only-testing:` accuracy without `.xcodeproj` parsing (heuristic-only for now; full support would require reading the project file).
- Changing the user-facing `TestConfig` schema.
- Modifying baseline capture cadence (owned by Task-156).

## 8. Completion Notes

- result: done — `scope.go` + `scope_test.go` (100 tests pass); `oracle.go` wired; all 17 DOD items checked
- follow-ups: Task-172 (cross-dir test resolution for pytest/Jest + Gradle transitive expansion); threshold tuning after seeing real project data; full iOS `.xcodeproj` parsing for accurate target detection; dependency-graph scope (GitNexus).
- upstream docs updated: resolves `SD-20 Q-1` (scope strategy = changed-packages with fallback).

## 9. Definition of Done Checklist

### Scope computation

- [x] `DOD-01` New `scope.go` implements `scopeTestCommand(repoDir, testDir, testCmd string, diff []ChangedFile) string`; `testDir` strips the module prefix from diff paths so all package/dir args are relative to the runner's working directory; returns `""` on no-scope or fallback conditions.
- [x] `DOD-02` **Go** — changed `.go` file paths map to `./pkg/...` args; command is `go test -v <pkg1>/... <pkg2>/...`; deduped and sorted.
- [x] `DOD-03` **Gradle** — walk up each changed file to find nearest `build.gradle[.kts]`; convert path to `:module:name` format; command is `./gradlew :m1:test :m2:test`.
- [x] `DOD-04` **iOS/xcodebuild** — heuristic: `Sources/FeatureX/` → `-only-testing:FeatureXTests`; fallback `""` when heuristic produces no match.
- [x] `DOD-05` **pytest** — unique dirs of changed `.py` files; fallback when `conftest.py` or shared fixture changed.
- [x] `DOD-06` **Jest** — `--testPathPattern=dir1|dir2` from changed `.ts/.tsx/.js/.jsx` files.
- [x] `DOD-07` Fallback to `""` (full suite) when: diff is empty, package/module count exceeds threshold (Go: 10, Gradle: 5, pytest/Jest: 8 dirs), or `testCmd` prefix is unrecognized.

### Integration

- [x] `DOD-08` `RunOracle` calls `scopeTestCommand` before `executeSuite`; uses scoped command when non-empty, original `testCmd` otherwise.
- [x] `DOD-09` `CaptureBaseline` is NOT changed — always runs full suite.
- [x] `DOD-10` A log line is emitted when scoping fires (`[gate] scoped oracle run: …`) and when it falls back (`[gate] scope fallback: …`).

### Tests

- [x] `DOD-11` `scope_test.go` — Go package extraction: `internal/foo/bar.go` → `./internal/foo/...`; deduplication; threshold cutoff.
- [x] `DOD-12` Gradle module extraction: `feature-auth/src/main/…` with `build.gradle` at `feature-auth/` → `:feature-auth`; nested module `core/network/` → `:core:network`.
- [x] `DOD-13` pytest dir extraction: `src/auth/service.py` → `src/auth/`; conftest fallback.
- [x] `DOD-14` Fallback when `testCmd` is unrecognized (e.g., `cargo test`).
- [x] `DOD-15` `go test ./internal/flowgate/...` passes (no regression in existing tests).

### Final review gate

- [x] `DOD-16` Impact analysis run on `RunOracle` / `executeSuite` before editing.
- [x] `DOD-17` Any unchecked item moved to a named follow-up with a reason.
