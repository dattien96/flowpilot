---
name: BUG-087-Codex-Resume-Turns-Bypass-Approval-MCP-And-AskUser
description: Codex follow-up and cross-account resumed turns use the CLI resume adapter, which bypasses FlowPilot's app-server approval/question bridge for YOLO-off command approvals, Google Drive MCP confirmations, and ask_user question UI.
metadata:
  type: bugfix
---

## Metadata

- Document ID: `BUG-087`
- Title: Codex Resume Turns Bypass Approval MCP And AskUser
- Phase: `bugfix`
- Status: `done`
- Owner: DatNguyen
- Reviewers: —
- Created: 2026-06-19
- Last Updated: 2026-06-19
- Parent Documents: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: —
- Related Documents: [BUG-070: Codex MCP Permission Approval Request Unsupported](../done/BUG-070-Codex-MCP-Permission-Approval-Request-Unsupported.md), [BUG-071: Codex YOLO-Off Workspace Write And Selected Skill Names Regressed](../done/BUG-071-Codex-Yolo-Off-Workspace-Write-And-Selected-Skill-Names-Regressed.md), [BUG-072: Codex MCP Elicitation Approval Unsupported](../done/BUG-072-Codex-MCP-Elicitation-Approval-Unsupported.md), [BUG-083: Desktop Chat Resume Replays Composed Prompt Not User Input](../done/BUG-083-Desktop-Chat-Resume-Replays-Composed-Prompt-Not-User-Input.md), [BUG-084: Codex History Chat Open Fails After Switching Active Account](../done/BUG-084-Codex-History-Chat-Open-Fails-After-Switching-Active-Account.md), [BUG-085: Codex Live Chat Splits Provider Session On Account Switch](../done/BUG-085-Codex-Live-Chat-Splits-Provider-Session-On-Account-Switch.md), [BUG-086: Delete Chat Leaves Codex Cross-Account Rollouts On Disk](../done/BUG-086-Delete-Chat-Leaves-Codex-Cross-Account-Rollouts-On-Disk.md)
- Replaces: —
- Tags: desktop, codex, approval, mcp, ask-user, yolo, cross-account-resume, local-runner, regression, severity-high

## AI Quick View

### Summary

- Symptom: with YOLO off, a fresh Codex chat shows command approval on the first file-write prompt, but later Google Drive MCP and `ask_user` prompts fail without FlowPilot confirmation/question UI.
- Symptom: after SD-14 cross-account chat continuation, the same three probes can all run through the resumed path, so even the file-write prompt may bypass approval and write directly.
- Confirmed root cause: once a Codex chat has a real rollout id, `startTurn` replaces the app-server adapter with `codexResumeAdapter`, which shells out to `codex exec resume` and does not have the app-server inbound approval/question bridge.
- Clarification: SD-14 resume behavior itself is still correct and should remain. The regression is not "resume is wrong"; it is that the resumed execution path lost the interactive approval/question bridge. Extra metadata alone is therefore not sufficient unless it unlocks a bridge-capable resumed runtime.
- Implemented result: resumed Codex turns now stay on the app-server bridge when the live `codexAdapter` is available, using `thread/resume` with the stable rollout id so command approvals, MCP approvals/elicitation, and `ask_user` all keep working.
- Follow-up fix: the turn is no longer considered ready for the next prompt until post-turn rollout discovery/logging/sync completes, preventing A -> B -> A cross-account turns from missing the newest sidecar rollout during the next relocation.

### Resolution

- Done in runner code and covered by focused plus full runner tests.

### Key Decisions

- `V-1` YOLO=false must gate command/file writes, MCP permission requests, MCP elicitations, and `ask_user` questions on every Codex turn: first turn, same-account follow-up, reopened history turn, and A -> B -> A cross-account turn.
- `V-2` The fix must not regress SD-14: stable `provider_session_id` remains the durable Codex resume handle, per-turn rollout ids remain sidecar replay/relocation inputs, and active-account relocation still prepares files before the turn.
- `V-3` The fix must not regress prior approval fixes from BUG-070, BUG-071, and BUG-072; those response shapes and YOLO mappings remain the contract.
- `V-4` Prefer one bridge-capable Codex resumed-turn path over duplicating partial approval behavior in multiple adapters.
- `V-5` Do not frame the implementation as a metadata-only fix. Metadata already supports SD-14 resume semantics; the missing piece is interactive bridging on resumed turns.

### Constraints

- Important: this bug fix must not make a regression in the approval feature or in the SD-14 cross-account chat sync/resume feature.
- Do not turn YOLO=false into auto-approval. YOLO=false stays `workspace-write` plus `untrusted`.
- Do not turn YOLO=true into manual gating. YOLO=true stays `danger-full-access` plus `never`.
- Do not replace the stable persisted Codex rollout id with later per-turn rollout ids.
- Do not drop SD-14 account-home relocation, same-home rebinding, stale-destination rollout update, or post-turn stable rollout sync.
- Do not treat this as a desktop rendering-only bug; the UI can only show cards when the runner emits `permission_required` or `user_question_required`.

### Open Questions

- `Q-1` Resolved for this bugfix: the live app-server path can resume by rollout id through `thread/resume`, and the existing inbound approval/question bridge remains intact on that path.
- `Q-2` The CLI `codex exec resume` path remains as a fallback only for non-app-server environments; BUG-087 does not extend it into a structured approval/question bridge.

### Source Refs

- User report on 2026-06-19 with screenshots:
  - Image 1: fresh simple chat, first command approval UI appears, later Google Drive MCP and `ask_user` fail.
  - Image 2: cross-account/synced chat, command write, Google Drive MCP, and `ask_user` all fail to show confirmation/question UI.
- Local history evidence:
  - `.flowpilot/chats/sessions.ndjson` lines for `run-24`: fresh run starts as `thread-25`, then persists rollout `019edcc9-66e5-7e42-9c6e-6f836cbf7864`; later MCP and `ask_user` prompts run after that real rollout id exists.
  - `.flowpilot/chats/sessions.ndjson` lines for `run-1`: account-switch chat already has stable rollout `019edcad-b66e-75a3-b4ce-0fe86635c580` before the three probe prompts, so all three probes use the resumed path.
- Code refs:
  - `apps/local-runner/internal/runner/interactive_service.go` -> `startTurn` adapter switch to `newCodexResumeAdapter`
  - `apps/local-runner/internal/runner/codex_resume_process.go` -> CLI resume adapter lacks inbound bridge
  - `apps/local-runner/internal/runner/codex_adapter.go` -> app-server `handleInbound`, approval mapping, MCP elicitation mapping, and `ask_user` dynamic tool bridge
  - `apps/local-runner/internal/runner/codex_appserver_test.go` -> existing app-server approval/MCP/ask_user tests
  - `apps/local-runner/internal/runner/codex_resume_process_test.go` -> existing CLI resume flag/output tests
  - `apps/local-runner/internal/runner/cross_account_resume_test.go` -> existing SD-14 A -> B -> A resume tests

## 1. Issue Summary

Codex approval and user-interaction behavior regressed for follow-up/resumed chat turns. The user's three standard YOLO=false probes are:

1. `Create a file called yolo-test.txt in the current directory with the text "yolo works" and after that is all skills name i mentioned`
2. `Using mcp google drive to let me know the current google email of this drive`
3. `Use the ask_user tool to ask me which programming language I prefer: Python, TypeScript, or Go. Then write a "Hello World" in whichever I pick.`

Expected behavior:

- prompt 1 shows FlowPilot command/file approval UI before writing
- prompt 2 shows FlowPilot MCP approval/confirmation UI before Google Drive access
- prompt 3 shows FlowPilot question UI with the language choices

Actual behavior:

- in a brand-new simple chat, prompt 1 still works because it is the first app-server turn, but prompts 2 and 3 fail because the run has switched to the resumed CLI path
- in a cross-account/synced chat, all three prompts can fail because the chat already has a stable Codex rollout id before the probes start

## 2. Parent Links

- impacted tech design: [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md)
- impacted tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- impacted system spec: [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot, local runner, Codex provider, YOLO=false, Codex app-server enabled for fresh turns, Google Drive MCP configured, at least one connected Codex account
- reproduction steps for Image 1:
  1. Start a new simple Codex chat with YOLO=false.
  2. Send probe 1.
  3. Approve the command/file approval card.
  4. Send probe 2.
  5. Observe MCP call failure/cancellation instead of a FlowPilot approval card.
  6. Send probe 3.
  7. Observe `ask_user` failure/fallback text instead of a FlowPilot question card.
- reproduction steps for Image 2:
  1. Start or reopen one Codex chat that already has a real stable rollout id.
  2. Switch active Codex accounts as in SD-14 A -> B -> A coverage, or use a synced/cross-account chat whose provider session has already been rebound.
  3. Send probes 1, 2, and 3 with YOLO=false.
  4. Observe prompt 1 can write directly without approval, and prompts 2/3 fail to show MCP/question UI.
- frequency: deterministic when the turn uses `codexResumeAdapter` instead of the app-server `codexAdapter`

## 4. Expected vs Actual

- expected:
  - every YOLO=false Codex turn, fresh or resumed, uses the same FlowPilot approval semantics
  - command/file writes emit `permission_required`
  - Google Drive MCP permission/elicitation emits `permission_required`
  - `ask_user` emits `user_question_required`
  - SD-14 stable-session resume and account-home relocation still work
- actual:
  - fresh first turn can still show approval because app-server `codexAdapter` handles inbound requests
  - follow-up/resumed turns use `codexResumeAdapter`, which has no inbound request handler and only captures final CLI output
  - missing bridge prevents the desktop timeline from receiving approval/question events

## 5. Impact

- users affected: desktop Codex chat users, especially users with YOLO=false, Google Drive MCP, selected skills, and multiple Codex accounts
- workflows affected:
  - safe file-writing approval flow
  - Google Drive MCP access/confirmation
  - model-driven `ask_user` language/choice questions
  - SD-14 cross-account chat continuation validation
- severity: High — the safe non-YOLO approval promise is broken on resumed turns, and the cross-account sync feature can expose the broken path on the first visible probe

## 6. Root Cause

- hypothesis:
  - SD-14 cross-account work caused account-switched chats to use a different Codex runtime path that does not support approval/question UI.
- confirmed cause:
  1. A fresh Codex chat starts with a synthetic thread id such as `thread-25`.
  2. The first successful Codex turn discovers a real rollout id and persists it as the stable `provider_session_id`.
  3. On later turns, `startTurn` checks `rs.providerKey == ProviderKeyCodex && rs.realProviderSessionID != "" && !strings.HasPrefix(rs.realProviderSessionID, "thread-")`.
  4. That condition replaces the app-server adapter with `newCodexResumeAdapter(...)`.
  5. `codexResumeAdapter.SendTurn` shells out to `codex exec resume <sessionId> --all ...`, captures stdout/stderr/output file, and emits only `message_completed` and `turn_completed`.
  6. The CLI resume adapter never registers app-server dynamic tools, never calls `bridge.RequestApproval`, never calls `bridge.AskQuestion`, and never maps inbound `item/permissions/requestApproval`, `mcpServer/elicitation/request`, or `item/tool/call`.
  7. Therefore the desktop cannot show approval/question UI because no runner event exists to render.
- clarification:
  - SD-14 already provides the important resume metadata and file-portability behavior:
    - stable persisted rollout id
    - account-home rebinding
    - sidecar rollout ids for replay and relocation
  - The regression is that follow-up turns execute through a runtime path that cannot surface FlowPilot interactive events.
  - Because of that, attaching more metadata by itself is unlikely to fix BUG-087 unless it is paired with a bridge-capable resumed-turn runtime.
- why Image 1 is affected:
  - prompt 1 is fresh and app-server-backed, so approval works
  - after prompt 1, the run has a real rollout id, so prompts 2 and 3 are resumed CLI turns and lose the bridge
- why Image 2 is affected:
  - the chat already had a real rollout id due to prior SD-14 cross-account/sync continuation
  - all three probes used the resumed CLI path, so command approval, MCP approval, and `ask_user` could all fail
- evidence:
  - existing app-server tests cover command/file approval, MCP permission approval, MCP elicitation approval, and `ask_user` dynamic tool round trips
  - existing resume adapter tests only assert CLI args and final-output mapping
  - no existing test proves a resumed Codex turn can still emit FlowPilot approval/question events

## 7. Fix Strategy

- `F-1` Add a failing regression test first for a same-account follow-up Codex turn: first turn persists a real rollout id, second YOLO=false file-write turn must call `bridge.RequestApproval` and emit `permission_required`.
- `F-2` Add failing regression tests for resumed MCP permission and MCP elicitation: resumed YOLO=false turn must route `item/permissions/requestApproval` and `mcpServer/elicitation/request` through the same response builders used by app-server turns.
- `F-3` Add a failing regression test for resumed `ask_user`: resumed turn must route `item/tool/call` for `tool:"ask_user"` to `bridge.AskQuestion` and produce `user_question_required`.
- `F-3A` Keep SD-14 as the session-portability model:
  - same stable resume handle
  - same cross-account relocation behavior
  - same sidecar rollout tracking
  - no rollback of SD-14 file-sync/resume semantics
- `F-4` Implement a bridge-capable Codex resumed-turn path. Preferred implementation: use app-server resume/continue support if available, preserving `codexAdapter.handleInbound` for approvals/questions and passing the stable rollout id as the resume handle.
- `F-5` If app-server resume by rollout id is not available, upgrade the CLI resume path only if it can consume structured JSON events and synchronously answer approval/question requests; do not ship a partial text-only parser.
- `F-5A` Do not pursue a metadata-only fix unless investigation proves a specific missing metadata field is the only blocker to using a bridge-capable resumed path. Current evidence says the main gap is runtime capability, not session metadata.
- `F-6` Preserve SD-14 before launching the turn:
  - keep `ensureResumeReady`
  - keep `prepareCrossAccountResume`
  - keep `relocateCodexTurnLogSessions`
  - keep same-home rebinding
  - keep stale same-session destination update protection
- `F-7` Preserve post-turn SD-14 behavior:
  - keep stable `provider_session_id`
  - keep sidecar logging of newer per-turn rollout ids
  - keep `syncCodexStableSessionToKnownAccounts`
- `F-8` Keep the existing YOLO SSOT:
  - YOLO=false -> `workspace-write` and `untrusted`, no runner auto-approval
  - YOLO=true -> `danger-full-access` and `never`, runner auto-approval allowed

## 8. Validation

- `V-1` Same-account simple chat regression:
  - start a fresh Codex chat with YOLO=false
  - prompt 1 writes `yolo-test.txt`
  - prompt 2 uses Google Drive MCP
  - prompt 3 uses `ask_user`
  - expected: command approval, MCP approval, and question UI all appear in order
- `V-2` Cross-account SD-14 regression:
  - start on account A
  - complete a first Codex turn and persist stable rollout id
  - switch to account B
  - continue the same chat with a YOLO=false command/MCP/ask_user probe
  - expected: provider files relocate correctly and approval/question UI still appears
- `V-3` A -> B -> A stability:
  - continue on A, then B, then A
  - expected: all turns use the same stable resume handle, copied sidecar rollout ids remain available, and no provider session split returns
- `V-4` YOLO=false command/file approval test:
  - resumed turn must produce `permission_required`
  - approval must unblock the turn
  - denial must not write the file
- `V-5` YOLO=false MCP permission test:
  - resumed turn must route `item/permissions/requestApproval`
  - approve response must be `{ permissions, scope }`
  - deny/expiry response must be empty permissions with `scope:"turn"`
- `V-6` YOLO=false MCP elicitation test:
  - resumed turn must route `mcpServer/elicitation/request`
  - approve maps to `action:"accept"`
  - deny maps to `action:"decline"`
  - cancel/abort maps to `action:"cancel"`
- `V-7` YOLO=false `ask_user` test:
  - resumed turn must route `item/tool/call` for `ask_user`
  - FlowPilot emits `user_question_required`
  - selected answer is returned to Codex as the dynamic tool result
- `V-8` YOLO=true non-regression:
  - command/MCP approval prompts must not appear
  - `ask_user` availability must follow the product's existing YOLO=true behavior and must not regress from the app-server path
- `V-9` Existing focused tests remain green:
  - `go test ./internal/runner -run 'TestCodex(AdapterAskUserDynamicToolRoundTrip|AdapterMcpElicitationApprovalRoundTrip|AdapterPermissionsApprovalRoundTrip|AdapterYoloApprovalMatrix|ResumeCommandUsesYoloDerivedSandboxAndApproval|FreshRunUsesAppServerPath|LiveCodexChatResumesAcrossProviderAccountSwitches)' -count=1`
- `V-10` Full runner suite remains green:
  - `go test ./internal/runner -count=1`
- actual verification on 2026-06-19:
  - `go test ./internal/runner -run 'TestCodexAdapter(ResumedTurnUsesThreadResumeForCommandApproval|ResumedTurnRoutesAskUserDynamicTool|ResumedTurnRoutesMcpApprovals)|Test(RestoredCodexRunUsesAppServerResumePathWhenAvailable|LiveCodexCrossAccountResumedTurnsKeepApprovalBridge|LiveCodexChatResumesAcrossProviderAccountSwitches|ResumeCommandUsesYoloDerivedSandboxAndApproval|RestoredCodexRunUsesCLIResumePath)' -count=1` ✅
  - `go test ./internal/runner -count=1` ✅

## 9. Regression Guard

- tests to add or update:
  - `apps/local-runner/internal/runner/codex_resume_process_test.go`
    - add coverage proving the old text-only CLI resume path is not used for interactive approval-capable chat turns, or proving it now bridges structured approval/question events if that becomes the chosen implementation
  - `apps/local-runner/internal/runner/codex_appserver_test.go`
    - add or extend app-server resume tests for command approval, MCP permission, MCP elicitation, and `ask_user`
  - `apps/local-runner/internal/runner/cross_account_resume_test.go`
    - extend A -> B -> A coverage to assert the resumed cross-account turn still routes approval/question events through the bridge
  - optional desktop reducer/store tests:
    - only if event sequencing changes; otherwise the runner event contract should keep desktop behavior unchanged
- DoD checklist:
  - `DOD-1` ✅ Added resumed-turn command approval coverage in `codex_appserver_test.go`.
  - `DOD-2` ✅ Added resumed-turn MCP permission and elicitation coverage in `codex_appserver_test.go`.
  - `DOD-3` ✅ Added resumed-turn `ask_user` coverage in `codex_appserver_test.go`.
  - `DOD-4` ✅ Approval response shapes remain on the existing app-server bridge and focused tests pass.
  - `DOD-5` ✅ Stable resume id vs sidecar rollout separation is preserved; resumed turns still use the stable rollout id while new rollout ids are logged for replay/relocation.
  - `DOD-6` ✅ Cross-account relocation/post-turn sync stays green, including A -> B -> A tests.
  - `DOD-7` Manual new-chat probe not run in this coding turn.
  - `DOD-8` Manual existing cross-account probe not run in this coding turn.
  - `DOD-9` Manual YOLO=true smoke not run in this coding turn.
  - `DOD-10` ✅ Automated resumed-turn coverage shows YOLO=false interactive actions still go through approval/question events instead of bypassing them.
- alerts: none currently
- audit checks:
  - run GitNexus impact analysis before editing `startTurn`, `codexResumeAdapter.SendTurn`, `codexAdapter.SendTurn`, `codexAdapter.handleInbound`, or any app-server resume helper once GitNexus tools are available
  - run `gitnexus_detect_changes()` before commit
  - explicitly review blast radius for HIGH/CRITICAL GitNexus findings before editing

## 10. Follow-Up Document Updates

- upstream docs that must change:
  - [SD-14](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md) should be updated to note that live resumed turns now prefer app-server `thread/resume` when available, while preserving the same stable-rollout and cross-account portability contract.
  - [SS-08](../../05-System-Specs/SS-08-Approve-Gate.md) should be updated only if the approval semantics change. The intended BUG-087 fix should not change them.
- notes left unchanged on purpose:
  - This bug does not change the user-visible SD-14 expectation that one FlowPilot Codex chat can continue across accounts A -> B -> A.
  - This bug does not change selected-skill injection behavior.
  - This bug does not change Google Drive MCP auth/setup behavior.
  - This bug does not change desktop timeline rendering unless the runner event contract changes.
