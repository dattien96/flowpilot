# Task-172: Scope Test Execution â€” Phase 2: Cross-Dir Resolution and Transitive Module Coverage

## Metadata

- Document ID: `Task-172`
- Title: `Scope Test Execution â€” Phase 2: Cross-Dir Resolution and Transitive Module Coverage`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-29`
- Last Updated: `2026-06-29`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: `None`
- Related Documents: [Task-158: Scope Test Execution To Changed Packages](../inprogress/Task-158-Scope-Test-Execution-To-Changed-Packages.md), [Task-156: Regression Oracle â€” Polyglot Signal And Baseline Cost](../done/Task-156-R-Test-Performance.md)
- Replaces: `None`
- Tags: `flowgate, oracle, regression, scope, pytest, jest, gradle, transitive, cross-dir`

## AI Quick View

### Summary

- Task-158 (Phase 1) scopes the oracle run to packages/modules touched by the diff. It works correctly for Go (`*_test.go` co-located with source) and Gradle (module task covers both `src/main` and `src/test`).
- **Gap 1 â€” pytest/Jest cross-dir layouts:** source lives in `src/auth/` but tests live in `tests/auth/` (pytest) or `__tests__/components/` (Jest). Phase 1 passes only the source dir to the test runner, which finds zero tests â€” a **silent false negative**.
- **Gap 2 â€” Gradle transitive breakage:** you change `:feature-auth`, Phase 1 runs only `:feature-auth:test`. But `:feature-core` imports `:feature-auth` and its tests now fail â€” never caught.
- This task closes both gaps: resolve mirrored test directories for pytest/Jest (T-1, T-2), and expand Gradle scoped runs to include direct reverse-dependents by parsing `build.gradle` dependency declarations (T-3).

### Current Ask

- Extend `scope.go` with mirrored test-dir resolution for pytest (`tests/X/` mirrors) and Jest (`__tests__/X/`).
- Extend Gradle scoping to expand changed modules by one level of reverse-dependents via lexical `build.gradle` parsing.
- Add a zero-test guard: if no test files are found in the scoped dirs, return `""` rather than a trivially passing empty run.
- All new DOD items covered by `scope_test.go`; `go test ./internal/flowgate/...` must stay green.

### Key Decisions

- `T-1` **pytest cross-dir:** for each source dir derived from a changed `.py` file, probe standard test mirrors (`tests/X/`, `test/X/`) in `repoDir`. Include mirrors that exist. If no test files are found in any of the scoped dirs after probing, fall back to full suite â€” a scoped run that finds zero tests is worse than no scoping.
- `T-2` **Jest cross-dir:** for each source dir, also probe `__tests__/X/` (sibling pattern) and `src/X/__tests__/` (co-located). Append found mirrors to `--testPathPattern`.
- `T-3` **Gradle transitive (direct reverse-deps only):** parse `build.gradle[.kts]` files across the repo for `implementation project(':X')` / `api project(':X')` / `runtimeOnly project(':X')` declarations. Build a reverse map at scope-time. When module A is in the scoped set, add all modules that directly depend on A. Apply the same max-5-module threshold to the **expanded** set â€” if expansion pushes total > 5 modules, fall back to full suite.
- `T-4` Transitive expansion is **one level deep only** (direct dependents). Grandparent chains are out of scope â€” they are rare, and the threshold catches the pathological case.
- `T-5` Go reverse-import graph is **not** in this task (the gap is less critical: Go test runs are seconds even for larger scopes, and the transitive impact is lower-stakes than a 20-min Gradle build). Deferred to a future task.
- `T-6` The existing `scopeTestCommand` signature and fallback contract (`""` = use full suite) is unchanged â€” this task only adds logic inside `scope.go`.

### Constraints

- Must not weaken oracle integrity (SS-14 AC-6): silent false negatives (scope finds zero tests) are treated as a fallback condition, not a pass.
- Must not change `TestConfig` schema, `oracle.go` call site, `baseline.go`, or any frontend file.
- `scope.go` is the only file touched in `flowgate`. New test file `scope_test.go` extended or `scope_crossdir_test.go` added.
- Gradle dependency graph parsing must be purely lexical (regex/string scan) â€” no Groovy/Kotlin AST parser. Fast enough to run synchronously before `executeSuite`.

### Open Questions

- `Q-1` For pytest: should `conftest.py` in a mirrored dir also trigger full-suite fallback, or only root-level `conftest.py`? Start conservative: any `conftest.py` change in any scoped dir â†’ fall back.
- `Q-2` For Gradle reverse-dep expansion: should `testImplementation project(':X')` also count? It means the module has test-only dependency on X â€” a change to X could break those tests too. Start with `implementation`, `api`, `runtimeOnly` only; `testImplementation` deferred.
- `Q-3` Threshold after expansion: is 5 Gradle modules still right when it now includes reverse-deps? A change to a shared `:core` module may expand to 10+ modules immediately. Consider lowering to 3 to keep oracle fast. Tune after seeing real data.

### Source Refs

- Spawned from Task-158 post-analysis. `SD-20 Â§5` (deferred cost); `CP-35 Â§4.5`. Code: `apps/local-runner/internal/flowgate/scope.go`.

---

## 1. Goal

Close the two correctness gaps identified after Task-158 was designed:

1. **Silent false negatives for pytest/Jest** when tests live in a separate directory from source â€” scoped command finds zero tests, always passes, regression undetected.
2. **Missed transitive breakage for Gradle** â€” changing module A breaks module B (which imports A), but only A's tests run.

The fix must not make the oracle slower than full-suite when expansion triggers fallback.

## 2. Parent Links

- coding plan: `CP-35 Â§4.5` (P-5 oracle/regression)
- tech design: `SD-20 Â§5` (deferred cost)
- system spec: `SS-14 AC-6` (oracle integrity â€” silent false negatives forbidden)
- upstream task: `Task-158` (Phase 1 scope-to-changed-packages)

## 3. Trigger

During design review of Task-158 two gaps were identified that Phase 1 does not close:

**Gap 1 â€” Cross-dir test layout (pytest/Jest):**
```
Project layout:
  src/auth/service.py          â† changed file
  tests/auth/test_service.py   â† test lives here

Phase 1 scoped command: pytest -v src/auth/
Result: 0 tests collected, exit 0   â† FALSE PASS
```

**Gap 2 â€” Gradle transitive dependency:**
```
:feature-auth  â† changed (AuthRepo.kt)
:feature-core  â† build.gradle has: implementation project(':feature-auth')
               â† AuthCoreService.kt calls AuthRepo.login()
               â† AuthCoreServiceTest now fails

Phase 1 scoped command: ./gradlew :feature-auth:test
Result: PASS (auth tests pass, core tests never run)  â† MISSED REGRESSION
```

## 4. Exact Change

### T-1 â€” pytest mirrored test directory resolution

In `scope.go`, after collecting unique source dirs from changed `.py` files:

```
For each source dir S derived from a changed .py file:
  candidates = [S]
  if S starts with "src/":
    rest = S after "src/"
    probe "tests/" + rest   â†’ if exists, add to candidates
    probe "test/" + rest    â†’ if exists, add to candidates
  else if S does NOT start with "test":
    probe "tests/" + S      â†’ if exists, add to candidates
    probe "test/" + S       â†’ if exists, add to candidates

if conftest.py changed in any candidate dir â†’ return "" (full suite)
if no .py test files exist in any candidate dir â†’ return "" (full suite)
scoped command: "pytest -v " + join(candidates, " ")
```

Probing is a simple `os.Stat(filepath.Join(repoDir, candidate))` â€” no recursive walk.

### T-2 â€” Jest mirrored `__tests__` directory resolution

In `scope.go`, after collecting unique source dirs from changed `.ts/.tsx/.js/.jsx` files:

```
For each source dir S:
  candidates = [S]
  probe S + "/__tests__"       â†’ if exists, add
  probe "__tests__/" + S       â†’ if exists (sibling __tests__ at same depth), add
  probe (replace "src/" with "__tests__/") in S â†’ if exists, add

scoped command: "jest --testPathPattern=" + join(candidates, "|")
fallback (>8 total dirs after expansion) â†’ return ""
```

### T-3 â€” Gradle direct reverse-dependency expansion

New helper inside `scope.go`:

```go
// buildGradleDepGraph scans all build.gradle[.kts] files under repoDir and
// returns a map: module â†’ set of modules that declare a dependency on it.
// Parsing is lexical only (regex). Returns empty map on any error.
func buildGradleDepGraph(repoDir string) map[string][]string
```

Regex pattern to match dependency declarations:
```
(implementation|api|runtimeOnly)\s+project\(['"](:[\w:/\-]+)['"]\)
```

Integration into Gradle scoping:
```
1. Collect directly changed modules (existing Phase 1 logic) â†’ changedModules
2. depGraph = buildGradleDepGraph(repoDir)
3. expandedModules = changedModules âˆª { m | depGraph[c] contains m, c in changedModules }
4. if len(expandedModules) > 5 â†’ return "" (threshold)
5. scoped command: "./gradlew " + join(expandedModules mapped to ":m:test", " ")
```

Log line when expansion fires:
```
[gate] gradle reverse-dep expansion: :feature-auth â†’ also testing :feature-core (1 dependent)
[gate] gradle expansion exceeded threshold (7 modules), falling back to full suite
```

### T-4 â€” Fallback when scoped dirs yield zero tests (pytest/Jest)

After resolving candidates but before returning the command, verify at least one test file exists:

```go
// hasTestFiles returns true if any file matching the pattern exists under dir.
func hasTestFiles(repoDir, dir, pattern string) bool
```

- pytest pattern: `test_*.py` or `*_test.py`
- Jest pattern: `*.test.{ts,tsx,js,jsx}` or `*.spec.{ts,tsx,js,jsx}` or files inside `__tests__/`

If zero test files found across all candidate dirs â†’ return `""`.

## 5. Touched Areas

- **modified**: `apps/local-runner/internal/flowgate/scope.go` â€” T-1 through T-4 added
- **modified/extended**: `apps/local-runner/internal/flowgate/scope_test.go` â€” new test cases
- **no change**: `oracle.go`, `baseline.go`, `gate_hook.go`, any frontend file

## 6. Acceptance Check

- pytest project with `src/auth/service.py` changed and tests in `tests/auth/test_service.py`: scoped command is `pytest -v src/auth/ tests/auth/`, test file found, suite runs.
- pytest project with `conftest.py` changed: falls back to full suite.
- pytest project where no test mirror exists for a changed source dir: falls back to full suite (not silent pass).
- Jest project with `src/components/Button.tsx` changed and tests in `__tests__/components/Button.test.tsx`: pattern includes `__tests__/components`.
- Gradle project with `:feature-auth` changed and `:feature-core` declaring `implementation project(':feature-auth')`: scoped command is `./gradlew :feature-auth:test :feature-core:test`.
- Gradle expansion produces >5 modules: falls back to full suite, log line confirms.
- `go test ./internal/flowgate/...` passes.

## 7. Out of Scope

- Go reverse-import graph (deferred â€” Go oracle runs are fast, lower urgency).
- `testImplementation project(':X')` Gradle dependency tracking (deferred â€” Q-2).
- Multi-level transitive chains (grandparent dependencies) â€” direct reverse-deps only.
- Actual Groovy/Kotlin AST parsing of `build.gradle` â€” lexical regex only.
- Changing `TestConfig` schema or any user-visible config.
- iOS/xcodebuild cross-dir or transitive improvements (heuristic-only per Task-158).

## 8. Completion Notes

- result: planned
- follow-ups: Go reverse-import graph (Phase 3); multi-level Gradle transitive chains; `testImplementation` tracking; threshold tuning after real project data.
- upstream docs updated: resolves residual open items from Task-158 design review.

## 9. Definition of Done Checklist

### pytest cross-dir (T-1)

- [ ] `DOD-01` Source dirs from changed `.py` files probe `tests/X/` and `test/X/` mirrors; found mirrors added to scoped dirs.
- [ ] `DOD-02` `conftest.py` change in any probed dir â†’ fallback to full suite.
- [ ] `DOD-03` No test files found in any candidate dir â†’ return `""` (not silent pass).
- [ ] `DOD-04` Test: `src/auth/service.py` changed, `tests/auth/` exists â†’ command includes both.
- [ ] `DOD-05` Test: `src/auth/service.py` changed, no `tests/auth/` â†’ falls back to full suite.

### Jest cross-dir (T-2)

- [ ] `DOD-06` Source dirs probe `__tests__/X/` (sibling) and `src/X/__tests__/` (co-located); found dirs added to pattern.
- [ ] `DOD-07` No test files found after probing â†’ return `""`.
- [ ] `DOD-08` Test: `src/components/Button.tsx` changed, `__tests__/components/` exists â†’ pattern includes it.

### Gradle transitive (T-3)

- [ ] `DOD-09` `buildGradleDepGraph` parses `implementation project(':X')`, `api project(':X')`, `runtimeOnly project(':X')` from all `build.gradle[.kts]` in repo.
- [ ] `DOD-10` Direct reverse-deps of changed modules added to scoped module set.
- [ ] `DOD-11` Expanded set > 5 modules â†’ fallback to full suite with log line.
- [ ] `DOD-12` Test: `:feature-auth` changed, `:feature-core` has `implementation project(':feature-auth')` â†’ command includes `:feature-core:test`.
- [ ] `DOD-13` Test: expansion to >5 modules â†’ `scopeTestCommand` returns `""`.
- [ ] `DOD-14` Log line emitted when reverse-dep expansion fires.

### Zero-test guard (T-4)

- [ ] `DOD-15` `hasTestFiles` check applied to pytest and Jest candidate dirs; returns `""` when empty.

### Regression

- [ ] `DOD-16` All Task-158 DOD items still pass (`go test ./internal/flowgate/...`).
- [ ] `DOD-17` Impact analysis run on `scopeTestCommand` before editing.

