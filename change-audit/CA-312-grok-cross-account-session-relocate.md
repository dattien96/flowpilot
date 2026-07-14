# CA-312: Grok cross-account session relocate + native resume

## Summary

Implement Claude/Codex-style cross-account Grok chat continuation (Task-210 Option B): promote real ACP session ids from the turn log, locate/relocate full Grok session directories into the active `GROK_HOME`, rebind `provider_account_id`, and pass the real id so the adapter calls `session/load` instead of failing with `session_unavailable` or minting a fresh session.

## What changed

- `LocateSessionFile` / `RelocateSessionFile` / `relocationTargetPath` support Grok session **directories** (`sessions/<encoded-cwd>/<id>/` with `chat_history.jsonl` required).
- Path-safe Grok session id validation (`isGrokRealSessionID`) + destination must stay under target home.
- `ensureGrokProviderResumeHandle` promotes latest `grok_session` turn-log id for legacy `thread-*` rows (run-370 shape).
- `relocateGrokTurnLogSessions` copies all real Grok session dirs referenced by the turn log during `prepareCrossAccountResume`.
- `refreshResumeHandleLocked` now sets `realProviderSessionID` for Grok; `runTurn` prefers it for all providers so follow-up turns use `session/load`.
- Delete cleanup uses `RemoveAll` for Grok session dirs and includes Grok turn-log session ids.
- New unit tests in `grok_cross_account_resume_test.go` cover locate/relocate, legacy promote+copy+load, conflict no-rebind, path traversal rejection, and same-account follow-up real id.

## Live spike

Copied `run-370` session `019f60f6-…` from `~/.grok` into `~/.grokHome2` and successfully `session/load` + prompt under account-2 auth (model answered `PONG` with prior context).

## Verification

```bash
cd apps/local-runner
go test ./internal/runner -run 'Test(LocateSessionFileGrok|RelocateSessionFileGrok|EnsureProviderResumeHandleGrok|PrepareCrossAccountResumeGrok|StartTurnGrok|RefreshResumeHandleGrok|IsGrokRealSessionID|LocateAndRelocateGrok|LocateSessionFile|RelocateSessionFile|PrepareCrossAccountResume|DiscoverCodex)' -count=1
```

## Residual

- Drive restore path still has no Grok directory packaging (out of scope for this local cross-account fix).

## Hotfix (run-536 / first-chat steal)

`refreshResumeHandleLocked` initially promoted the **newest workspace session dir** into `realProviderSessionID`. Multiple chats share one `GROK_HOME`+cwd, so a **failed first turn of a brand-new chat** could persist another chat’s ACP id (live: `run-536` wrote `019f60f2-…` and failed with `Invalid params`).

Fix: promote **only** the adapter-reported `LastGrokSessionID()` from a successful `session/new`/`session/load`. Never invent an id from disk discovery. Regression test: `TestRefreshResumeHandleGrokDoesNotStealOtherChatSession`.

Also remove `discoverGrokSessionDirs` fallback from `ensureGrokProviderResumeHandle` (Codex re-review): pre-turn single-dir discovery could still steal on resume/account mismatch.

## Hotfix (run-584 / Invalid params on first chat)

Live `run-584` kept `thread-*` (steal fixed) but still failed `session/new` with `Invalid params`. Root cause: `grokACPExtraMCPServers` sent stdio `env` as a plain string map. Grok ACP requires env as `[{name,value}]` (same as headers). Google Drive MCP always carries env → every first Grok turn failed when Drive was connected.

Fix: `grokACPNameValueList` for both env and headers. Live-probed map fails, array succeeds. Tests: `TestGrokACPExtraMCPServersStdioEnvIsNameValueArray`.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-210
change_type: feature
summary: Grok cross-account chat continues by relocating session dirs into active GROK_HOME and session/load of real ACP ids
# --->8---
