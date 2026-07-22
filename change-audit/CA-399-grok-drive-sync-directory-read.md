# CA-399: Grok Drive sync no longer reads the session directory as a file

## Summary

Found live while investigating the CP-51 C6 "unsynced count stuck at 9" report
(same session as CA-398/BUG-309): after fixing BUG-309, a fresh live re-test
against the running dev server showed 9 Gate-sandbox runs still genuinely
failing to sync (not a reporting bug). Calling `POST .../sync-chat` directly
on each returned real errors, and 5 of the 9 (all Grok-provider runs) failed
identically with:

```
{"error":{"code":"workflow_state_unavailable","message":"read C:\Users\dat.nguyen\.grok\sessions\D%3A%5Cworking%5Cgate-sandbox\<id>: Incorrect function."}}
```

## Root cause

`resolveChatSessionTranscript` (`chat_session_sync.go:401`) called
`os.ReadFile` directly on whatever `LocateSessionFile` returned, assuming it
was always a single transcript file (true for Codex/Claude). For Grok,
`LocateSessionFile` (`session_file_locator.go:70`) intentionally returns the
session **directory** -- Grok stores `chat_history.jsonl` plus sidecar files
there (Task-210 Option B). Reading a directory path with `os.ReadFile` fails;
Windows surfaces this as `ERROR_INVALID_FUNCTION` ("Incorrect function"),
other platforms as `EISDIR`. This meant Drive sync had never worked for any
Grok chat, on any machine -- full detail in
[BUG-310](../requirements/09-BugFix/done/BUG-310-Grok-Drive-Sync-Always-Fails-Reading-Session-Directory-As-File.md).

## Fix

`resolveChatSessionTranscript` now joins `"chat_history.jsonl"` onto the
directory path specifically when `session.ProviderKey == ProviderKeyGrok`,
before reading. Codex/Claude are unaffected (unchanged direct read). The
uploaded/hashed bytes are the raw `chat_history.jsonl` contents, matching what
sits on disk -- not a transformed representation.

Scoped to exactly the confirmed defect: no change to `LocateSessionFile`,
`grokSessionDirPath`, or any Grok cross-account/resume code path. Grok's
restore side remains a separate, pre-existing, already-acknowledged gap
(`restoreTargetPath` only supports Codex/Claude; Task-210 itself notes "Drive
restore packaging for Grok session directories" as out of scope) -- not
addressed by this fix.

## additive-tests-only compliance

New file only: `bug310_test.go`. No existing test file touched.

## Verification

- `go build ./...`, `go vet ./internal/runner/`: clean.
- New test `TestResolveChatSessionTranscriptReadsGrokChatHistoryFile`: PASS on
  the fix. Confirmed FAIL on the pre-fix baseline via `git stash`, reproducing
  the exact live error text.
- Regression sweep (`TestDispatch*`, `TestSync*`, `TestChatSession*`,
  `TestGrok*`, `TestCrossAccount*`, etc.): only 2 pre-existing failures, both
  confirmed present on baseline via `git stash` and unrelated --
  `TestRestoreChatRunFromDriveMissingActiveAccountHome` (CA-397) and
  `TestNextAccountHomePathGrokUsesGrokHomePrefix` (depends on real
  machine-local provider-account slot state).
- Live re-verification: restarted the dev stack (`just dev`), then called
  `POST /client/workflow-runs/{runId}/sync-chat` directly on all 5 Grok runs
  in project "Gate-sandbox" that failed pre-fix -- all 5 now return
  `syncStatus: "synced"`.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: BUG-310
change_type: bugfix
summary: resolveChatSessionTranscript now reads chat_history.jsonl inside Grok's session directory instead of trying to os.ReadFile the directory itself, fixing Drive sync that had never worked for any Grok chat.
# --->8---
