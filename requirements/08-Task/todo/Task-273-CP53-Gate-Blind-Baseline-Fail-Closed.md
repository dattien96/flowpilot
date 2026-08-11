# Task-273 — CP-53 P-1 Gate Blind Baseline Fail-Closed

## Metadata

- Document ID: `Task-273`
- Title: `CP-53 P-1 — gate_blind for missing baseline / EnvError / red-at-capture`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `<chờ phân công>`
- Created: `2026-08-11`
- Last Updated: `2026-08-11`
- Parent Documents: [CP-53](../../07-Coding-Plan/inprogress/CP-53-Review-Loop.md)
- Child Documents: `<none>`
- Related Documents: [CP-53-Test-Steps](../../07-Coding-Plan/inprogress/CP-53-Test-Steps.md), Task-156, Task-242, BUG-288, BUG-289, Task-272, [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `<none>`
- Tags: `flowgate, gate-blind, baseline, cp-53, p-1`

## AI Quick View

### Summary

- Close **H-1/H-2**: missing baseline / oracle EnvError / red-at-capture must surface as first-class **`gate_blind`**, not silent green.
- Preserve BUG-288 corrupt-baseline fail-closed unchanged.
- Add baseline health/freshness + `flaky-quarantine.json` so red-at-capture is fixed, not tolerated forever.

### Current Ask

- Implement `gate_blind` classification + enforce block; additive matrix tests; flaky quarantine file schema.

### Key Decisions

- `T-1` Missing baseline (`LoadBaseline` → nil,nil) + production code in diff → `gate_blind` block in `enforce`.
- `T-2` First successful auto-capture still allowed — blind only when capture skipped/failed **and** turn has code changes.
- `T-3` Corrupt baseline path stays BUG-288 fail-closed (error ≠ missing).
- `T-4` Red-at-capture (`SuitePassed=false`) → `gate_blind` warn/block per mode + quarantine allowlist for known flaky names.
- `T-5` Go/TS baselines independent later (P-3); this task focuses runner Go path used by post-turn gate.

### Constraints

- `feature_key: context-regression-engine`
- Do **not** undo BUG-288 R13-13 corrupt fail-closed / always-block `r-reg`/`r-tests`.
- additive-tests-only; ask before any old-test edit.
- Cry-wolf mitigation: blind must be rare (CP-53 R-1 / F-6).

### Open Questions

- Exact freshness window (hours/commits) for baseline health — propose default in implementation, document in CA.

### Source Refs

- CP-53 H-1, H-2, S-1, P-1, D-1, D-6 (prep), F-1/F-6 plan-review

## 1. Goal

Make "gate is blind" an explicit blocking/warning state so regressions cannot hide behind a missing or unhealthy baseline.

## 2. Parent Links

- coding plan: CP-53 P-1
- specific upstream ids: H-1, H-2, S-1, D-1

## 3. Trigger

As-is: missing baseline disables `r-reg`/`r-tests` silently (`gate_hook` + `Ran=false` path). Highest-leverage cheap fix after metrics spike.

## 4. Exact Change

- `T-1` Introduce `gate_blind` reason enum/string (missing_baseline | env_error | red_at_capture | stale_baseline).
- `T-2` Wire into `runFlowGate` evaluate path: emit `EventFlowGateViolation` with status block/warn; do not silently skip oracle evaluation messaging.
- `T-3` Baseline health/freshness check helper.
- `T-4` Schema + load/save for `.flowpilot/settings/flaky-quarantine.json`.
- `T-5` Additive tests (matrix): missing baseline + code diff blocks in **chat and flow**; missing baseline + docs-only does not cry-wolf; EnvError blinds; corrupt still blocks via existing path; quarantine suppresses named flaky only.
- `T-6` Update CP-53-Test-Steps §P-1 automated + manual.
- `T-7` Before code: read BUG-288 CA notes (fail-closed contracts) — **will not undo** corrupt-baseline block, ClearOverrideIfGreen fail-closed, epoch durability.

## 5. Touched Areas

- files: `internal/flowgate/{baseline,oracle,rules,enforce}.go`, `internal/runner/gate_hook.go`, new `cp53_gate_blind_*_test.go`
- modules: `flowgate`, `runner`
- routes: none
- tables: none (local JSON)

## 6. Acceptance Check

- [ ] Delete baseline + edit production file → enforce mode emits `gate_blind` block (manual + test).
- [ ] Corrupt baseline still fails closed (existing BUG-288 behavior preserved — re-run related old tests, do not edit them).
- [ ] EnvError surfaces blind, not silent pass.
- [ ] Red-at-capture with quarantine entry does not falsely claim suite-green.
- [ ] Old suite untouched + green; CA written; provider-agnostic evidence.

## 7. Out of Scope

- Dogfood scripts (Task-275)
- Waiver ledger (Task-276)
- Review-loop done verdict (Task-274)
- Desktop UI for blind state (SSE event enough)

## 8. Completion Notes

- result: `<pending>`
- follow-ups: TS baseline independence finalized in Task-275
- upstream docs updated: `<pending>`
