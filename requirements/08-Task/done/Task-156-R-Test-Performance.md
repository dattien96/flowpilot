# Task-156: Regression Oracle â€” Polyglot Signal And Baseline Cost

## Metadata

- Document ID: `Task-156`
- Title: `Regression Oracle â€” Polyglot Signal And Baseline Cost`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-158: Scope Test Execution to Changed Packages](../todo/Task-158-Scope-Test-Execution-To-Changed-Packages.md)
- Related Documents: [Task-100: Regression Suite And Oracle Rule](../done/Task-100-Regression-Suite-And-Oracle-Rule.md), [Task-155: Regression Block Decision Card (r-reg)](./Task-155-update-r-reg.md)
- Replaces: `None`
- Tags: `flowgate, oracle, regression, baseline, polyglot, performance, local-runner`

## AI Quick View

### Summary

- Two real holes in the regression oracle today: (a) it only works on Go/Node/Python â€” `DetectTestRunner` recognizes those three and `oracle.go` parses only `go test`/`npm`/`pytest` stdout, so on Android/iOS/Java/Rust the baseline is never captured and `r-tests`/`r-reg` **silently never fire**; (b) it runs the project's **full** suite on **every turn** off a baseline captured **once** and never refreshed (`SD-20 Â§5`, `D-4`, `Q-2`).
- Make the signal **universal**: drive regression detection from the suite's **exit code** against an explicit per-project `test_command`, so any language with a runnable test command is covered. Keep named-test granularity only where a structured output format exists.
- Make execution **affordable and fresh**: refresh the baseline keyed on HEAD SHA + working-tree state (skip re-runs when nothing changed), scope to changed packages where the ecosystem allows, run capture out-of-band, and key the cadence to **per chat**, not per turn.
- This forces amendments to `SD-20` (`D-4` "capture once" and `Â§5` deferred-cost) and likely a new `SS-14` AC for cross-language universality.

### Current Ask

- Replace name-parsing-only detection with an exit-code-based universal regression signal, and replace capture-once/run-every-turn with a HEAD-keyed, scoped, async baseline-refresh strategy â€” without weakening oracle integrity.

### Key Decisions

- `T-1` **Universal signal:** baseline records suite outcome (green/red) + optional named-failures; a regression = suite was green at baseline and is red now. Named-test diffing is a refinement layered on structured formats (`go test -json`, JUnit/XUnit XML, `.xcresult`), never a precondition.
- `T-2` **Explicit `test_command` is authoritative; auto-detect is only a default** (`SS-14 R-5`). Per-project config in `flow-rules.json` already exists â€” extend it with `test_dir` and an optional `result_format`.
- `T-3` **Baseline freshness:** the baseline carries the HEAD SHA (and a dirty-tree marker) it was captured at; it is re-captured when HEAD/working-tree differs, not on a fixed once-only basis â€” superseding `SD-20 D-4`.
- `T-4` **Cost control:** cadence is per chat (not per turn); scope to changed packages/modules where the ecosystem supports it; capture/refresh runs async/non-blocking and never blocks the raw artifact save (`SS-14 AC-9`).
- `T-5` When no `test_command` resolves and none is configured, the oracle degrades to "not run" and **says so** (surfaced as a low-confidence/disabled state) rather than implying safety it cannot provide.

### Constraints

- Must preserve oracle integrity (`SS-14 AC-6`): never edit/skip tests to go green.
- Must not regress the always-block behavior of `r-tests`/`r-reg` for the ecosystems that work today (Go/Node/Python).
- Provider-agnostic; non-fatal (`SS-14 AC-9`).
- Coordinate with Task-155: the decision card only appears when the oracle actually fires, so the polyglot fix is a prerequisite for 155 being meaningful on non-Go/Node/Python targets.

### Open Questions

- `Q-1` Regression scope (`SD-20 Q-1`): full suite vs changed-packages vs affected-tests-via-GitNexus â€” pick the v2 default here.
- `Q-2` Baseline refresh trigger granularity: HEAD-change only, or also after a clean commit mid-chat? (`SD-20 Q-2`.)
- `Q-3` Which structured formats to parse for named granularity first (Go JSON + JUnit XML cover most), and how to surface "coarse mode" (exit-code only) to the user.
- `Q-4` Per-turn vs per-chat: does dropping to per-chat miss intra-chat regressions that should block before the next turn? Trade-off to confirm.

### Source Refs

- `SD-20 Â§2.3`/`Â§2.4` (`r-tests`/`r-reg`), `Â§4` (baseline lifecycle), `Â§5` (deferred performance), `D-4`, `D-6`, `Q-1`, `Q-2`.
- `SS-14 AC-6`, `AC-9`, `AC-14`, `R-5`, `E-1`. `CP-35 Â§4.5`.
- Code: `apps/local-runner/internal/flowgate/{oracle.go,baseline.go}`, `apps/local-runner/internal/runner/gate_hook.go` (`ensureBaseline`, `runFlowGate`).

## 1. Goal

Make the regression oracle (a) actually work on the languages FlowPilot targets â€” Android, iOS, Go, Node, and others â€” by keying off the test command's exit code rather than parsing one of three stdout formats, and (b) affordable and current by refreshing the baseline only when the tree changed and running it out-of-band, without ever weakening a test to pass.

## 2. Parent Links

- coding plan: `CP-35` Â§4.5 (P-5)
- tech design: `SD-20` Â§2.3/Â§2.4/Â§4/Â§5; `SD-17` Â§7.2, `D-10`, `D-12`
- system spec: `SS-14` `AC-6`, `AC-9`, `AC-14`, `E-1`, `R-5`
- specific upstream ids: amends `SD-20 D-4` (capture-once) and resolves `SD-20 Â§5`/`Q-1`/`Q-2`

## 3. Trigger

Two field realities: target projects are polyglot (Android/iOS/Go/â€¦), so a Go/Node/Python-only oracle gives a **false sense of regression safety** on most of them; and running the full suite every turn off a stale, never-refreshed baseline is both slow on large suites and wrong after a legitimate committed behavior change. Both must be fixed for the regression promise (`SS-14`) to be real.

### Key concepts: "Oracle" and "Polyglot Signal"

- **Oracle** â€” the regression component (`flowgate/oracle.go`). The name comes from `SS-14`'s **test-as-oracle** principle: tests are the source of truth about correct behavior. A previously-green test that breaks is treated as *truth* ("you regressed something"), never edited away. `RunOracle` answers one question: *did this change break a test that used to pass?*
- **Signal** â€” how the oracle decides pass-vs-fail. Today the signal is **language-specific and brittle**: `oracle.go` scrapes stdout per ecosystem (`go test` â†’ `--- FAIL: TestName`, `npm` â†’ `âœ—`/`failing`, `pytest` â†’ `FAILED â€¦`). It effectively only "speaks" Go/Node/Python; on Android (Gradle), iOS (xcodebuild), Java, Rust, .NET, `DetectTestRunner` finds no runner â†’ no baseline â†’ `Tests.Ran=false` â†’ `r-reg`/`r-tests` **never fire**.
- **Polyglot signal** â€” a single, language-agnostic way to detect a regression: use the test command's **exit code** as the primary signal. The baseline stores `suite_passed`; after the change the configured `test_command` is re-run; **non-zero now + zero at baseline = regression**. This works for anything runnable (`./gradlew test`, `xcodebuild test`, `cargo test`, `dotnet test`, â€¦). Named-test granularity ("which test") becomes an **optional refinement** layered on structured output (`go test -json`, JUnit/XUnit XML, `.xcresult`), never a precondition.

| | Today (parse-based) | Polyglot signal (exit-code-based) |
|---|---|---|
| How | scrape stdout for failing test names | baseline `suite_passed`; re-run `test_command`; non-zero now + zero before = regression |
| Works on | go / npm / pytest only | any language with a runnable test command |
| Granularity | named tests (where parseable) | "the suite regressed"; named tests optional via structured output |

The deliberate trade-off: exit code tells you *something* broke, not always *which* test (without structured parsing). Correctly blocking "the suite broke" in **every** language beats precisely naming tests in only three.

## 4. Exact Change

- `T-1` `baseline.go` â€” extend `DetectTestRunner` to honor an explicit configured `test_command`/`test_dir` first; record `{committed_at, head_sha, dirty, test_command, test_dir, result_format, suite_passed, green_tests[]}` in `test_baseline.json`.
- `T-2` `oracle.go` â€” primary signal = exit code: `regressed` when `baseline.suite_passed && now suite fails && no test file in diff`. Named-test diffing kept as a refinement for `result_format âˆˆ {go-json, junit-xml, xcresult, pytest}`; remove reliance on brittle stdout substring matching as the only path.
- `T-3` `gate_hook.go`/`ensureBaseline` â€” refresh the baseline when `head_sha`/dirty differs from the stored one; otherwise reuse. Move cadence from per-turn to **per chat** (capture at chat start, refresh on HEAD change), and run capture async so it never blocks finalize.
- `T-4` scope-to-changed-packages where the ecosystem allows (e.g. `go test ./<changed-pkgs>`); fall back to full suite when scoping is not safe/available.
- `T-5` degraded state â€” when no command resolves, mark the oracle disabled for the project and emit a low-confidence/disabled indicator so the UI does not imply protection.
- `T-6` tests â€” exit-code regression formula; HEAD-keyed refresh skips re-run when unchanged; configured-command precedence; degraded "not run" path; one structured-format parser (Go JSON or JUnit XML) for named granularity.

## 5. Touched Areas

- files: `apps/local-runner/internal/flowgate/{oracle.go,baseline.go}`, `apps/local-runner/internal/runner/gate_hook.go`, `flow-rules.json` schema (add `test_dir`, `result_format`)
- modules: `flowgate`, `runner`
- routes: `flow_gate_violation` / a gate-status indicator carrying oracle-disabled/coarse-mode
- tables: none (baseline stays local under `.flowpilot/guard/`, never synced â€” `SD-17 Â§5.1`)

## 6. Acceptance Check

- On a non-Go/Node/Python project with a configured `test_command` (e.g. an Android Gradle or iOS xcodebuild command), breaking a previously-green suite blocks the step via `r-reg`; today it does not fire at all.
- The baseline is **not** re-run when HEAD and the working tree are unchanged between chats; it **is** re-captured after the tree changes.
- A project with no resolvable/configured test command shows an explicit "regression check unavailable" state rather than silently passing as safe.
- Go/Node/Python behavior is unchanged or better; named-test granularity still works where a structured format is parsed.
- Oracle integrity holds: editing a pre-existing test to pass is still flagged (`r-tamper`) and never satisfies the block.
- `go test ./internal/flowgate/...` passes.

## 7. Out of Scope

- The regression resolution UX (Task-155).
- Package-scoped test execution (scope to changed packages/modules per ecosystem) — split into **Task-158**, which implements T-4 of this task.
- Feature-key / history-context accuracy (Task-157).

## 8. Completion Notes

- result: completed
- follow-ups: `SD-20 D-4` amended conceptually; SS-14 AC for cross-language universality pending doc update. T-4 (package scoping) carried forward to **Task-158**.
- upstream docs updated: none yet â€” `SD-20` amendment required as part of this task.

## 9. E2E Manual Test Guide

> **Testbed:** the Go sandbox at `D:\working\gate-sandbox` (`calc.go` / `calc_test.go`, bound as a FlowPilot project, gate mode `enforce`).
> **Prerequisite:** `D:\working\gate-sandbox` is bound as an active FlowPilot project. All commands run from that directory unless noted.

### Prep â€” verify sandbox is clean

```powershell
cd D:\working\gate-sandbox
go test ./...   # must print: ok   gate-sandbox
git status      # should be clean
```

---

### E2E-1 â€” Stale baseline triggers refresh when HEAD changes

**Goal:** verify that `RefreshBaselineIfStale` re-captures when HEAD changes, and that the new baseline records the current HEAD SHA and `suite_passed=true`.

1. Delete any existing baseline:
   ```powershell
   Remove-Item .flowpilot\guard\test_baseline.json -ErrorAction SilentlyContinue
   ```
2. Open a FlowPilot chat on this project. The runner calls `ensureBaseline` asynchronously on chat open.
3. Wait ~10 seconds, then inspect:
   ```powershell
   Get-Content .flowpilot\guard\test_baseline.json
   ```
   **Expect:** `head_sha` matches `git rev-parse HEAD`, `suite_passed: true`, `green_tests` includes `TestAdd` and `TestSubtract`.
4. Make a harmless commit (e.g. add a blank line to `README.md`, then `git commit -am “blank”`).
5. Open a new chat turn (any prompt). After the turn completes, inspect the baseline again.
   **Expect:** `head_sha` updated to the new commit SHA â€” baseline was re-captured.

---

### E2E-2 â€” Unchanged HEAD reuses cached baseline (no re-run)

**Goal:** verify that `RefreshBaselineIfStale` returns the cached baseline without re-running tests when HEAD is unchanged.

1. Ensure a fresh baseline exists from E2E-1 (with `head_sha` set).
2. Run a turn: “Tell me what `Add` does. Do not edit files.”
3. In the runner logs, confirm there is no `[gate] ensureBaseline refresh` line showing `executeSuite` was re-run. The baseline `captured_at` timestamp is unchanged.
4. Confirm the turn completed quickly (< 5s gate check time).

---

### E2E-3 â€” Exit-code regression detection â€” coarse mode (sentinel)

**Goal:** verify that a panicking suite triggers `r-reg` with the `”suite_regressed”` sentinel when no named test can be parsed from stdout.

1. Establish a clean baseline (`suite_passed: true`).
2. Ask the AI in a new turn: “In `calc.go`, make `Add` call `panic(\”broken\”)` instead of returning the sum.”
3. After the turn:
   **Expect:** `go test ./...` exits non-zero. Oracle emits `gateRegressedTests: [“suite_regressed”]` (panic output has no `--- FAIL:` lines). Desktop shows the 3-option decision card: “The test suite regressed (exit-code signal; no structured output).”
4. Cleanup: `git checkout calc.go`

---

### E2E-4 â€” Named regression detection (structured output)

**Goal:** verify that a normal test failure (not a panic) produces named test IDs, not the coarse sentinel.

1. Clean baseline. Suite green.
2. Run a turn: “In `calc.go`, change `Add` to `return a - b`.”
3. After the turn:
   **Expect:** `go test -v ./...` exits non-zero. Oracle parses `--- FAIL: TestAdd` from stdout. Gate event carries `gateRegressedTests: [“TestAdd”]`, NOT `[“suite_regressed”]`. Desktop shows: “Previously-passing tests now fail: **TestAdd**.”

---

### E2E-5 â€” Explicit `test_command` in TestConfig takes precedence

**Goal:** verify that `.flowpilot/settings/test-config.json` overrides the auto-detected command.

1. Create the config:
   ```powershell
   New-Item -ItemType Directory -Force .flowpilot\settings | Out-Null
   '{“test_command”: “go test -v -run TestAdd ./...”}' | Set-Content .flowpilot\settings\test-config.json
   ```
2. Delete the baseline and open a new chat. After baseline capture:
   ```powershell
   Get-Content .flowpilot\guard\test_baseline.json
   ```
   **Expect:** `test_command: “go test -v -run TestAdd ./...”` (the configured value). `green_tests: [“TestAdd”]` â€” only the scoped test captured.
3. Run a turn that breaks only `TestSubtract` (change `Subtract` to return a wrong value).
   **Expect:** `r-reg` does **not** fire â€” `TestSubtract` was not in the baseline green list (scoped out by config). This is the correct behavior for the configured scope.
4. Cleanup:
   ```powershell
   Remove-Item .flowpilot\settings\test-config.json
   Remove-Item .flowpilot\guard\test_baseline.json -ErrorAction SilentlyContinue
   git checkout calc.go
   ```

---

### E2E-6 â€” No test command â†' oracle disabled state

**Goal:** verify that a project with no detectable test runner surfaces an explicit “regression check unavailable” state rather than silently passing.

1. Create a temp project dir with no `go.mod` / `package.json` / `pytest.ini` and configure it as the active cwd in FlowPilot runner.
2. Run a turn in that project.
3. In the runner logs:
   **Expect:** `[gate] oracle disabled â€” no test command` log line. `r-reg` and `r-tests` do not fire (`Tests.Ran = false`). Gate event, if emitted, carries `status: “warn”` indicating the oracle is disabled.

---

### Cleanup (after all E2E)

```powershell
cd D:\working\gate-sandbox
git checkout calc.go
Remove-Item .flowpilot\guard\test_baseline.json -ErrorAction SilentlyContinue
Remove-Item .flowpilot\settings\test-config.json -ErrorAction SilentlyContinue
```
