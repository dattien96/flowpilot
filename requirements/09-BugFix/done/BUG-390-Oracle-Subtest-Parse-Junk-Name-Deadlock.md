# BUG-390: oracle `--- FAIL:` subtest lines parse to junk name `"---"` → baseline pollution → reproduce deadlock

## Metadata

- Document ID: `BUG-390`
- Title: `Indented --- FAIL:/--- PASS: subtest lines parse to "---" → pollutes baseline green_tests → HasRegression suppresses Failed → "suite passed" violation on a RED suite`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-50-Context-Source-Completion](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md), [CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md)
- Feature Keys: `reproduce-first-gate`, `context-regression-engine`

## AI Quick View

### Summary

- Indented Go subtest lines (`    --- FAIL: TestX/sub`) fail `TrimPrefix("--- FAIL:")` → `Fields()[0]` yields the literal junk name `"---"`. The same parse on `--- PASS:` lines writes literal `"---"` entries into `test_baseline.json` `green_tests`.
- `isInBaseline("---", green)` then matches every subtest failure → `oracle.Regressed` → `HasRegression=true` → the `!oracle.SuitePassed && !oracle.HasRegression` guard suppresses `tr.Tests.Failed` to `nil` → `checkReproduceRule` reports "the suite passed, so the bug was not reproduced" while the runner's own `[gate] suite end` logged `err=exit status 1` with 9–22 `--- FAIL:` lines.
- Two independent live traces: CP-50 run-169 (6 gate suite runs all `exit status 1`, reprompted every time) and CP-42 run-442 (≥5 reprompts + 4 `ask_user` escalations on a provably-RED suite) — deterministic deadlock.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** `reproduce_test` can never reach DONE when the suite output contains subtests (or when the reproduced defect regresses baseline-green tests — the normal case for a real bug). Every evaluation emits the false "suite passed" reprompt despite observed `exit status 1` oracle output.
- **Expected:** Named subtest failures parse to their real names; real new-failure tests land in `tr.Tests.Failed`; the reproduce gate passes on a genuinely-RED reproduction.
- **Actual:** Junk `"---"` test names pollute the baseline and the oracle's regressed set; `HasRegression` suppresses the `Failed` list on both gate-hook paths; the rule sees `Failed=[]` and reprompts forever (cap never reached — BUG-391). No operator "skip node" surface exists (`applyFlowControl` accepts only `done|continue|escalate`, `interactive_service.go:1494-1501`); both runs ended only via manual interrupt.
- **Impact:** Every bug-harness run whose baseline captured `---` artifacts — or whose repro regresses baseline-green tests — deadlocks at `reproduce_test`. `ReproduceGateEnabled()` is hardwired `true` (`reproduce_gate.go:43-45`), so all bug-harness runs are exposed; each reprompt is a full provider turn (6+ observed).

## Reproduction

1. Baseline the suite while output contains indented `--- PASS:`/`--- FAIL:` subtest lines (or capture a red baseline) → `"---"` lands in `green_tests`.
2. Run `bug-harness`; the reproducer writes a real RED test that also regresses baseline-green tests.
3. Observe every gate pass reprompt "the suite passed…" while `[gate] suite end … err=exit status 1` logs real failures.
- runIds: `run-1`/`run-169` (CP-50; reprompt gens 1–6, `attempt=0` each, `gate_blind`/`red_at_capture` ×2, manual `POST …/interrupt`); `run-1`/`run-442` (CP-42; 22 `--- FAIL:` lines verified at bed commit `32a2998`, reprompts at runner.log:3855, :4400, :4931, :5578, :6855; escalations q-928/q-1707/q-2198/q-4147).

## Root cause

1. `apps/local-runner/internal/flowgate/oracle.go:289-293` — `strings.Contains(line, "--- FAIL:")` then `TrimPrefix(line, "--- FAIL:")` + `Fields()[0]`; indented subtest lines are not stripped → name `"---"`. Same defect on `--- PASS:` (`oracle.go:284`) at baseline capture → literal `"---"` `green_tests` entries.
2. `apps/local-runner/internal/flowgate/oracle.go:456-468` — `isInBaseline("---", green)` returns true via `b == testName` on stored `"---"` entries → subtest failures classify `Regressed` → `HasRegression=true` (`oracle.go:111-115, 141`); `isTestFromChangedFile`/`IsOverridden` don't filter `"---"`.
3. `apps/local-runner/internal/runner/gate_hook.go:262` and `:1220` — both gate-hook paths populate `failedTests` only under `!oracle.SuitePassed && !oracle.HasRegression` → `tr.Tests.Failed = nil` while `oracle.Failed` held the real failures.
4. `apps/local-runner/internal/flowgate/reproduce_rule.go:109-114` — `checkReproduceRule` reads only `tr.Tests.Failed` (never `Regressed`) → "the suite passed" violation → reprompt loop.
- Contributing (CP-50): baseline captured while tree already dirty lacks `suite_passed` (omitempty → false) → `ClassifyGateBlind` → `red_at_capture`, leaving the named-failure path (poisoned by `"---"`) as the only route out.

## Evidence

- `~/fp-beds/lt-evidence/cp50/RESULT.md` (BUG-LIVE-01 — full root-cause chain, 5-step trace)
- `~/fp-beds/lt-evidence/cp50/run1/test_baseline.json` — literal `"---"` entries in `green_tests`; `run1/gate-metrics.ndjson` — `gate_blind` ×2; `run1/bed-state/go-test-output.txt` — 9 `--- FAIL:` lines; `run1/prompts/reproducer-reprompt-835.txt` — "suite passed" reprompt text; `run1/dispatch.ndjson` — gens 1..6
- `~/fp-beds/lt-evidence/cp42/RESULT.md` (BUG-LIVE-1 — r-reproduce deadlock; mechanism quote `gate_hook.go:1215-1237`)
- `~/fp-beds/lt-evidence/cp42/runner.log` — suite `exit status 1` at 01:08:34/01:09:55/01:20:41/01:21:13 vs reprompts :3855/:4400/:4931/:5578/:6855

## Severity

- `high` — deterministic deadlock of the reproduce gate on the normal "real bug regresses existing tests" case; poisoned baselines make it trigger on subtests alone. No in-product escape.

## Completion Notes (implemented 2026-09-23, CA-919b)

- parseSuiteTestNames uses strings.Index for --- PASS:/--- FAIL: markers (indent-tolerant); isInBaseline rejects '---' junk entries. Test: bug390_subtest_parse_test.go.
