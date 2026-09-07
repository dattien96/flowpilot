# Task-274 — CP-53 P-2 Review-Loop Done Requires Machine Verdict

## Metadata

- Document ID: `Task-274`
- Title: `CP-53 P-2 — synthesis→done requires submit-review-outcome PASS`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `<chờ phân công>`
- Created: `2026-08-11`
- Last Updated: `2026-09-07`
- Parent Documents: [CP-53](../../07-Coding-Plan/done/CP-53-Review-Loop.md)
- Child Documents: `<none>`
- Related Documents: [CP-53-Test-Steps](../../07-Coding-Plan/done/CP-53-Test-Steps.md), [CP-61](../../07-Coding-Plan/inprogress/CP-61-Harness-Done-Verdict-Gate.md) (harness leftover), `tools/submit-review-outcome.yaml`, review-loop.yaml, CP-55 Task-270 (flow migration), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `<none>`
- Tags: `agent-flow-engine, review-loop, submit-review-outcome, cp-53, p-2`

## AI Quick View

### Summary

- Close **H-3 (Ralph Wiggum done)**: synthesizer prose alone cannot advance `synthesis → done`.
- Require a **machine-checkable** PASS from `submit-review-outcome` before done edge fires.
- Optional: reviewer model/effort asymmetry vs coder (CP-53 D-2 / S-2).

### Current Ask

- Closed 2026-09-07 — CA-439 (review-loop `synthesis→done`). Harness 3-hub leftover → CP-61. See §8.

### Key Decisions

- `T-1` Tool already exists — this task enforces consumption, not a new tool face (unless schema gap found).
- `T-2` Missing / FAIL / escalate verdict → cannot take `when: done`; must continue / escalate / ask_user per existing edges.
- `T-3` Provider class = shared flow runtime → **R2 matrix required**.
- `T-4` Normal chat without review-loop unchanged.

### Constraints

- `feature_key: agent-flow-engine`
- Do not break CP-55 freeze→coder topology on review-loop.
- **Will not undo:** CA-428 (Canonical finalize only at terminal done), CA-431 (review-loop migration / context render), acceptance_nodes on synthesis.
- additive-tests-only; prefer new `cp53_review_done_verdict_*_test.go`.
- Reviewer cost increase is accepted (CP-53 R-2) — measure via Task-272 metrics when available.

### Open Questions

- Q-B: exact reviewer model/effort defaults — pick documented defaults in this Task; can tune later.

### Source Refs

- CP-53 H-3, S-2, P-2, D-2; review-loop.yaml edges synthesis→done

## 1. Goal

Make "done" a verified terminal decision, not an LLM self-grade.

## 2. Parent Links

- coding plan: CP-53 P-2
- specific upstream ids: H-3, S-2, D-2

## 3. Trigger

Coverage gap: new behavior with no failing test + synthesizer saying done = silent ship. Gate oracle cannot see it.

## 4. Exact Change

- `T-1` Hub/synthesizer: require recorded PASS verdict before allowing `done` transition.
- `T-2` Clear error / continue path when verdict absent or FAIL.
- `T-3` Config for reviewer model/effort asymmetry (defaults documented).
- `T-4` Additive tests:
  - no verdict → cannot done
  - FAIL verdict → cannot done
  - PASS → done allowed
  - continue still works without claiming done
  - **matrix** Claude / Codex / Grok fake adapters
  - Normal chat unaffected
- `T-5` Update CP-53-Test-Steps §P-2.

## 5. Touched Areas

- files: `runner/flow_*.go`, `flow_validate_audit_dispatch.go` / hub notify paths, `agentpack/flow-pack/flows/review-loop.yaml` (if needed), synthesizer agent md, new tests
- modules: `runner`, `agentpack`
- routes: none
- tables: none

## 6. Acceptance Check

- [x] review-loop cannot reach terminal done without PASS verdict (automated).
- [x] Claude + Codex + Grok matrix green (fake adapters).
- [x] Normal chat / non-review-loop flows unchanged.
- [x] Old tests untouched + green.
- [x] CA notes provider classification + matrix evidence.

## 7. Out of Scope

- Rewriting reviewer prompts for content quality (only verdict gate)
- `r-newtest` (Task-277)
- Dogfood hooks (Task-275)

## 8. Completion Notes

- result: landed CA-439 (`84fb557`): `runner/review_done_verdict.go`, `cp53_review_done_verdict_test.go`. Hub `approved→done` on `acceptance_nodes` including synthesis requires recorded `submit_review_outcome` PASS. Matrix claude/codex/grok.
- follow-ups: T-3 reviewer asymmetry (Q-B) → [CP-61](../../07-Coding-Plan/inprogress/CP-61-Harness-Done-Verdict-Gate.md) P-2. Harness hubs `plan_synthesis` / `synthesis` / `cp_synthesis` → CP-61 P-1 (landed CA-757). Test-Steps §P-2 → CP-61 P-3.
- upstream docs updated: [CA-439](../../../change-audit/CA-439-cp53-p2-synthesis-done-requires-machine-verdict.md); parent [CP-53](../../07-Coding-Plan/done/CP-53-Review-Loop.md) filed `done`; leftover owned by [CP-61](../../07-Coding-Plan/inprogress/CP-61-Harness-Done-Verdict-Gate.md).
