# Task-291: Chat YOLO On Must Not Prompt Ordinary File Write

## Metadata

- Document ID: `Task-291`
- Title: `Chat YOLO On Must Not Prompt Ordinary File Write`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-13`
- Last Updated: `2026-08-13`
- Feature Keys: `yolo-policy, cli-tui, mcp-tools`
- Parent Documents: [SS-08 Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md), [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-2 YOLO controls), [Task-280](./Task-280-TUI-Session-Controls-Provider-Model-Reasoning-Yolo.md), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md)
- Child Documents: `none`
- Related Documents: [Task-286](./Task-286-TUI-Approval-And-Question-Gates.md), [CP-46](../../07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md) (Grok `session/request_permission` + YOLO), [BUG-296](../../09-BugFix/done/BUG-296-Claude-Flow-Coding-Child-Yolo-Write-Denied-No-Prompt-Tool.md), [BUG-299](../../09-BugFix/done/BUG-299-Flow-Mode-Allowed-YOLO-Off-Causing-Missed-Approvals-And-Stalls.md)
- Replaces: `None`
- Tags: `yolo-policy, cli-tui, approval, file-write`

## AI Quick View

### Summary

- Operator in normal chat: YOLO=on, asked the model to write `test.txt` with content `test`, still got a permission prompt.
- SS-08: YOLO bypasses approval gates. Chat-mode ordinary workspace writes must auto-run when YOLO is on.
- Root cause is not confirmed. Inspect TUI turn `yoloMode`, Grok session YOLO map / `--always-approve`, and Claude/Codex posture before changing code.

### Current Ask

Capture the slice. Implement only when picked up: reproduce, classify provider path, then fix so YOLO=on does not ask for this write.

### Key Decisions

- `T-1` Scope is normal chat, YOLO=on, ordinary workspace file write (`test.txt` / content `test`). Not flow-mode (already YOLO-forced, BUG-299).
- `T-2` Case 3 (per-provider): verify Claude, Codex, and Grok. One provider green is not enough.
- `T-3` Keep YOLO=off gating, `ask_user` questions, and the git-commit denylist (`ForceShellBridge` / V9-21).
- `T-4` Do not auto-answer `ask_user` when fixing write permissions.
- `T-5` Additive tests only; do not edit the legacy suite to hide a remaining prompt.

### Constraints

- SS-08 YOLO meaning stays: bypass approval gates, not skip human questions.
- CP-56 D-1: if the defect is TUI not sending `yoloMode`, fix the client. If the defect is adapter/bridge auto-approve, runner edits are in scope for this task (it is not a chrome-only CP-56 slice).
- GitNexus impact before editing `handleInbound`, `resolveYoloPosture`, `cmdGrokYoloPosture`, or turn send helpers when those tools are available. This capture thread had no GitNexus MCP.

### Open Questions

- `Q-1` Which provider was live (statusline)? Capture `yoloMode` on the turn JSON and, for Grok, whether `grokProcessKey` included `alwaysApprove=true`.
- `Q-2` TUI `/yolo` flips local `m.yolo` before `POST /provider-accounts/grok-yolo-posture` returns (Task-280 T-5 wanted success-first). Could the next prompt race a process still launched without `--always-approve`?
- `Q-3` Grok `yoloModes[sessionID]` is set per `SendTurn` and deleted when the turn ends. Confirm inbound `session/request_permission` still sees `true` for this write.
- `Q-4` Was the prompt an MCP/Drive write vs local `test.txt`? Local workspace write is the acceptance case.

### Source Refs

- SS-08 §3; CP-56 D-5, D-15, P-2; Task-280 T-1/T-5; `yolo_resolver.go` `RunnerAutoApprove`; Grok `grokAdapter.handleInbound`; TUI `turn_stream.go` `YoloMode`.

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

- `T-1` Reproduce with YOLO=on in chat: write `test.txt` / `test`. Record provider, turn body `yoloMode`, whether a `permission_required` event or TUI gate appeared, and whether the file was written.
- `T-2` If TUI local YOLO and HTTP `yoloMode` disagree, send a non-nil `yoloMode: true` on every chat turn (Task-280 T-1) and wait for Grok posture success before advertising ON.
- `T-3` If the runner received YOLO=true but still asked: fix the provider path that still calls `RequestApproval` for ordinary file write (Grok `yoloModes` / process `--always-approve`, Claude permission mode, Codex approval never). Auto-approve at the bridge remains the SSOT for YOLO=on (`interactive_service` `RunnerAutoApprove`).
- `T-4` Matrix tests (new file): YOLO=on write does not emit a blocking approval for Claude, Codex, and Grok fakes; YOLO=off still gates write; `ask_user` still prompts under YOLO=on.
- `T-5` Manual: same `test.txt` prompt in TUI with YOLO on the statusline.

## 5. Touched Areas

- files (inspect first; edit only the failing layer):
  - `apps/local-runner/internal/tui/app/app.go` (`/yolo`, `cmdGrokYoloPosture`)
  - `apps/local-runner/internal/tui/app/turn_stream.go` / `helpers.go` (`YoloMode` on `TurnInput`)
  - `apps/local-runner/internal/runner/yolo_resolver.go`
  - `apps/local-runner/internal/runner/interactive_service.go` (auto-approve when `RunnerAutoApprove`)
  - `apps/local-runner/internal/runner/grok_adapter.go` (`yoloModes`, `handleInbound`)
  - `apps/local-runner/internal/runner/grok_process.go` (`--always-approve`)
  - Claude/Codex adapter permission wiring if those providers reproduce
- modules: `yolo-policy`, `cli-tui`, `ai-providers`
- routes: existing `POST .../turns`, `POST /provider-accounts/grok-yolo-posture`, approval APIs
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

## 8. Completion Notes

- result: Captured only. Not implemented in this turn.
- follow-ups: If live logs show a leaked runner binary that never received YOLO, treat that as CA-474 class process hygiene, not a product YOLO exception.
- upstream docs updated: CP-56 Child Documents / Task Cut pointer only; SS-08 unchanged.
- verification: none run (no code in this turn). Inspected TUI `/yolo` + `TurnInput.YoloMode`, `resolveYoloPosture`, Grok `handleInbound` `yoloModes[sessionID]`.
