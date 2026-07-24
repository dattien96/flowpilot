# BUG-291: Regression Gate Dismiss Orphans Child Run

## Metadata

- Document ID: `BUG-291`
- Title: `Regression Gate Dismiss Orphans Child Run`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex`
- Created: `2026-07-20`
- Last Updated: `2026-07-20`
- Parent Documents: [Task-155](../../08-Task/done/Task-155-update-r-reg.md), [Task-242](../../08-Task/done/Task-242-Flow-Mode-Three-Tier-Gate.md), [SD-20](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `none`
- Related Documents: [CP-51](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [BUG-288](../done/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md), [BUG-289](./BUG-289-Flow-Mode-Invariant-Audit-25-Unhandled-Bug-And-Edge-Cases.md), [CA-367](../../../change-audit/CA-367-run9437-hub-park-active-child.md), [CA-369](../../../change-audit/CA-369-regression-gate-dismiss-terminal-action.md)
- Replaces: `none`
- Tags: `agent-flow-engine, flow-gate, regression, child-run, ui, a1`

## AI Quick View

### Summary

- A regression decision card belongs to a live child gate in the runner; it is not informational.
- Its former `Dismiss` button hid only the desktop modal, leaving the coder blocked/running and the parent cohort waiting with no actionable UI.

### Current Ask

- Prevent local dismissal from orphaning a regression-gated child; preserve the existing remediation choices and offer a safe terminal alternative.

### Key Decisions

- `F-1`: Regression decision cards use `Stop flow` as their secondary action; it calls the existing cascade-stop path for parent and children.
- `F-2`: Plain gate notifications without regression options remain locally acknowledgeable.

### Constraints

- Desktop-only presentation correction; do not alter gate evaluation, provider turns, or existing tests.
- Add regression coverage only in a new dedicated test file.

### Open Questions

- Live runner inspection was unavailable from this workspace (`127.0.0.1:4318` refused the request), so the reported run state is evidenced by the supplied screenshot and source trace.

### Source Refs

- User report: run `run-11466`; supplied screenshot header displayed `run-11471`.
- `ChatWorkspace.tsx` `GateBlockModal`; `store.ts` `dismissGateBlock` and cascade `stop`.
- Task-155, Task-242, CP-51 A1, BUG-289.

## 1. Issue Summary

In A1 live testing, a coder child showed the Regression Failed decision card. Selecting `Dismiss` hid the card but did not resolve or cancel its runner-side pending gate. The coder remained `running`, and the main review loop waited indefinitely from the operator's perspective.

## 2. Parent Links

- Task-155 owns the r-reg regression-decision card.
- Task-242 and SD-20 define flow-gate behavior for coding children.
- SS-14 owns the regression-safety rule that requires an explicit remediation decision.

## 3. Environment and Reproduction

- Environment: FlowPilot desktop against a local runner; user live test on 2026-07-20.
- Reproduction steps:
  1. Run a review-loop coder child that reaches an r-reg Regression Failed gate.
  2. Select `Dismiss` on the decision modal without selecting a remediation option.
  3. Observe the modal disappear while the coder remains running and main waits.
- Frequency: deterministic by code path; `dismissGateBlock` only cleared client state.

## 4. Expected vs Actual

- Expected: a regression gate always leaves the user an actionable remediation path, or a terminal Stop action; no child is hidden while still blocking the parent.
- Actual: `Dismiss` removed the only regression-gate surface locally while the runner's child gate remained pending.

## 5. Impact

- Users affected: review-loop operators who dismiss a regression decision card.
- Workflows affected: A1 coder-child regression gates and any r-reg decision card.
- Severity: high usability/liveness regression; no source or test data is lost.

## 6. Root Cause

- Hypothesis: A1's dual-UI suppression removed the parent fallback while a child decision card was active.
- Confirmed cause: `dismissGateBlock()` only executed `set({ gateBlock: undefined })`. Meanwhile `gate_hook.go` intentionally suppresses parent escalation for r-reg child gates to prevent an unsafe hub Continue while the child gate is pending. Together, dismissing orphaned the blocked child from every actionable desktop surface.
- Evidence: `ChatWorkspace.tsx` rendered `Dismiss` for option-bearing cards; `store.ts` cleared only the modal; `gate_hook.go` comment and condition suppress the parent escalation when gate options are present.

## 7. Fix Strategy

- `F-1`: classify option-bearing regression gate cards as non-dismissible.
- `F-2`: replace their local `Dismiss` action with `Stop flow`, which reuses the existing parent-and-child cascade-stop contract.
- `F-3`: retain `Dismiss` semantics for plain gate notifications that are informational rather than an unresolved decision.

## 8. Validation

- `V-1`: `npx tsx --test apps/desktop-flowpilot/src/components/gateBlockActions.test.ts` passed 2/2.
- `V-2`: `npm --prefix apps/desktop-flowpilot run build` passed (`tsc --noEmit` and Vite/Electron builds).
- `V-3`: live retest pending: trigger a regression decision card and verify the secondary action reads `Stop flow`; choosing it terminalizes main and coder instead of hiding the pending gate.

## 9. Regression Guard

- New `gateBlockActions.test.ts` asserts option-bearing regression cards choose `stop-flow`, while plain cards remain dismissible.
- Existing production `stop()` cascade remains the single terminalization path; no legacy test was changed.

## 10. Follow-Up Document Updates

- Upstream docs unchanged: this restores the existing liveness/UI contract rather than changing regression-gate policy.
- CP-51 A1 live-test log should record the failed pre-fix run and the post-fix retest result.
