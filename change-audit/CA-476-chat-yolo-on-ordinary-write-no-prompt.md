---
id: CA-476
feature_key: yolo-policy
title: Chat YOLO=on ordinary file write must not prompt
date: 2026-08-14
status: COMPLETE
---

## Change

Task-291: normal-chat YOLO=on must auto-run an ordinary workspace write (`test.txt`) without a permission card.

TUI previously mounted `[APPROVAL]` then auto-approved. Under YOLO=on it now auto-approves without showing the gate. YOLO=off still waits. `ask_user` is unchanged.

`--yolo` on Grok only flipped local `m.yolo`. After session defaults load, TUI now calls the existing `POST /provider-accounts/grok-yolo-posture` (same as `/yolo` and Desktop). `startTurn` does **not** copy turn YOLO onto `grokDesiredAlwaysApprove` (that would force `--always-approve` on Flow/Workflow Grok children and skip V9-21 commit denylist).

`resolveTurnYolo` remains a behavior-preserving extract used by `runTurn` only.

## Provider impact

Case 3 (per-provider wiring, shared bridge):

- Claude: `bypassPermissions` (no `--permission-prompt-tool`); `RequestApproval` auto-approves if a write still arrives.
- Codex: `approval_mode=never` + same bridge auto-approve.
- Grok: `--always-approve` only via `ApplyGrokYoloPosture` (TUI `--yolo`/`/yolo`, Desktop toggle). Inbound `yoloModes` still auto-replies `session/request_permission` when the turn YOLO is on. TUI hides a late card.

## Tests

New `task291_chat_yolo_ordinary_write_test.go` (A2.10–A2.14) and `task291_chat_yolo_write_test.go`. Old tests untouched.

# ---8<--- flowpilot:change-ledger
feature_key: yolo-policy
source_doc_id: Task-291
change_type: bugfix
summary: TUI YOLO=on hides write approval card; Grok --yolo uses existing posture endpoint, not startTurn
# --->8---
