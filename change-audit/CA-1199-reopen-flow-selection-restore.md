# CA-1199 — reopen restores running flow selection (BUG-1199)

## Evidence

Live run `run-225691` (chat-launched `vibe-tasks` → engine rebound `vibe-sprint`):
persisted row carries `run_kind=chat`, `flow_arm=started`,
`chat_flow_ref=flowpilot-core-flow-pack/vibe-sprint`, **no `workflowId`**.
Reopening the chat restored `chatMode=workflow_step_auto` (BUG-575 path via
`flowArm`) but `launchMode`/`selectedWorkflowId` were only set when
`historyItem.workflowId` was present — so the Workflow pane rendered with no
flow ticked even though a flow had run. Ledger:
`desktop|reopen-loses-flow-mode-selection`.

## Root cause

1. `openHistoryRun` keyed `launchMode`/`selectedWorkflowId` restore on
   `historyItem.workflowId`, which chat-launched (flowRef) runs never have.
2. Even a resolved selection could render blank: `vibe-sprint` is a system
   flow kept out of `visibleWorkflows` by `filterWorkflowsForWorkingMode`,
   and a `<select>` whose value matches no option shows nothing.

## Fix

- `store.ts` `openHistoryRun`: for every flow-driven run
  (`isWorkflowHistoryItem`) restore `launchMode:"workflow"` and resolve
  `selectedWorkflowId` from `historyItem.workflowId` → `handle.workflowId` →
  `flowRef` matched against catalog rows by `bareFlowId(packFlowId ?? id)`.
  Unresolvable refs clear the stale pick rather than show another run's flow.
  `pendingFlowArm` unchanged — `flowArm==="started"` never re-arms.
- `ChatWorkspace.tsx`: append the selected-but-filtered row to the select
  options and render non-startable rows `disabled`, so the running system
  flow displays as the current value but can't be manually re-armed.

## Tests

`store.bug1199-reopen-flow-selection.test.ts`:
- pack-prefixed `flowRef` + no `workflowId` → selects the vibe-sprint mirror
  row, `launchMode=workflow`, `flowStarted`, no `pendingFlowArm`.
- `historyItem.workflowId` stays authoritative over a mismatched `flowRef`.
- unresolvable ref clears a stale `selectedWorkflowId`.

Regression: `store.bug575-history-flow-mode`, `store.chat-mode-persist`,
`store.chat-detached-parity`, `ca1070_vibe_flow_surface`,
`ca1071_posture_toggle_tabs` — all green. `tsc --noEmit` clean.
