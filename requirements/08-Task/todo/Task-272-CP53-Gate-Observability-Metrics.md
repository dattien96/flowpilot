# Task-272 — CP-53 P-6 Gate Observability Metrics Spike

## Metadata

- Document ID: `Task-272`
- Title: `CP-53 P-6 — Gate observability metrics spike (block / override / cost-per-accepted-change)`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `<chờ phân công>`
- Created: `2026-08-11`
- Last Updated: `2026-08-11`
- Parent Documents: [CP-53](../../07-Coding-Plan/inprogress/CP-53-Review-Loop.md)
- Child Documents: `<none>`
- Related Documents: [CP-53-Test-Steps](../../07-Coding-Plan/inprogress/CP-53-Test-Steps.md), Task-155, Task-242, BUG-288, [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `<none>`
- Tags: `flowgate, observability, cp-53, p-6`

## AI Quick View

### Summary

- Land **P-6 first** so later H-1 vs H-3 prioritization is data-backed (CP-53 Q-A / Q-2).
- Emit durable, inspectable gate metrics: block / reprompt / escalate / override rates + cost-per-accepted-change.
- Minimal spike — log + local artifact only; no desktop dashboard.

### Current Ask

- Implement metric emit path in `gate_hook` / settle path; additive tests; document how to inspect.

### Key Decisions

- `T-1` Metrics written under `.flowpilot/` (or structured log lines `[gate-metric]`) — operator-readable without UI.
- `T-2` cost-per-accepted-change = provider token/cost fields already on turn when present; else count turns-to-accept.
- `T-3` Does not change gate pass/block decisions — observe only.

### Constraints

- safe-fix: additive tests only; `feature_key: context-regression-engine`.
- Must not slow gate path meaningfully (append-only write / buffered log).
- No Supabase dependency.

### Open Questions

- Exact cost field source when provider omits usage — fallback to turn-count is OK for v1.

### Source Refs

- CP-53 §5 P-6, §8 Validation, D-decision Q-2 (P-6 lands first)

## 1. Goal

Ship a minimal observability spike so operators can answer "which hole leaks most?" before tuning P-1 vs P-2 aggressiveness.

## 2. Parent Links

- coding plan: CP-53 P-6
- tech design: SD-17 Plane C (gate) where applicable
- system spec: SS verifier/gate intents via CP-35 heritage
- specific upstream ids: CP-53 P-6, Q-A

## 3. Trigger

CP-53 plan-cut 2026-08-11 — observability must land before or with first hard fail-closed change to avoid cry-wolf blind tuning.

## 4. Exact Change

- `T-1` Define metric event schema (rule_id, action, mode, blind?, override?, run_id, step_id, timestamps).
- `T-2` Emit on gate evaluate outcomes (block / warn / pass / reprompt / escalate / override accept).
- `T-3` Emit accepted-change cost summary when turn finally proceeds after gate.
- `T-4` Additive tests: emit on block, emit on override, degrade when cost fields missing.
- `T-5` Short operator note in CP-53-Test-Steps §P-6 how to read metrics.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/gate_hook.go` (+ new `gate_metrics*.go` if needed), new `*_test.go`
- modules: `runner`, optionally `flowgate`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] Metrics appear for block and override paths in unit/integration tests (new file).
- [ ] Missing cost fields do not panic; fallback documented.
- [ ] Old `flowgate` / `gate_hook` tests untouched and green.
- [ ] Provider class: agnostic — evidence in CA.
- [ ] CA note written; CP-53-Test-Steps P-6 steps runnable.

## 7. Out of Scope

- Desktop dashboard / Settings UI
- Changing enforce/warn semantics
- P-1 `gate_blind` behavior

## 8. Completion Notes

- result: `<pending>`
- follow-ups: feed Q-A answer into Task-273/274 priority if data contradicts plan order
- upstream docs updated: `<pending>`
