# CA-787 — vibe cp_writer inherits session model

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: cp_writer/task_slicer default model is empty (inherit chat session); ignore Flow: Doc Writer gpt-5.4 role row; per-node Settings still win
# --->8---

## Why

Live run-214615 F2 showed `cp_writer` as `codex/gpt-5.4` while the session was opencode. Pass 3 `flow-agent-delegate-doc-writer` pinned it.

## Change

`vibeInheritsSessionModel`: skip unscoped node_id (pass 2) and role-row (pass 3)
for `cp_writer` and `task_slicer`. Empty → parent provider/model. Flow-scoped
step row (pass 1 / Settings) still overrides.

## Tests

`ca787_cp_writer_inherits_session_test.go` (role row, unique node_id, flow-scoped settings). Old role-row tests untouched.

## Providers

Agnostic: no `providerKey` branch. Inherit uses the run's current session.

## Will not undo

CA-786 ingest→CP chain. CA-616 planner inherit.
