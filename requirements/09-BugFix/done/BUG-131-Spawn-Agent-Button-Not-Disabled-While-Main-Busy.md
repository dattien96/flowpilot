# BUG-131: Spawn Agent Button Not Disabled While Main Busy

## Metadata

- Document ID: `BUG-131`
- Title: `Spawn Agent Button Not Disabled While Main Busy`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [BUG-120: UI Spawn Leaves Parent Running And Modal Open](./BUG-120-UI-Spawn-Leaves-Parent-Running-And-Modal-Open.md), [CA-120: Agent spawn UX hardening](../../../change-audit/CA-120-agent-spawn-ux-hardening.md)
- Replaces: `None`
- Tags: `multi-agent, desktop, agents-panel, ui, spawn`

## AI Quick View

### Summary

- While the main run is blocked on a `wait=true` child (or otherwise busy), the chat send button is correctly disabled, but the `+ Spawn agent` button stays enabled, letting the user start many agents at once.
- Fix: disable the `+ Spawn agent` button and the spawn modal's `Spawn ▸` button under the same busy condition the send button uses (`mainCardBusy`), with an explanatory tooltip.

### Current Ask

- Disable the spawn-agent control while the main run is busy so the UI can't fan out unbounded concurrent spawns, mirroring the send button.

### Key Decisions

- `V-1` The spawn control's disabled state reuses `mainCardBusy` (main run status is `running`/`waiting_approval`/`waiting_question`) — the same signal the send button blocks on.
- `V-2` Both the panel `+ Spawn agent` button and the modal `Spawn ▸` button are guarded, so an already-open modal can't bypass the gate.

### Constraints

- Do not block spawning when the main run is idle (normal case must be unaffected).
- Keep the backend spawn path unchanged; this is a UI guard only.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx` — `mainCardBusy`, `+ Spawn agent` button, modal `Spawn ▸` button.
- `apps/desktop-flowpilot/src/components/ChatInput.tsx` — `blocked`/`canSend` (the send-button gate being mirrored).

## 1. Issue Summary

The Agents panel `+ Spawn agent` button was only disabled when there was no main run or no spawn client. It did not consider whether the main run was busy. When a `wait=true` child blocks the main turn, the send button disables (correct) but the spawn button stayed active, so the user could queue many agents simultaneously.

## 2. Parent Links

- impacted coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md) (P-7 Agents panel)
- impacted tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md) (§7.2 direct UI spawn, Test 4)
- impacted system spec: [SS-06](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)

## 3. Environment and Reproduction

- environment: FlowPilot desktop, a chat with a `wait=true` child spawned from the UI.
- reproduction steps:
  1. Spawn an agent with Wait = ON; the main run blocks on the child.
  2. Observe the send button is disabled but `+ Spawn agent` is still clickable.
  3. Click it again and spawn another agent while the first is still blocking.
- frequency: always while the main run is busy.

## 4. Expected vs Actual

- expected: while the main run is busy, the spawn control is disabled like the send button.
- actual: the spawn control stayed enabled, allowing concurrent spawns.

## 5. Impact

- users affected: anyone spawning from the UI during a busy main run.
- workflows affected: UI spawn; future auto agent-management relies on a single bounded spawn entry.
- severity: Medium — can fan out unintended concurrent agents.

## 6. Root Cause

- confirmed cause: the `+ Spawn agent` button's `disabled` expression omitted the `mainCardBusy` signal that the panel already computes (and that the chat send button uses).

## 7. Fix Strategy

- `F-1` Add `mainCardBusy` to the `+ Spawn agent` button's `disabled` condition and add a busy tooltip.
- `F-2` Add `mainCardBusy` to the modal `Spawn ▸` button's `disabled` condition so an open modal can't bypass the gate.

## 8. Validation

- `V-1` `npx tsc --noEmit` (desktop) — pass.
- `V-2` Manual (SD-16 Test 4): while a `wait=true` child blocks the main run, the `+ Spawn agent` and `Spawn ▸` buttons are disabled with a tooltip; when the main run returns to idle, they re-enable; idle-state spawning is unchanged.
- `V-3` Unit test harness: the desktop vitest suite currently fails to load on this machine with a pre-existing ESM config error affecting all suites, so component tests could not be executed here; type-checking is green.

## 9. Regression Guard

- tests: type-check; SD-16 Test 4 documents the manual check.
- alerts: a spawnable button during a busy main run indicates the guard regressed.
- audit checks: spawn controls must share the send button's busy signal.

## 10. Follow-Up Document Updates

- upstream docs that must change: SD-16 — added Test 4 for this gate.
- notes left unchanged on purpose: the backend spawn path and direct-UI-spawn semantics are unchanged.
