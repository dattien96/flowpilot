# CA-165: Support Inline-Behavior Flow Entry Nodes (BUG-NOTE-CP42 #9)

## Scope

Verified and fixed a P1 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: `startResolvedFlow` could not start `rag-harness` at all.

## The bug

`entryDelegateNodes` only returns no-dependency nodes whose behavior is `agent.delegate`. `rag-harness.yaml`'s entry node (`context`) declares `run: inline` / `behavior: context.produce` — an inline node, not a delegate node. `startResolvedFlow` would therefore find zero entry nodes for `rag-harness`, log "has no entry agent.delegate node; nothing to start", and return — the flow would resolve successfully (no error surfaced anywhere) but nothing would ever run. `review-loop` was unaffected (its entry node, `coder`, is `agent.delegate`), which is why this went unnoticed: it's specific to `rag-harness`'s shape.

The existing legacy step-based Flow Mode path (`injectFlowContextIfCoding` in `flow_context_handoff.go`) does handle context production, but only for the older multi-step `workflow_steps`-driven run type — it is not reachable from the newer interactive-chat `flowRef` selection path `startResolvedFlow` implements.

## Fix

Added `startInlineEntryChain` (`flow_executor.go`), invoked when `entryDelegateNodes` returns nothing. It handles the one shape either built-in flow actually declares: exactly one no-dependency entry node that is itself inline-scope (per `DefaultBehaviorRegistry`), with a single forward `"done"` edge to an `agent.delegate` node.

1. Dispatches the inline node's behavior synchronously via `DefaultBehaviorRegistry().Dispatch` — no provider call, matching `BehaviorScopeInline`'s contract (Task-176).
2. For `context.produce`, the dispatch returns a `FlowContextPackage` in `Payload["package"]`; this is rendered into the delegate node's prompt via the existing `renderFlowContextPrompt` → `ComposeFlowCodingPrompt` path — the same rendering the legacy step-based path already uses, so the resulting coder prompt shape is unchanged from what `rag-harness` users would have seen through Flow Mode.
3. Spawns the delegate node exactly like the delegate-entry path in `startResolvedFlow`, including setting `rs.activeFlowEdges`/`activeFlowNodes` *before* spawning (same fast-completion ordering fix as CA-162/BUG#4).

Multiple entry nodes, entry nodes with no reachable `agent.delegate` target, or chains longer than one inline hop are explicitly out of scope — none of that shape exists in either built-in flow today, and guessing at an unsupported topology would be worse than falling through to the pre-existing "nothing to start" log line, which is what happens instead.

## Verification

- New test `TestStartResolvedFlowStartsInlineEntryFlow` (`flow_executor_test.go`): starts `flowpilot-core-flow-pack/rag-harness` end to end via `startResolvedFlow` and asserts the `implement` delegate node is spawned as a child, with `activeFlowEdges` tracked.
- Full flow_executor/interactive_service flow-related test group (18 tests: `TestStartResolvedFlow*`, `TestCoderCompletion*`, `TestContinueReinvoke*`, `TestStartTurnWithFlowRef*`, edge/node helper unit tests) passes unchanged.
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: add startInlineEntryChain so a resolved flowRef whose entry node is an inline behavior (rag-harness's context.produce) can actually start, instead of silently spawning nothing
# --->8---
