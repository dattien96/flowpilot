# Task-237: Generalize Post-Node "Done" Edge-Walking (Audit And Hub-Inline Successor Chaining)

## Metadata

- Document ID: `Task-237`
- Title: `Generalize Post-Node "Done" Edge-Walking (Audit And Hub-Inline Successor Chaining)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (`P-3`), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) (`D-1`, `D-4`, `D-7`, `F-3`), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md)
- Child Documents: [Task-235: Hub Notify Node Behavior](./Task-235-Hub-Notify-Node-Behavior-Reinvoke-Hub-For-AI-Composed-Notification.md) (further generalizes this task's `activeHubNodeID`-less single-hub-node assumption to support a SECOND hub-driven node), [Task-236: telegram.notify — Deterministic (No-Agent) Telegram Send Node Behavior](./Task-236-Telegram-Notify-Deterministic-Inline-Node-Behavior.md) (first real consumer of this task's successor-chaining as a `synthesis`/`audit` successor)
- Related Documents: [CP-36: Agent Review Loop And Main-Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md) (original `applyFlowControl`/`submit_review_outcome` design), [Task-171: Audit Step Draft And Commit Prep](../done/Task-171-Audit-Step-Draft-And-Commit-Prep.md) (original `runAuditNode`/`BuildAuditDraft` wiring this task changes the tail of)
- Replaces: `None`
- Tags: `agent-flow-engine, flow-control, node-behavior, edge-walking, synthesis, audit`

## AI Quick View

### Summary

- Before this task, a flow's `synthesis` (`hub.inline`) node calling `submit_review_outcome(status="done")` and an `audit` (`artifact.audit_draft`) node finishing its draft BOTH called `applyFlowControl(status:"done")` directly — hardcoding "I am the flow's last node" regardless of what `workflows.edges_json` actually declares after them. Placing ANY node after `synthesis` or `audit` (e.g. a notification step) was structurally impossible: the edge would simply never be walked.
- Fixes this by making both completion paths **edge-aware**: if the completing node's forward `"done"` edge targets a REAL successor node (not the terminal `"done"`/`"ask_user"`), dispatch that successor via the existing `advanceToNextInlineOrDelegate` helper instead of settling the whole flow immediately; if the edge targets the terminal (every current built-in flow's shape), fall through to `applyFlowControl` exactly as before.
- Verified additive-safe for every existing built-in: `review-loop.yaml` and `context-coding-review-synthesis.yaml` both wire `synthesis --done--> done` and `rag-harness.yaml` wires `audit --done--> done` directly at the terminal, so the new edge check resolves to "no real successor" in all three and falls through unchanged — confirmed by a dedicated no-op regression test plus the full existing review-loop/rag-harness test suites passing unmodified.
- This is the shared foundation `Task-235` (`hub.notify`) and `Task-236` (`telegram.notify`) both build on to be placeable after `synthesis`/`audit` at all.

### Current Ask

- Make `runAuditNode`'s completion and the hub's `submit_review_outcome`/`flow_control("done")` completion both edge-aware, reusing `advanceToNextInlineOrDelegate` rather than duplicating edge-walk logic, with zero behavior change for any flow whose completing node's `"done"` edge already points at the terminal.

### Key Decisions

- `T-1` `runAuditNode` gains `edges`/`nodes` parameters; before calling `applyFlowControl`, it checks `edgeTargetFrom(edges, node.ID, "done", "forward")` — a real (non-terminal) target dispatches via `advanceToNextInlineOrDelegate`, otherwise the original direct `applyFlowControl` call is unchanged.
- `T-2` New `advanceHubDoneThroughEdge(targetRunID, in)` is called from `SubmitFlowControl` immediately before `applyFlowControl`. It finds the flow's `hub.inline` node (`hubInlineNodeID(nodes)`) and applies the identical edge-aware check; returns `(result, true)` when it took over the transition, `(_, false)` when `applyFlowControl` should settle exactly as before (including for every non-flow / no-topology run, which never reaches the check at all).
- `T-3` Both changes reuse `advanceToNextInlineOrDelegate` — no new edge-walking logic was written twice.
- `T-4` This task deliberately does NOT solve chaining a SECOND hub-driven node after the first (e.g. two `hub.notify`/`hub.inline` nodes in sequence) — `hubInlineNodeID` finds only the flow's first `hub.inline` node, so a second hub-turn node would resolve against the wrong predecessor. `Task-235` closes that gap with `activeHubNodeID` tracking; this task's scope is exactly "one real successor after the flow's sole hub-inline node or after audit", matching every flow that existed when this task was written.

### Constraints

- Do not touch `applyFlowControl`'s own internal "done" handling — it remains the single settle point for every flow, reached either directly (unchanged shape) or via the new edge check falling through.
- Every existing built-in flow (`review-loop.yaml`, `rag-harness.yaml`, `context-coding-review-synthesis.yaml`) must be provably unaffected — verified via a dedicated no-op test plus the full pre-existing test suites for each.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`) — proceeded via careful manual inspection of every touched symbol instead of automated impact analysis.

### Open Questions

- `Q-1` (resolved by `Task-235`) Chaining a SECOND hub-driven node (e.g. `synthesis --done--> notify(hub.notify) --done--> done`) required `activeHubNodeID` tracking on top of this task's single-hub-node assumption — see `Task-235`.
- `Q-2` This generalization only covers the `"done"` transition. `"escalate"` (`ask_user`) and the cohort-join `"continue"` transition were not made edge-aware in this slice — out of scope (§7).

### Source Refs

- `CP-42` `P-3`; `SD-19` `D-1` (domain-free engine — the engine still has no use-case-specific branching, only a generic "does this node's own done-edge point somewhere real" check), `D-4` (generic `flow_control`), `D-7`, `F-3` (bounded/settlement contract — unaffected, this task only changes what happens BEFORE a genuine settle).
- `Task-171` (original `runAuditNode`/`BuildAuditDraft` tail this task's `T-1` changes).
- Code anchors: `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` (`runAuditNode`, `advanceToNextInlineOrDelegate`, `edgeTargetFrom`), `apps/local-runner/internal/runner/interactive_service.go` (`advanceHubDoneThroughEdge`, `SubmitFlowControl`), `apps/local-runner/internal/runner/flow_step_runtime.go` (`hubInlineNodeID`).

## 1. Goal

Let a flow author place a real successor node after `synthesis` (or `audit`) in the graph — instead of those nodes always being hardcoded as the flow's final step — by making their own "done" completion consult `workflows.edges_json` before settling, with zero behavior change for any flow that does not do this.

## 2. Parent Links

- coding plan: [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) — `P-3` (behavior/topology driven by node/edge data, not hardcoded step identity)
- tech design: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) — `D-1` (domain-free engine), `D-4` (generic `flow_control`), `D-7` (`FlowEdge{from,to,when,kind}` is the authoritative topology this task starts actually consulting at the completion point, not just at spawn time), `F-3` (settlement contract, unaffected)
- system spec: [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md)
- specific upstream ids: `CP-42 P-3`; `SD-19 D-1`, `D-4`, `D-7`

## 3. Trigger

While designing a flow to notify via Telegram after a review-loop's `synthesis` step, direct code inspection showed `synthesis`'s `submit_review_outcome(done)` and `audit`'s own completion both called `applyFlowControl(status:"done")` unconditionally — neither ever read `workflows.edges_json` at that point, so a `synthesis --done--> notify` (or `audit --done--> notify`) edge would simply never be walked; the flow would settle at `synthesis`/`audit` regardless of what was drawn after it.

## 4. Exact Change

- `T-1` Change `runAuditNode`'s signature to accept `edges []agentpack.FlowEdge, nodes []agentpack.FlowNode`; before its existing `applyFlowControl(status:"done", ...)` call, add `if target, ok := edgeTargetFrom(edges, node.ID, "done", "forward"); ok && target != "done" && target != "ask_user" { return s.advanceToNextInlineOrDelegate(...) }` — falls through unchanged when the edge resolves to the terminal.
- `T-2` Update both existing call sites of `runAuditNode` (`tryAdvanceFlowThroughInline`'s switch, `advanceToNextInlineOrDelegate`'s own switch) to pass `edges`/`nodes` through.
- `T-3` Add `advanceHubDoneThroughEdge(targetRunID, in FlowControlInput) (FlowControlResult, bool)` (`interactive_service.go`): bails (`false`) unless `in.Status == "done"` and the run is flow-engine-driven; otherwise finds `hubInlineNodeID(nodes)`, checks its forward `"done"` edge target the same way as `T-1`; a real successor dispatches via `advanceToNextInlineOrDelegate` and returns `(result, true)` with the loop left running/settled per the successor's own outcome; a terminal (or no hub-inline node / no tracked topology) returns `(_, false)`.
- `T-4` Wire `advanceHubDoneThroughEdge` into `SubmitFlowControl`, called right before the existing `applyFlowControl(targetRunID, in)` — only overrides the transition when it returns `handled=true`.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` (`runAuditNode` signature + edge check, its two call sites), `interactive_service.go` (`advanceHubDoneThroughEdge`, `SubmitFlowControl`)
- modules: flow executor dispatch (audit tail), hub flow-control routing
- routes: none
- tables: none

## 6. Acceptance Check

- `synthesis --done--> done` (every existing built-in's shape): `advanceHubDoneThroughEdge` returns `(_, false)`; `SubmitFlowControl` settles via `applyFlowControl` exactly as before this task.
- `synthesis --done--> notify --done--> done` (a user-authored addition): `advanceHubDoneThroughEdge` returns `(result, true)`, dispatches `notify` via `advanceToNextInlineOrDelegate`, and does not settle the flow at `synthesis`.
- `audit --done--> done` (`rag-harness.yaml`'s shape): `runAuditNode` still calls `applyFlowControl` directly, unchanged.
- `audit --done--> <real node>`: `runAuditNode` dispatches the successor via `advanceToNextInlineOrDelegate` instead of settling at `audit`.
- Full existing test suites for `review-loop`, `rag-harness`, and `context-coding-review-synthesis`-shaped flows pass unmodified (no new failures against the pre-task baseline).
- `go build ./...`, `go vet ./internal/runner` clean.

## 7. Out of Scope

- Chaining a SECOND hub-driven node (two `hub.inline`/`hub.notify` nodes in sequence) — `hubInlineNodeID` only ever finds the flow's first such node; closed by `Task-235`'s `activeHubNodeID` tracking.
- Making the `"escalate"`/`"continue"` transitions edge-aware — only `"done"` was generalized in this slice (`Q-2`).
- Any new node behavior (`telegram.notify`, `hub.notify`) — this task only makes the completion PATH edge-aware; the behaviors that benefit from it are `Task-236`/`Task-235`.

## 8. Completion Notes

- result: done. `T-1`–`T-4` implemented; verified additive-safe for every existing built-in flow via a dedicated terminal-edge no-op test plus full pre-existing regression suites; `go build`/`go vet` clean.
- This document backfills a slice implemented earlier in the same working session as `Task-235`, in direct response to the user's chat request rather than through the `/add-new-task` skill at the time it was built; written now, after the fact, to close that traceability gap (flagged as a follow-up in `Task-235`'s Completion Notes).
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`, no `gitnexus_*` tool resolved) — proceeded via manual inspection of every touched symbol instead of automated impact analysis.
- follow-ups: `Q-1` (resolved by `Task-235`), `Q-2` (escalate/continue edge-awareness, not attempted).
- upstream docs updated: none — this is an internal completion-path change consistent with `SD-19 D-4`'s existing generic `flow_control` contract; no upstream meaning changed.
