# BUG-324: Grok mid-chat `/model` ignored the new model or wiped ACP history

## Metadata

- Document ID: `BUG-324`
- Title: `Grok mid-chat model switch either ignored --model after session/load or (rejected path) started session/new and dropped history`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-13`
- Last Updated: `2026-08-13`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP](../../07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- Child Documents: `none`
- Related Documents: [Task-206](../../08-Task/done/Task-206-Grok-ACP-Transport-And-Process-Dispatcher.md), [Task-207](../../08-Task/done/Task-207-Grok-Controlled-Adapter-MVP.md), [Task-210](../../08-Task/done/Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md), [BUG-295](BUG-295-Claude-Session-Rotation-Loses-Turn-History-On-Replay.md), [CA-312](../../../change-audit/CA-312-grok-cross-account-session-relocate.md), [CA-445](../../../change-audit/CA-445-repair-incomplete-tui-bugfixes.md), [CA-471](../../../change-audit/CA-471-revert-grok-model-new-session.md), [CA-472](../../../change-audit/CA-472-grok-session-set-model.md), [CA-473](../../../change-audit/CA-473-grok-run-scoped-session-index.md)
- Replaces: `none`
- Tags: `ai-providers, grok, acp, session-set-model, midchat-model, regression, severity-high`

## AI Quick View

### Summary

- Desktop and TUI both send turn-level `Model` on the shared runner. Claude honors it per spawn (`--model`); Codex honors it on `thread/start` / `thread/resume`. Grok kept the ACP session via `session/load` (history) but never told the live session to switch model, so `/model` looked like a no-op.
- An earlier unshipped attempt (CA-452, reverted in CA-471) dropped `ProviderSessionID` so `ensureSession` called `session/new`. That switched model by wiping history. Wrong.
- Grok CLI **1.0.3** implements ACP `session/set_model` (`sessionId` + `modelId`). Live probe 2026-08-13: same session id, codeword recall after switch, `_meta.modelId` updated. The 0.2.93 fixture in `testdata/grok_acp/` predates this method.

### Current Ask

- After `session/new` or `session/load`, when `TurnRequest.ModelName` is non-empty, call `session/set_model` on that session, then `session/prompt`. Keep the same ACP id across `/model`, `/reasoning`, and YOLO process respawns (`session/load`, never `session/new`). Do not add model fields to `session/new` (CA-445). Do not drop resume ids (CA-471). Apply in the runner so Desktop and TUI both get it.

### Key Decisions

- `D-1` Call `session/set_model` from `grokAdapter.SendTurn` after `ensureSession`. Empty / whitespace `ModelName` is a no-op so existing SendTurn tests never hit the RPC.
- `D-2` Degrade on RPC error (method not found on Grok &lt; 1.0.3) and on `_meta.model.Err`: log and still `session/prompt`. Do not fail the turn.
- `D-3` Leave `grokProcessKey` including model (legacy tests require respawn on `--model` change). After respawn, `session/load` the same id then `session/set_model`.
- `D-4` Case 3 per-provider. Claude/Codex adapters are unchanged. InteractiveService still promotes real ids for Grok/Codex and keeps Claude's synthetic pool key (BUG-295).

### Constraints

- additive-tests-only: do not edit pre-existing tests.
- CA-445: `session/new` stays `cwd` + `mcpServers` only.
- CA-312 / Task-210: `RelocateSessionFile` + `session/load` for account switch stays intact.
- GitNexus MCP was unavailable in this thread; edits proceeded after local caller inspection of `SendTurn` / `ensureSession`.

### Open Questions

- `Q-1` Does `session/set_model` after `session/load` into a process spawned with a *different* `--model` always stick on every 1.0.3 patch? Unit tests fake the RPC; live probe was same-process `set_model` only.
- `Q-2` Should a later change drop model from `grokProcessKey` so mid-chat `/model` reuses one process? That would require operator approval to update `grok_registry_test.go`.

### Source Refs

- Live probe 2026-08-13: `grok 1.0.3` at `C:\Users\dat.nguyen\.grok\bin\grok.exe`. Sequence: `session/new` (`currentModelId=grok-4.5`) → `session/prompt` codeword `FLOWPILOT_PROBE_CODEWORD_BANANA_917` → `session/set_model` `{sessionId, modelId: grok-4.6}` result `{_meta.model.Ok: grok-4.6}` + `sessionUpdate: model_changed` same sessionId → next `session/prompt` recalled the codeword with `_meta.modelId=grok-4.6`.
- Code: `apps/local-runner/internal/runner/grok_adapter.go` (`SendTurn`, `applyGrokSessionModel`), `grok_acp.go` (`grokACPSessionSetModelParams`), `grok_process.go` (comment only).
- Tests: `apps/local-runner/internal/runner/bug324_grok_session_set_model_test.go`.

## 1. Issue Summary

In a Grok chat, changing model mid-conversation (Desktop model picker or TUI `/model`) did not apply the new model on the next prompt while keeping provider history. The runner already forwarded `ModelName` every turn. Grok's adapter resumed the ACP session (`session/load`) and prompted, but never issued a session-level model switch. Launch-time `--model` only applied to a *new* process; loading an existing session kept the session's previous model. A rejected alternative (`session/new` after dropping the resume id) would have applied the new launch flag by discarding history.

## 2. Parent Links

- impacted coding plan: [CP-46](../../07-Coding-Plan/done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md) (ACP `session/new` / `session/load` / `session/prompt`; Task-207 `SendTurn` honors `ModelName`)
- impacted tech design: [SD-06](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- impacted system spec: [SS-05](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)

## 3. Environment and Reproduction

- environment: FlowPilot local-runner Grok ACP (`grok agent stdio`), Desktop chat and TUI `flowpilot chat`; Grok CLI 1.0.3
- reproduction steps: start a Grok chat on grok-4.5; send a prompt that creates recallable context; switch model to grok-4.6; send a follow-up that needs that context and the new model
- frequency: every mid-chat Grok model change

## 4. Expected vs Actual

- expected: next turn uses the selected model on the **same** ACP session; history remains; Desktop and TUI behave the same because both use `/client/.../turns`
- actual: session id stayed (history path) but the model did not switch; or, on the reverted CA-452 path, a new session started and history disappeared

## 5. Impact

- users affected: anyone using Grok in Desktop or TUI who changes model mid-chat
- workflows affected: normal chat follow-ups; same runner path as flow children that inherit a sticky per-turn model
- severity: high (wrong model or lost context)

## 6. Root Cause

- hypothesis: Grok ACP had no session-level model RPC, so FlowPilot could only respawn `grok agent --model` (comment in `ensureGrokProcess` from the 0.2.93 era).
- confirmed cause: Grok 1.0.3 added `session/set_model`. FlowPilot never called it. `SendTurn` ignored `req.ModelName` for ACP. Dropping the resume id (CA-452) was the wrong fix because `session/new` wipes history.
- evidence: live 1.0.3 probe (Source Refs); `SendTurn` had no `set_model` call before this fix; `grokACPSessionNewParams` has no model field by design (CA-445); Claude uses `--model` per spawn; Codex passes model on `thread/start` and `thread/resume`.

## 7. Fix Strategy

- `F-1` Add `grokACPSessionSetModelParams(sessionId, modelId)` in `grok_acp.go`. Do not add model to `session/new`.
- `F-2` After `ensureSession`, `applyGrokSessionModel` when `ModelName` is non-empty. Degrade on RPC / `Err` and continue the prompt.
- `F-3` Update the stale `ensureGrokProcess` comment. Do **not** change `grokProcessKey`.
- `F-4` Keep Claude's synthetic pool key (BUG-295 / CA-471). Grok-only: `turnResumeProviderSessionID` falls back to `lastGrokTurnSessionID` when `providerSessionID` is still `thread-*` and `realProviderSessionID` is empty so a `--model` respawn `session/load`s the ACP id from the previous turn (run-92955).
- `F-5` Drop `isReplay` Grok `session/update` frames in the mapper so `session/load` replay does not emit live tool/message events.
- `F-6` TUI: replace the `thinking…` placeholder even when it is not the last message (tools after it) and even when a prior turn already has assistant text.
- `F-7` run-93161: `session/set_model` after `session/new` does not keep history. Keep a runner-wide runID→ACP id map across `grokProcessKey` respawns (`/model`, `/reasoning`, YOLO). `ensureSession` `session/load`s that id when `TurnRequest` still has `thread-*`. Promote in-memory Grok `providerSessionID` off `thread-*` after the first successful turn.

## 8. Validation

- `V-1` New tests in `bug324_grok_session_set_model_test.go`: resume → `session/load` + `set_model` + `prompt` (no `session/new`); empty/whitespace ModelName skips `set_model`; synthetic `thread-*` uses `session/new` then `set_model`; `session/new` omits model/modelId; method-not-found and `_meta.model.Err` still prompt.
- `V-2` `TestChatModeModelChangeKeepsProviderSessionID`: Grok and Codex keep the real resume id when model changes; Claude keeps the synthetic pool key. Additional: lastGrokTurnSessionID fallback when real is empty; replay updates dropped; TUI thinking placeholder replaced behind tool rows.
- `V-3` Related old tests run green (untouched): `TestGrokAdapterSendTurnStreamsAndCompletes`, `TestGrokAdapterSyntheticThreadIDUsesSessionNewNotLoad`, `TestChatModeTurnLevelControlsOverrideRunDefaults`, `TestBug295RunTurn*`.
- `V-4` Live `session/set_model` probe was run in the prior research turn (2026-08-13), not re-run after this code change. run-92955 log confirmed `session/new` on model change + thinking hang; this follow-up targets that.
- `V-5` Live run-93161 (2026-08-13): thinking hang gone; model 4.6 reported via `--model` on a new process; history-check turn `session/new` `019ffa14` instead of `session/load` `019ffa13`. Dispatch envelope stayed `thread-93162`. CA-473 adds the run-scoped index so a respawn with `thread-*` still loads.

## 9. Regression Guard

- tests: `bug324_grok_session_set_model_test.go`, `bug324_thinking_placeholder_test.go` (new files only)
- alerts: none
- audit checks: [CA-472](../../../change-audit/CA-472-grok-session-set-model.md), [CA-473](../../../change-audit/CA-473-grok-run-scoped-session-index.md)

## 10. Follow-Up Document Updates

- upstream docs that must change: flag only — CP-46's 0.2.93 live narrative ("no session-level model switch") is historically true for that version; 1.0.3 added `session/set_model`. This BugFix is the delta. Do not rewrite CP-46 as if 0.2.93 had the method.
- notes left unchanged on purpose: `session/new` shape (CA-445); Grok session-dir relocate (CA-312); Claude synthetic pool key (BUG-295); TUI `/provider` mid-run still blocked ("Use /new") — out of scope.
