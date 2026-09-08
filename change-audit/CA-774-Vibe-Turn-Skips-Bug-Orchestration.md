# CA-774 — vibe-ingest first turn not blocked by chat subMode=bug

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-326
change_type: bugfix
summary: Skip Chat Mode orchestration validation for /flow vibe and harness refs; TUI vibe arm omits subMode=bug
# --->8---

## Why

Live Test 4: `400 invalid_flow_ref: flowRef "vibe-ingest" is not a valid built-in orchestration option for subMode "bug"`. `builtinArm` always sent `subMode=bug` (review-loop chat picker). `handleStartTurn` validated against Chat Mode options; vibe-ingest is not one.

## Change

- `SkipChatOrchestrationCheck` for harness + vibe family
- `handleStartTurn` skips picker validation for those refs
- `builtinArm` omits bug extras when FlowRef looks like vibe

## Tests

New: `task326_vibe_turn_orchestration_test.go`, `task326_vibe_arm_no_bug_submode_test.go`. Old `TestLaunchArm_FirstTurnSendsSubModeAndFlowRef` (review-loop) untouched.

## Will not undo

CA-773 picker filter. Chat Mode bug picker still rejects unknown review-loop refs.
