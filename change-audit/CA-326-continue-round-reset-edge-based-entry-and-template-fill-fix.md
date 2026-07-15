# CA-326: "Continue" Round Reset Uses Edge-Derived Re-Entry Node; hub.notify Prompt Enforces Template Fill

## Scope

Fixes BUG-286 (two independent problems from the same live report): (1) the "continue" (new review round) step-timeline reset resolved its re-entry node via `flowEntryNodeID`/`entryDelegateNodes`, which excludes any node with a non-empty `DependsOn` — true for `coder` on every Supabase-mirrored flow (BUG-282's edge-derived `DependsOn`) — so it always returned `""`, silently no-op'ing the `RUNNING` transition and blanket-resetting every OTHER node (including the once-only, upstream `context` entry, which then never re-ran and stayed stuck `PENDING` forever). (2) `composeHubNotifyPrompt` showed the Message Template verbatim with no instruction to actually complete it from the run's real results, so the model could (and did) leave placeholder text unfilled.

## Changes

- `apps/local-runner/internal/runner/flow_executor.go`: added `forwardReachableNodeIDs(edges, startID)` — BFS over `forward`-kind edges only (a back edge is never walked), excluding the start node itself and the `done`/`ask_user` terminals.
- `apps/local-runner/internal/runner/flow_step_runtime.go`: added `activeFlowEdgesFor(parentRunID)`, mirroring the existing `activeFlowNodesFor`.
- `apps/local-runner/internal/runner/interactive_service.go`: the `NextAction=="looping"` reset block in `applyFlowControl` now resolves its re-entry node via `resolveContinueBackEdgeTarget(edges)` (falling back to `flowEntryNodeID` only when no "continue" back-edge is declared), and resets to `PENDING` only nodes in `forwardReachableNodeIDs(edges, reentryID)` — except when `activeFlowEdges` is entirely empty, which preserves the pre-fix blanket-reset behavior (needed to keep BUG-233's own minimal 2-node test fixture, which declares no edges at all, passing).
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`: `composeHubNotifyPrompt` now instructs the model to replace any template placeholders/section labels with the ACTUAL outcome of the run (visible in the same hub session's own context) and warns against sending the template's placeholder text unfilled or inventing detail not actually shown.
- New tests: `flow_continue_reset_test.go` (`TestForwardReachableNodeIDsExcludesUpstreamNodesAndBackEdges`, `TestForwardReachableNodeIDsIgnoresBackEdges`, `TestApplyFlowControlContinueResetsOnlyForwardReachableNodesKeepingUpstreamEntryDone`, `TestApplyFlowControlContinueFallsBackToEntryNodeWithoutBackEdge`); `flow_hub_notify_test.go` (`TestComposeHubNotifyPromptInstructsFillingTemplateFromRealResults`).

## Verification

- `go build ./...` — passed. `go vet ./internal/runner` — no issues.
- `go test ./internal/runner -run 'ForwardReachable|ApplyFlowControlContinue|ComposeHubNotify|HubNotify'` — 16 passed.
- Re-ran the pre-existing `TestApplyFlowControlLoopingResetsStepsSynchronously` (BUG-233): initially broke by this change (its fixture declares no edges at all); fixed by adding the empty-edges fallback rather than weakening the new edge-based scoping. Now passes alongside everything else.
- Broad regression sweep (`Flow|Review|Orchestrat|Cohort|Hub|Reinvoke|Telegram|Audit|Validate|Behavior|Resume|Gate|Continue|Loop`): 478 passed, 7 pre-existing/unrelated codex-binary-dependent failures (confirmed baseline via `git stash -u` earlier this session), 0 new failures.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via direct inspection of the diagnostic log (`run-9225`) and the named code paths.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-286
change_type: bugfix
summary: resolve the continue-round step-reset re-entry node from the flow's own edges instead of DependsOn-based entry detection (which silently failed on every Supabase-mirrored flow), scope the reset to forward-reachable nodes so an upstream once-only entry node stops getting stuck PENDING, and make the hub.notify write-contract require filling any message template from the run's real results
# --->8---
