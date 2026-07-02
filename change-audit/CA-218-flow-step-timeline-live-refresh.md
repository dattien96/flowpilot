# CA-218: Live Refresh Of The Flow Step Timeline On Agent-Graph Updates

## Summary

Fixed `BUG-180`: the Flow-mode step timeline only updated when the user switched focus between agents. The executor's step transitions are store writes with no dedicated event, and the desktop fetched step runtime only on mount/focus-change. Since every step transition rides alongside an `agent_graph_updated` event, the desktop now refreshes the step runtime when it receives one.

## What Changed

- `apps/desktop-flowpilot/src/state/store.ts`: in `consumeOrchestrationStream`, after applying an `agent_graph_updated` event, call `refreshWorkflowStepRuntime()` (which self-guards on chatMode + load-seq + target-run), so the step timeline updates live on every node spawn/complete without a focus switch.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- Not verified live (the timeline only renders in a live flow run needing the runner + a provider account); desktop `vitest` cannot run here (pre-existing ESM config error) — flagged in `BUG-180` (`V-2`).

## Notes

- Reuses the existing agent-graph lifecycle correlation and the existing `steps-runtime` endpoint; no new backend event. A dedicated `workflow_step_updated` event is a possible future refinement.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-180
change_type: bugfix
summary: Refresh the Flow-mode step timeline on agent_graph_updated events so it advances live during a run instead of only on a focus switch
# --->8---
