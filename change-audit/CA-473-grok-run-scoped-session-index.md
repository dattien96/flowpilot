---
id: CA-473
feature_key: ai-providers
title: Grok run-scoped ACP session index keeps history across model/effort/YOLO respawn
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: ai-providers
prior_ca: CA-472-grok-session-set-model
will_not_undo: CA-445 Grok session/new cwd+mcpServers only; CA-312/Task-210 RelocateSessionFile + session/load; CA-471 do not drop ProviderSessionID; CA-472 session/set_model; BUG-295 Claude synthetic pool key
```

## Change

Live `run-93161` (2026-08-13): thinking hang was already fixed; `/model` grok-4.6 reported correctly via launch `--model`; history still dropped.

ACP log for the history-check turn (`turn-93200`): **`session/new`** `019ffa14-…`, not `session/load` of turn-1 id `019ffa13-…`. No `session/set_model`. Dispatch envelope still had `provider_session_id_at_prepare=thread-93162`. Same-model follow-up (`turn-93180`) had `session/load` of `019ffa13` because that adapter instance still existed.

`grokProcessKey` includes model + reasoningEffort + alwaysApprove. A mid-chat `/model`, `/reasoning`, or YOLO flip respawns `grok agent` and a **new** `grokAdapter` with empty `lastSessionID`. `SendTurn` also clears `lastSessionID` at start. `ensureSession` then saw synthetic `thread-*` and called `session/new`.

Fix: runner-wide `grokRunSessions` map (FlowPilot run id → ACP session id), wired into every process spawned by `ensureGrokProcess`. `ensureSession` loads that id when `TurnRequest.ProviderSessionID` is empty/`thread-*`. Keyed by run id so parent/child chats do not steal (CA-312 / run-536). After a successful Grok turn, promote in-memory `providerSessionID` from `thread-*` to the real ACP id (Claude pool key unchanged). Reconstruct also seeds `lastGrokTurnSessionID`.

`session/set_model` still runs after load when `ModelName` is set. `grokProcessKey` is unchanged (legacy respawn tests).

## Provider impact

Case 3 (per-provider). Grok adapter + Grok refresh/reconstruct only. Claude still `--model` per spawn and keeps the synthetic pool key (BUG-295). Codex still prefers `realProviderSessionID` / rollout files; no grok run-map.

## Tests

New cases in `bug324_grok_session_set_model_test.go` only: run-scoped index loads after a fake process respawn with `thread-*` still on the request; sibling run ids do not steal; `grokEnsureResumeID` prefers a real request id over the map; `/reasoning` and YOLO `runTurn` keep the ACP resume id; refresh promotes Grok `thread-*` to the real id; adapter-level effort/YOLO respawn still `session/load`; index ignores empty/`thread-*`; adopted prompt `sessionId` updates the map; reconstruct seeds `lastGrokTurnSessionID`; `grokProcessKey` still differs for model/effort/YOLO; Claude keeps the synthetic pool key and Codex ignores `lastGrokTurnSessionID`.

Related old tests (untouched) green: `TestGrokAdapterSendTurnStreamsAndCompletes`, `TestGrokAdapterSyntheticThreadIDUsesSessionNewNotLoad`, `TestRefreshResumeHandleGrokPersistsRealProviderSessionID`, `TestChatModeModelChangeKeepsProviderSessionID`.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-324
change_type: bugfix
summary: Grok keeps ACP history across model/reasoning/YOLO process respawn via run-scoped session/load index
# --->8---
