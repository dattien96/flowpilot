# CA-161: Edge-Driven "Continue" Reinvoke Target (Task-180 Follow-Up)

## Scope

Per explicit user direction, replace the one remaining role-hardcoded decision in the live cohort/hub loop — "which child does a `continue` flow_control signal reinvoke" — with a resolution driven by the flow's own edge data, for runs started from a resolved `flowRef`.

## Why this was scoped narrowly (and what stays hardcoded)

A full generalization ("Go reads `edges_json` to decide who to spawn at every step") was previously flagged as a large, risky rewrite of the live cohort/hub loop. Investigating the actual call graph narrowed the real gap considerably:

- **Forward progression** (coder → reviewers → synthesis) is already AI-driven: the hub's own reasoning, following its skill/prompt guidance, calls `spawn_agent` itself to advance the flow. This is not a Go-side hardcoded decision to begin with — there was nothing to "generalize" here.
- **The one genuine hardcode** is the back-edge: when `synthesis` (via the `submit_review_outcome`/`flow_control` tool) reports `status: continue`, `maybeReinvokeCoderForContinue` scanned running children for one with role `"coder"` by string match, instead of asking "which node does the flow's back-edge point to."

This CA closes exactly that gap — the back-edge resolution — not the full topology walk. `isCoderRun`'s role-based check remains as the correct fallback for AI-initiated flows (no resolved `FlowDefinition`, so no edges to read).

## Completed

- `interactiveRun` gained `activeFlowEdges []agentpack.FlowEdge`, set once when `startResolvedFlow` successfully spawns a flow's entry node(s) (`record.Definition.Edges`). Empty/nil for any run not started via a resolved `flowRef`.
- `resolveContinueBackEdgeTarget(edges)` (`flow_executor.go`): finds the edge with `Kind: "back"` and `When: "continue"`, returns its `To` node id. `review-loop.yaml` declares exactly one such edge (`synthesis -> coder`).
- `maybeReinvokeCoderForContinue` now: if the parent run has tracked edges and a back-edge target resolves, matches a spawned child by `label == targetNodeID` (labels are already set to the flow's node id at spawn time — `Label: node.ID` in `startResolvedFlow`) — `isCoderRun` is not consulted at all in this branch. If no edges are tracked (or no back-edge resolves), the code falls through to the original `isCoderRun` role match, byte-for-byte unchanged.

## Verification

- `TestResolveContinueBackEdgeTargetFindsBackContinueEdge` / `...NoMatchReturnsFalse` — pure unit coverage of the edge-lookup helper.
- `TestContinueReinvokeUsesEdgeResolvedTargetForFlowStartedRun` — full integration: `startResolvedFlow` spawns the entry node and tracks edges, `applyFlowControl("continue")` reinvokes the coder child with the review feedback embedded in its prompt, proving the edge-driven path fires end to end for a flowRef-started run.
- **Zero regression on the existing role-based path**: `TestE2EReviewLoopChangesRequestedFeedbackReachesCoderPrompt` and the other 4 `TestE2EReviewLoop*` tests (which spawn the coder manually, with no tracked flow edges) all still pass unchanged — confirming the fallback branch is byte-identical to the pre-existing behavior.
- Domain hardcode guard baseline unchanged (`interactive_service.go` stays at 1 — `isCoderRun`'s literal `"coder"` is still there, now purely as the fallback for AI-initiated flows); comment updated to describe the new primary/fallback split.
- Full suite: 996 passed (up from 993), identical 15 pre-existing/unrelated failures.

## Still open

- **Superseded**: the forward edges (coder → reviewers → synthesis) were judged AI-decision-driven at the time this note was written. A user question ("how does the AI actually know the topology?") revealed that judgment was wrong — nothing informed the hub's reasoning of the flow shape at all. `tryAdvanceFlowFromNode` (added afterward, see `flow_executor.go` and `TestCoderCompletionAutoSpawnsReviewerCohort` in `flow_executor_test.go`) closed this gap: Go now auto-spawns the reviewer cohort deterministically from `edges_json`, with no AI decision involved. See [CA-163](./CA-163-forward-edge-auto-spawn-reviewer-cohort.md) for that change's own record.
- A flow with more than one `Kind: "back", When: "continue"` edge (not the case for either built-in flow today) would only ever use the first declared match — nothing in the current live path disambiguates by which node emitted the `continue` signal.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-180
change_type: refactor
summary: resolve the continue-signal reinvoke target from the flow's own back-edge data instead of role-name matching, for flowRef-started runs
# --->8---
