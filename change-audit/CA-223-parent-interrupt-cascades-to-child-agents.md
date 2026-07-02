# CA-223: Cascade Main Stop To Running Child Agents

## Summary

Fixed a Flow Mode stop behavior gap: pressing the main chat Stop button called the generic run interrupt endpoint, which only canceled the parent run's own turn. Running child agents could keep working, and the flow loop itself could remain active. Interrupting a root/parent run now also cancels every running child agent turn, the loop stop path now cancels the parent turn too, and the desktop main-run Stop action uses the flow loop-stop path while a focused child still only stops that child.

## What Changed

- `apps/local-runner/internal/runner/interactive_service.go`: `Interrupt(parentRunID)` now collects the parent cancel plus running child-agent cancel functions and invokes them outside the service lock.
- `apps/local-runner/internal/runner/interactive_service.go`: `stopAgentLoop(parentRunID)` now also cancels the parent turn and clears pending auto-reinvoke state.
- `apps/desktop-flowpilot/src/state/store.ts`: main Flow Mode `stop()` now calls `stopAgentLoop(parentRunID)` before interrupting the parent run, while child-focused stop remains child-only.
- `apps/local-runner/internal/runner/interactive_service_test.go` and `apps/desktop-flowpilot/src/state/store.test.ts`: added coverage for parent cancel, parent loop stop, and child-focused stop routing.

## Verification

- `go test ./internal/runner -run 'TestStopAgentLoopCancelsParentTurn|TestInterruptParentCancelsRunningChildAgents|TestAutoReinvokeHubStopCancels'` passes.
- `npm run typecheck` passes in `apps/desktop-flowpilot`.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-42
change_type: bugfix
summary: Make Flow Mode stop halt the parent turn, child turns, and loop state so stopping the main run actually stops the whole live flow
# --->8---
