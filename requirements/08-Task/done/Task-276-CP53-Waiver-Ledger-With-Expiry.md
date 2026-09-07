# Task-276 — CP-53 P-4 Waiver Ledger With Expiry

## Metadata

- Document ID: `Task-276`
- Title: `CP-53 P-4 — test-override waiver ledger with reason + expiry re-arm`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `<chờ phân công>`
- Created: `2026-08-11`
- Last Updated: `2026-09-07`
- Parent Documents: [CP-53](../../07-Coding-Plan/done/CP-53-Review-Loop.md)
- Child Documents: `<none>`
- Related Documents: [CP-53-Test-Steps](../../07-Coding-Plan/done/CP-53-Test-Steps.md), Task-155 (decision card / overrides), BUG-289, [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `<none>`
- Tags: `flowgate, waiver, override, cp-53, p-4`

## AI Quick View

### Summary

- Close **H-4**: accepted test overrides must become **recorded debt** with reason + expiry, not silent forever waivers.
- On expiry, re-arm `r-reg` for that test.
- Surface open waivers for operator visibility.

### Current Ask

- Closed 2026-09-07 — CA-441. See §8.

### Key Decisions

- `T-1` Ledger under `.flowpilot/settings/` (or change-audit-linked index) — durable across restarts.
- `T-2` Accept without reason rejected or requires placeholder reason (prefer require non-empty reason).
- `T-3` Default expiry duration documented (e.g. 7–14 days) — configurable.
- `T-4` Sticky-until-green clear (BUG-289) still applies when test goes green before expiry.

### Constraints

- `feature_key: context-regression-engine`
- Do not remove Task-155 decision card UX — extend it.
- additive-tests-only.
- Will not undo: override clear-when-green behavior.

### Open Questions

- Exact default TTL — propose in implementation, confirm in CA.

### Source Refs

- CP-53 H-4, S-4, P-4, D-5

## 1. Goal

Every accepted regression override is time-bounded debt that comes back unless the suite is truly green.

## 2. Parent Links

- coding plan: CP-53 P-4
- specific upstream ids: H-4, S-4, D-5, Task-155

## 3. Trigger

One UI click can waive `r-reg` forever under pressure; soft skills cannot see UI accepts.

## 4. Exact Change

- `T-1` Waiver record schema: test_id, reason, accepted_at, expires_at, run_id, actor.
- `T-2` On override accept → write ledger entry.
- `T-3` On gate evaluate → expired waivers ignored / deleted → `r-reg` re-arms.
- `T-4` List/open waivers helper (CLI or log surface for v1).
- `T-5` Additive tests: accept writes ledger; expiry re-arms; green clears before expiry; missing reason rejected.
- `T-6` CP-53-Test-Steps §P-4.

## 5. Touched Areas

- files: `flowgate` override loaders, `gate_hook` accept path, desktop decision-card handler if accept is UI-owned, new tests
- modules: `flowgate`, `runner`, possibly `apps/desktop-flowpilot` accept UI
- routes: existing decision-card API if any
- tables: none (local JSON)

## 6. Acceptance Check

- [x] Accept with reason creates ledger entry with expiry.
- [x] After expiry, same failing test blocks again via `r-reg`.
- [x] Green before expiry still clears override (prior contract).
- [x] Old tests untouched + green; CA written; provider-agnostic.

## 7. Out of Scope

- Permanent waive without expiry
- Changing always-block semantics of `r-reg` itself
- `r-newtest` (Task-277)

## 8. Completion Notes

- result: landed CA-441 (`56a9997`): `flowgate/waiver_ledger.go` (`.flowpilot/settings/waiver_ledger.json`, 14-day TTL), `SaveOverrideWithReason`, empty reason rejected, expired re-arm on load. Tests in `cp53_waiver_newtest_test.go`.
- follow-ups: desktop client must send `reason` on gate-agreement; open-waiver dashboard deferred.
- upstream docs updated: [CA-441](../../../change-audit/CA-441-cp53-p4-waiver-ledger-expiry.md); parent [CP-53](../../07-Coding-Plan/done/CP-53-Review-Loop.md) filed `done`.
