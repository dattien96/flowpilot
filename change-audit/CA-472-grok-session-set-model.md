---
id: CA-472
feature_key: ai-providers
title: Grok mid-chat model switch via ACP session/set_model
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: ai-providers
prior_ca: CA-471-revert-grok-model-new-session
will_not_undo: CA-445 Grok session/new cwd+mcpServers only; CA-312/Task-210 RelocateSessionFile + session/load; CA-471 do not drop ProviderSessionID; BUG-295 Claude synthetic pool key
```

## Change

Grok Desktop/TUI `/model` now reaches ACP `session/set_model` on the live session after `session/new` or `session/load`. History stays. `session/new` is unchanged. Missing-method / `_meta.model.Err` logs and still `session/prompt`.

`grokProcessKey` still includes model (legacy respawn tests). Adapter calls `set_model` after load so a respawned `--model` process still switches the loaded session.

run-92955 follow-up: a 4.5→4.6 respawn called `session/new` (history gone) even though the run had persisted ACP id `019ff9fb`. Grok resume id is now resolved under lock via `turnResumeProviderSessionID` (real id, else `lastGrokTurnSessionID`; Claude pool key unchanged). Mapper drops `isReplay` load-replay frames. TUI replaces `thinking…` even when tool rows sit after the placeholder or a prior assistant exists.

## Provider impact

Case 3 (per-provider). `grokAdapter.SendTurn` + Grok-only resume fallback in `turnResumeProviderSessionID` + Grok mapper `isReplay`. Claude still `--model` per spawn and keeps the synthetic pool key (BUG-295). Codex still model on `thread/start` / `thread/resume` and still prefers `realProviderSessionID` only. TUI thinking placeholder is provider-agnostic display.

## Tests

New file only: `apps/local-runner/internal/runner/bug324_grok_session_set_model_test.go`.

Related old tests (untouched) green: `TestGrokAdapterSendTurnStreamsAndCompletes`, `TestGrokAdapterSyntheticThreadIDUsesSessionNewNotLoad`, `TestChatModeTurnLevelControlsOverrideRunDefaults`, `TestBug295RunTurn*`.

`TestEnsureGrokProcessCoexistsAcrossModelsOnSameScope` failed on this Windows host (`exec: "sh": executable file not found`) — pre-existing env, not this change (comment-only in `grok_process.go`).

Live 1.0.3 `session/set_model` probe was prior-turn evidence; not re-run after the adapter patch.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: BUG-324
change_type: bugfix
summary: Grok mid-chat /model loads persisted ACP session then session/set_model; drop load-replay; TUI clears thinking on follow-up
# --->8---
