# CA-172: Forward Auto-Advance No Longer Gated on isCoderRun (BUG-NOTE-CP42 #17)

## Scope

Verified and fixed a P2 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: the generic forward-edge auto-advance path (`tryAdvanceFlowFromNode`, added in CA-163) was itself fully generic, but the call site gating entry into it was not.

## The bug

The `EventTurnCompleted` handler in `interactive_service.go` only dispatched `advanceOrNotifyHub` (which tries `tryAdvanceFlowFromNode` first) when `s.agentOrchestrator.loopMode(rs.parentRunID) == "explicit" && isCoderRun(rs)`. `isCoderRun` is a role/agent-name substring match on `"coder"`. Both built-in flows happen to name their entry delegate agent `coder`, so this went unnoticed — but any user-authored flow whose entry node uses a different agent (e.g. `planner`, `writer`) would never reach the generic auto-advance path at all. Its completion instead fell through to a passive `"Sub-agent %q completed"` note that never reinvokes the hub, silently stalling the flow — the flow's forward edges exist and are correctly declared, but nothing would ever walk them.

## Fix

Added `parentHasTrackedFlow(s *InteractiveService, parentRunID string) bool` (`interactive_service.go`), which checks whether the parent run has a resolved flow's edges tracked on it (`activeFlowEdges`/`activeFlowNodes` non-empty — set by `startResolvedFlow`/`startInlineEntryChain`). The gate is now `isCoderRun(rs) || parentHasTrackedFlow(s, rs.parentRunID)`.

This only *adds* coverage, it narrows nothing: `tryAdvanceFlowFromNode`'s own internal checks (tracked edges present on the parent, a forward `"done"` edge actually exists from this specific completing node's ID, the target is a spawnable `agent.delegate` node) still safely return `false` and fall through to the exact same legacy note+reinvoke-hub behavior for anything the new gate lets through that doesn't actually correspond to a real flow-tracked node. The pre-existing `isCoderRun` branch — needed for AI-driven (non-flowRef, no tracked edges) flows, where `spawn_agent` calls decide progression themselves — is unchanged.

## Verification

- New test `TestForwardAutoAdvanceFiresForNonCoderNamedEntryNode` (`flow_executor_test.go`): tracks a synthetic two-node flow (`planner -> reviewer`, forward/done) directly on a parent run, spawns a child named `"planner"` (not in the catalog, so `isCoderRun` is false for it), and asserts the `reviewer` target still auto-spawns once `planner` completes.
- Full suite: 1004 passed, 15 pre-existing/environmental failures (unchanged from before this fix).
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: gate entry into the forward-edge auto-advance path on parentHasTrackedFlow in addition to isCoderRun, so a flow whose entry node isn't named "coder" can still auto-advance instead of silently stalling
# --->8---
