# CA-322: Generalize Post-Node "Done" Edge-Walking (Audit And Hub-Inline Successor Chaining)

## Scope

Implements Task-237: before this change, `synthesis` (`hub.inline`, via `submit_review_outcome`) and `audit` (`artifact.audit_draft`) both settled the whole flow by calling `applyFlowControl(status:"done")` directly, never consulting `workflows.edges_json` at that point — so a real node placed after either of them in the graph could never be reached. Generalizes both completion paths to check the completing node's own forward `"done"` edge first: a real successor dispatches via the existing `advanceToNextInlineOrDelegate` helper; the terminal (every existing built-in flow's shape) falls through to the original, unchanged `applyFlowControl` call.

This is the shared foundation Task-235 (`hub.notify`) and Task-236 (`telegram.notify`) both need to be placeable after `synthesis`/`audit` at all. It was implemented earlier in the same working session as CA-320/CA-321, in direct response to the user's chat request rather than through the `/add-new-task` skill at the time; this note backfills that gap.

## Changes

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go`:
  - `runAuditNode` gained `edges []agentpack.FlowEdge, nodes []agentpack.FlowNode` parameters; before its existing `applyFlowControl(status:"done", ...)` call, added a check via `edgeTargetFrom(edges, node.ID, "done", "forward")` — a real (non-terminal) target dispatches via `advanceToNextInlineOrDelegate` instead; a terminal target falls through to the original call, byte-for-byte unchanged.
  - Both existing call sites of `runAuditNode` (`tryAdvanceFlowThroughInline`'s switch, `advanceToNextInlineOrDelegate`'s own switch) updated to pass `edges`/`nodes` through.
- `apps/local-runner/internal/runner/interactive_service.go`:
  - added `advanceHubDoneThroughEdge(targetRunID string, in FlowControlInput) (FlowControlResult, bool)` — bails immediately (`false`) unless `in.Status == "done"` and the run is flow-engine-driven; otherwise resolves `hubInlineNodeID(nodes)` and applies the identical edge-aware check; a real successor dispatches via `advanceToNextInlineOrDelegate` and returns `handled=true`; a terminal (or no hub-inline node / no tracked topology, i.e. every non-flow run) returns `handled=false`.
  - wired `advanceHubDoneThroughEdge` into `SubmitFlowControl`, called immediately before the existing `applyFlowControl(targetRunID, in)` call — only overrides the transition when it returns `handled=true`.
- Tests added at the time (folded into later hub.notify test files this session): a chains-to-a-real-successor test and a no-op-for-terminal-synthesis regression test proving every existing built-in flow (`review-loop.yaml`, `context-coding-review-synthesis.yaml`, both wiring `synthesis --done--> done` directly) is unaffected.
- New Task doc: `requirements/08-Task/done/Task-237-Generalize-Post-Node-Done-Edge-Walking-Audit-And-Hub-Inline-Successor-Chaining.md`.

## Verification

- `go build ./...` (apps/local-runner) — passed. `go vet ./internal/runner` — no issues.
- Dedicated no-op regression test confirms every existing built-in flow's `synthesis --done--> done` / `audit --done--> done` shape resolves to the terminal and falls through to `applyFlowControl` unchanged.
- Full pre-existing `review-loop`/`rag-harness`/`context-coding-review-synthesis`-related test suites pass with no new failures against the pre-change baseline.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`, no `gitnexus_*` tool resolved) — proceeded via careful manual inspection of every touched symbol instead of automated impact analysis. `gitnexus_detect_changes()` not run for the same reason.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-237
change_type: refactor
summary: make synthesis (hub.inline) and audit (artifact.audit_draft) completion edge-aware before settling the flow, so a real successor node placed after either of them is actually reachable, with zero behavior change for every existing built-in flow
# --->8---
