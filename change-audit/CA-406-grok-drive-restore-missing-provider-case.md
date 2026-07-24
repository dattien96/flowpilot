# CA-406: Grok Drive restore missing provider case in restoreTargetPath

## Summary

Operator reported live: after loading Grok chats into REMOTE CHATS (BUG-310 +
CA-404 fixed sync-up and listing the same day), clicking "Restore" on any
Grok chat failed with a path-related error and permanently disabled that row.
Live-reproduced via a direct `POST /client/chat-sessions/restore` call against
the running dev server (project "Gate-sandbox"):

```
409 {"error":{"code":"sync_integrity_failed","message":"remote provider session file path is invalid"}}
```

Confirmed unrelated to CA-404's index-repair logic (Drive-side manifest paths
are computed identically for every provider via `chatSessionManifestPath`).
The defect is entirely local: `restoreTargetPath` (`session_file_locator.go`)
only had `case`s for `ProviderKeyCodex` and `ProviderKeyClaude`; Grok fell
through to `default: "unsupported provider session relocation"`. This is the
exact gap [BUG-310](../requirements/09-BugFix/done/BUG-310-Grok-Drive-Sync-Always-Fails-Reading-Session-Directory-As-File.md)'s
own Follow-Up section and Task-210's own out-of-scope note both already named
and deferred -- full root cause and fix detail in
[BUG-312](../requirements/09-BugFix/done/BUG-312-Grok-Drive-Restore-Rejects-Every-Session-Missing-Provider-Case.md).

## Fix

Added a `ProviderKeyGrok` case to `restoreTargetPath` that recomputes the
destination session directory from the **local machine's own `cwd`** via
`grokSessionDirPath` (the same helper `relocationTargetPath`'s pre-existing
Grok case already uses for local cross-account relocation), instead of
reusing the source machine's embedded, percent-encoded cwd segment verbatim --
which would "succeed" but write into a directory `LocateSessionFile` could
never look up again under a different machine/cwd. Validates the relative
path ends in `chat_history.jsonl`, the session id via `isGrokRealSessionID`,
requires non-empty `cwd`, and checks `pathUnderRoot`.

No change to `resolveChatSessionTranscript`, `LocateSessionFile`,
`relocationTargetPath`, or `restoreChatRunTreeFromDrive`'s caller logic --
the bug was entirely inside `restoreTargetPath`'s own `switch`.

## Incidental fix: test helper leaked into the real machine's provider-accounts.json

While writing this bug's own end-to-end test, found that `newChatSyncService`
(`chat_session_sync_test.go`, shared by 40+ tests) only overrides `HOME`, not
`USERPROFILE`/`APPDATA`/`FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH`. On Windows,
provider-account auto-discovery and persistence both read `USERPROFILE`/
`APPDATA` directly, bypassing `HOME`. Confirmed live, twice, on this exact
dev machine: test runs corrupted a real Grok account's `id` field, then (during
a git-stash baseline comparison) flipped the `is_active` flag on two real
accounts. Both repaired by hand (recomputing the correct deterministic id via
the actual `deterministicProviderAccountID` function) and the helper fixed to
fully sandbox `USERPROFILE`/`APPDATA`/`FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH`,
matching the isolation already used by `provider_accounts_test.go` and every
Grok/cross-account test file.

## additive-tests-only compliance

New test functions only: `TestRestoreTargetPathGrokAcceptsWellFormedPathAndRejectsBadInput`
and `TestRestoreSessionFileGrokRoundTripsAcrossDifferentCwdEncoding`
(`grok_cross_account_resume_test.go`, appended), `TestRestoreChatRunFromDriveGrokRecomputesPathForDifferentCwd`
(`chat_session_sync_test.go`, appended). `newChatSyncService`'s env-sandboxing
lines are additive (new `t.Setenv` calls only); no existing test's setup or
assertions were changed.

## Verification

- `go build ./...`, `go vet ./internal/runner/`: clean.
- 3 new tests: PASS on the fix. Confirmed FAIL on the pre-fix baseline via
  `git stash` (isolated to `session_file_locator.go`), reproducing the
  identical `sync_integrity_failed` / `"unsupported provider session
  relocation"` errors as the live report.
- Targeted sweep (`TestGrok*`, `TestRestore*`, `TestChatSession*`,
  `TestCrossAccount*`, `TestSync*`, `TestListRemoteChatSessions*`,
  `TestLocateSessionFile*`, `TestRelocateSessionFile*`): clean except one
  pre-existing intermittent flake (`TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows`,
  a CA-404 concurrency test), confirmed to fail at the same rate on the clean
  baseline via `git stash`.
- Full `go test ./...`: 15 failures, all confirmed pre-existing -- 14
  reproduce identically on the clean baseline (Windows HOME/USERPROFILE env
  gaps, real `codex`/`git` CLI dependencies, a Grok account-slot machine-state
  test), and the 15th (`TestFinalizerHookSurfacesArtifacts`) is flaky even run
  alone, repeatedly, in isolation.
- Real `provider-accounts.json` checksum monitored across the full
  verification process and confirmed stable once the sandboxing fix landed.
- Live re-verification: restarted the dev stack, re-issued the exact restore
  call that previously 409'd for `run-22023` (Grok, project "Gate-sandbox") --
  now `200 restored`. Restored a second Grok run (`run-28116`) the same way.
  Confirmed on disk both landed under this machine's own cwd-encoded
  directory (`D%3A%5Cworking%5Cgate-sandbox`), byte-identical transcript
  content.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: BUG-312
change_type: bugfix
summary: restoreTargetPath now has a Grok case that recomputes the destination session directory from the local cwd instead of rejecting every Grok restore; also sandboxed a test helper that was leaking into the real machine's provider-accounts.json.
# --->8---
