# Task-275 — CP-53 P-3 Dogfood Gate-Check + Hooks

## Metadata

- Document ID: `Task-275`
- Title: `CP-53 P-3 — scripts/gate-check dogfood via pre-commit + Claude Stop hook`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `<chờ phân công>`
- Created: `2026-08-11`
- Last Updated: `2026-09-07`
- Parent Documents: [CP-53](../../07-Coding-Plan/done/CP-53-Review-Loop.md)
- Child Documents: `<none>`
- Related Documents: [CP-53-Test-Steps](../../07-Coding-Plan/done/CP-53-Test-Steps.md), Task-273, D-3/D-6, [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `<none>`
- Tags: `dogfood, gate-check, pre-commit, cp-53, p-3`

## AI Quick View

### Summary

- Close **H-5**: FlowPilot development must run the same regression oracle it ships.
- Deliver `scripts/gate-check` with **separate Go and TS baselines** (D-6), wire git pre-commit + Claude Code `Stop` hook.
- Linux/CI is the proof environment; Windows documented (Git Bash/WSL) — no false claim of native Windows spawn parity.

### Current Ask

- Closed 2026-09-07 — CA-440. See §8.

### Key Decisions

- `T-1` Two baselines, independent block decisions (D-6).
- `T-2` TS baseline not ready → large warn / optional soft-block until green (R-4), Go still hard-blocks.
- `T-3` Inner-loop selective suite vs full suite at hook — document flags.
- `T-4` Hooks removable independently for emergency (CP-53 §9 fallback).

### Constraints

- `feature_key: context-regression-engine`
- Must not weaken BUG-288 contracts.
- safe-fix R1: script tests additive; do not "fix" failing old adapter tests on Windows by editing them.
- Ma sát (R-3): keep default check fast enough for commit.

### Open Questions

- Q-C owner of `flaky-quarantine.json` maintenance process (document interim owner = FlowPilot).

### Source Refs

- CP-53 H-5, S-3, P-3, D-3, D-6, R-3, R-4, F-3

## 1. Goal

Dogfood: a green→red regression in FlowPilot's own Go (and TS when ready) suite cannot be committed silently.

## 2. Parent Links

- coding plan: CP-53 P-3
- specific upstream ids: H-5, S-3, D-3, D-6

## 3. Trigger

Dev harness uses soft skills + bypassPermissions; mechanical gate does not run on our commits today.

## 4. Exact Change

- `T-1` Add `scripts/gate-check` (Go + TS paths, exit codes documented).
- `T-2` Bootstrap script to capture/update dual baselines.
- `T-3` Install path for `.git/hooks/pre-commit` (or husky-equivalent if already used — prefer documented simple hook).
- `T-4` Claude Code `Stop` hook in `.claude/settings.json`.
- `T-5` Additive tests or scripted self-check where feasible; manual DoD for hook wiring.
- `T-6` CP-53-Test-Steps §P-3 with Linux primary steps + Windows notes.

## 5. Touched Areas

- files: `scripts/gate-check*`, `.claude/settings.json`, hook templates, baseline bootstrap under `.flowpilot/` docs
- modules: reuses `flowgate` oracle APIs where practical
- routes: none
- tables: none

## 6. Acceptance Check

- [x] Intentional Go regression → `gate-check` non-zero; pre-commit refuses.
- [ ] Stop hook invokes same check (manual). (script shipped; live Linux/CI proof not in CA-440)
- [x] TS missing/unhealthy baseline does not disable Go gate (D-6).
- [x] Documented uninstall/fallback.
- [x] CA written; provider-agnostic.

## 7. Out of Scope

- Full Windows native shell portability for provider adapters
- Changing user-facing `gate_mode` defaults
- Waiver ledger (Task-276)

## 8. Completion Notes

- result: landed CA-440 (`2746fd3`): `apps/local-runner/cmd/gate-check/main.go`, `flowgate/dogfood_check.go`, `scripts/gate-check`, `scripts/install-gate-hooks.sh`, `scripts/hooks/pre-commit`, `cp53_dogfood_check_test.go`. Go missing baseline hard-blocks; TS missing warn-only.
- follow-ups: live Linux/CI pre-commit proof not recorded in CA-440; harden TS baseline to hard-block once green; Windows = Git Bash/WSL only.
- upstream docs updated: [CA-440](../../../change-audit/CA-440-cp53-p3-dogfood-gate-check-hooks.md); parent [CP-53](../../07-Coding-Plan/done/CP-53-Review-Loop.md) filed `done`.
