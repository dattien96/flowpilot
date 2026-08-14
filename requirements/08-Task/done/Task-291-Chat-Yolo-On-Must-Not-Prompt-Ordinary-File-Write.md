# Task-291: Chat YOLO On Must Not Prompt Ordinary File Write

## Metadata

- Document ID: `Task-291`
- Title: `Chat YOLO On Must Not Prompt Ordinary File Write`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-13`
- Last Updated: `2026-08-14`
- Feature Keys: `yolo-policy, cli-tui, mcp-tools`
- Parent Documents: [SS-08 Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md), [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-2 YOLO controls), [Task-280](../todo/Task-280-TUI-Session-Controls-Provider-Model-Reasoning-Yolo.md), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md)
- Child Documents: [CA-476](../../change-audit/CA-476-chat-yolo-on-ordinary-write-no-prompt.md)
- Related Documents: [Task-286](../todo/Task-286-TUI-Approval-And-Question-Gates.md), [CP-46](../../07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md) (Grok `session/request_permission` + YOLO), [BUG-296](../../09-BugFix/done/BUG-296-Claude-Flow-Coding-Child-Yolo-Write-Denied-No-Prompt-Tool.md), [BUG-299](../../09-BugFix/done/BUG-299-Flow-Mode-Allowed-YOLO-Off-Causing-Missed-Approvals-And-Stalls.md)
- Replaces: `None`
- Tags: `yolo-policy, cli-tui, approval, file-write`

## AI Quick View

### Summary

- Chat YOLO=on ordinary workspace writes (`test.txt`) no longer show a permission card.
- TUI `--yolo` on Grok calls existing `ApplyGrokYoloPosture` after session defaults (same endpoint as `/yolo` / Desktop). `startTurn` is unchanged.
- TUI no longer mounts `[APPROVAL]` when YOLO is on even if a late `permission_required` arrives.
- Claude bypassPermissions + Codex approval-never + shared `RequestApproval` auto-approve remain the per-provider wiring.

### Current Ask

Done. P-2 residual: silent TUI auto-approve + Grok `--yolo` uses the existing posture endpoint (no runner `startTurn` sync).

### Key Decisions

- `T-1` Scope is normal chat, YOLO=on, ordinary workspace file write (`test.txt` / content `test`). Not flow-mode (already YOLO-forced, BUG-299).
- `T-2` Case 3 (per-provider): verify Claude, Codex, and Grok. One provider green is not enough.
- `T-3` Keep YOLO=off gating, `ask_user` questions, and the git-commit denylist (`ForceShellBridge` / V9-21).
- `T-4` Do not auto-answer `ask_user` when fixing write permissions.
- `T-5` Additive tests only; do not edit the legacy suite to hide a remaining prompt.
- `T-6` Grok `--always-approve` stays on `ApplyGrokYoloPosture`. TUI `--yolo` posts that endpoint after session defaults; do not copy turn YOLO onto `grokDesiredAlwaysApprove` in `startTurn` (would force Flow Grok children to skip V9-21).

### Constraints

- SS-08 YOLO meaning stays: bypass approval gates, not skip human questions.
- CP-56 D-1: TUI is a thin client. This residual stays on TUI chrome + the existing Grok posture route; it does not change `startTurn`.
- GitNexus MCP was unavailable in this thread; edits limited to TUI `permission_required`, TUI `--yolo` posture cmd, and reverting the `startTurn` Grok sync.

### Open Questions

- `Q-1` RESOLVED: defect is provider-shared card UX plus Grok launch-flag lag, not a missing TUI `yoloMode` field.
- `Q-2` RESOLVED: TUI `/yolo` still flips local state immediately (legacy tests). `--yolo` now posts Grok posture after session defaults so launch does not depend on a later `/yolo`.
- `Q-3` RESOLVED: `yoloModes[sessionID]` still auto-replies inbound under YOLO; `--always-approve` is the extra launch-time channel so Grok may not ask at all.
- `Q-4` RESOLVED: acceptance is local workspace write, not Drive MCP.

### Source Refs

- SS-08 §3; CP-56 D-5, D-15, P-2; Task-280 T-1/T-5; `yolo_resolver.go` `RunnerAutoApprove`; TUI `startupGrokYoloPostureCmd`; TUI `handleEvent` `permission_required`.

---

## 1. Goal

In normal chat with YOLO=on, asking the model to create `test.txt` containing `test` writes the file without a permission/approval prompt. YOLO=off still prompts. Claude, Codex, and Grok all match.

## 2. Parent Links

- coding plan: CP-56 P-2 (TUI YOLO controls); Grok write gating also CP-46 P-5/P-9
- tech design: 04-02 runner approval/turn contract; SD-06 provider adapters
- system spec: SS-08 §3 YOLO bypasses approval gates
- specific upstream ids: Task-280 T-1, T-5; CP-56 D-15; `resolveYoloPosture(true).RunnerAutoApprove`

## 3. Trigger

Operator report (2026-08-13, TUI chat session): YOLO showed on, prompt was to write `test.txt` with content `test`, the client still asked for permission.

This is a defect relative to SS-08, captured as a Task so it stays on the CP-56 / YOLO execution chain. Promote to a BugFix doc only if implementation finds a regression id that should live in `09-BugFix`.

## 4. Exact Change

- `T-1` Classified Case 3: Claude `bypassPermissions`, Codex `approval_mode=never`, Grok `--always-approve` + inbound auto-reply. Shared `RequestApproval` auto-approves writes when `b.yolo` is true.
- `T-2` TUI already sends `yoloMode` on chat turns. Left `/yolo` immediate flip (legacy tests). Silent-auto-approve if a `permission_required` still arrives while statusline YOLO is on.
- `T-3` TUI `--yolo` + Grok calls `startupGrokYoloPostureCmd` → existing posture POST. `startTurn` is not modified.
- `T-4` New tests: `task291_chat_yolo_ordinary_write_test.go` (A2.10–A2.14) and `task291_chat_yolo_write_test.go` (TUI card + Grok startup posture).
- `T-5` Manual: same `test.txt` prompt in TUI with YOLO on the statusline.

## 5. Touched Areas

- `apps/local-runner/internal/runner/yolo_resolver.go` (`resolveTurnYolo` extract for `runTurn` only)
- `apps/local-runner/internal/runner/interactive_service.go` (`runTurn` uses `resolveTurnYolo`; no `startTurn` Grok sync)
- `apps/local-runner/internal/runner/task291_chat_yolo_ordinary_write_test.go`
- `apps/local-runner/internal/tui/app/app.go` (`permission_required` / `ApprovalResolvedMsg` / `startupGrokYoloPostureCmd`)
- `apps/local-runner/internal/tui/app/task291_chat_yolo_write_test.go`
- `change-audit/CA-476-chat-yolo-on-ordinary-write-no-prompt.md`
- modules: `yolo-policy`, `cli-tui`, `ai-providers`
- routes: existing `POST .../turns`; Grok posture endpoint unchanged
- tables: none

## 6. Acceptance Check

- Chat YOLO=on + “write `test.txt` with content `test`”: no permission prompt; file exists with `test`.
- Chat YOLO=off + same prompt: permission prompt; deny blocks the write.
- Repeat for Claude, Codex, and Grok (or document a provider that cannot reach native write in this environment).
- `ask_user` / questions still appear under YOLO=on.
- Flow coding git-commit denylist still denies commits (do not regress V9-21).
- New tests green; related old tests untouched and green.

## 7. Out of Scope

- Changing Flow/Workflow YOLO lock (BUG-299).
- Auto-approving `ask_user`, elicitation, or regression gates.
- Desktop Settings YOLO checkboxes.
- Load-earlier windowing (Task-290).
- Changing TUI `/yolo` to wait for Grok posture before flipping local state (would break existing Grok `/yolo` tests).

## 8. Completion Notes

- result: TUI does not show an approval card under YOLO=on; Grok `--yolo` uses existing posture endpoint; `startTurn` / Desktop launch SSOT unchanged.
- follow-ups: Optional Desktop-parity wait on Grok `/yolo` once legacy TUI tests are allowed to change.
- upstream docs updated: CP-56-Test-Steps A2.10–A2.14; CA-476.
- verification: `go test ./internal/runner/ -run 'TestChatYolo|TestResolveTurnYolo'` and `go test ./internal/tui/app/ -run 'TestChatYolo'` green.
