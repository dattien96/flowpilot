# CA-775 — Bare `vibe-ingest` flowRef must resolve (not demote to chat)

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-326
change_type: bugfix
summary: Resolve bare pack flow ids so TUI /flow vibe-ingest starts ss_lock nodes instead of a chat step
# --->8---

## Why

Live run-211700: statusline Flow=vibe-ingest, timeline only `chat RUNNING`. `handleStartTurn` calls `explicitFlowRefResolves`; `ResolveFlowRef("vibe-ingest")` required `packId/flowId`. Bare id failed → flowRef cleared → normal chat. Restore path also forced `subMode=bug` on saved vibe arms.

## Change

- `ResolveFlowRef`: no slash → `ResolveBuiltin(flowpilot-core-flow-pack, bareId)`
- `applySavedModeAndFlow`: vibe FlowRef does not force bug extras

## Tests

`task326_bare_vibe_flowref_test.go`, `task326_restore_vibe_arm_test.go`

## Will not undo

CA-774 skip chat orchestration for vibe/harness.
