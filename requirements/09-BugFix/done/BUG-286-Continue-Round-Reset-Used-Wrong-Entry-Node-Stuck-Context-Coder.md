# BUG-286: "Continue" Round Reset Used The Wrong Entry Node, Leaving Coder/Context Stuck

## Metadata

- Document ID: `BUG-286`
- Title: `"Continue" Round Reset Used The Wrong Entry Node, Leaving Coder/Context Stuck`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [Task-237: Generalize Post-Node "Done" Edge-Walking](../../08-Task/done/Task-237-Generalize-Post-Node-Done-Edge-Walking-Audit-And-Hub-Inline-Successor-Chaining.md), [Task-235: Hub Notify Node Behavior](../../08-Task/done/Task-235-Hub-Notify-Node-Behavior-Reinvoke-Hub-For-AI-Composed-Notification.md), [BUG-282: Step Definition Stores Flow-Scoped depends_on_json](./BUG-282-Step-Definition-Stores-Flow-Scoped-Depends-On-Breaking-Cross-Flow-Reuse.md) (the edge-derived `DependsOn` this bug's root cause depends on)
- Child Documents: `None`
- Related Documents: `.flowpilot/logs/features/agent-flow-engine/run-9225.ndjson` (evidence)
- Replaces: `None`
- Tags: `agent-flow-engine, node-behavior, step-timeline, review-loop, telegram, ui, prompt`

## AI Quick View

### Summary

- Two independent problems surfaced from the same live run (`run-9225`), reported together: (1) the desktop UI showed BOTH `context` and `coder` stuck `PENDING` through an entire new review round — only flipping `coder` to `DONE` once it actually finished, never showing it `RUNNING` in between, and never un-sticking `context` at all; (2) the Telegram message the hub composed did not follow the user's Message Template's intended structure.
- Root cause of (1): the "continue" (new review round) step-reset resolved its re-entry node via `flowEntryNodeID` → `entryDelegateNodes`, which excludes any node with a non-empty `DependsOn`. Every Supabase-mirrored flow's `coder` node carries `DependsOn=["context"]` (edge-derived, per `BUG-282`), so `entryDelegateNodes` always excluded it — `flowEntryNodeID` returned `""` for this exact, common flow shape. `setFlowStepStatus(parentRunID, "", RUNNING)` was then a silent no-op (coder never shown RUNNING), and EVERY other node (including the once-only, upstream `context` entry) was blanket-reset to `PENDING` and never revisited.
- Root cause of (2): `composeHubNotifyPrompt` showed the Message Template verbatim but never instructed the model to actually FILL it from the turn's real results — a receiving model could (and did) treat it as a template to echo rather than a document to complete.

### Current Ask

- Resolve the "continue" round's re-entry node from the flow's own `edges` (the same mechanism the flow engine already uses elsewhere) instead of `DependsOn`-based entry detection, and reset only nodes actually forward-reachable from it. Make the hub.notify write-contract explicitly require filling any template from real results.

### Key Decisions

- `V-1` Reuse the existing `resolveContinueBackEdgeTarget(edges)` helper (already used elsewhere for the exact same "which node does 'continue' re-enter" question) as the re-entry node resolver, falling back to the pre-existing `flowEntryNodeID` only when the flow declares no "continue" back-edge at all.
- `V-2` New `forwardReachableNodeIDs(edges, startID)` helper: only nodes forward-reachable (via `forward`-kind edges only — a back edge must never be walked) from the re-entry node reset to `PENDING`. An upstream, once-only entry node (e.g. `context`, `lifecycle: once`) is never forward-reachable from `coder` and therefore correctly keeps its prior `DONE` status.
- `V-3` Defensive fallback: when the run's `activeFlowEdges` is entirely empty (no topology recorded at all — not expected for any real flow, but true of one pre-existing minimal test fixture), fall back to the pre-fix blanket reset rather than silently resetting nothing.
- `V-4` `composeHubNotifyPrompt` gained an explicit instruction: fill any template placeholders/section labels from the ACTUAL outcome of the run (visible in the same hub session's own context), and do not send the template's placeholder text unfilled or invent detail not actually shown.

### Constraints

- Must not change behavior for any existing flow whose re-entry node has NO `DependsOn` at all (the pack-YAML-authored, non-Supabase-mirrored shape) — `resolveContinueBackEdgeTarget` resolves the same node either way.
- Must not regress `TestApplyFlowControlLoopingResetsStepsSynchronously` (BUG-233), whose 2-node test fixture declares NO edges at all — required adding the `V-3` fallback specifically to keep that test's premise (blanket reset when there is no topology to reason about) intact.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via direct inspection.

### Open Questions

- `Q-1` The Message Template still has no MECHANICAL (Go-side) enforcement that the model actually replaces every placeholder — `V-4` is a prompt instruction, not a hard guarantee. A future gate rule (mirroring `Task-233`'s `r-artifact-telegram-sent`, but checking for the ABSENCE of literal placeholder text like `{{status}}` or `(agent fills)` in the sent message) would close this gap if it recurs.

### Source Refs

- `Task-237` (`resolveContinueBackEdgeTarget`, the pre-existing continue-reset block this bug fixes), `BUG-282` (edge-derived `DependsOn`, the mechanism that makes `entryDelegateNodes` exclude `coder`).
- `run-9225` diagnostic log: round-1 `step_status_transition` events at `10:54:01.4` show EVERY node (`context`, `coder`, `reviewer_correctness`, `reviewer_security`, `synthesis`, `tele-step`) set to `PENDING` in the same burst, with no `RUNNING` transition for `coder` anywhere until it actually completed at `10:54:16.2436`.
- Code anchors: `apps/local-runner/internal/runner/interactive_service.go` (the `NextAction=="looping"` reset block in `applyFlowControl`), `apps/local-runner/internal/runner/flow_executor.go` (`forwardReachableNodeIDs`, `resolveContinueBackEdgeTarget`), `apps/local-runner/internal/runner/flow_step_runtime.go` (`activeFlowEdgesFor`), `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` (`composeHubNotifyPrompt`).

## 1. Issue Summary

Reported via two screenshots of a live run: (1) the step timeline showed `context` and `coder` both stuck `PENDING` through an entire new review round, with `coder` only ever flipping directly to `DONE` (never visibly `RUNNING`), and `context` never un-stuck at all; (2) the Telegram message the AI-composed `hub.notify` path sent did not follow the intended structure of the user's Message Template.

## 2. Parent Links

- impacted coding plan: none directly
- impacted tech design: none — no `SD` describes the step-timeline reset's exact node-resolution mechanism
- impacted system spec: none

## 3. Environment and Reproduction

- environment: Desktop app, `context-coding-review-synthesis`-shaped flow, review round 2+ (a "continue" verdict from round 1's synthesis).
- reproduction steps:
  1. Run a review-loop-shaped flow (context → coder → reviewer cohort → synthesis) to its first `submit_review_outcome(changes_requested)` verdict.
  2. Observe the desktop step timeline immediately after: `context` and `coder` both read `PENDING`; `coder` never shows `RUNNING` even though it is actively re-running — only flips to `DONE` once it finishes.
  3. Observe `context` remains `PENDING` for the rest of the run (it never re-runs, since it is upstream of the loop, so nothing ever revisits it).
  4. Separately: bind a `telegram.v1` OUTPUT with a structured Message Template (e.g. "Bug/Task: ... / Fix: ... / State: ...") to a `hub.notify` node; observe the sent message does not follow that structure.
- frequency: deterministic for (1) on every Supabase-mirrored review-loop-shaped flow's second-and-later round; deterministic for (2) whenever a Message Template is set.

## 4. Expected vs Actual

- expected: (1) the re-entering node (`coder`) shows `RUNNING` immediately when a new round starts; an upstream once-only node (`context`) keeps its prior `DONE` status. (2) the sent message follows the template's structure, filled from the run's real results.
- actual: (1) `coder` silently never transitioned to `RUNNING` (`setFlowStepStatus` was called with an empty node id); `context` was wrongly reset to `PENDING` and stayed there. (2) the template was shown to the model with no instruction to actually complete it from real results.

## 5. Impact

- users affected: anyone running a multi-round review loop on a Supabase-mirrored flow (i.e. any Flow-Mode run through the desktop, not the file-pack-only Chat Mode path) — this is the common case, not an edge case.
- workflows affected: every review-loop-shaped flow's second-and-later round; every `hub.notify` node with a structured Message Template.
- severity: medium — cosmetic/UX-facing (the underlying orchestration and gating still worked correctly; this is a step-timeline display and message-quality defect), but confusing enough to read as "is the flow actually working?" during live use.

## 6. Root Cause

- hypothesis: the "continue" reset's re-entry-node resolution assumed a node's `DependsOn` reliably distinguishes "is this the flow's entry" — true only for pack-YAML-authored flows, not Supabase-mirrored ones.
- confirmed cause: `flowEntryNodeID(nodes)` → `entryDelegateNodes` filters out any node with `len(node.DependsOn) > 0`. `BUG-282` made every Supabase-mirrored flow's node carry edge-derived `DependsOn` (so `coder.DependsOn == ["context"]` from the `context->coder` forward edge) — meaning `entryDelegateNodes` excludes `coder` for essentially every real Flow-Mode run, and `flowEntryNodeID` returns `""`. `s.setFlowStepStatus(parentRunID, "", StepStatusRunning)` is a silent no-op (no node id to update), and the reset loop then marked EVERY OTHER node — including `context`, upstream of the loop and never meant to re-run — `PENDING`.
- evidence: `run-9225`'s diagnostic log shows the exact burst of `PENDING` transitions for all 6 nodes with no accompanying `coder: RUNNING` event, and `context` never receiving another `step_status_transition` for the rest of the run.
- Message-quality root cause: `appendTelegramOutputPrompt` (reused by `composeHubNotifyPrompt`) shows the Message Template's raw text but stops there — nothing in the prompt tells the model the template is something to COMPLETE using this turn's real results rather than a fixed string to relay.

## 7. Fix Strategy

- `F-1` Resolve the re-entry node via `resolveContinueBackEdgeTarget(edges)` (the flow's own declared "continue" back-edge target) instead of `flowEntryNodeID`; fall back to `flowEntryNodeID` only when no such back-edge is declared at all.
- `F-2` Add `forwardReachableNodeIDs(edges, reentryID)`: walks only `forward`-kind edges from the re-entry node; a node not reachable this way (e.g. `context`) is left untouched rather than reset.
- `F-3` Preserve the pre-fix blanket-reset behavior when `activeFlowEdges` is entirely empty (no topology recorded), so a run with genuinely no edge data does not silently reset nothing.
- `F-4` Add `activeFlowEdgesFor(parentRunID)` (mirrors the existing `activeFlowNodesFor`) so the reset block can read the run's edges the same way it already reads nodes.
- `F-5` `composeHubNotifyPrompt` gains an explicit instruction to replace any template placeholders with the real, visible outcome of this run, and not to invent or leave placeholder text unfilled.

## 8. Validation

- `V-1` `TestForwardReachableNodeIDsExcludesUpstreamNodesAndBackEdges`, `TestForwardReachableNodeIDsIgnoresBackEdges` (new, `flow_continue_reset_test.go`): direct unit proof of the new helper's scoping and back-edge exclusion.
- `V-2` `TestApplyFlowControlContinueResetsOnlyForwardReachableNodesKeepingUpstreamEntryDone` (new): full `applyFlowControl(continue)` integration test on a `context-coding-review-synthesis`-shaped fixture (with `coder.DependsOn=["context"]`, mirroring the live `BUG-282` condition) — asserts `context` stays `DONE`, `coder` becomes `RUNNING`, and every actually-downstream node (`reviewer_correctness`, `reviewer_security`, `synthesis`, `tele-step`) resets to `PENDING`.
- `V-3` `TestApplyFlowControlContinueFallsBackToEntryNodeWithoutBackEdge` (new): a flow with edges but no declared "continue" back-edge still resolves a re-entry node via the pre-existing fallback.
- `V-4` `TestComposeHubNotifyPromptInstructsFillingTemplateFromRealResults` (new): asserts the prompt contains the "fill from the ACTUAL outcome" instruction and the "do not send the template's placeholder text unfilled" warning.
- `V-5` Regression: re-ran the pre-existing `TestApplyFlowControlLoopingResetsStepsSynchronously` (BUG-233) — initially broke because its 2-node fixture declares no edges at all; fixed by adding the `F-3` empty-edges fallback rather than weakening the new scoping.
- `V-6` `go build ./...`, `go vet ./internal/runner` clean. Broad regression sweep (`Flow|Review|Orchestrat|Cohort|Hub|Reinvoke|Telegram|Audit|Validate|Behavior|Resume|Gate|Continue|Loop`): 478 passed, 7 pre-existing/unrelated codex-binary-dependent failures (same baseline confirmed earlier this session via `git stash -u`), 0 new failures.

## 9. Regression Guard

- tests: all of `V-1`–`V-4` above (`flow_continue_reset_test.go`, `flow_hub_notify_test.go`); `TestApplyFlowControlLoopingResetsStepsSynchronously` now also guards the empty-edges fallback path.
- alerts: none.
- audit checks: `change-audit/CA-326` records this fix.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: `entryDelegateNodes`/`flowEntryNodeID` themselves are unchanged (still correct for their original callers — real flow ENTRY resolution at start, where `BUG-282`'s edge-derived `DependsOn` is exactly the intended signal); only this ONE call site (the mid-flow "continue" round reset) used them for the wrong question and has been redirected to edge-based re-entry resolution instead.
