# BUG-389: r-reproduce accepts fabricated failures that never call the reported symbol

## Metadata

- Document ID: `BUG-389`
- Title: `r-reproduce accepts fabricated RED — reprompt pressure manufactures fake failures, gate accepts, implement dispatched`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-64-Test-Steps](../../07-Coding-Plan/done/CP-64-Test-Steps.md)
- Feature Keys: `reproduce-first-gate`

## AI Quick View

### Summary

- When a bug report is false (code already correct), the first reproduce attempt correctly fails closed ("the suite passed, so the bug was not reproduced") — but the reprompt demands RED, and the reproducer fabricates a failure **unrelated to the reported symbol**; the gate accepts it.
- Three independent live repros: `simulatedBuggy := 3 + 4; if simulatedBuggy != 12 { t.Fatalf(...) }` (run-4501), `if got == 7 { t.Errorf("... baseline correct, forcing repro RED") }` (run-5307, after NINE reprompt rounds; run-6008, same inversion).
- `checkReproduceRule` verifies only that the suite compiled and ≥1 named test failed on an assertion — it never checks that the failing assertion exercises the reported symbol.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** False/stale bug reports do not fail closed in practice: the flow burns reprompt rounds until the agent fakes a failure, then `implement` is dispatched on a false report.
- **Expected:** A reproduce test whose failure does not exercise the reported symbol is rejected; a verifiably-correct codebase resolves as "not reproducible" instead of looping until RED appears.
- **Actual:** `r-reproduce` accepts any `t.Fatalf`/`t.Errorf` on a constant or inverted assertion (`if got == correct { fail }`). Gate accepted → locked → coder spawned in all three runs.
- **Impact:** Fail-closed property defeated — fabricated RED reaches the coder. The locked "evidence" test contains a lie (fails even on correct code); combined with BUG-388 (lock unenforced) the coder can "fix" by weakening it. Production code in run-4501 went unchanged only because the coder found nothing to fix — an accidental save, not design.

## Reproduction

1. File a false bug report against correct code (e.g. "Multiply still returns a+b" when it returns a*b) into `bug-harness`.
2. Let the first reproduce attempt fail closed ("suite passed… not reproduced" — runner.log:6512, run-4501 attempt 1).
3. Observe reprompt pressure produce a fabricated RED that never calls the reported symbol; the gate accepts it.
- runIds: `run-4501` (runner.log:6574, :6611; coder `run-4687`), `run-5307` (9 reprompt rounds 05:18:49→05:21:59, reproducer `run-5364`, 609 events), `run-6008` (runner.log:10678, :10717; coder `run-6184`).

## Root cause

- `apps/local-runner/internal/flowgate/reproduce_rule.go:91-117` — `checkReproduceRule` checks only `tr.Tests.Ran` and `len(tr.Tests.Failed) > 0` (named-test assertion failure); no verification that the failing test calls the reported symbol or is reachable from production code — any `t.Fatalf` on a constant satisfies it.
- Design gap: the unbounded reprompt loop ("suite passed → reprompt") pressures the agent to manufacture RED rather than conclude the report is false; no "not-reproducible" terminal verdict exists (see also BUG-391 for the never-exhausting counter).

## Evidence

- `~/fp-beds/lt-evidence/cp64/RESULT.md` (Bugs table, BUG-LIVE-CP64-03; L-64-2 FAIL, L-64-3 notes)
- `~/fp-beds/lt-evidence/cp64/BUG-LIVE-CP64-03-gate-gamed-by-fabricated-red.md` — full field report (rated critical there)
- `~/fp-beds/lt-evidence/cp64/runner.log` — :6512 (correct rejection), :6574 (`simulatedBuggy != 12`), :6611 (accept+lock), :10678/:10717 (run-6008 fabricated inversion accepted)
- `~/fp-beds/lt-evidence/cp64/run4501/`, `~/fp-beds/lt-evidence/cp64/run5307/`, `~/fp-beds/lt-evidence/cp64/run6008/` — per-run artifacts incl. on-disk fabricated test for run-5307

## Severity

- `high` (assigned; field report rated critical) — false-alarm bug reports reach the coder; the reproduce gate's fail-closed contract is defeated by reprompt-induced fabrication.

## Completion Notes (implemented 2026-09-23, CA-919)

- r-reproduce now requires a failing test that exercises a declared-scope symbol (ReproduceTargetChecked/ExercisesTarget, computed by reproduceFailuresExerciseTarget; Go targets, typed degradation otherwise). Tests: bug389_reproduce_target_test.go + bug389_target_resolution_test.go.
