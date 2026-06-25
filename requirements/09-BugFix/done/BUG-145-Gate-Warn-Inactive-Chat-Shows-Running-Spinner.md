# BUG-145 — Gate-Warn Inactive Chat Shows Running Spinner

## Metadata

- Document ID: `BUG-145`
- Title: `Gate-warn inactive chat shows running spinner when another chat is active`
- Phase: `bugfix`
- Status: `done`
- Owner: `DatNguyen`
- Reviewers: `CP-35`
- Created: `2026-06-25`
- Last Updated: `2026-06-25`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: none
- Related Documents: [BUG-137: Navigator Spinner Persists For Gate-Blocked Inactive Chat](../done/BUG-137-Navigator-Spinner-Persists-For-Gate-Blocked-Inactive-Chat.md), [BUG-138: Gate Block Modal Re-Pops Every Chat Open](../done/BUG-138-Gate-Block-Modal-Re-Pops-Every-Chat-Open.md), [BUG-146](./BUG-146-Chat-Scrolls-Up-After-Gate-Reprompt.md), [CA-131](../../../change-audit/CA-131-fix-gate-warn-spinner-and-reprompt-scroll.md)
- Replaces: none
- Tags: `context-regression-engine, ui, navigator, flow-gate, warn, regression`

## AI Quick View

### Summary

- When a chat ends with a `warn`-status gate violation (`flow_gate_violation.status === "warn"`), `statusFromEvent` correctly settles the store `status` to `"completed"`. The active navigator entry shows no spinner.
- However, the backend runner keeps the run in `"running"` state (waiting for the user's next turn). The `_gateBlockedRunIds` guard — introduced by BUG-137 to override `item.status` for blocked-but-inactive chats — is only set for `status === "block"`, not for `status === "warn"`.
- When the user switches to another chat, `effectiveStatus = gateBlockedRunIds[item.runId] ? "completed" : item.status` falls through to `item.status = "running"`, rendering an infinite spinner for the gate-warned run's navigator entry.

### Current Ask

- Extend the `_gateBlockedRunIds` guard (or introduce a parallel `_gateSettledRunIds` set) to also cover `warn`-status gate violations, suppressing the "running" spinner for inactive chat entries whose store status has already settled to "completed".

### Key Decisions

- `V-1` After opening a chat that received a `warn` gate violation and then switching to another chat, the gate-warned chat's navigator entry must show no spinner (status icon = completed/idle).
- `V-2` Genuinely running chats (new turn started after the warn) must still show a spinner when the user's next reprompt fires.

### Constraints

- Fix must mirror the existing BUG-137 pattern: clear the settled-run flag on the next `turn_started` for that run so a fresh user turn correctly restores the live spinner.
- Must not affect `block`-status runs (already handled) or `reprompt`-status runs (store stays "running" — correct).

### Open Questions

- `Q-1` Should `_gateBlockedRunIds` be renamed to `_gateSettledRunIds` to reflect that it covers both "block" and "warn" settled states, or is a separate set cleaner?

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts` — `applyEvent` `flow_gate_violation` handler (line ~1708), `_gateBlockedRunIds` declaration and usage.
- `apps/desktop-flowpilot/src/components/Navigator.tsx` — `effectiveStatus` derivation (line ~498).
- `apps/desktop-flowpilot/src/state/timelineReducer.ts` — `statusFromEvent` for `flow_gate_violation` (line ~59–64).
- `SD-20 §3`, `SD-20 D-2` (gate_mode downgrades reprompt/block rules to warn).

## 1. Issue Summary

After a chat receives a `warn`-status flow-gate violation, it appears correct while that chat is active (store `status = "completed"`). But when the user opens any OTHER chat from the navigator, the gate-warned chat's sidebar entry switches to the polled backend status — which is still `"running"` because the runner has not formally closed the run (it is waiting for the user's next prompt). The result is an infinite spinning indicator on the navigator entry for the gate-warned chat, lasting until the user reopens it or the run times out.

## 2. Parent Links

- impacted coding plan: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md)
- impacted tech design: [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- impacted system spec: [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)

## 3. Environment and Reproduction

- environment: Desktop app (any provider); target project with `gate_mode = "warn"` or any rule that fires `flow_gate_violation.status === "warn"`.
- reproduction steps:
  1. Start a chat and submit a prompt that triggers a gate `warn` (e.g., code changed but no change-audit note found, with `gate_mode = "warn"`).
  2. Observe the ⚠ gate message in the chat timeline; the chat input is accessible (status settled to "completed").
  3. Click a DIFFERENT chat in the navigator.
  4. Observe the gate-warned chat's navigator entry — it shows an infinite spinning indicator.
  5. Click back to the gate-warned chat — the spinner disappears (active chat uses live store status).
- frequency: Reproducible every time a `warn`-status gate fires and the user switches away.

## 4. Expected vs Actual

- expected: The gate-warned chat's navigator entry shows a stable completed/idle icon regardless of which chat is currently active (same behaviour as gate-blocked chats after BUG-137).
- actual: The navigator entry shows an infinite running spinner whenever another chat is active, because `item.status = "running"` from the backend and `_gateBlockedRunIds` does not cover the `warn` case.

## 5. Impact

- users affected: Any user whose project fires `warn`-status gate violations (e.g., `gate_mode = "warn"` projects or `r-dep` warn rule).
- workflows affected: Normal chat navigation — switching between chats in the navigator sidebar.
- severity: Medium — cosmetic misleading state; does not block functionality.

## 6. Root Cause

- hypothesis: `_gateBlockedRunIds` guard introduced in BUG-137 is limited to `flow_gate_violation.status === "block"`.
- confirmed cause: `applyEvent` only adds to `_gateBlockedRunIds` when `e.status === "block"` (store.ts ~line 1708). For `status === "warn"`, no guard is set. The navigator `effectiveStatus` falls through to `item.status` (polled backend = "running"), showing a spinner for inactive warn-settled runs.
- evidence: Code inspection of `applyEvent` in `store.ts` and `effectiveStatus` in `Navigator.tsx`. The logic is identical to the BUG-137 root cause, but the fix only covered the "block" branch.

## 7. Fix Strategy

- `F-1` In `applyEvent` in `store.ts`, extend the `_gateBlockedRunIds` set to also add the run when `e.type === "flow_gate_violation" && e.status === "warn"` (in addition to the existing `status === "block"` branch). The existing `turn_started` handler already clears the entry when the user re-prompts, which is correct for both cases.
- `F-2` (Optional) Rename `_gateBlockedRunIds` → `_gateSettledRunIds` and update the comments and Navigator usage to reflect the broader semantic.
- `F-3` Add a unit test in `store.test.ts` mirroring the existing BUG-137 test for block, but for `warn` status.

## 8. Validation

- `V-1` After gate `warn` + chat switch: gate-warned chat navigator entry shows completed icon, not spinner.
- `V-2` After gate `warn` + user re-prompts (turn_started fires): spinner correctly reappears for the now-active run.
- `V-3` Existing BUG-137 block tests still pass.
- `V-4` Genuinely running chats (unrelated to gate) still show spinner correctly.

## 9. Regression Guard

- tests: New unit test in `store.test.ts` for the `warn` case; existing `_gateBlockedRunIds` tests for block case.
- alerts: None.
- audit checks: Review any new gate-status values added to `statusFromEvent` to ensure they are covered by the guard.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this is a gap in the BUG-137 fix, not a design change.
- notes left unchanged on purpose: SD-20 does not need updating; the gate semantics are correct, only the UI guard was incomplete.
