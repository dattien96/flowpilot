# BUG-152: Flow Gate Fires On Child Agent Turns Mid-Loop

## Metadata

- Document ID: `BUG-152`
- Title: `Flow Gate Fires On Child Agent Turns Mid-Loop`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-30`
- Last Updated: `2026-06-30`
- Parent Documents: `requirements/07-Coding-Plan/CP-36-Generic-Agent-Flow-Engine.md`, `requirements/07-Coding-Plan/CP-35-Context-Regression-Engine.md`
- Child Documents: `none`
- Related Documents: `change-audit/CA-147-flow-gate-exempt-child-runs.md`
- Replaces: `none`
- Tags: `agent-flow-engine, flow-gate, cp-36, cp-35, interactive-service`

## AI Quick View

### Summary

- The post-turn flow gate (CP-35 P-4/P-5) fires unconditionally after every completed turn, including turns belonging to child agent runs (coder, reviewer, etc.).
- Child agents make code changes as part of their job but are never responsible for writing change-audit (CA) notes; the hub/root run writes the CA note on a later turn.
- Firing the gate on a child turn therefore always produces a false `code_changed` violation ("code changed but no change-audit note found"), which blocks the child run completion and interrupts the hub waiting for the child.
- The user saw the flow gate modal appear immediately after the hub said "I'm waiting for the sub-agent to complete", which prevented the review loop from proceeding.

### Current Ask

- Exempt child runs from the flow gate; only root/hub runs should be subject to gate enforcement.

### Key Decisions

- `V-1` The guard is `rs.parentRunID == ""` at the gate call site in `interactive_service.go`. Child runs have `rs.parentRunID != ""`; root runs have `rs.parentRunID == ""`.
- `V-2` No change to `runFlowGate` itself — the exemption is a single condition at the call site, keeping the gate logic self-contained.

### Constraints

- The fix must not break gate enforcement for root/hub runs — those still need full gate behaviour.
- Child-run CA-note enforcement, if ever required, must be added explicitly; silent exemption is the safe default.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` — post-turn gate block at lines 2180–2188

## 1. Issue Summary

During CP-36 Case 1 manual E2E testing the hub sends "I'm waiting for the coder sub-agent to complete" and immediately afterward the flow gate violation modal appears, interrupting the flow. The coder sub-agent also does not visibly start in the Agents panel (the gate fires before the panel can reflect the running agent).

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/CP-36-Generic-Agent-Flow-Engine.md`
- impacted coding plan: `requirements/07-Coding-Plan/CP-35-Context-Regression-Engine.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md`
- impacted system spec: none

## 3. Environment and Reproduction

- environment: Windows 11, `just dev`, CP-36 Case 1 (hub spawns coder with wait=true)
- reproduction steps:
  1. Start the desktop app with `just dev`.
  2. Create a new chat, switch to a project with a review-loop flow enabled (CP-36).
  3. Send a prompt that causes the hub to spawn a coder sub-agent.
  4. Observe: hub says "waiting for sub-agent", then the flow gate modal immediately appears.
- frequency: 100 % when a child agent makes any code change (file write, git commit) during its turn.

## 4. Expected vs Actual

- expected: Child agent completes its turn normally; flow gate is silent; hub receives the coder result and continues the loop.
- actual: Flow gate fires on the coder's turn completion, emits a `flow_gate_violation` event, blocks the coder's turn from finalising, and surfaces the modal to the user.

## 5. Impact

- users affected: anyone using CP-36 review-loop flow (Case 1 and beyond).
- workflows affected: every multi-agent review loop where the coder writes code.
- severity: high — makes CP-36 review loops completely unusable.

## 6. Root Cause

- hypothesis: Gate call site in `interactive_service.go` does not distinguish between root and child runs.
- confirmed cause: `runFlowGate` is invoked unconditionally for all completed turns (line 2182). `rs.parentRunID` is `""` for root/hub runs and non-empty for child agent runs. The gate's `code_changed` rule fires whenever code was written AND no CA note is present in the git diff. Child agents write code but never write CA notes — that is the hub's responsibility on a subsequent turn. The unconditional call therefore always triggers the rule for any child agent that touched the filesystem.
- evidence: Log message "code changed but no change-audit note found" appeared immediately after the coder's `turn_completed` event in the runner logs.

## 7. Fix Strategy

- `F-1` In `interactive_service.go` at the gate call site (lines 2182–2186): change `if completed {` to `if completed && rs.parentRunID == "" {`. This exempts child runs from gate enforcement while leaving root/hub run behaviour unchanged.

## 8. Validation

- `V-1` Go `go build ./...` passes with no errors.
- `V-2` Manual: CP-36 Case 1 — hub spawns coder, coder writes code, coder completes without triggering gate modal; hub receives result and continues loop.
- `V-3` Manual: root/hub run that writes code without a CA note still triggers the gate as before.

## 9. Regression Guard

- tests: existing E2E tests in `interactive_service_e2e_test.go` exercise the gate path on root runs; child-run exemption is implicit (child turns are not gated in those tests).
- alerts: none.
- audit checks: `change-audit/CA-147-flow-gate-exempt-child-runs.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — the gate design in CP-35 always intended root-run enforcement; child exemption is a clarification of existing intent, not a new design decision.
- notes left unchanged on purpose: `runFlowGate` internals are unchanged; only the call-site guard is new.
