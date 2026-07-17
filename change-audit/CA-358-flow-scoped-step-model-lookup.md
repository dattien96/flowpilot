# CA-358 — Scope step model lookup by flowRef (not bare node_id)

## Context

Chat Mode Review Loop (`flowRef=flowpilot-core-flow-pack/review-loop`) spawned
coder as **claude-haiku** even though Supabase row **Review Loop: Coder** is
**grok-composer-2.5-fast**. CSV showed two `node_id=coder` rows:

| name | model |
|------|--------|
| Context Coding Review Synthesis: Coder | claude-haiku |
| Review Loop: Coder | grok-composer-2.5-fast |

`resolveConfiguredModelForAgent` returned the **first** `node_id` match
(name.asc → Context Coding wins).

## Fix

- Pass parent `chatFlowRef` into model lookup via `resolveFlowNodeModel(ctx, parentRunID, node)`.
- Pass 1: match `node_id` **and** step_type belonging to that flow prefix
  (`flowpilot_core_flow_pack_review_loop_*`).
- Pass 2: unscoped `node_id` only if **exactly one** non-generic hit.
- Pass 3: role `flow-agent-delegate-<agent>` (unchanged).
- Never first-match among multiple `node_id=coder` rows.

## Tests

- `TestResolveFlowNodeModelScopesByFlowRef`
- Existing BUG-241 / node-specific / hub.inline tests updated for new signature

```bash
cd apps/local-runner
go test ./internal/runner/ -count=1 -timeout 3m -run 'TestResolveFlowNodeModel'
```

## Live

Rebuild serve → new Review Loop run → coder should use **Review Loop: Coder**
model (grok-composer-2.5-fast per current Supabase), not Context Coding's haiku.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Scope flow node model lookup by flowRef so Review Loop coder is not clobbered by another flow's node_id=coder
# --->8---
