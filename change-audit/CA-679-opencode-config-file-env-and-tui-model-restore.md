# CA-679 — OpenCode config env points at config file + TUI restore keeps user model

## Problem

Two operator-reported OpenCode (CP-57) defects:

1. **Chat with an OpenCode model always failed.** Every turn died with
   `turn_failed: "opencode initialize: opencode acp stream closed: EOF"`.
   Live reproduction: the runner launches `opencode acp` with
   `OPENCODE_CONFIG=<home>/.config/opencode` (a DIRECTORY), but opencode
   treats `OPENCODE_CONFIG` as a config FILE path. The process died at boot
   with `BadResource: FileSystem.readFile (/Users/tiendat/.config/opencode)`.
   The same directory value also crashed `opencode models` probes — the live
   model catalog was only surviving via the CA-657 disk cache.
2. **Selected OpenCode model lost after restarting the TUI.** Restart restore
   re-applies the runner's active chat posture profile, whose pinned
   `model: grok-4.5` overwrote the user's `/model opencode/...` choice AND
   re-persisted it to `tui-session.json` via `restoreChatPostureProfile` →
   `persistSessionPrefs`. Same clobber class as CA-638 (reasoning), unapplied
   to model. A pinned provider also reset the model through
   `setPostureProvider` (provider's first model) even when the provider did
   not change.

## Fix (env, `ai-providers`)

- New helper `opencodeConfigFilePath(homePath)` (opencode_auth_paths.go):
  returns `<home>/.config/opencode/opencode.json` via `filepath.Join`
  (OS-correct separators on Windows and POSIX); empty home → "".
- All `OPENCODE_CONFIG` writers now point at the FILE:
  - `provider_registry.go` opencode adapter factory (account + env HOME branches)
  - `opencode_process.go` `opencodeProcessEnv` fallback
  - `opencode_account.go` probe env (extracted to `opencodeAccountCommandEnv`)
  - `runner.go` `getEnvForExecution` opencode branch
  - `runner.go` `providerEnvSetCommand` posix (`.../.config/opencode/opencode.json`)
    and windows (`...\opencode\opencode.json`) shell exports
- `discoverOpencodeAccountHomes` now accepts BOTH value styles: file
  (`.../opencode/opencode.json`) and legacy directory (`.../opencode`,
  `.../.config`) — operator shells carrying the old form keep working.
  Separators normalized with `filepath.ToSlash`/`FromSlash`.
- Verified live (1.18.18): `OPENCODE_CONFIG=<file>` (existing or missing) →
  `opencode acp` initialize + `opencode models` OK; `<dir>` → both crash.
- Full E2E after fix: chat run with `providerKey=opencode` streamed
  `message_delta` → `token_usage_updated` (ctx 1,048,576) → `turn_completed`,
  real `ses_*` persisted via `ProviderSessionStore`.

## Fix (TUI posture restore keeps user model)

- `postureModelPinWins` helper: resume-flavored posture applications (restart
  restore, `/new` re-applying the already-active posture — the CA-641
  `keepReasoning` semantics) keep the user's persisted `/model` choice. The
  pin wins only when the profile switches to a DIFFERENT provider (the
  user's model is invalid there) or the session has no model yet.
- `restoreChatPostureProfile`: skips `setPostureProvider` when the pinned
  provider equals the current one (it would reset the model through the back
  door) and adopts the pin only on a real provider switch or empty model.
- Real posture switches (`/mode plan`) and provider-changing re-applies still
  pin provider+model exactly as before (old parity guarded by tests).

## Tests (additive)

- `internal/runner/ca679_opencode_config_env_test.go` — helper path join,
  process env sets file not dir, caller-supplied value preserved, discovery
  accepts file+legacy dir, `getEnvForExecution` opencode file config with
  codex/grok branches asserted unchanged (parity), posix/windows shell
  exports, account probe env.
- `internal/tui/app/ca679_model_restore_keeps_choice_test.go` — restore keeps
  user model (same provider), prefs file not clobbered, pin adopted when
  model empty, pin wins on provider switch, `/new` re-apply keeps user model,
  `/new` re-apply pins when profile switched provider, real switch still pins.

## Baseline note

`go test ./internal/runner/` and `./internal/tui/app/` contain pre-existing
failures from uncommitted in-progress work on this branch (verified identical
failure sets with the CA-679 changes stashed: 9 runner + 7 tui render tests).
No old test was edited; CA-679 adds failures to none of them.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: OpenCode OPENCODE_CONFIG file-path fix (acp/models boot crash) and TUI posture restore keeps user model choice
# --->8---
