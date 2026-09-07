# Task-277 — CP-53 P-5 r-newtest Reprompt Rule

## Metadata

- Document ID: `Task-277`
- Title: `CP-53 P-5 — r-newtest rule: production code change requires new additive test`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `<chờ phân công>`
- Created: `2026-08-11`
- Last Updated: `2026-09-07`
- Parent Documents: [CP-53](../../07-Coding-Plan/done/CP-53-Review-Loop.md)
- Child Documents: `<none>`
- Related Documents: [CP-53-Test-Steps](../../07-Coding-Plan/inprogress/CP-53-Test-Steps.md), oracle-rule, additive-tests-only, [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `<none>`
- Tags: `flowgate, r-newtest, tdd, cp-53, p-5`

## AI Quick View

### Summary

- Close remaining **H-3 coverage gap**: production code edits without any **new** test file/cases get **reprompt** (`r-newtest`).
- Aligns with safe-fix / oracle-rule: remediation is **ADD tests**, never edit/delete old tests to go green.
- Complements Task-274 (done verdict) — this gives the oracle teeth on brand-new behavior.

### Current Ask

- Closed 2026-09-07 — CA-442. See §8.

### Key Decisions

- `T-1` Trigger: turn git diff touches production paths AND **no newly added** test file (`*_test.go` / `*.test.ts` added in the same turn). **Editing an existing test file does NOT satisfy** this rule (aligns with oracle-rule / additive-tests-only).
- `T-2` Action default: **reprompt** (not always-block) to avoid cry-wolf (R-1); configurable.
- `T-3` Doc-only / CA-only / requirements-only diffs exempt.
- `T-4` Remediation text must say: ADD a new test file/cases — never edit/delete old tests to pass.

### Constraints

- `feature_key: context-regression-engine`
- Must coexist with `r-tamper` (warn on editing old tests).
- additive-tests-only for this Task's own tests.
- Prefer enable via `flow-rules.json` so false positives can disable without revert.

### Open Questions

- Exact production-path allowlist vs exclude list (vendor, generated) — define in implementation + CA.

### Source Refs

- CP-53 H-3, S-5, P-5, D-4, F-4 plan-review

## 1. Goal

Ensure new production behavior is accompanied by a new failing-or-asserting test derived from AC before the turn is accepted.

## 2. Parent Links

- coding plan: CP-53 P-5
- specific upstream ids: H-3, S-5, D-4

## 3. Trigger

Oracle only sees regressions of previously green tests; brand-new untested behavior still passes gate.

## 4. Exact Change

- `T-1` Add rule `r-newtest` to default rule set (enabled with safe defaults).
- `T-2` Diff classifier: production vs test vs exempt paths.
- `T-3` Remediation message: add new test covering AC; do not weaken old tests.
- `T-4` Additive tests:
  - prod change, no new test → reprompt
  - prod change + new test file → pass
  - docs-only → no fire
  - only old-test edit → does not satisfy r-newtest (still may hit r-tamper)
- `T-5` CP-53-Test-Steps §P-5.

## 5. Touched Areas

- files: `flowgate/rules.go`, `enforce.go`, `gate_hook.go` evaluation wiring, `flow-rules` defaults, new tests
- modules: `flowgate`, `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [x] Production-only diff without new tests → `r-newtest` reprompt.
- [x] Same + new `*_test.go` → no `r-newtest`.
- [x] Docs/CA-only → no fire.
- [x] Rule disable via config works (fallback).
- [x] Old tests untouched + green; CA written; provider-agnostic.
- [x] Remediation text forbids editing old tests.

## 7. Out of Scope

- Generating the new test automatically (LLM may do it after reprompt)
- Always-block for r-newtest in v1
- Changing oracle-rule skill prose beyond cross-link

## 8. Completion Notes

- result: landed CA-442 (`98701f6`): `r-newtest` in `flowgate/rules.go` (reprompt, `production_change_no_new_test`); `HasNewTestFileAdded()` only git-added `*_test.go` / `*.test.ts`. Old-test edit does not satisfy. Tests in `cp53_waiver_newtest_test.go`.
- follow-ups: vendor/generated production-path allowlist if false positives; disable via `flow-rules.json`; promote to block after false-positive rate (Task-272 metrics).
- upstream docs updated: [CA-442](../../../change-audit/CA-442-cp53-p5-r-newtest-reprompt-rule.md); parent [CP-53](../../07-Coding-Plan/done/CP-53-Review-Loop.md) filed `done`.
