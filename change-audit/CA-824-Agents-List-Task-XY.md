# CA-824 — /agents picker shows task x/y for vibe-sprint children

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: BUG-369
change_type: bugfix
summary: Stamp each spawned child with vibe task x/y and show it on the /agents picker, dump, and sidebar
# --->8---

## Why

Live `/agents` listed three `preflight_contract_plan` and several `coder`
rows. Only run ids differed. Operator could not map a row to Task-910 vs
911 vs 912.

## Change

- `AgentRunSummary.vibeTaskIndex/Total/Name` (runner, TUI client, desktop)
- Spawn copies parent `vibeTaskProgress` onto the child
- `upsertSummary` preserves those fields when a later status update omits them
- Child session persist/reconstruct
- `/agents` picker detail, dump, and chat-mode sidebar: `task 2/3 Task-911.md`

## Tests

New `TestBUG369_*` only. Old `/agents` picker tests green.

## Providers

Agnostic Case 1; TUI chips Claude/Codex/Grok.

## Will not undo

BUG-367 parent chrome chip. Agent focus via `/agents <name>`.
