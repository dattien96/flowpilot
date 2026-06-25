# Task-159: Polyglot Ecosystem Coverage — Auto-Detect All Target Platforms

## Metadata

- Document ID: `Task-159`
- Title: `Polyglot Ecosystem Coverage — Auto-Detect All Target Platforms`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: none
- Related Documents: [Task-156: Regression Oracle — Polyglot Signal And Baseline Cost](./Task-156-R-Test-Performance.md), [Task-155: Regression Block Decision Card (r-reg)](./Task-155-update-r-reg.md)
- Replaces: `None`
- Tags: `flowgate, oracle, regression, baseline, polyglot, ecosystem, local-runner`

## AI Quick View

### Summary

- Task-156 delivered the exit-code primary signal (any language with a runnable test command can trigger `r-reg`), but `DetectTestRunner` still only auto-detects Go / Python / Node. Flutter, Android Kotlin, iOS Xcode, Java, Angular, React Native, Vue, and ReactJS all require manual `test-config.json` or get **no regression safety at all**.
- All three gaps are **pure FlowPilot code changes**. The target project needs zero additional configuration — FlowPilot should just work on all these platforms the same way it already works on Go/Node/Python.
- Three concrete fixes: expand `DetectTestRunner` with new markers (`pubspec.yaml`, `build.gradle`, `pom.xml`), expand `IsTestFile` with new patterns (`_test.dart`, `*Test.java`), and guard Angular's watch-mode hang by appending `--watch=false --no-progress` automatically.
- Named-test granularity (which test failed) remains Go/Node/Python-only. All new ecosystems use the coarse exit-code signal (`"suite_regressed"` sentinel), which is correct and sufficient per `T-1` of Task-156.

### Current Ask

- Extend `DetectTestRunner`, `IsTestFile`, and the Angular command rewrite in `baseline.go` so that Flutter, Android Kotlin, iOS Xcode, Java, React Native, Vue.js, ReactJS, Angular, and Node.js all get automatic regression detection with no target-project configuration required.

### Key Decisions

- `T-1` **Zero target-project config is the contract.** FlowPilot reads the project's own marker files (`pubspec.yaml`, `build.gradle`, `pom.xml`, `angular.json`, `package.json`) to infer the runner. The target project must not add `.flowpilot/` files for this to work. `test-config.json` remains the escape hatch for non-standard layouts only.
- `T-2` **Coarse mode is acceptable for all new ecosystems.** Exit-code signal fires `r-reg` with `"suite_regressed"` when no structured output is parseable. This is correct per `Task-156 T-1` — blocking "the suite broke" in every language beats naming tests in only three.
- `T-3` **Angular watch-mode hang is fixed in `DetectTestRunner`, not in `executeSuite`.** When `angular.json` is detected, the resolved command includes `--watch=false --no-progress` so `executeSuite` always gets a process that exits.
- `T-4` **`IsTestFile` expansion is required for correctness, not just coverage.** Without recognising `_test.dart` and `*Test.java`, FlowPilot cannot suppress false-positive regressions when the AI intentionally rewrites a test file, and cannot detect tamper (AI weakening a test to pass). Both break oracle integrity.

### Constraints

- Must not regress existing Go / Node / Python behaviour.
- `executeSuite` timeout stays at 5 minutes — it already covers slow Gradle/xcodebuild runs.
- Do not add JUnit XML / `.xcresult` / Dart JSON parsing in this task (named granularity for new ecosystems is Task-160+ if needed).
- Provider-agnostic; non-fatal (`SS-14 AC-9`).

### Open Questions

- `Q-1` Android monorepos often have `build.gradle` at root and per-module `build.gradle` files. Should `DetectTestRunner` prefer `./gradlew test` (full suite at root) or walk for the first module-level `build.gradle`? Default: root `./gradlew test`, walk only if no root-level Gradle wrapper exists.
- `Q-2` `xcodebuild test` requires `-scheme` and `-destination` which are project-specific. Can we infer the scheme from `*.xcworkspace`/`*.xcodeproj`? Default: emit a degraded "test command requires configuration" log and fall through to `test-config.json` if no scheme can be inferred.
- `Q-3` Flutter's `flutter test` requires the Flutter SDK on PATH. Should `DetectTestRunner` check `which flutter` before returning the command, to avoid a confusing error in `executeSuite`? Default: yes — skip if `flutter` is not on PATH.

### Source Refs

- Task-156 `T-1` (universal exit-code signal), `T-2` (explicit config precedence), `T-5` (degraded state).
- `SS-14 AC-6` (oracle integrity), `AC-9` (non-fatal), `R-5` (explicit test_command).
- Code: `apps/local-runner/internal/flowgate/{baseline.go, oracle.go}`.

---

## 1. Goal

Extend FlowPilot's regression oracle so that **every platform FlowPilot targets** — Flutter, Android Kotlin, iOS Xcode, Java (Maven/Gradle), React Native, Vue.js, ReactJS, Angular, and Node.js — gets automatic `r-reg` detection without requiring the target project to add any configuration file. The fix is entirely in FlowPilot's own code.

## 2. Parent Links

- coding plan: `CP-35` §4.5 (P-5) — same oracle work stream as Task-156
- tech design: `SD-20` §2.3/§2.4/§4; `SD-17` §7.2
- system spec: `SS-14` `AC-6`, `AC-9`, `R-5`
- specific upstream ids: extends Task-156 `T-1`/`T-2`/`T-5`; does not amend SD-20 decisions

## 3. Trigger

Task-156 delivered the exit-code primary signal and the `test-config.json` escape hatch, but the **auto-detection layer** (`DetectTestRunner`) still only recognises three ecosystems. Every other platform FlowPilot targets silently falls through to `oracle.Disabled = true` and gives no regression safety. Field reality: the projects FlowPilot runs on daily include React Native, Android Kotlin, and Flutter — none of which trigger `r-reg` today without manual setup.

### Why FlowPilot needs to know the test runner and test file patterns

The regression oracle does two independent jobs: **detect regressions** and **prevent tampering**. Both jobs require FlowPilot to know two things about the target project: *how* to run its tests, and *which files are test files*.

```
AI finishes turn
    └── runFlowGate (gate_hook.go)
            │
            ├─ 1. ObserveGitDiffSince      what files changed this turn?
            ├─ 2. LoadBaseline             what did "green" look like at HEAD?
            ├─ 3. RunOracle                did something that was green now fail?
            │       │
            │       ├─ executeSuite        run test_command → exit code (primary signal)
            │       │                                       → parse stdout (named tests, best-effort)
            │       │
            │       ├─ IsTestFile ×diff    for each failing test:
            │       │    YES (AI changed    → skip — intentional rewrite, not a regression
            │       │         that file)
            │       │    NO               → count as regressed → r-reg fires
            │       │
            │       └─ IsTestFile + M      tamper check: AI modified a pre-existing test file
            │                              → r-tamper violation (AI weakened the test to pass)
            │
            └─ Evaluate violations → block / reprompt / warn
```

**`DetectTestRunner`** — required to run the suite. Without it `baseline.TestCmd == ""` → `RunOracle` returns `Disabled: true` → `r-reg` never fires → false safety.

**`IsTestFile`** — required for oracle correctness, not just coverage:
1. **False-positive suppression** *(named-test mode only)*: when the AI intentionally rewrites a test file and the suite fails, the failing test names are checked against `changedTestFiles`. If the test came from a file the AI edited, it is skipped — not a regression. This guard only fires when `executeSuite` returns named test IDs (`nowFailed` non-empty: Go, pytest, npm). For coarse-mode ecosystems (Flutter, Java, Android, Swift), `nowFailed` is empty and the `"suite_regressed"` sentinel is emitted directly, bypassing this check — named-test parsing for those ecosystems would be required to fully close this gap.
2. **Tamper detection** *(all ecosystems)*: if the AI silently modifies a pre-existing test file (`Status == "M"`), `r-tamper` fires regardless of whether the suite passes or fails. This is the immediate correctness benefit of the `IsTestFile` expansion — it applies to every ecosystem, named-test mode or not.

#### Example — Case 1: False-positive suppression (Go, named-test mode)

**Setup:** Go project, baseline green. User asks AI to update `TestAdd` because `Add` now always adds 1 (new requirement).

The AI edits `calc_test.go`:
```go
// BEFORE (was green at baseline)
func TestAdd(t *testing.T) {
    if Add(2, 3) != 5 { t.Fatal("wrong") }
}

// AFTER (AI rewrites per new requirement)
func TestAdd(t *testing.T) {
    if Add(2, 3) != 6 { t.Fatal("wrong") }  // new: always +1
}
```

`go test -v` exits non-zero, parses `--- FAIL: TestAdd`. Now `runOracle`:

```
changedTestFiles = []                            ← WITHOUT fix: IsTestFile("calc_test.go")
                                                   already true — Go was covered from day 1

// For a NEW ecosystem (e.g. Kotlin), same logic applies once IsTestFile recognises *Test.kt:
changedTestFiles = ["com/example/AddTest.kt"]    ← WITH Task-159 fix

isTestFromChangedFile("TestAdd", changedTestFiles)
  → stem of "AddTest.kt" = "addtest" / "add"
  → "TestAdd" contains "add" → true → SKIP

regressed = []                                   ← nothing counted as regressed
r-reg does NOT fire → AI's intentional rewrite is allowed ✓
```

Without `IsTestFile` recognising the file, `isTestFromChangedFile` returns false → `TestAdd` is counted as a regression → `r-reg` fires and blocks the AI even though the user asked for the change.

> **Scope note:** this path only fires when `nowFailed` is non-empty. For coarse-mode ecosystems (Flutter, Android, Java, Swift), `"suite_regressed"` is emitted unconditionally when the suite fails, regardless of which files changed. Named-test parsing for those ecosystems is needed to make this guard effective there too.

---

#### Example — Case 2: Tamper detection (Java, all modes)

**Setup:** Java Maven project. `TestAdd` regressed — `Add` returns the wrong value. Rather than fixing the code, the AI silently guts the test.

The AI edits `AddTest.java`:
```java
// BEFORE (was green at baseline, now failing because Add is broken)
@Test public void testAdd() {
    assertEquals(5, calculator.add(2, 3));
}

// AFTER (AI weakens the test so it always passes)
@Test public void testAdd() {
    assertTrue(true);   // ← gutted
}
```

`mvn test -q` exits 0 — suite is "green" again. The regression check never fires. The only guard is the tamper check inside `RunOracle`:

```go
for _, f := range diff {
    if IsTestFile(f.Path) && f.Status == "M" {
        tampered = append(tampered, f.Path)
    }
}
```

**Without `*Test.java` in `IsTestFile`:**
```
f.Path = "src/test/java/com/example/AddTest.java"
IsTestFile(...)  →  false
tampered = []
r-tamper does NOT fire → AI has silently gutted the test, undetected ✗
```

**With `*Test.java` in `IsTestFile` (Task-159):**
```
IsTestFile("src/test/java/com/example/AddTest.java")  →  true  (reTestJava matches)
tampered = ["src/test/java/com/example/AddTest.java"]
HasTampering = true
```

`runFlowGate` appends an `r-tamper` violation and the gate blocks the step:
> `"pre-existing test file modified: src/test/java/com/example/AddTest.java"`

The AI cannot gut the test undetected — works in coarse mode and named-test mode alike. ✓

---

## 4. Exact Change

### Current ecosystem coverage

| Ecosystem | Auto-detect today | r-reg fires | Named tests | IsTestFile |
|---|---|---|---|---|
| Go | ✓ `go.mod` | ✓ | ✓ | ✓ `_test.go` |
| Python (pytest) | ✓ `pytest.ini` / `pyproject.toml` | ✓ | ✓ | ✓ `test_*.py` |
| Node.js / React / Vue / React Native | ✓ `package.json` + test script | ✓ | best-effort | ✓ `.test.js/ts/tsx` `.spec.js/ts` |
| Angular | ⚠️ detected but hangs (watch mode) | ✗ hangs | — | ✓ `.spec.ts` |
| Flutter | ✗ no `pubspec.yaml` check | ✗ | ✗ | ✗ `_test.dart` missing |
| Android Kotlin | ✗ no `build.gradle` check | ✗ | ✗ | ✗ `*Test.kt` missing |
| iOS Xcode | ✗ no `*.xcworkspace` check | ✗ | ✗ | ✗ `*Tests.swift` missing |
| Java (Maven) | ✗ no `pom.xml` check | ✗ | ✗ | ✗ `*Test.java` missing |
| Java (Gradle) | ✗ no `build.gradle` check | ✗ | ✗ | ✗ `*Test.java` missing |

### T-1 — Expand `detectRunnerInDir` in `baseline.go`

Add detection for:

```go
// Flutter
if _, err := os.Stat(filepath.Join(dir, "pubspec.yaml")); err == nil {
    if flutterOnPath() {
        return "flutter test"
    }
}
// Android / Java Gradle
if _, err := os.Stat(filepath.Join(dir, "build.gradle")); err == nil {
    if _, err2 := os.Stat(filepath.Join(dir, "gradlew")); err2 == nil {
        return "./gradlew test"
    }
}
// Java Maven
if _, err := os.Stat(filepath.Join(dir, "pom.xml")); err == nil {
    return "mvn test -q"
}
```

Add `flutterOnPath() bool` helper (runs `flutter --version` with a 5s timeout; returns false on error).

Angular already auto-detects via `package.json` but the command hangs — fix in T-2.

### T-2 — Fix Angular watch-mode hang in `detectRunnerInDir`

After resolving `npm test` via `package.json`, check for `angular.json` in the same directory. If present, override the command:

```go
if hasNpmTestScript(pkgPath) {
    if _, err := os.Stat(filepath.Join(dir, "angular.json")); err == nil {
        return "npx ng test --watch=false --no-progress"
    }
    return "npm test"
}
```

### T-3 — Expand `IsTestFile` in `baseline.go`

Add patterns for the missing ecosystems:

```go
var reTestKt    = regexp.MustCompile(`Test\.kt$`)
var reTestJava  = regexp.MustCompile(`Tests?\.java$`)
var reTestSwift = regexp.MustCompile(`Tests?\.swift$`)
var reTestDart  = regexp.MustCompile(`_test\.dart$`)

func IsTestFile(path string) bool {
    return reTestGo.MatchString(path) ||
        reTestJS.MatchString(path) ||
        reTestPy.MatchString(path) ||
        reTestKt.MatchString(path) ||
        reTestJava.MatchString(path) ||
        reTestSwift.MatchString(path) ||
        reTestDart.MatchString(path)
}
```

### T-4 — Update `runnerRank` priority

Add new ecosystems to the priority ordering so monorepo walk picks the best runner:

```go
func runnerRank(cmd string) int {
    switch {
    case strings.HasPrefix(cmd, "go test"):   return 0
    case strings.HasPrefix(cmd, "pytest"):    return 1
    case strings.HasPrefix(cmd, "flutter"):   return 2
    case strings.HasPrefix(cmd, "./gradlew"): return 3
    case strings.HasPrefix(cmd, "mvn"):       return 4
    default:                                  return 5  // npm / npx
    }
}
```

### T-5 — Tests (`flowgate_test.go`)

- `TestDetectFlutterRunner` — project with `pubspec.yaml` returns `"flutter test"` (mock `flutterOnPath`).
- `TestDetectGradleRunner` — project with `build.gradle` + `gradlew` returns `"./gradlew test"`.
- `TestDetectMavenRunner` — project with `pom.xml` returns `"mvn test -q"`.
- `TestAngularNoWatchMode` — project with `package.json` (test script) + `angular.json` returns command containing `--watch=false`.
- `TestIsTestFileDart`, `TestIsTestFileKotlin`, `TestIsTestFileJava`, `TestIsTestFileSwift` — assert new patterns match; assert non-test files do not.
- `TestTamperDetectionKotlinTestFile` — `IsTestFile("com/example/AddTest.kt")` → true → `r-tamper` fires when status=="M".

## 5. Touched Areas

- files: `apps/local-runner/internal/flowgate/baseline.go`, `apps/local-runner/internal/flowgate/flowgate_test.go`
- modules: `flowgate`
- routes: none
- tables: none

No changes to `oracle.go`, `gate_hook.go`, or any UI component.

## 6. Acceptance Check

- A Flutter project with `pubspec.yaml` and `flutter` on PATH: baseline is captured with `test_command: "flutter test"`; breaking a test fires `r-reg` with `"suite_regressed"`. No `test-config.json` required.
- An Android project with `build.gradle` + `gradlew`: baseline captured with `"./gradlew test"`; breaking a test fires `r-reg`. No config required.
- A Java Maven project with `pom.xml`: baseline captured with `"mvn test -q"`; `r-reg` fires on suite failure. No config required.
- An Angular project: `detectRunnerInDir` returns a command with `--watch=false`; `executeSuite` exits within 5 minutes (no hang).
- Modifying `AddTest.kt`, `AddTests.java`, `AddTests.swift`, or `add_test.dart` in a diff triggers `r-tamper` detection (was not the case before).
- Existing Go / Python / Node tests in `flowgate_test.go` still pass.
- `go test ./internal/flowgate/...` passes.

## 7. Out of Scope

- Named-test granularity for Flutter, Android, iOS, Java (parsing structured output formats such as JUnit XML or Dart JSON) — future task.
- iOS Xcode auto-detection via `*.xcworkspace` / `*.xcodeproj` — deferred to Q-2 resolution; iOS remains `test-config.json` only until scheme inference is solved.
- Package-scoped test execution (Task-158).
